package integration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/engine"
	"github.com/soloengine/ai-impact-scrapper/internal/storage"
)

func TestServiceRunResetsProviderBudgetsPerRun(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	for i := range cfg.Pipelines.Profiles {
		if cfg.Pipelines.Profiles[i].Name == "high_recall" {
			cfg.Pipelines.Profiles[i].MinRelevanceScore = 0
			cfg.Pipelines.Profiles[i].AmbiguityThreshold = 0
			cfg.Pipelines.Profiles[i].NoveltyThreshold = 1
		}
	}

	cfg.Providers = config.ProvidersFile{
		Defaults: config.ProviderDefaults{CircuitBreakerFailures: 2, CircuitBreakerSeconds: 60, PromptVersion: "v1"},
		Providers: []config.Provider{
			{Name: "openai", Model: "gpt-4o-mini", Enabled: true, PricePer1KInput: 0.1, PricePer1KOutput: 0.1},
		},
		FallbackChain:          []string{"openai:gpt-4o-mini"},
		PerRunTokenBudget:      10,
		PerProviderTokenBudget: 10,
	}

	svc := engine.NewService(cfg, storage.NewInMemoryStore())
	now := time.Now().UTC()
	articles := []core.Article{{
		ID:          "a1",
		SourceID:    "economic-times-markets",
		SourceName:  "Economic Times Markets",
		URL:         "https://example.com/a1",
		Title:       "Infosys short text with OpenAI mention",
		Summary:     "INFY and OPENAI small payload",
		Body:        "tiny body with AI demand signal",
		PublishedAt: now,
		IngestedAt:  now,
	}}

	first, err := svc.Run(context.Background(), core.RunRequest{PipelineProfile: "high_recall", Entities: []string{"Infy"}}, articles)
	if err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	if len(first.Events) == 0 {
		t.Fatalf("expected first run to emit event")
	}
	if first.Events[0].Event.Metadata.Provider == "" {
		t.Fatalf("expected first run provider to be populated")
	}

	second, err := svc.Run(context.Background(), core.RunRequest{PipelineProfile: "high_recall", Entities: []string{"Infy"}}, articles)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if len(second.Events) == 0 {
		t.Fatalf("expected second run to emit event")
	}
	if second.Events[0].Event.Metadata.Provider == "" {
		t.Fatalf("expected second run provider to be populated")
	}
}
