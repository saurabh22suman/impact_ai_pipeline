package integration

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/soloengine/ai-impact-scrapper/internal/storage"
)

func TestStoreFromEnvFileMode(t *testing.T) {
	root := filepath.Join(t.TempDir(), "outputs")
	setEnv(t, "STORAGE_MODE", "file")
	setEnv(t, "FILE_STORE_DIR", root)

	store, cleanup, err := storage.NewStoreFromEnv(context.Background())
	if err != nil {
		t.Fatalf("new store from env: %v", err)
	}
	defer func() { _ = cleanup() }()

	if err := store.Put(context.Background(), "runs/test/artifact.txt", []byte("ok")); err != nil {
		t.Fatalf("put artifact: %v", err)
	}
	if _, ok := store.Get(context.Background(), "runs/test/artifact.txt"); !ok {
		t.Fatalf("expected artifact roundtrip from file mode store")
	}
}
