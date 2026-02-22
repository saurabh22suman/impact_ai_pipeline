package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/engine"
	"github.com/soloengine/ai-impact-scrapper/internal/sourceadmin"
	"github.com/soloengine/ai-impact-scrapper/internal/storage"
)

func TestHandleConfigIncludesEnabledSourceMetadataAndEntities(t *testing.T) {
	srv := &server{rt: &engine.Runtime{Config: config.AppConfig{
		ConfigVersion: "2026-02-22",
		Sources: config.SourcesFile{Sources: []config.Source{
			{ID: "bbc-business", Name: "BBC Business", Region: "global", Language: "en", Kind: config.SourceKindRSS, CrawlFallback: false, Enabled: true},
			{ID: "disabled-source", Name: "Disabled", Region: "global", Language: "en", Kind: config.SourceKindDirect, CrawlFallback: true, Enabled: false},
			{ID: "nyt-technology", Name: "NYT Technology", Region: "us", Language: "en", Kind: config.SourceKindDirect, CrawlFallback: true, Enabled: true},
		}},
		EntitiesDefault: config.EntitiesFile{Entities: []config.Entity{
			{ID: "nse-infy", Symbol: "INFY", Name: "Infosys", Exchange: "NSE", Sector: "IT", Type: "equity", Aliases: []string{"Infosys", "INFY"}, Enabled: true},
		}},
		EntitiesCustom: config.EntitiesFile{Entities: []config.Entity{
			{ID: "nse-index-niftit", Symbol: "NIFTIT", Name: "Nifty IT", Exchange: "NSE", Sector: "Index", Type: "index", Aliases: []string{"NIFTIT"}, Enabled: true},
			{ID: "disabled-custom", Symbol: "FOO", Name: "Foo", Enabled: false},
		}},
		Pipelines: config.PipelinesFile{Profiles: []config.PipelineProfile{{Name: "cost_optimized"}}},
	}}}

	req := httptest.NewRequest(http.MethodGet, "/v1/config", nil)
	rec := httptest.NewRecorder()

	srv.handleConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var payload struct {
		ConfigVersion  string `json:"config_version"`
		SourcesEnabled int    `json:"sources_enabled"`
		Sources        []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			Region        string `json:"region"`
			Language      string `json:"language"`
			Kind          string `json:"kind"`
			CrawlFallback bool   `json:"crawl_fallback"`
		} `json:"sources"`
		EntitiesEffective int `json:"entities_effective"`
		Entities          []struct {
			ID       string   `json:"id"`
			Symbol   string   `json:"symbol"`
			Name     string   `json:"name"`
			Exchange string   `json:"exchange"`
			Sector   string   `json:"sector"`
			Type     string   `json:"type"`
			Aliases  []string `json:"aliases"`
		} `json:"entities"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.ConfigVersion != "2026-02-22" {
		t.Fatalf("expected config_version 2026-02-22, got %q", payload.ConfigVersion)
	}
	if payload.SourcesEnabled != 2 {
		t.Fatalf("expected sources_enabled=2, got %d", payload.SourcesEnabled)
	}
	if len(payload.Sources) != 2 {
		t.Fatalf("expected 2 enabled sources in response, got %d", len(payload.Sources))
	}

	if payload.Sources[0].ID != "bbc-business" || payload.Sources[0].Name != "BBC Business" {
		t.Fatalf("unexpected first source: %+v", payload.Sources[0])
	}
	if payload.Sources[0].Kind != config.SourceKindRSS || payload.Sources[0].CrawlFallback {
		t.Fatalf("unexpected first source kind/fallback: %+v", payload.Sources[0])
	}
	if payload.Sources[1].ID != "nyt-technology" || payload.Sources[1].Region != "us" {
		t.Fatalf("unexpected second source: %+v", payload.Sources[1])
	}
	if payload.Sources[1].Kind != config.SourceKindDirect || !payload.Sources[1].CrawlFallback {
		t.Fatalf("unexpected second source kind/fallback: %+v", payload.Sources[1])
	}

	if payload.EntitiesEffective != 2 {
		t.Fatalf("expected entities_effective=2, got %d", payload.EntitiesEffective)
	}
	if len(payload.Entities) != 2 {
		t.Fatalf("expected entities array size 2, got %d", len(payload.Entities))
	}
	if payload.Entities[0].Symbol == "" || payload.Entities[0].Type == "" {
		t.Fatalf("expected entity symbol/type in response, got %+v", payload.Entities[0])
	}
}

func TestHandleSourcesCreatePersistsAndReloadsConfig(t *testing.T) {
	dir := t.TempDir()
	writeAPITestBaseConfigFiles(t, dir)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	srv := &server{
		rt: &engine.Runtime{
			Config:    cfg,
			ConfigDir: dir,
			Service:   engine.NewService(cfg, storage.NewInMemoryStore()),
		},
		sourceAdmin: sourceadmin.NewService(dir),
	}

	body := []byte(`{"id":"test-source","name":"Test Source","kind":"rss","url":"https://example.com/rss.xml","region":"india","language":"en","enabled":true,"crawl_fallback":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/sources", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	srv.handleSources(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d body=%s", rec.Code, rec.Body.String())
	}

	reloaded, err := config.Load(dir)
	if err != nil {
		t.Fatalf("reload config after source create: %v", err)
	}
	found := false
	for _, src := range reloaded.Sources.Sources {
		if src.ID == "test-source" {
			found = true
			if src.Kind != config.SourceKindRSS || !src.CrawlFallback || !src.Enabled {
				t.Fatalf("unexpected persisted source values: %+v", src)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected persisted source in sources.yaml")
	}

	if len(srv.rt.Config.EnabledSources()) != len(reloaded.EnabledSources()) {
		t.Fatalf("expected runtime config reload after source create")
	}
}

func TestHandleRunCreateRejectsUnknownEntitySelection(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	now := time.Now().UTC()
	fetcher := apiStubFetcher{articles: []core.Article{{
		ID:          "a1",
		SourceID:    "bbc-business",
		SourceName:  "BBC Business",
		URL:         "https://example.com/a1",
		Title:       "Infosys update",
		Summary:     "INFY update",
		Body:        "INFY update",
		Language:    "en",
		Region:      "global",
		PublishedAt: now,
		IngestedAt:  now,
	}}}

	srv := &server{rt: &engine.Runtime{
		Config:  cfg,
		Service: engine.NewService(cfg, storage.NewInMemoryStore()),
		Fetcher: fetcher,
	}}

	req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader([]byte(`{"entities":["NOT_A_SYMBOL"]}`)))
	rec := httptest.NewRecorder()

	srv.handleRunCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "entity") {
		t.Fatalf("expected entity validation error message, got %q", rec.Body.String())
	}
}

