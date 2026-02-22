package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/storage"
)

func TestStoreFromEnvMemoryMode(t *testing.T) {
	setEnv(t, "STORAGE_MODE", "memory")

	store, cleanup, err := storage.NewStoreFromEnv(context.Background())
	if err != nil {
		t.Fatalf("new store from env: %v", err)
	}
	defer func() { _ = cleanup() }()

	run := core.RunResult{
		RunID:         "run-memory-1",
		Status:        core.RunStatusCompleted,
		CreatedAt:     time.Now().UTC(),
		StartedAt:     time.Now().UTC(),
		FinishedAt:    time.Now().UTC(),
		ConfigVersion: "v1",
		Profile:       "cost_optimized",
	}
	if err := store.SaveRun(context.Background(), run); err != nil {
		t.Fatalf("save run: %v", err)
	}

	if err := store.SaveFeatureRows(context.Background(), run.RunID, []core.FeatureRow{{RunID: run.RunID, Symbol: "INFY"}}); err != nil {
		t.Fatalf("save features: %v", err)
	}
	rows := store.GetFeatureRows(context.Background(), run.RunID)
	if len(rows) != 1 {
		t.Fatalf("expected 1 feature row, got %d", len(rows))
	}

	if err := store.Put(context.Background(), "k1", []byte("v1")); err != nil {
		t.Fatalf("put artifact: %v", err)
	}
	payload, ok := store.Get(context.Background(), "k1")
	if !ok || string(payload) != "v1" {
		t.Fatalf("artifact roundtrip failed")
	}
}

func setEnv(t *testing.T, key, value string) {
	t.Helper()
	previous, hadPrevious := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("setenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if !hadPrevious {
			_ = os.Unsetenv(key)
			return
		}
		_ = os.Setenv(key, previous)
	})
}
