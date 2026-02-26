package unit

import (
	"context"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/engine"
	"github.com/soloengine/ai-impact-scrapper/internal/enrich"
	"github.com/soloengine/ai-impact-scrapper/internal/enrich/providers"
	"github.com/soloengine/ai-impact-scrapper/internal/storage"
)

func TestServiceRunBuildsParentChildCrossProductRows(t *testing.T) {
	cfg := loadImpactTestConfig(t)
	svc := engine.NewService(cfg, storage.NewInMemoryStore())

	now := time.Now().UTC()
	articles := []core.Article{{
		ID:          "a1",
		SourceID:    "test-source",
		SourceName:  "Test Source",
		URL:         "https://example.com/a1",
		Title:       "INFY expands cloud work with OPENAI and GEMINI",
		Summary:     "INFY announces platform partnerships",
		Body:        "INFY OPENAI GEMINI cloud",
		Language:    "en",
		Region:      "india",
		PublishedAt: now,
		IngestedAt:  now,
	}}

	result, err := svc.Run(context.Background(), core.RunRequest{Entities: []string{"INFY"}, PipelineProfile: "impact_test"}, articles)
	if err != nil {
		t.Fatalf("service run: %v", err)
	}

	if len(result.FeatureRows) != 2 {
		t.Fatalf("expected 2 feature rows, got %d", len(result.FeatureRows))
	}

	pairs := map[string]bool{}
	weightSum := 0.0
	for _, row := range result.FeatureRows {
		pairs[row.ParentEntity+"|"+row.ChildEntity] = true
		if !strings.Contains(row.SentimentDisplay, "(") || !strings.Contains(row.SentimentDisplay, ")") {
			t.Fatalf("expected sentiment display with parentheses, got %q", row.SentimentDisplay)
		}
		if row.Weight <= 0 {
			t.Fatalf("expected positive weight, got %f", row.Weight)
		}
		if row.Weight >= 1.0 {
			t.Fatalf("expected weight < 1.0 for multi-row impact event, got %f", row.Weight)
		}
		if row.ConfidenceScore <= 0 {
			t.Fatalf("expected positive confidence score, got %f", row.ConfidenceScore)
		}
		weightSum += row.Weight
	}
	if math.Abs(weightSum-1.0) > 1e-9 {
		t.Fatalf("expected row weights to sum to 1.0, got %f", weightSum)
	}
	if !pairs["INFY|OPENAI"] {
		t.Fatalf("missing INFY|OPENAI pair")
	}
	if !pairs["INFY|GEMINI"] {
		t.Fatalf("missing INFY|GEMINI pair")
	}
}

func TestServiceRunImpactModeMarksMixedProviderWhenPairProviderDiffersFromBase(t *testing.T) {
	cfg := loadImpactMixedProviderConfig(t)
	svc := engine.NewService(cfg, storage.NewInMemoryStore())

	now := time.Now().UTC()
	articles := []core.Article{{
		ID:          "a-mixed-provider",
		SourceID:    "test-source",
		SourceName:  "Test Source",
		URL:         "https://example.com/a-mixed-provider",
		Title:       "INFY reports strong cloud growth with OPENAI",
		Summary:     "INFY OPENAI cloud growth",
		Body:        "INFY OPENAI cloud growth",
		Language:    "en",
		Region:      "india",
		PublishedAt: now,
		IngestedAt:  now,
	}}

	result, err := svc.Run(context.Background(), core.RunRequest{Entities: []string{"INFY"}, PipelineProfile: "impact_test_llm"}, articles)
	if err != nil {
		t.Fatalf("service run: %v", err)
	}

	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.Events))
	}
	if !strings.EqualFold(result.Events[0].Event.Metadata.Provider, "rules") {
		t.Fatalf("expected base event provider rules, got %s", result.Events[0].Event.Metadata.Provider)
	}
	if len(result.FeatureRows) != 1 {
		t.Fatalf("expected 1 feature row, got %d", len(result.FeatureRows))
	}

	row := result.FeatureRows[0]
	if row.Provider != "mixed" {
		t.Fatalf("expected mixed provider attribution for combined base+pair usage, got %q", row.Provider)
	}
	if row.Model != "mixed" {
		t.Fatalf("expected mixed model attribution for combined base+pair usage, got %q", row.Model)
	}
}


