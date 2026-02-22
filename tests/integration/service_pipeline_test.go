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

func TestServiceRunProducesProvenancedOutputs(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	svc := engine.NewService(cfg, storage.NewInMemoryStore())

	now := time.Now().UTC()
	articles := []core.Article{
		{
			ID:          "a1",
			SourceID:    "moneycontrol-markets",
			SourceName:  "Moneycontrol",
			URL:         "https://example.com/a1",
			Title:       "Infosys sees strong AI demand and capex expansion with compliance hiring",
			Summary:     "Enterprise AI adoption accelerates with regulation focus",
			Body:        "AI demand remains strong with datacenter capex growth and hiring momentum",
			Language:    "en",
			Region:      "india",
			PublishedAt: now,
			IngestedAt:  now,
		},
	}

	result, err := svc.Run(context.Background(), core.RunRequest{PipelineProfile: "high_recall", Entities: []string{"INFY"}}, articles)
	if err != nil {
		t.Fatalf("service run: %v", err)
	}
	if result.RunID == "" {
		t.Fatalf("expected run id")
	}
	if len(result.Events) == 0 {
		t.Fatalf("expected at least one event")
	}
	for _, event := range result.Events {
		meta := event.Event.Metadata
		if meta.RunID == "" || meta.ConfigVersion == "" || meta.PipelineProfile == "" || meta.PromptVersion == "" {
			t.Fatalf("missing provenance metadata: %+v", meta)
		}
	}

	jsonl, err := svc.ExportJSONL(context.Background(), result.RunID)
	if err != nil || len(jsonl) == 0 {
		t.Fatalf("jsonl export failed: %v", err)
	}
	csvData, err := svc.ExportCSV(context.Background(), result.RunID)
	if err != nil || len(csvData) == 0 {
		t.Fatalf("csv export failed: %v", err)
	}
	toon, err := svc.ExportTOON(context.Background(), result.RunID)
	if err != nil || len(toon) == 0 {
		t.Fatalf("toon export failed: %v", err)
	}
}
