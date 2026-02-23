package unit

import (
	"path/filepath"
	"testing"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
)

func TestLoadConfigIncludesNiftyITByDefault(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	entities := cfg.EffectiveEntities()
	if len(entities) < 10 {
		t.Fatalf("expected default NIFTY IT universe, got %d entities", len(entities))
	}

	foundTCS := false
	for _, entity := range entities {
		if entity.Symbol == "TCS" {
			foundTCS = true
			break
		}
	}
	if !foundTCS {
		t.Fatalf("expected TCS in default universe")
	}
}

func TestLoadConfigRejectsUnknownFallbackProvider(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: a\n    name: A\n    kind: rss\n    url: https://example.com\n    region: global\n    language: en\n    enabled: true\n    crawl_fallback: false\n")
	mustWrite(t, dir, "providers.yaml", "version: v1\ndefaults:\n  routing_policy: lowest\n  prompt_version: v1\n  circuit_breaker_failures: 3\n  circuit_breaker_seconds: 30\n  retry_count: 2\n  backoff_millis: 100\nproviders:\n  - name: gemini\n    model: gemini-2.0-flash\n    enabled: true\n    price_per_1k_input: 0.1\n    price_per_1k_output: 0.2\n    max_input_tokens: 1000\n    max_output_tokens: 500\n    timeout_seconds: 10\n    max_requests_per_min: 60\nfallback_chain: [openai:gpt-4o-mini]\nper_run_token_budget: 1000\nper_provider_token_budget: 1000\nper_run_cost_budget_usd: 1\nper_provider_cost_budget_usd: 1\n")

	_, err := config.Load(dir)
	if err == nil {
		t.Fatalf("expected validation error for unknown fallback provider")
	}
}

func TestLoadConfigRejectsDuplicateSourceIDs(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: dup\n    name: First\n    kind: rss\n    url: https://example.com/one\n    region: global\n    language: en\n    enabled: true\n    crawl_fallback: false\n  - id: Dup\n    name: Second\n    kind: direct\n    url: https://example.com/two\n    region: global\n    language: en\n    enabled: true\n    crawl_fallback: false\n")

	_, err := config.Load(dir)
	if err == nil {
		t.Fatalf("expected duplicate source id validation error")
	}
}

func TestLoadConfigRejectsInvalidSourceKind(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: a\n    name: A\n    kind: atom\n    url: https://example.com\n    region: global\n    language: en\n    enabled: true\n    crawl_fallback: false\n")

	_, err := config.Load(dir)
	if err == nil {
		t.Fatalf("expected invalid source kind validation error")
	}
}

func TestLoadConfigAcceptsPulseSourceKind(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: pulse-a\n    name: Pulse A\n    kind: pulse\n    url: https://example.com/pulse\n    region: global\n    language: en\n    enabled: true\n    crawl_fallback: false\n")

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Sources.Sources) != 1 {
		t.Fatalf("expected one source, got %d", len(cfg.Sources.Sources))
	}
	if cfg.Sources.Sources[0].Kind != config.SourceKindPulse {
		t.Fatalf("expected source kind %q, got %q", config.SourceKindPulse, cfg.Sources.Sources[0].Kind)
	}
}

func TestLoadConfigDefaultsEmptySourceKindToRSS(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: a\n    name: A\n    url: https://example.com\n    region: global\n    language: en\n    enabled: true\n    crawl_fallback: false\n")

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Sources.Sources) != 1 {
		t.Fatalf("expected one source, got %d", len(cfg.Sources.Sources))
	}
	if cfg.Sources.Sources[0].Kind != config.SourceKindRSS {
		t.Fatalf("expected default source kind %q, got %q", config.SourceKindRSS, cfg.Sources.Sources[0].Kind)
	}
}

func TestDefaultConfigIncludesZerodhaPulseSource(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	found := false
	for _, source := range cfg.Sources.Sources {
		if source.ID != "zerodha-pulse" {
			continue
		}
		found = true
		if source.Kind != config.SourceKindPulse {
			t.Fatalf("expected zerodha-pulse kind %q, got %q", config.SourceKindPulse, source.Kind)
		}
	}
	if !found {
		t.Fatalf("expected zerodha-pulse in default sources")
	}
}

func TestLoadConfigRejectsSourceWithMissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: \"\"\n    name: A\n    kind: rss\n    url: https://example.com\n    region: global\n    language: en\n    enabled: true\n    crawl_fallback: false\n")

	_, err := config.Load(dir)
	if err == nil {
		t.Fatalf("expected validation error for missing required source fields")
	}
}

func writeBaseConfigFiles(t *testing.T, dir string) {
	t.Helper()
	mustWrite(t, dir, "entities.niftyit.yaml", "version: v1\nentities:\n  - id: t\n    symbol: TCS\n    name: Tata Consultancy Services\n    aliases: [TCS]\n    exchange: NSE\n    sector: IT\n    type: equity\n    enabled: true\n")
	mustWrite(t, dir, "entities.custom.yaml", "version: v1\nentities: []\n")
	mustWrite(t, dir, "factors.yaml", "version: v1\nfactors:\n  - id: f\n    name: Demand\n    category: demand\n    keywords: [ai]\n    weight: 1\n")
	mustWrite(t, dir, "providers.yaml", "version: v1\ndefaults:\n  routing_policy: lowest\n  prompt_version: v1\n  circuit_breaker_failures: 3\n  circuit_breaker_seconds: 30\n  retry_count: 2\n  backoff_millis: 100\nproviders:\n  - name: gemini\n    model: gemini-2.0-flash\n    enabled: true\n    price_per_1k_input: 0.1\n    price_per_1k_output: 0.2\n    max_input_tokens: 1000\n    max_output_tokens: 500\n    timeout_seconds: 10\n    max_requests_per_min: 60\nfallback_chain: [gemini:gemini-2.0-flash]\nper_run_token_budget: 1000\nper_provider_token_budget: 1000\nper_run_cost_budget_usd: 1\nper_provider_cost_budget_usd: 1\n")
	mustWrite(t, dir, "pipelines.yaml", "version: v1\ndefault_profile: cost\nprofiles:\n  - name: cost\n    description: d\n    ambiguity_threshold: 0.5\n    novelty_threshold: 0.6\n    min_relevance_score: 0.2\n    enable_raw_artifacts: false\n    llm_budget_tokens: 1000\n    session: nse\n")
}
