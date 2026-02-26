package config

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	SourceKindRSS    = "rss"
	SourceKindDirect = "direct"

	EntityTypeEquity = "equity"
	EntityTypeIndex  = "index"

	EntityRoleParent = "parent"
	EntityRoleChild  = "child"
)

func NormalizeSourceKind(raw string) string {
	kind := strings.ToLower(strings.TrimSpace(raw))
	if kind == "" {
		return SourceKindRSS
	}
	return kind
}

type Source struct {
	ID            string `yaml:"id"`
	Name          string `yaml:"name"`
	Kind          string `yaml:"kind"`
	URL           string `yaml:"url"`
	Region        string `yaml:"region"`
	Language      string `yaml:"language"`
	Enabled       bool   `yaml:"enabled"`
	CrawlFallback bool   `yaml:"crawl_fallback"`
}

type SourcesFile struct {
	Version string   `yaml:"version"`
	Sources []Source `yaml:"sources"`
}

type Entity struct {
	ID       string   `yaml:"id"`
	Symbol   string   `yaml:"symbol"`
	Name     string   `yaml:"name"`
	Aliases  []string `yaml:"aliases"`
	Exchange string   `yaml:"exchange"`
	Sector   string   `yaml:"sector"`
	Role     string   `yaml:"role"`
	Type     string   `yaml:"type"`
	Enabled  bool     `yaml:"enabled"`
}

type EntitiesFile struct {
	Version  string   `yaml:"version"`
	Entities []Entity `yaml:"entities"`
}

type EntityGroup struct {
	ParentSymbol string   `yaml:"parent_symbol"`
	ChildSymbols []string `yaml:"child_symbols"`
}

type EntityGroupsFile struct {
	Version string        `yaml:"version"`
	Groups  []EntityGroup `yaml:"groups"`
}

type Factor struct {
	ID       string   `yaml:"id"`
	Name     string   `yaml:"name"`
	Category string   `yaml:"category"`
	Keywords []string `yaml:"keywords"`
	Weight   float64  `yaml:"weight"`
}

type FactorsFile struct {
	Version string   `yaml:"version"`
	Factors []Factor `yaml:"factors"`
}

type Provider struct {
	Name              string  `yaml:"name"`
	Model             string  `yaml:"model"`
	Enabled           bool    `yaml:"enabled"`
	PricePer1KInput   float64 `yaml:"price_per_1k_input"`
	PricePer1KOutput  float64 `yaml:"price_per_1k_output"`
	MaxInputTokens    int     `yaml:"max_input_tokens"`
	MaxOutputTokens   int     `yaml:"max_output_tokens"`
	TimeoutSeconds    int     `yaml:"timeout_seconds"`
	MaxRequestsPerMin int     `yaml:"max_requests_per_min"`
}

type ProviderDefaults struct {
	RoutingPolicy          string `yaml:"routing_policy"`
	PromptVersion          string `yaml:"prompt_version"`
	CircuitBreakerFailures int    `yaml:"circuit_breaker_failures"`
	CircuitBreakerSeconds  int    `yaml:"circuit_breaker_seconds"`
	RetryCount             int    `yaml:"retry_count"`
	BackoffMillis          int    `yaml:"backoff_millis"`
}

type ProvidersFile struct {
	Version                 string           `yaml:"version"`
	Defaults                ProviderDefaults `yaml:"defaults"`
	Providers               []Provider       `yaml:"providers"`
	FallbackChain           []string         `yaml:"fallback_chain"`
	PerRunTokenBudget       int              `yaml:"per_run_token_budget"`
	PerProviderTokenBudget  int              `yaml:"per_provider_token_budget"`
	PerRunCostBudgetUSD     float64          `yaml:"per_run_cost_budget_usd"`
	PerProviderCostBudgetUS float64          `yaml:"per_provider_cost_budget_usd"`
}

type PipelineProfile struct {
	Name               string  `yaml:"name"`
	Description        string  `yaml:"description"`
	AmbiguityThreshold float64 `yaml:"ambiguity_threshold"`
	NoveltyThreshold   float64 `yaml:"novelty_threshold"`
	MinRelevanceScore  float64 `yaml:"min_relevance_score"`
	EnableRawArtifacts bool    `yaml:"enable_raw_artifacts"`
	LLMBudgetTokens    int     `yaml:"llm_budget_tokens"`
	Session            string  `yaml:"session"`
}

type PipelinesFile struct {
	Version        string            `yaml:"version"`
	DefaultProfile string            `yaml:"default_profile"`
	Profiles       []PipelineProfile `yaml:"profiles"`
}

type AppConfig struct {
	Sources         SourcesFile
	EntitiesDefault EntitiesFile
	EntitiesCustom  EntitiesFile
	EntityGroups    EntityGroupsFile
	Factors         FactorsFile
	Providers       ProvidersFile
	Pipelines       PipelinesFile
	ConfigVersion   string
	LoadedAt        time.Time
}

func NormalizeEntityType(raw string) string {
	typ := strings.ToLower(strings.TrimSpace(raw))
	switch typ {
	case EntityTypeEquity, EntityTypeIndex:
		return typ
	default:
		return EntityTypeEquity
	}
}

func NormalizeEntityRole(raw string) string {
	role := strings.ToLower(strings.TrimSpace(raw))
	switch role {
	case EntityRoleParent, EntityRoleChild:
		return role
	default:
		return ""
	}
}

func DefaultEntityTypeForSymbol(symbol string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(symbol))
	if strings.HasPrefix(trimmed, "NIFT") || strings.HasPrefix(trimmed, "NIFTY") {
		return EntityTypeIndex
	}
	return EntityTypeEquity
}

func (a AppConfig) EffectiveEntities() []Entity {
	combined := append([]Entity{}, a.EntitiesDefault.Entities...)
	combined = append(combined, a.EntitiesCustom.Entities...)

	seen := map[string]Entity{}
	for _, entity := range combined {
		if !entity.Enabled {
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(entity.Symbol))
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(entity.Name))
		}
		if key == "" {
			continue
		}
		if strings.TrimSpace(entity.Type) == "" {
			entity.Type = DefaultEntityTypeForSymbol(entity.Symbol)
		} else {
			entity.Type = NormalizeEntityType(entity.Type)
		}
		seen[key] = entity
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	entities := make([]Entity, 0, len(keys))
	for _, key := range keys {
		entities = append(entities, seen[key])
	}
	return entities
}

func (a AppConfig) Profile(name string) (PipelineProfile, error) {
	lookup := strings.TrimSpace(name)
	if lookup == "" {
		lookup = strings.TrimSpace(a.Pipelines.DefaultProfile)
	}
	for _, profile := range a.Pipelines.Profiles {
		if profile.Name == lookup {
			return profile, nil
		}
	}
	return PipelineProfile{}, fmt.Errorf("pipeline profile %q not found", lookup)
}

func (a AppConfig) EnabledSources() []Source {
	enabled := make([]Source, 0, len(a.Sources.Sources))
	for _, source := range a.Sources.Sources {
		if source.Enabled {
			enabled = append(enabled, source)
		}
	}
	return enabled
}

func (a AppConfig) EnabledProviders() []Provider {
	enabled := make([]Provider, 0, len(a.Providers.Providers))
	for _, provider := range a.Providers.Providers {
		if provider.Enabled {
			enabled = append(enabled, provider)
		}
	}
	return enabled
}
