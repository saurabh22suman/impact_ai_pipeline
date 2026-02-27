package unit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/engine"
	"github.com/soloengine/ai-impact-scrapper/internal/storage"
)

func TestServiceRunAcceptsNiftyITAliasWithoutSpace(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs"))
	cfg.Providers = config.ProvidersFile{
		Defaults:      config.ProviderDefaults{PromptVersion: "v1"},
		Providers:     []config.Provider{{Name: "openai", Model: "gpt-4o-mini", Enabled: true, PricePer1KInput: 0.1, PricePer1KOutput: 0.1}},
		FallbackChain: []string{"openai:gpt-4o-mini"},
	}
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	svc := engine.NewService(cfg, storage.NewInMemoryStore())
	now := time.Now().UTC()
	articles := []core.Article{{
		ID:          "a1",
		SourceID:    "economic-times-markets",
		SourceName:  "Economic Times Markets",
		URL:         "https://example.com/a1",
		Title:       "Nifty IT holds steady as broad market consolidates",
		Summary:     "NiftyIT index remains in focus",
		Body:        "Session commentary with limited AI factors",
		PublishedAt: now,
		IngestedAt:  now,
	}}

	_, err = svc.Run(context.Background(), core.RunRequest{PipelineProfile: "high_recall", Entities: []string{"NiftyIT"}}, articles)
	if err != nil {
		t.Fatalf("expected NiftyIT alias to be accepted; provider must be available in test env, got %v", err)
	}
}

func TestServiceRunFiltersEntitiesToRequestedSet(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Providers = config.ProvidersFile{
		Defaults:      config.ProviderDefaults{PromptVersion: "v1"},
		Providers:     []config.Provider{{Name: "openai", Model: "gpt-4o-mini", Enabled: true, PricePer1KInput: 0.1, PricePer1KOutput: 0.1}},
		FallbackChain: []string{"openai:gpt-4o-mini"},
	}

	svc := engine.NewService(cfg, storage.NewInMemoryStore())
	now := time.Now().UTC()
	articles := []core.Article{{
		ID:          "a1",
		SourceID:    "economic-times-markets",
		SourceName:  "Economic Times Markets",
		URL:         "https://example.com/a1",
		Title:       "Infosys and Nifty IT rally on AI demand signals",
		Summary:     "INFY and NIFTIT lead gains",
		Body:        "Sector momentum remains strong despite mixed earnings",
		PublishedAt: now,
		IngestedAt:  now,
	}}

	_, err = svc.Run(context.Background(), core.RunRequest{PipelineProfile: "high_recall", Entities: []string{"HCLTECH"}}, articles)
	if err == nil {
		t.Fatalf("expected run to fail when requested entity selection yields no events")
	}
	if !strings.Contains(err.Error(), "has no events") {
		t.Fatalf("expected no-events error, got %v", err)
	}
}
