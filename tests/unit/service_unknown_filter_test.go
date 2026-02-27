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

func TestServiceRunOmitsEventsWithNoRequestedEntityMatch(t *testing.T) {
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

	// Article that does NOT mention NIFTIT/NiftyIT or any alias.
	// The stub provider returns empty entity disambiguation.
	// Since user requested specific entity ["NiftyIT"] and article doesn't match,
	// it should be excluded from output events (no UNKNOWN rows).
	articles := []core.Article{{
		ID:          "a1",
		SourceID:    "bbc-business",
		SourceName:  "BBC Business",
		URL:         "https://example.com/unrelated",
		Title:       "European central bank holds rates steady",
		Summary:     "ECB decision was widely expected",
		Body:        "Bond markets reacted calmly to the announcement",
		PublishedAt: now,
		IngestedAt:  now,
	}}

	_, err = svc.Run(context.Background(), core.RunRequest{
		PipelineProfile: "high_recall",
		Entities:        []string{"NiftyIT"},
	}, articles)
	if err == nil {
		t.Fatalf("expected run to fail when requested entity selection yields no events")
	}
	if !strings.Contains(err.Error(), "has no events") {
		t.Fatalf("expected no-events error, got %v", err)
	}
}

func TestServiceRunKeepsMatchedEntityEvents(t *testing.T) {
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

	// Article that DOES mention Infosys (keyword match).
	articles := []core.Article{{
		ID:          "a2",
		SourceID:    "economic-times-markets",
		SourceName:  "Economic Times Markets",
		URL:         "https://example.com/infy-article",
		Title:       "Infosys wins enterprise ai contract with OpenAI for datacenter expansion",
		Summary:     "INFY cites model deployment, ai capex, and compliance roadmap",
		Body:        "Infosys reported enterprise ai adoption with ai contract wins, model deployment, ai capex, datacenter expansion, compliance, and ai hiring momentum.",
		PublishedAt: now,
		IngestedAt:  now,
	}}

	_, err = svc.Run(context.Background(), core.RunRequest{
		PipelineProfile: "high_recall",
		Entities:        []string{"INFY"},
	}, articles)
	if err == nil {
		t.Fatalf("expected run to fail when impact mode receives parent-only match without configured child entity match")
	}
	if !strings.Contains(err.Error(), "has no events") {
		t.Fatalf("expected no-events error, got %v", err)
	}
}
