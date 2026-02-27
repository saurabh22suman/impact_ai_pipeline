package unit

import (
	"context"
	"testing"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/enrich"
)

func TestEnricherAlwaysUsesRouterWhenAvailable(t *testing.T) {
	router := enrich.NewProviderRouter(config.ProvidersFile{
		Providers: []config.Provider{
			{Name: "openai", Model: "gpt-4o-mini", Enabled: true, PricePer1KInput: 0.1, PricePer1KOutput: 0.1},
		},
		FallbackChain: []string{"openai:gpt-4o-mini"},
	})

	enricher := enrich.NewEnricher([]config.Entity{{
		ID:      "nse-infy",
		Symbol:  "INFY",
		Name:    "Infosys Limited",
		Aliases: []string{"Infy", "Infosys"},
		Enabled: true,
	}}, router)

	article := core.Article{
		ID:          "a1",
		SourceID:    "s1",
		SourceName:  "S1",
		URL:         "https://example.com/a1",
		Title:       "Infy reports stable quarterly update",
		Summary:     "General commentary",
		Body:        "No configured factor keywords present",
		PublishedAt: time.Now().UTC(),
		IngestedAt:  time.Now().UTC(),
	}

	profile := config.PipelineProfile{
		Name:               "high_recall",
		AmbiguityThreshold: 0.45,
		NoveltyThreshold:   0.50,
		MinRelevanceScore:  0.35,
	}

	event, err := enricher.EnrichArticle(
		context.Background(),
		core.RunMetadata{RunID: "run-1", ConfigVersion: "v1", PipelineProfile: profile.Name, PromptVersion: "v1"},
		article,
		[]config.Factor{{ID: "ai-demand", Name: "AI Demand", Category: "demand", Keywords: []string{"enterprise ai"}, Weight: 0.8}},
		[]config.Entity{{ID: "nse-infy", Symbol: "INFY", Name: "Infosys Limited", Aliases: []string{"Infy", "Infosys"}, Enabled: true}},
		profile,
	)
	if err != nil {
		t.Fatalf("enrich article: %v", err)
	}
	if event.Metadata.Provider != "rules" {
		t.Fatalf("expected deterministic provider rules when LLM is not required, got %q", event.Metadata.Provider)
	}
	if event.Metadata.Model != "rules" {
		t.Fatalf("expected deterministic model rules when LLM is not required, got %q", event.Metadata.Model)
	}
}