func TestServiceRunBuildsParentOnlyRowsWithNAChild(t *testing.T) {
	cfg := loadImpactTestConfig(t)
	svc := engine.NewService(cfg, storage.NewInMemoryStore())

	now := time.Now().UTC()
	articles := []core.Article{{
		ID:          "a2",
		SourceID:    "test-source",
		SourceName:  "Test Source",
		URL:         "https://example.com/a2",
		Title:       "INFY reports cloud contract wins",
		Summary:     "INFY wins deals",
		Body:        "INFY cloud",
		Language:    "en",
		Region:      "india",
		PublishedAt: now,
		IngestedAt:  now,
	}}

	result, err := svc.Run(context.Background(), core.RunRequest{Entities: []string{"INFY"}, PipelineProfile: "impact_test"}, articles)
	if err != nil {
		t.Fatalf("service run: %v", err)
	}

	if len(result.FeatureRows) != 1 {
		t.Fatalf("expected 1 feature row, got %d", len(result.FeatureRows))
	}
	if result.FeatureRows[0].ParentEntity != "INFY" {
		t.Fatalf("expected parent INFY, got %s", result.FeatureRows[0].ParentEntity)
	}
	if result.FeatureRows[0].ChildEntity != "N/A" {
		t.Fatalf("expected child N/A, got %s", result.FeatureRows[0].ChildEntity)
	}
	if !strings.Contains(result.FeatureRows[0].SentimentDisplay, "(") || !strings.Contains(result.FeatureRows[0].SentimentDisplay, ")") {
		t.Fatalf("expected sentiment display with parentheses, got %q", result.FeatureRows[0].SentimentDisplay)
	}
	if result.FeatureRows[0].Weight <= 0 {
		t.Fatalf("expected positive weight, got %f", result.FeatureRows[0].Weight)
	}
	if result.FeatureRows[0].ConfidenceScore <= 0 {
		t.Fatalf("expected positive confidence score, got %f", result.FeatureRows[0].ConfidenceScore)
	}
}

func TestServiceRunSkipsArticlesWithoutParentMatchInImpactMode(t *testing.T) {
	cfg := loadImpactTestConfig(t)
	svc := engine.NewService(cfg, storage.NewInMemoryStore())

	now := time.Now().UTC()
	articles := []core.Article{{
		ID:          "a3",
		SourceID:    "test-source",
		SourceName:  "Test Source",
		URL:         "https://example.com/a3",
		Title:       "OPENAI cloud tooling update",
		Summary:     "OPENAI launches feature",
		Body:        "OPENAI cloud",
		Language:    "en",
		Region:      "india",
		PublishedAt: now,
		IngestedAt:  now,
	}}

	result, err := svc.Run(context.Background(), core.RunRequest{Entities: []string{"INFY"}, PipelineProfile: "impact_test"}, articles)
	if err != nil {
		t.Fatalf("service run: %v", err)
	}

	if len(result.FeatureRows) != 0 {
		t.Fatalf("expected 0 feature rows, got %d", len(result.FeatureRows))
	}
}

func TestServiceRunImpactModeIgnoresNonParentRequestedEntities(t *testing.T) {
	cfg := loadImpactTestConfigWithNiftyIT(t)
	svc := engine.NewService(cfg, storage.NewInMemoryStore())

	now := time.Now().UTC()
	articles := []core.Article{{
		ID:          "a4",
		SourceID:    "test-source",
		SourceName:  "Test Source",
		URL:         "https://example.com/a4",
		Title:       "NIFTIT index update",
		Summary:     "NIFTIT index moves higher",
		Body:        "NIFTIT cloud index",
		Language:    "en",
		Region:      "india",
		PublishedAt: now,
		IngestedAt:  now,
	}}

	result, err := svc.Run(context.Background(), core.RunRequest{Entities: []string{"INFY", "NIFTIT"}, PipelineProfile: "impact_test"}, articles)
	if err != nil {
		t.Fatalf("service run: %v", err)
	}

	if len(result.FeatureRows) != 0 {
		t.Fatalf("expected 0 feature rows when only non-parent requested entity matches, got %d", len(result.FeatureRows))
	}
}

