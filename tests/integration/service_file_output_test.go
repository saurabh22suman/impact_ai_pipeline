package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/engine"
	"github.com/soloengine/ai-impact-scrapper/internal/storage"
)

func TestServiceRunWritesFileOutputsWithMockData(t *testing.T) {
	setEnv(t, "TZ", "UTC")
	cfg, err := config.Load(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	root := t.TempDir()
	store, err := storage.NewFileStore(root)
	if err != nil {
		t.Fatalf("new file store: %v", err)
	}
	svc := engine.NewService(cfg, store)

	now := time.Date(2026, 2, 22, 10, 30, 0, 0, time.UTC)
	articles := []core.Article{{
		ID:          "mock-a1",
		SourceID:    "moneycontrol-markets",
		SourceName:  "Moneycontrol",
		URL:         "https://example.com/mock-a1",
		Title:       "Infosys AI demand rises with datacenter capex and compliance wins",
		Summary:     "Mock summary for integration coverage",
		Body:        "Infosys reports strong AI demand, compliance projects, and cloud capex momentum.",
		Language:    "en",
		Region:      "india",
		PublishedAt: now,
		IngestedAt:  now,
	}}

	req := core.RunRequest{
		Entities:        []string{"INFY"},
		Sources:         []string{"moneycontrol-markets"},
		DateFrom:        now.Add(-24 * time.Hour),
		DateTo:          now,
		RawDataToggle:   true,
		PipelineProfile: "high_recall",
	}

	result, err := svc.Run(context.Background(), req, articles)
	if err != nil {
		t.Fatalf("service run: %v", err)
	}
	if result.RunID == "" {
		t.Fatalf("expected run id")
	}

	runDir := filepath.Join(root, "runs", result.RunID)
	requiredFiles := []string{
		filepath.Join(runDir, "run.json"),
		filepath.Join(runDir, "events.json"),
		filepath.Join(runDir, "feature_rows.json"),
		filepath.Join(runDir, "request.json"),
		filepath.Join(runDir, "exports", "events.jsonl"),
		filepath.Join(runDir, "exports", "features.csv"),
		filepath.Join(runDir, "exports", "events.toon.jsonl"),
	}
	for _, path := range requiredFiles {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected output file %s: %v", path, err)
		}
	}

	requestBytes, err := os.ReadFile(filepath.Join(runDir, "request.json"))
	if err != nil {
		t.Fatalf("read request.json: %v", err)
	}
	var savedReq core.RunRequest
	if err := json.Unmarshal(requestBytes, &savedReq); err != nil {
		t.Fatalf("unmarshal request.json: %v", err)
	}
	if savedReq.PipelineProfile != req.PipelineProfile || !savedReq.RawDataToggle {
		t.Fatalf("saved request mismatch: %+v", savedReq)
	}
	if len(savedReq.Entities) != 1 || savedReq.Entities[0] != "INFY" {
		t.Fatalf("expected saved request entities, got %+v", savedReq.Entities)
	}

	csvBytes, err := os.ReadFile(filepath.Join(runDir, "exports", "features.csv"))
	if err != nil {
		t.Fatalf("read features.csv: %v", err)
	}
	csvText := string(csvBytes)
	if !strings.Contains(csvText, "article_id") || !strings.Contains(csvText, "INFY") {
		t.Fatalf("csv export missing expected headers/data")
	}

	jsonlBytes, err := os.ReadFile(filepath.Join(runDir, "exports", "events.jsonl"))
	if err != nil {
		t.Fatalf("read events.jsonl: %v", err)
	}
	if len(strings.TrimSpace(string(jsonlBytes))) == 0 {
		t.Fatalf("expected non-empty events.jsonl")
	}
}
