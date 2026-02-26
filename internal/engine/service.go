package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/enrich"
	"github.com/soloengine/ai-impact-scrapper/internal/export"
	"github.com/soloengine/ai-impact-scrapper/internal/ingest"
	"github.com/soloengine/ai-impact-scrapper/internal/market"
	"github.com/soloengine/ai-impact-scrapper/internal/storage"
)

var ErrInvalidEntitySelection = errors.New("invalid entity selection")

type Service struct {
	cfg       config.AppConfig
	store     storage.EngineStore
	exporter  export.Exporter
	router    *enrich.ProviderRouter
	enricher  *enrich.Enricher
	dedupe    ingest.DedupeEngine
	relevance ingest.RelevanceGate
	runIDGen  *core.RunIDGenerator
}

func NewService(cfg config.AppConfig, store storage.EngineStore) *Service {
	router := enrich.NewProviderRouter(cfg.Providers)
	return &Service{
		cfg:       cfg,
		store:     store,
		exporter:  export.NewExporter(),
		router:    router,
		enricher:  enrich.NewEnricher(cfg.EffectiveEntities(), router),
		dedupe:    ingest.NewDedupeEngine(),
		relevance: ingest.NewRelevanceGate(),
		runIDGen:  core.NewRunIDGenerator(),
	}
}

func (s *Service) Run(ctx context.Context, req core.RunRequest, ingested []core.Article) (core.RunResult, error) {
	if s.store == nil {
		return core.RunResult{}, fmt.Errorf("engine store is nil")
	}
	profile, err := s.cfg.Profile(req.PipelineProfile)
	if err != nil {
		return core.RunResult{}, err
	}

	runID := s.runIDGen.Next()
	started := time.Now().UTC()

	baseMeta := core.RunMetadata{
		RunID:           runID,
		ConfigVersion:   s.cfg.ConfigVersion,
		PipelineProfile: profile.Name,
		PromptVersion:   s.cfg.Providers.Defaults.PromptVersion,
		CreatedAt:       started,
	}

	articles := s.dedupe.Filter(ingested)
	events := make([]core.MarketAlignedEvent, 0)
	featureRows := make([]core.FeatureRow, 0)

	entities, impactCfg, err := s.selectEntities(req.Entities)
	if err != nil {
		return core.RunResult{}, err
	}
	for _, article := range articles {
		relevanceScore := s.relevance.Score(article, s.cfg.Factors.Factors, entities)
		if relevanceScore < profile.MinRelevanceScore {
			continue
		}

		enriched, err := s.enricher.EnrichArticle(ctx, baseMeta, article, s.cfg.Factors.Factors, entities, profile)
		if err != nil {
			return core.RunResult{}, err
		}

		session := market.AlignToNSESession(article.PublishedAt)
		aligned := core.MarketAlignedEvent{
			Event:         enriched,
			Session:       session,
			LabelWindowTo: market.LabelWindowEnd(article.PublishedAt),
		}
		events = append(events, aligned)
		featureRows = append(featureRows, buildFeatureRows(aligned, impactCfg)...)
	}

	result := core.RunResult{
		RunID:         runID,
		Status:        core.RunStatusCompleted,
		CreatedAt:     started,
		StartedAt:     started,
		FinishedAt:    time.Now().UTC(),
		ConfigVersion: s.cfg.ConfigVersion,
		Profile:       profile.Name,
		Events:        events,
		FeatureRows:   featureRows,
	}

	for _, row := range featureRows {
		result.InputTokens += row.InputTokens
		result.OutputTokens += row.OutputTokens
		result.EstimatedCost += row.EstimatedCostUS
	}

	result.ArtifactCounts = map[string]int{
		"articles_total":   len(ingested),
		"articles_deduped": len(articles),
		"events_output":    len(events),
	}

	if err := s.store.SaveRun(ctx, result); err != nil {
		return core.RunResult{}, err
	}
	if err := s.store.SaveEvents(ctx, runID, events); err != nil {
		return core.RunResult{}, err
	}
	if err := s.store.SaveFeatureRows(ctx, runID, featureRows); err != nil {
		return core.RunResult{}, err
	}
	if err := s.persistRunArtifacts(ctx, runID, req); err != nil {
		return core.RunResult{}, err
	}
	return result, nil
}

func (s *Service) ExportJSONL(ctx context.Context, runID string) ([]byte, error) {
	events := s.store.GetEvents(ctx, runID)
	if len(events) == 0 {
		return nil, fmt.Errorf("run %s has no events", runID)
	}
	return s.exporter.JSONL(events)
}

