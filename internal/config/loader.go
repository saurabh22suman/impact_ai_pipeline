package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func Load(configDir string) (AppConfig, error) {
	if strings.TrimSpace(configDir) == "" {
		return AppConfig{}, fmt.Errorf("config directory is required")
	}

	sources, err := loadYAML[SourcesFile](filepath.Join(configDir, "sources.yaml"))
	if err != nil {
		return AppConfig{}, err
	}
	entitiesDefault, err := loadYAML[EntitiesFile](filepath.Join(configDir, "entities.niftyit.yaml"))
	if err != nil {
		return AppConfig{}, err
	}
	entitiesCustom, err := loadYAML[EntitiesFile](filepath.Join(configDir, "entities.custom.yaml"))
	if err != nil {
		return AppConfig{}, err
	}
	factors, err := loadYAML[FactorsFile](filepath.Join(configDir, "factors.yaml"))
	if err != nil {
		return AppConfig{}, err
	}
	providers, err := loadYAML[ProvidersFile](filepath.Join(configDir, "providers.yaml"))
	if err != nil {
		return AppConfig{}, err
	}
	pipelines, err := loadYAML[PipelinesFile](filepath.Join(configDir, "pipelines.yaml"))
	if err != nil {
		return AppConfig{}, err
	}

	cfg := AppConfig{
		Sources:         sources,
		EntitiesDefault: entitiesDefault,
		EntitiesCustom:  entitiesCustom,
		Factors:         factors,
		Providers:       providers,
		Pipelines:       pipelines,
		LoadedAt:        time.Now().UTC(),
	}
	normalizeSourceKinds(&cfg)
	normalizeEntityTypes(&cfg)
	cfg.ConfigVersion = computeConfigVersion(cfg)

	if err := validate(cfg); err != nil {
		return AppConfig{}, err
	}

	return cfg, nil
}

func loadYAML[T any](path string) (T, error) {
	var out T
	payload, err := os.ReadFile(path)
	if err != nil {
		return out, fmt.Errorf("read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(payload, &out); err != nil {
		return out, fmt.Errorf("parse %s: %w", path, err)
	}
	return out, nil
}

func computeConfigVersion(cfg AppConfig) string {
	parts := []string{
		cfg.Sources.Version,
		cfg.EntitiesDefault.Version,
		cfg.EntitiesCustom.Version,
		cfg.Factors.Version,
		cfg.Providers.Version,
		cfg.Pipelines.Version,
	}
	return strings.Join(parts, "+")
}

func normalizeSourceKinds(cfg *AppConfig) {
	if cfg == nil {
		return
	}
	for i := range cfg.Sources.Sources {
		cfg.Sources.Sources[i].Kind = NormalizeSourceKind(cfg.Sources.Sources[i].Kind)
	}
}

func normalizeEntityTypes(cfg *AppConfig) {
	if cfg == nil {
		return
	}
	for i := range cfg.EntitiesDefault.Entities {
		typ := cfg.EntitiesDefault.Entities[i].Type
		if strings.TrimSpace(typ) == "" {
			typ = DefaultEntityTypeForSymbol(cfg.EntitiesDefault.Entities[i].Symbol)
		}
		cfg.EntitiesDefault.Entities[i].Type = NormalizeEntityType(typ)
	}
	for i := range cfg.EntitiesCustom.Entities {
		typ := cfg.EntitiesCustom.Entities[i].Type
		if strings.TrimSpace(typ) == "" {
			typ = DefaultEntityTypeForSymbol(cfg.EntitiesCustom.Entities[i].Symbol)
		}
		cfg.EntitiesCustom.Entities[i].Type = NormalizeEntityType(typ)
	}
}

func validate(cfg AppConfig) error {
	sourceIDs := map[string]struct{}{}
	for i, source := range cfg.Sources.Sources {
		if strings.TrimSpace(source.ID) == "" || strings.TrimSpace(source.Name) == "" || strings.TrimSpace(source.URL) == "" || strings.TrimSpace(source.Region) == "" || strings.TrimSpace(source.Language) == "" {
			return fmt.Errorf("source at index %d must include non-empty id, name, url, region, and language", i)
		}
		sourceID := strings.ToLower(strings.TrimSpace(source.ID))
		if _, exists := sourceIDs[sourceID]; exists {
			return fmt.Errorf("duplicate source id %q", source.ID)
		}
		sourceIDs[sourceID] = struct{}{}

		switch source.Kind {
		case SourceKindRSS, SourceKindDirect, SourceKindPulse:
		default:
			return fmt.Errorf("source %s has unsupported kind %q", source.ID, source.Kind)
		}
	}

	if len(cfg.EnabledSources()) == 0 {
		return fmt.Errorf("at least one source must be enabled")
	}
	if len(cfg.EntitiesDefault.Entities) == 0 {
		return fmt.Errorf("entities.niftyit.yaml must include default NIFTY IT entities")
	}
	if len(cfg.EffectiveEntities()) == 0 {
		return fmt.Errorf("effective entity universe is empty")
	}
	if len(cfg.Factors.Factors) == 0 {
		return fmt.Errorf("at least one factor must be configured")
	}
	if len(cfg.EnabledProviders()) == 0 {
		return fmt.Errorf("at least one provider must be enabled")
	}
	if len(cfg.Pipelines.Profiles) == 0 {
		return fmt.Errorf("at least one pipeline profile must be configured")
	}
	if _, err := cfg.Profile(cfg.Pipelines.DefaultProfile); err != nil {
		return err
	}

	providerModels := map[string]struct{}{}
	for _, provider := range cfg.EnabledProviders() {
		providerModels[provider.Name+":"+provider.Model] = struct{}{}
	}
	for _, fallback := range cfg.Providers.FallbackChain {
		if _, ok := providerModels[fallback]; !ok {
			return fmt.Errorf("fallback chain entry %q not present in enabled providers", fallback)
		}
	}

	return nil
}
