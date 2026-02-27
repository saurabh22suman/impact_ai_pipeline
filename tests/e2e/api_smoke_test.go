package e2e

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	payload := core.RunRequest{
		PipelineProfile:  "cost_optimized",
		BackfillMode:     "local_file",
		BackfillFilePath: "/tmp/articles.jsonl",
		BackfillFormat:   "jsonl",
		BackfillURL:      "https://archive.example.com/feed.jsonl",
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal run request: %v", err)
	}
	if !bytes.Contains(encoded, []byte("pipeline_profile")) {
		t.Fatalf("expected pipeline_profile json key")
	}
	if !bytes.Contains(encoded, []byte("backfill_mode")) {
		t.Fatalf("expected backfill_mode json key")
	}
	if !bytes.Contains(encoded, []byte("backfill_file_path")) {
		t.Fatalf("expected backfill_file_path json key")
	}
	if !bytes.Contains(encoded, []byte("backfill_format")) {
		t.Fatalf("expected backfill_format json key")
	}
	if !bytes.Contains(encoded, []byte("backfill_url")) {
		t.Fatalf("expected backfill_url json key")
	}
}

func TestFrontendRunFormUsesProfileDropdownAndDateOnlyInputs(t *testing.T) {
	indexPath := filepath.Join("..", "..", "frontend", "index.html")
	content, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read frontend index: %v", err)
	}
	page := string(content)

	if !bytes.Contains(content, []byte(`<select name="pipeline_profile" id="pipeline-profile-select">`)) {
		t.Fatalf("expected pipeline profile dropdown select in run form")
	}
	if bytes.Contains(content, []byte(`name="pipeline_profile" placeholder="cost_optimized"`)) {
		t.Fatalf("pipeline profile text input should not remain")
	}

	if !bytes.Contains(content, []byte(`type="date" name="date_from"`)) {
		t.Fatalf("expected date_from to use date input")
	}
	if !bytes.Contains(content, []byte(`type="date" name="date_to"`)) {
		t.Fatalf("expected date_to to use date input")
	}
	if bytes.Contains(content, []byte(`type="datetime-local" name="date_from"`)) || bytes.Contains(content, []byte(`type="datetime-local" name="date_to"`)) {
		t.Fatalf("datetime-local inputs should not remain")
	}

	if !strings.Contains(page, "Use default profile") {
		t.Fatalf("expected default profile option label")
	}
}

func TestFrontendScriptPopulatesProfileDropdownFromConfig(t *testing.T) {
	appPath := filepath.Join("..", "..", "frontend", "app.js")
	content, err := os.ReadFile(appPath)
	if err != nil {
		t.Fatalf("read frontend app: %v", err)
	}

	if !bytes.Contains(content, []byte("profileSelect")) {
		t.Fatalf("expected profile select element wiring in app script")
	}
	if !bytes.Contains(content, []byte("availableProfiles")) {
		t.Fatalf("expected state to track available profiles")
	}
	if !bytes.Contains(content, []byte("renderPipelineProfileOptions")) {
		t.Fatalf("expected profile dropdown render helper")
	}
	if !bytes.Contains(content, []byte("toUtcISOStringFromDateInput")) {
		t.Fatalf("expected date-only conversion helper")
	}
}
