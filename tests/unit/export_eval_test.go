package unit

import (
	"strings"
	"testing"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/eval"
	"github.com/soloengine/ai-impact-scrapper/internal/export"
)

func TestExporterOutputsJSONLCSVTOON(t *testing.T) {
	exp := export.NewExporter()
	event := sampleAlignedEvent()

	jsonl, err := exp.JSONL([]core.MarketAlignedEvent{event})
	if err != nil {
		t.Fatalf("jsonl export failed: %v", err)
	}
	if !strings.Contains(string(jsonl), `"run_id":"run-1"`) {
		t.Fatalf("jsonl missing provenance run_id")
	}

	csvData, err := exp.CSV([]core.FeatureRow{sampleFeatureRow()})
	if err != nil {
		t.Fatalf("csv export failed: %v", err)
	}
	csvStr := string(csvData)
	for _, key := range []string{"run_id", "config_version", "pipeline_profile", "provider", "model", "prompt_version"} {
		if !strings.Contains(csvStr, key) {
			t.Fatalf("csv missing provenance column %s", key)
		}
	}

	toon, err := exp.TOON([]core.MarketAlignedEvent{event})
	if err != nil {
		t.Fatalf("toon export failed: %v", err)
	}
	toonStr := string(toon)
	if !strings.Contains(toonStr, `"run_id":"run-1"`) || !strings.Contains(toonStr, `"prompt_version":"v1"`) {
		t.Fatalf("toon missing required provenance fields")
	}
	if strings.Contains(toonStr, `"config_version"`) {
		t.Fatalf("toon should not include config_version")
	}
}

func TestBuildPurgedWalkForwardWithEmbargo(t *testing.T) {
	rows := make([]core.FeatureRow, 0)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		rows = append(rows, core.FeatureRow{
			SessionDate: start.AddDate(0, 0, i),
		})
	}
	folds := eval.BuildPurgedWalkForward(rows, 5, 2, 1)
	if len(folds) == 0 {
		t.Fatalf("expected at least one fold")
	}
	for _, fold := range folds {
		if !fold.TestStart.After(fold.TrainEnd) {
			t.Fatalf("expected embargo gap between train and test")
		}
	}
}

func sampleAlignedEvent() core.MarketAlignedEvent {
	return core.MarketAlignedEvent{
		Event: core.EnrichedEvent{
			Metadata: core.RunMetadata{
				RunID:           "run-1",
				ConfigVersion:   "2026-02-22",
				PipelineProfile: "cost_optimized",
				Provider:        "mimo",
				Model:           "mimo-v2-synthetic",
				PromptVersion:   "v1",
				InputTokens:     100,
				OutputTokens:    50,
				EstimatedCostUS: 0.02,
			},
			Article:        core.Article{ID: "a1", SourceID: "s1", Title: "AI demand rises", URL: "https://example.com/a1"},
			Entities:       []core.EntityMatch{{EntityID: "e1", Symbol: "TCS", Name: "Tata Consultancy Services", Confidence: 0.9}},
			Factors:        []core.FactorTag{{FactorID: "ai-demand", Name: "AI Demand Signal", Category: "demand", Score: 0.8}},
			SentimentLabel: "positive",
			SentimentScore: 0.4,
			RelevanceScore: 0.7,
		},
		Session: core.MarketSession{SessionDate: time.Date(2026, 2, 22, 0, 0, 0, 0, time.UTC), SessionLabel: "post_close"},
	}
}

func sampleFeatureRow() core.FeatureRow {
	return core.FeatureRow{
		RunID:           "run-1",
		ConfigVersion:   "2026-02-22",
		PipelineProfile: "cost_optimized",
		Provider:        "mimo",
		Model:           "mimo-v2-synthetic",
		PromptVersion:   "v1",
		ArticleID:       "a1",
		Symbol:          "TCS",
		SessionDate:     time.Date(2026, 2, 22, 0, 0, 0, 0, time.UTC),
		SessionLabel:    "post_close",
		SentimentScore:  0.4,
		RelevanceScore:  0.7,
		FactorVector:    []string{"ai-demand"},
		InputTokens:     100,
		OutputTokens:    50,
		EstimatedCostUS: 0.02,
	}
}
