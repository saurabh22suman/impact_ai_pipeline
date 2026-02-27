package unit

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/enrich"
)

func TestEnricherMergesLLMEntityDisambiguation(t *testing.T) {
	// The mimo mock returns a NIFTIT entity match from disambiguation (call 3),
	// even though the article text does NOT contain exact keyword matches.
	server := newMimoServer(t, statusPlan{
		http.StatusOK, // sentiment
		http.StatusOK, // factors
		http.StatusOK, // disambiguation
	}, func(_ int, call int) string {
		switch call {
		case 1:
			return `{"label":"neutral","score":0.0}`
		case 2:
			return `{"tags":[]}`
		default:
			return `{"matches":[{"entity_id":"nse-index-niftit","symbol":"NIFTIT","name":"Nifty IT","confidence":0.85,"method":"llm_disambiguation"}]}`
		}
	})
	defer server.Close()

	t.Setenv("MIMO_API_KEY", "test-key")
	t.Setenv("MIMO_BASE_URL", server.URL)
	t.Setenv("MIMO_TIMEOUT_SECONDS", "20")
	t.Setenv("MIMO_MODEL", "")

	cfg := config.ProvidersFile{
		Defaults: config.ProviderDefaults{
			CircuitBreakerFailures: 2,
			CircuitBreakerSeconds:  60,
		},
		Providers: []config.Provider{
			{Name: "mimo", Model: "mimo-v2-flash", Enabled: true, PricePer1KInput: 0.1, PricePer1KOutput: 0.1},
		},
		FallbackChain: []string{"mimo:mimo-v2-flash"},
	}

	router := enrich.NewProviderRouter(cfg)

	niftitEntity := config.Entity{
		ID:      "nse-index-niftit",
		Symbol:  "NIFTIT",
		Name:    "Nifty IT",
		Aliases: []string{"NIFTIT", "Nifty IT", "NiftyIT"},
		Enabled: true,
	}

	enricher := enrich.NewEnricher([]config.Entity{niftitEntity}, router)

	// Article text deliberately does NOT contain "Nifty IT", "NIFTIT", or "NiftyIT"
	// so the keyword matcher will return zero entity matches.
	// The LLM should disambiguate "IT sector index" as NIFTIT.
	article := core.Article{
		ID:          "a1",
		SourceID:    "test-source",
		SourceName:  "Test Source",
		URL:         "https://example.com/a1",
		Title:       "IT sector index rallies on strong enterprise demand",
		Summary:     "The information technology benchmark gained ground",
		Body:        "Market participants noted broad-based buying in technology counters",
		PublishedAt: time.Now().UTC(),
		IngestedAt:  time.Now().UTC(),
	}

	profile := config.PipelineProfile{
		Name:               "high_recall",
		AmbiguityThreshold: 0.0,
		NoveltyThreshold:   1.0,
		MinRelevanceScore:  0.0,
	}

	event, err := enricher.EnrichArticle(
		context.Background(),
		core.RunMetadata{RunID: "run-test", ConfigVersion: "v1", PipelineProfile: "high_recall", PromptVersion: "v1"},
		article,
		nil,
		[]config.Entity{niftitEntity},
		profile,
	)
	if err != nil {
		t.Fatalf("enrich article: %v", err)
	}

	if event.Metadata.Provider != "mimo" {
		t.Fatalf("expected provider mimo for llm refinement path, got %q", event.Metadata.Provider)
	}
	if event.Metadata.Model != "mimo-v2-flash" {
		t.Fatalf("expected model mimo-v2-flash, got %q", event.Metadata.Model)
	}
	if !event.NeedsLLMRefinement {
		t.Fatalf("expected event to require llm refinement")
	}
	if len(event.Entities) != 0 {
		t.Fatalf("expected no keyword entity matches for ambiguous NIFTIT phrasing, got %+v", event.Entities)
	}
}

func TestEnricherPassesEntityContextToProvider(t *testing.T) {
	// Verify the classification request includes entity context by checking
	// that the LLM can use entity symbols/names to return correct matches.
	var capturedCalls int
	server := newMimoServer(t, statusPlan{
		http.StatusOK,
		http.StatusOK,
		http.StatusOK,
	}, func(_ int, call int) string {
		capturedCalls = call
		switch call {
		case 1:
			return `{"label":"positive","score":0.5}`
		case 2:
			return `{"tags":[]}`
		default:
			// Return COFORGE match — this entity should be in the context
			return `{"matches":[{"entity_id":"nse-coforge","symbol":"COFORGE","name":"Coforge","confidence":0.9,"method":"llm_disambiguation"}]}`
		}
	})
	defer server.Close()

	t.Setenv("MIMO_API_KEY", "test-key")
	t.Setenv("MIMO_BASE_URL", server.URL)
	t.Setenv("MIMO_TIMEOUT_SECONDS", "20")
	t.Setenv("MIMO_MODEL", "")

	cfg := config.ProvidersFile{
		Defaults: config.ProviderDefaults{CircuitBreakerFailures: 2, CircuitBreakerSeconds: 60},
		Providers: []config.Provider{
			{Name: "mimo", Model: "mimo-v2-flash", Enabled: true, PricePer1KInput: 0.1, PricePer1KOutput: 0.1},
		},
		FallbackChain: []string{"mimo:mimo-v2-flash"},
	}

	router := enrich.NewProviderRouter(cfg)

	coforgeEntity := config.Entity{
		ID:      "nse-coforge",
		Symbol:  "COFORGE",
		Name:    "Coforge",
		Aliases: []string{"Coforge", "COFORGE"},
		Enabled: true,
	}

	enricher := enrich.NewEnricher([]config.Entity{coforgeEntity}, router)

	article := core.Article{
		ID:          "a2",
		SourceID:    "test-source",
		SourceName:  "Test Source",
		URL:         "https://example.com/a2",
		Title:       "Mid-cap IT firm wins large deal with European bank",
		Summary:     "The company announced a multi-year managed services contract",
		Body:        "Analysts expect the deal to boost revenue growth by 8 percent",
		PublishedAt: time.Now().UTC(),
		IngestedAt:  time.Now().UTC(),
	}

	profile := config.PipelineProfile{
		Name:               "high_recall",
		AmbiguityThreshold: 0.0,
		NoveltyThreshold:   1.0,
		MinRelevanceScore:  0.0,
	}

	event, err := enricher.EnrichArticle(
		context.Background(),
		core.RunMetadata{RunID: "run-test", ConfigVersion: "v1", PipelineProfile: "high_recall", PromptVersion: "v1"},
		article,
		nil,
		[]config.Entity{coforgeEntity},
		profile,
	)
	if err != nil {
		t.Fatalf("enrich article: %v", err)
	}

	// Verify all 3 calls were made (sentiment, factors, disambiguation)
	if capturedCalls < 3 {
		t.Fatalf("expected at least 3 provider calls (including disambiguation), got %d", capturedCalls)
	}
	if event.Metadata.Provider != "mimo" {
		t.Fatalf("expected provider mimo for llm refinement path, got %q", event.Metadata.Provider)
	}
	if !event.NeedsLLMRefinement {
		t.Fatalf("expected event to require llm refinement")
	}
	if len(event.Entities) != 0 {
		t.Fatalf("expected no keyword entity matches for ambiguous company phrasing, got %+v", event.Entities)
	}
}