func TestServiceRunImpactModeRespectsPerParentChildMapping(t *testing.T) {
	cfg := loadImpactPerParentMappingConfig(t)
	svc := engine.NewService(cfg, storage.NewInMemoryStore())

	now := time.Now().UTC()
	articles := []core.Article{{
		ID:          "a5",
		SourceID:    "test-source",
		SourceName:  "Test Source",
		URL:         "https://example.com/a5",
		Title:       "INFY and TCS announce platform updates with OPENAI and AWS",
		Summary:     "INFY TCS OPENAI AWS collaboration",
		Body:        "INFY TCS OPENAI AWS cloud",
		Language:    "en",
		Region:      "india",
		PublishedAt: now,
		IngestedAt:  now,
	}}

	result, err := svc.Run(context.Background(), core.RunRequest{Entities: []string{"INFY", "TCS"}, PipelineProfile: "impact_test"}, articles)
	if err != nil {
		t.Fatalf("service run: %v", err)
	}

	pairs := map[string]bool{}
	for _, row := range result.FeatureRows {
		pairs[row.ParentEntity+"|"+row.ChildEntity] = true
	}

	if !pairs["INFY|OPENAI"] {
		t.Fatalf("expected INFY|OPENAI pair")
	}
	if !pairs["TCS|AWS"] {
		t.Fatalf("expected TCS|AWS pair")
	}
	if pairs["INFY|AWS"] {
		t.Fatalf("unexpected invalid cross pair INFY|AWS")
	}
	if pairs["TCS|OPENAI"] {
		t.Fatalf("unexpected invalid cross pair TCS|OPENAI")
	}
	if len(result.FeatureRows) != 2 {
		t.Fatalf("expected exactly 2 feature rows, got %d", len(result.FeatureRows))
	}
}

func TestServiceRunImpactModeConservesBaseUsageAcrossRows(t *testing.T) {
	cfg := loadImpactPerParentMappingConfig(t)
	router := enrich.NewProviderRouter(cfg.Providers)
	svc := engine.NewService(cfg, storage.NewInMemoryStore())

	now := time.Now().UTC()
	articles := []core.Article{{
		ID:          "a6",
		SourceID:    "test-source",
		SourceName:  "Test Source",
		URL:         "https://example.com/a6",
		Title:       "INFY and TCS report growth with OPENAI and AWS",
		Summary:     "INFY TCS OPENAI AWS growth update",
		Body:        "INFY TCS OPENAI AWS growth",
		Language:    "en",
		Region:      "india",
		PublishedAt: now,
		IngestedAt:  now,
	}}

	pairTextINFYOpenAI := strings.TrimSpace(strings.Join([]string{
		"Parent: INFY",
		"Child: OPENAI",
		"Title: INFY and TCS report growth with OPENAI and AWS",
		"Summary: INFY TCS OPENAI AWS growth update",
		"Body: INFY TCS OPENAI AWS growth",
	}, "\n"))
	pairTextTCSAWS := strings.TrimSpace(strings.Join([]string{
		"Parent: TCS",
		"Child: AWS",
		"Title: INFY and TCS report growth with OPENAI and AWS",
		"Summary: INFY TCS OPENAI AWS growth update",
		"Body: INFY TCS OPENAI AWS growth",
	}, "\n"))

	firstPairUsage, err := router.Enrich(context.Background(), providers.ClassificationRequest{Text: pairTextINFYOpenAI})
	if err != nil {
		t.Fatalf("router pair usage 1: %v", err)
	}
	secondPairUsage, err := router.Enrich(context.Background(), providers.ClassificationRequest{Text: pairTextTCSAWS})
	if err != nil {
		t.Fatalf("router pair usage 2: %v", err)
	}

	result, err := svc.Run(context.Background(), core.RunRequest{Entities: []string{"INFY", "TCS"}, PipelineProfile: "impact_test"}, articles)
	if err != nil {
		t.Fatalf("service run: %v", err)
	}

	if len(result.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(result.Events))
	}
	if len(result.FeatureRows) != 2 {
		t.Fatalf("expected 2 feature rows, got %d", len(result.FeatureRows))
	}

	baseInput := result.Events[0].Event.Metadata.InputTokens
	baseOutput := result.Events[0].Event.Metadata.OutputTokens
	baseCost := result.Events[0].Event.Metadata.EstimatedCostUS

	totalInput := 0
	totalOutput := 0
	totalCost := 0.0
	for _, row := range result.FeatureRows {
		totalInput += row.InputTokens
		totalOutput += row.OutputTokens
		totalCost += row.EstimatedCostUS
	}

	expectedPairInput := firstPairUsage.InputTokens + secondPairUsage.InputTokens
	expectedPairOutput := firstPairUsage.OutputTokens + secondPairUsage.OutputTokens
	expectedPairCost := firstPairUsage.EstimatedCost + secondPairUsage.EstimatedCost

	if totalInput != baseInput+expectedPairInput {
		t.Fatalf("expected total input tokens %d (base %d + pair %d), got %d", baseInput+expectedPairInput, baseInput, expectedPairInput, totalInput)
	}
	if totalOutput != baseOutput+expectedPairOutput {
		t.Fatalf("expected total output tokens %d (base %d + pair %d), got %d", baseOutput+expectedPairOutput, baseOutput, expectedPairOutput, totalOutput)
	}
	if math.Abs(totalCost-(baseCost+expectedPairCost)) > 1e-9 {
		t.Fatalf("expected total cost %.12f (base %.12f + pair %.12f), got %.12f", baseCost+expectedPairCost, baseCost, expectedPairCost, totalCost)
	}
}

