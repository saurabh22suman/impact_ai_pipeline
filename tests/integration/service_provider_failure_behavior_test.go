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

func TestServiceRunSkipsArticlesWhenProviderUnavailable(t *testing.T) {
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
		Defaults: config.ProviderDefaults{CircuitBreakerFailures: 1, CircuitBreakerSeconds: 60, PromptVersion: "v1"},
		Providers: []config.Provider{
			{Name: "openai", Model: "gpt-4o-mini", Enabled: true, PricePer1KInput: 0.1, PricePer1KOutput: 0.1},
		},
		FallbackChain:          []string{"openai:gpt-4o-mini"},
		PerRunTokenBudget:      80,
		PerProviderTokenBudget: 80,
	}

	svc := engine.NewService(cfg, storage.NewInMemoryStore())
	now := time.Now().UTC()
	articles := []core.Article{
		{
			ID:          "a1",
			SourceID:    "economic-times-markets",
			SourceName:  "Economic Times Markets",
			URL:         "https://example.com/a1",
			Title:       "Infosys first article with OpenAI partnership update",
			Summary:     "INFY provider routing test with OPENAI",
			Body:        "minimal payload to consume budget with ai demand signal",
			PublishedAt: now,
			IngestedAt:  now,
		},
		{
			ID:          "a2",
			SourceID:    "economic-times-markets",
			SourceName:  "Economic Times Markets",
			URL:         "https://example.com/a2",
			Title:       "Infosys second article should fail provider availability",
			Summary:     "INFY provider routing test with OPENAI",
			Body:        "minimal payload to trigger no provider available with ai demand signal",
			PublishedAt: now,
			IngestedAt:  now,
		},
	}

	result, err := svc.Run(context.Background(), core.RunRequest{PipelineProfile: "high_recall", Entities: []string{"Infy"}}, articles)
	if err != nil {
		t.Fatalf("expected run to complete with provider routing, got %v", err)
	}
	if result.Status != core.RunStatusCompleted {
		t.Fatalf("expected completed status, got %s", result.Status)
	}
	if len(result.Events) != 2 {
		t.Fatalf("expected both articles to emit events, got %d events", len(result.Events))
	}
	if result.ArtifactCounts["articles_total"] != 2 {
		t.Fatalf("expected artifact articles_total=2, got %d", result.ArtifactCounts["articles_total"])
	}
	if result.ArtifactCounts["events_output"] != 2 {
		t.Fatalf("expected artifact events_output=2, got %d", result.ArtifactCounts["events_output"])
	}
}
