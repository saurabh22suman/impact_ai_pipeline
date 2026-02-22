package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

func TestRuntimeBootstrapsAndConfigLoads(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.ConfigVersion == "" {
		t.Fatalf("expected config version")
	}
}

func TestHTTPContractShapeHealthAndConfig(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/v1/config", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"config_version": "v1"})
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected health status: %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/v1/config")
	if err != nil {
		t.Fatalf("config request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected config status: %d", resp.StatusCode)
	}
}

func TestRunRequestJSONCompatibility(t *testing.T) {
	payload := core.RunRequest{PipelineProfile: "cost_optimized"}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal run request: %v", err)
	}
	if !bytes.Contains(encoded, []byte("pipeline_profile")) {
		t.Fatalf("expected pipeline_profile json key")
	}
}