func loadImpactTestConfig(t *testing.T) config.AppConfig {
	t.Helper()
	return loadImpactTestConfigWithCustomEntities(t, "version: v1\nentities:\n  - id: child-openai\n    symbol: OPENAI\n    name: OpenAI\n    aliases: [OPENAI, OpenAI]\n    exchange: GLOBAL\n    sector: AI\n    role: child\n    type: equity\n    enabled: true\n  - id: child-gemini\n    symbol: GEMINI\n    name: Gemini\n    aliases: [GEMINI, Gemini]\n    exchange: GLOBAL\n    sector: AI\n    role: child\n    type: equity\n    enabled: true\n")
}

func loadImpactTestConfigWithNiftyIT(t *testing.T) config.AppConfig {
	t.Helper()
	return loadImpactTestConfigWithCustomEntities(t, "version: v1\nentities:\n  - id: nse-niftit\n    symbol: NIFTIT\n    name: Nifty IT Index\n    aliases: [NIFTIT, Nifty IT]\n    exchange: NSE\n    sector: Index\n    role: child\n    type: index\n    enabled: true\n  - id: child-openai\n    symbol: OPENAI\n    name: OpenAI\n    aliases: [OPENAI, OpenAI]\n    exchange: GLOBAL\n    sector: AI\n    role: child\n    type: equity\n    enabled: true\n  - id: child-gemini\n    symbol: GEMINI\n    name: Gemini\n    aliases: [GEMINI, Gemini]\n    exchange: GLOBAL\n    sector: AI\n    role: child\n    type: equity\n    enabled: true\n")
}

func loadImpactPerParentMappingConfig(t *testing.T) config.AppConfig {
	t.Helper()
	return loadImpactTestConfigWithInputs(
		t,
		"version: v1\nentities:\n  - id: nse-infy\n    symbol: INFY\n    name: Infosys Limited\n    aliases: [INFY, Infosys]\n    exchange: NSE\n    sector: IT\n    role: parent\n    type: equity\n    enabled: true\n  - id: nse-tcs\n    symbol: TCS\n    name: Tata Consultancy Services\n    aliases: [TCS]\n    exchange: NSE\n    sector: IT\n    role: parent\n    type: equity\n    enabled: true\n",
		"version: v1\nentities:\n  - id: child-openai\n    symbol: OPENAI\n    name: OpenAI\n    aliases: [OPENAI, OpenAI]\n    exchange: GLOBAL\n    sector: AI\n    role: child\n    type: equity\n    enabled: true\n  - id: child-aws\n    symbol: AWS\n    name: Amazon Web Services\n    aliases: [AWS]\n    exchange: GLOBAL\n    sector: Cloud\n    role: child\n    type: equity\n    enabled: true\n",
		"version: v1\ngroups:\n  - id: nifty-it-impact\n    parent_symbol: INFY\n    child_symbols: [OPENAI]\n  - id: nifty-it-impact\n    parent_symbol: TCS\n    child_symbols: [AWS]\n",
	)
}