func (s *Service) ExportCSV(ctx context.Context, runID string) ([]byte, error) {
	_, ok := s.store.GetRun(ctx, runID)
	if !ok {
		return nil, fmt.Errorf("run %s not found", runID)
	}
	rows := s.store.GetFeatureRows(ctx, runID)
	return s.exporter.CSV(rows)
}

func (s *Service) ExportTOON(ctx context.Context, runID string) ([]byte, error) {
	events := s.store.GetEvents(ctx, runID)
	if len(events) == 0 {
		return nil, fmt.Errorf("run %s has no events", runID)
	}
	return s.exporter.TOON(events)
}

func (s *Service) GetRun(ctx context.Context, runID string) (core.RunResult, bool) {
	return s.store.GetRun(ctx, runID)
}

func (s *Service) ListRuns(ctx context.Context) []core.RunResult {
	return s.store.ListRuns(ctx)
}

func (s *Service) persistRunArtifacts(ctx context.Context, runID string, req core.RunRequest) error {
	if fs, ok := s.store.(*storage.FileStore); ok {
		if err := fs.SaveRunRequest(runID, req); err != nil {
			return err
		}
	}

	jsonl, err := s.ExportJSONL(ctx, runID)
	if err != nil {
		return err
	}
	if err := s.store.Put(ctx, filepath.ToSlash(filepath.Join("runs", runID, "exports", "events.jsonl")), jsonl); err != nil {
		return err
	}

	csvData, err := s.ExportCSV(ctx, runID)
	if err != nil {
		return err
	}
	if err := s.store.Put(ctx, filepath.ToSlash(filepath.Join("runs", runID, "exports", "features.csv")), csvData); err != nil {
		return err
	}

	toonData, err := s.ExportTOON(ctx, runID)
	if err != nil {
		return err
	}
	if err := s.store.Put(ctx, filepath.ToSlash(filepath.Join("runs", runID, "exports", "events.toon.jsonl")), toonData); err != nil {
		return err
	}

	return nil
}

func (s *Service) selectEntities(requested []string) ([]config.Entity, impactModeConfig, error) {
	all := s.cfg.EffectiveEntities()
	if len(requested) == 0 {
		return all, impactModeConfig{}, nil
	}

	entityLookup := map[string]config.Entity{}
	bySymbol := map[string]config.Entity{}
	for _, entity := range all {
		symbol := strings.ToUpper(strings.TrimSpace(entity.Symbol))
		if symbol != "" {
			entityLookup[symbol] = entity
			bySymbol[symbol] = entity
		}
		name := strings.ToUpper(strings.TrimSpace(entity.Name))
		if name != "" {
			entityLookup[name] = entity
		}
		for _, alias := range entity.Aliases {
			aliasKey := strings.ToUpper(strings.TrimSpace(alias))
			if aliasKey != "" {
				entityLookup[aliasKey] = entity
			}
		}
	}

	seen := map[string]struct{}{}
	selected := make([]config.Entity, 0, len(requested))
	missing := make([]string, 0)
	for _, item := range requested {
		key := strings.ToUpper(strings.TrimSpace(item))
		if key == "" {
			continue
		}
		entity, ok := entityLookup[key]
		if !ok {
			missing = append(missing, strings.TrimSpace(item))
			continue
		}
		entityKey := strings.ToUpper(strings.TrimSpace(entity.Symbol))
		if entityKey == "" {
			entityKey = strings.ToUpper(strings.TrimSpace(entity.Name))
		}
		if _, exists := seen[entityKey]; exists {
			continue
		}
		seen[entityKey] = struct{}{}
		selected = append(selected, entity)
	}

	if len(missing) > 0 || len(selected) == 0 {
		if len(missing) == 0 {
			missing = append(missing, "no matching entities")
		}
		return nil, impactModeConfig{}, fmt.Errorf("%w: unknown entities: %s", ErrInvalidEntitySelection, strings.Join(missing, ", "))
	}

	impact := newImpactModeConfig(s.cfg.EntityGroups.Groups, selected, all)
	if !impact.Enabled {
		return selected, impact, nil
	}

	for childSymbol := range impact.ChildSymbols {
		if _, exists := seen[childSymbol]; exists {
			continue
		}
		entity, ok := bySymbol[childSymbol]
		if !ok {
			continue
		}
		seen[childSymbol] = struct{}{}
		selected = append(selected, entity)
	}

	return selected, impact, nil
}