func writeAPITestBaseConfigFiles(t *testing.T, dir string) {
	t.Helper()
	mustWriteFile(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: base-source\n    name: Base Source\n    kind: rss\n    url: https://example.com/rss\n    region: global\n    language: en\n    enabled: true\n    crawl_fallback: false\n")
	mustWriteFile(t, dir, "entities.niftyit.yaml", "version: v1\nentities:\n  - id: nse-infy\n    symbol: INFY\n    name: Infosys\n    aliases: [Infosys, INFY]\n    exchange: NSE\n    sector: IT\n    type: equity\n    enabled: true\n")
	mustWriteFile(t, dir, "entities.custom.yaml", "version: v1\nentities: []\n")
	mustWriteFile(t, dir, "factors.yaml", "version: v1\nfactors:\n  - id: f1\n    name: Demand\n    category: demand\n    keywords: [ai]\n    weight: 1\n")
	mustWriteFile(t, dir, "providers.yaml", "version: v1\ndefaults:\n  routing_policy: lowest\n  prompt_version: v1\n  circuit_breaker_failures: 3\n  circuit_breaker_seconds: 30\n  retry_count: 2\n  backoff_millis: 100\nproviders:\n  - name: gemini\n    model: gemini-2.0-flash\n    enabled: true\n    price_per_1k_input: 0.1\n    price_per_1k_output: 0.2\n    max_input_tokens: 1000\n    max_output_tokens: 500\n    timeout_seconds: 10\n    max_requests_per_min: 60\nfallback_chain: [gemini:gemini-2.0-flash]\nper_run_token_budget: 1000\nper_provider_token_budget: 1000\nper_run_cost_budget_usd: 1\nper_provider_cost_budget_usd: 1\n")
	mustWriteFile(t, dir, "pipelines.yaml", "version: v1\ndefault_profile: cost\nprofiles:\n  - name: cost\n    description: d\n    ambiguity_threshold: 0.5\n    novelty_threshold: 0.6\n    min_relevance_score: 0.2\n    enable_raw_artifacts: false\n    llm_budget_tokens: 1000\n    session: nse\n")
}

func mustWriteFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

type apiStubFetcher struct {
	articles []core.Article
}

func (f apiStubFetcher) Fetch(_ context.Context, _ config.Source) ([]core.Article, error) {
	return append([]core.Article{}, f.articles...), nil
}

func (f apiStubFetcher) FetchWithNotices(_ context.Context, source config.Source) ([]core.Article, []string, error) {
	return append([]core.Article{}, f.articles...), []string{fmt.Sprintf("fetched source %s", source.ID)}, nil
}