func loadImpactMixedProviderConfig(t *testing.T) config.AppConfig {
	t.Helper()
	return loadImpactTestConfigWithInputs(
		t,
		"version: v1\nentities:\n  - id: nse-infy\n    symbol: INFY\n    name: Infosys Limited\n    aliases: [INFY, Infosys]\n    exchange: NSE\n    sector: IT\n    role: parent\n    type: equity\n    enabled: true\n",
		"version: v1\nentities:\n  - id: child-openai\n    symbol: OPENAI\n    name: OpenAI\n    aliases: [OPENAI, OpenAI]\n    exchange: GLOBAL\n    sector: AI\n    role: child\n    type: equity\n    enabled: true\n",
		"version: v1\ngroups:\n  - id: nifty-it-impact\n    parent_symbol: INFY\n    child_symbols: [OPENAI]\n",
	)
}

func loadImpactTestConfigWithCustomEntities(t *testing.T, customEntitiesYAML string) config.AppConfig {
	t.Helper()
	return loadImpactTestConfigWithInputs(
		t,
		"version: v1\nentities:\n  - id: nse-infy\n    symbol: INFY\n    name: Infosys Limited\n    aliases: [INFY, Infosys]\n    exchange: NSE\n    sector: IT\n    role: parent\n    type: equity\n    enabled: true\n",
		customEntitiesYAML,
		"version: v1\ngroups:\n  - id: nifty-it-impact\n    parent_symbol: INFY\n    child_symbols: [OPENAI, GEMINI]\n",
	)
}

func loadImpactTestConfigWithInputs(t *testing.T, defaultEntitiesYAML, customEntitiesYAML, entityGroupsYAML string) config.AppConfig {
	t.Helper()

	dir := t.TempDir()
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: test-source\n    name: Test Source\n    kind: rss\n    url: https://example.com/rss\n    region: india\n    language: en\n    enabled: true\n    crawl_fallback: false\n")
	mustWrite(t, dir, "entities.niftyit.yaml", defaultEntitiesYAML)
	mustWrite(t, dir, "entities.custom.yaml", customEntitiesYAML)
	mustWrite(t, dir, "entity_groups.yaml", entityGroupsYAML)
	mustWrite(t, dir, "factors.yaml", "version: v1\nfactors:\n  - id: f-cloud\n    name: Cloud\n    category: demand\n    keywords: [cloud]\n    weight: 1\n")
	mustWrite(t, dir, "providers.yaml", "version: v1\ndefaults:\n  routing_policy: lowest_cost\n  prompt_version: v1\n  circuit_breaker_failures: 3\n  circuit_breaker_seconds: 30\n  retry_count: 2\n  backoff_millis: 100\nproviders:\n  - name: gemini\n    model: gemini-2.0-flash\n    enabled: true\n    price_per_1k_input: 0.1\n    price_per_1k_output: 0.1\n    max_input_tokens: 1024\n    max_output_tokens: 512\n    timeout_seconds: 10\n    max_requests_per_min: 60\nfallback_chain: [gemini:gemini-2.0-flash]\nper_run_token_budget: 100000\nper_provider_token_budget: 100000\nper_run_cost_budget_usd: 100\nper_provider_cost_budget_usd: 100\n")
	mustWrite(t, dir, "pipelines.yaml", "version: v1\ndefault_profile: impact_test\nprofiles:\n  - name: impact_test\n    description: impact test profile\n    ambiguity_threshold: 0.9\n    novelty_threshold: 1.0\n    min_relevance_score: 0.0\n    enable_raw_artifacts: false\n    llm_budget_tokens: 1000\n    session: nse\n  - name: impact_test_llm\n    description: impact test profile with llm refinement\n    ambiguity_threshold: 0.0\n    novelty_threshold: 1.0\n    min_relevance_score: 0.0\n    enable_raw_artifacts: false\n    llm_budget_tokens: 1000\n    session: nse\n")

	cfg, err := config.Load(filepath.Clean(dir))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}
