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

func TestExportCSVReadsFeatureRepository(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	store := storage.NewInMemoryStore()
	svc := engine.NewService(cfg, store)

	run := core.RunResult{
		RunID:         "run-persist-1",
		Status:        core.RunStatusCompleted,
		CreatedAt:     time.Now().UTC(),
		StartedAt:     time.Now().UTC(),
		FinishedAt:    time.Now().UTC(),
		ConfigVersion: cfg.ConfigVersion,
		Profile:       "cost_optimized",
		FeatureRows:   nil,
	}
	if err := store.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("save run: %v", err)
	}
	if err := store.SaveFeatureRows(context.Background(), run.RunID, []core.FeatureRow{
		{
			RunID:           run.RunID,
			ConfigVersion:   cfg.ConfigVersion,
			PipelineProfile: "cost_optimized",
			Provider:        "mimo",
			Model:           "mimo-v2-synthetic",
			PromptVersion:   "v1",
			ArticleID:       "a1",
			Symbol:          "INFY",
			SessionDate:     time.Now().UTC(),
			SessionLabel:    "post_close",
			SentimentScore:  0.2,
			RelevanceScore:  0.6,
			FactorVector:    []string{"ai-demand"},
			InputTokens:     11,
			OutputTokens:    7,
			EstimatedCostUS: 0.01,
		},
	}); err != nil {
		t.Fatalf("save features: %v", err)
	}

	csvBytes, err := svc.ExportCSV(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("export csv: %v", err)
	}
	csv := string(csvBytes)
	if !strings.Contains(csv, "INFY") {
		t.Fatalf("expected feature row symbol in csv output")
	}
}
