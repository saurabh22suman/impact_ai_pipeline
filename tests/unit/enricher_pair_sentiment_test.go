package unit

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/enrich"
)

func TestEnricherClassifyPairSentimentUsesRouter(t *testing.T) {
	cfg := loadPairSentimentTestConfig(t)
	router := enrich.NewProviderRouter(cfg.Providers)
	enricher := enrich.NewEnricher(cfg.EffectiveEntities(), router)

	article := core.Article{
		ID:      "article-1",
		Title:   "INFY posts strong growth with OPENAI collaboration",
		Summary: "Management highlights strong demand from AI programs",
		Body:    "INFY and OPENAI announced strong growth and upgrade outlook.",
	}
	parent := core.EntityMatch{Symbol: "INFY", Confidence: 0.9}
	child := &core.EntityMatch{Symbol: "OPENAI", Confidence: 0.8}

	result := enricher.ClassifyPairSentiment(context.Background(), article, parent, child)

	if result.InputTokens <= 0 {
		t.Fatalf("expected router usage input tokens > 0, got %d", result.InputTokens)
	}
	if result.OutputTokens <= 0 {
		t.Fatalf("expected router usage output tokens > 0, got %d", result.OutputTokens)
	}
	if result.EstimatedCostUS <= 0 {
		t.Fatalf("expected router usage estimated cost > 0, got %f", result.EstimatedCostUS)
	}
	if result.Label == "" {
		t.Fatalf("expected sentiment label, got empty")
	}
	if result.Score <= 0 {
		t.Fatalf("expected positive pair score from router path, got %f", result.Score)
	}
	if strings.EqualFold(result.Provider, "rules") {
		t.Fatalf("expected non-rules provider, got %s", result.Provider)
	}
}

func loadPairSentimentTestConfig(t *testing.T) config.AppConfig {
	t.Helper()

	dir := t.TempDir()
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: test-source\n    name: Test Source\n    kind: rss\n    url: https://example.com/rss\n    region: india\n    language: en\n    enabled: true\n    crawl_fallback: false\n")
	mustWrite(t, dir, "entities.niftyit.yaml", "version: v1\nentities:\n  - id: nse-infy\n    symbol: INFY\n    name: Infosys Limited\n    aliases: [INFY, Infosys]\n    exchange: NSE\n    sector: IT\n    role: parent\n    type: equity\n    enabled: true\n")
	mustWrite(t, dir, "entities.custom.yaml", "version: v1\nentities:\n  - id: child-openai\n    symbol: OPENAI\n    name: OpenAI\n    aliases: [OPENAI, OpenAI]\n    exchange: GLOBAL\n    sector: AI\n    role: child\n    type: equity\n    enabled: true\n")
	mustWrite(t, dir, "entity_groups.yaml", "version: v1\ngroups:\n  - id: nifty-it-impact\n    parent_symbol: INFY\n    child_symbols: [OPENAI]\n")
	mustWrite(t, dir, "factors.yaml", "version: v1\nfactors:\n  - id: f-growth\n    name: Growth\n    category: demand\n    keywords: [growth]\n    weight: 1\n")
	mustWrite(t, dir, "providers.yaml", "version: v1\ndefaults:\n  routing_policy: lowest_cost\n  prompt_version: v1\n  circuit_breaker_failures: 3\n  circuit_breaker_seconds: 30\n  retry_count: 2\n  backoff_millis: 100\nproviders:\n  - name: openai\n    model: gpt-4o-mini\n    enabled: true\n    price_per_1k_input: 0.1\n    price_per_1k_output: 0.2\n    max_input_tokens: 1024\n    max_output_tokens: 256\n    timeout_seconds: 10\n    max_requests_per_min: 60\nfallback_chain: [openai:gpt-4o-mini]\nper_run_token_budget: 100000\nper_provider_token_budget: 100000\nper_run_cost_budget_usd: 100\nper_provider_cost_budget_usd: 100\n")
	mustWrite(t, dir, "pipelines.yaml", "version: v1\ndefault_profile: impact_test\nprofiles:\n  - name: impact_test\n    description: impact test profile\n    ambiguity_threshold: 0.9\n    novelty_threshold: 0.1\n    min_relevance_score: 0.0\n    enable_raw_artifacts: false\n    llm_budget_tokens: 1000\n    session: nse\n")

	cfg, err := config.Load(filepath.Clean(dir))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}
