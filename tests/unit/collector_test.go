package unit

import (
	"context"
	"errors"
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
)

type collectorStubFetcher struct {
	articles      []core.Article
	err           error
	notices       []string
	callsBySource map[string]int
}

func (f *collectorStubFetcher) Fetch(_ context.Context, source config.Source) ([]core.Article, error) {
	if f.callsBySource == nil {
		f.callsBySource = map[string]int{}
	}
	f.callsBySource[source.ID]++
	if f.err != nil {
		return nil, f.err
	}
	return f.articles, nil
}

func (f *collectorStubFetcher) FetchWithNotices(_ context.Context, source config.Source) ([]core.Article, []string, error) {
	if f.callsBySource == nil {
		f.callsBySource = map[string]int{}
	}
	f.callsBySource[source.ID]++
	if f.err != nil {
		return nil, append([]string{}, f.notices...), f.err
	}
	return append([]core.Article{}, f.articles...), append([]string{}, f.notices...), nil
}

func TestCollectArticlesUsesFetcherNotices(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	sources := []config.Source{{ID: "moneycontrol", Kind: config.SourceKindRSS, CrawlFallback: true}}
	fetcher := collectorStubFetcher{
		articles: []core.Article{{ID: "a1", PublishedAt: now}},
		notices:  []string{"source moneycontrol rss fetch failed; used direct fallback"},
	}

	articles, notices, err := engine.CollectArticles(context.Background(), &fetcher, sources, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("collect articles: %v", err)
	}
	if len(articles) != 1 {
		t.Fatalf("expected 1 article, got %d", len(articles))
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "used direct fallback") {
		t.Fatalf("expected propagated fallback notice, got %v", notices)
	}
	for _, n := range notices {
		if strings.Contains(n, "pending implementation") {
			t.Fatalf("unexpected placeholder fallback notice: %q", n)
		}
	}
}

func TestCollectArticlesReturnsErrorWhenFallbackFails(t *testing.T) {
	sources := []config.Source{{ID: "moneycontrol", Kind: config.SourceKindRSS, CrawlFallback: true}}
	fetcher := collectorStubFetcher{
		err:     errors.New("rss fetch failed: bad status; direct fallback failed: parse listing"),
		notices: []string{"source moneycontrol rss fetch failed; direct fallback failed"},
	}

	_, notices, err := engine.CollectArticles(context.Background(), &fetcher, sources, time.Time{}, time.Time{})
	if err == nil {
		t.Fatalf("expected collection error when fallback fails")
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "direct fallback failed") {
		t.Fatalf("expected fallback failure notice, got %v", notices)
	}
}

func TestCollectArticlesOnlyFetchesSelectedSources(t *testing.T) {
	allSources := []config.Source{
		{ID: "zerodha-pulse", Kind: config.SourceKindPulse, Enabled: true},
		{ID: "moneycontrol", Kind: config.SourceKindRSS, Enabled: true},
	}
	cfg := config.AppConfig{Sources: config.SourcesFile{Sources: allSources}}

	selectedSources, err := engine.ResolveSources(cfg, []string{"moneycontrol"})
	if err != nil {
		t.Fatalf("resolve selected sources: %v", err)
	}

	fetcher := collectorStubFetcher{articles: []core.Article{}}
	_, _, err = engine.CollectArticles(context.Background(), &fetcher, selectedSources, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("collect selected sources: %v", err)
	}

	if fetcher.callsBySource["moneycontrol"] != 1 {
		t.Fatalf("expected moneycontrol fetched once, got %d", fetcher.callsBySource["moneycontrol"])
	}
	if fetcher.callsBySource["zerodha-pulse"] != 0 {
		t.Fatalf("expected zerodha-pulse not fetched when not selected, got %d", fetcher.callsBySource["zerodha-pulse"])
	}
}

func TestCollectArticlesForRequestLoadsBackfillLocalFileWithDateFilter(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	from := now.Add(-2 * time.Hour)
	to := now.Add(-30 * time.Minute)

	payload := strings.Join([]string{
		`{"id":"a1","source_id":"moneycontrol","source_name":"Moneycontrol","url":"https://example.com/a1","title":"A1","summary":"S1","body":"B1","language":"en","region":"india","published_at":"2026-02-22T10:30:00Z"}`,
		`{"id":"a2","source_id":"moneycontrol","source_name":"Moneycontrol","url":"https://example.com/a2","title":"A2","summary":"S2","body":"B2","language":"en","region":"india","published_at":"2026-02-22T12:30:00Z"}`,
	}, "\n")
	path := filepath.Join(t.TempDir(), "articles.jsonl")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write backfill file: %v", err)
	}

	req := core.RunRequest{
		BackfillMode:     "local_file",
		BackfillFilePath: path,
		BackfillFormat:   "jsonl",
		DateFrom:         from,
		DateTo:           to,
	}
	sources := []config.Source{{ID: "moneycontrol", Kind: config.SourceKindRSS, Enabled: true}}

	articles, notices, err := engine.CollectArticlesForRequest(context.Background(), nil, sources, req)
	if err != nil {
		t.Fatalf("collect backfill articles: %v", err)
	}
	if len(articles) != 1 {
		t.Fatalf("expected 1 filtered article, got %d", len(articles))
	}
	if articles[0].ID != "a1" {
		t.Fatalf("expected article a1 after date filtering, got %s", articles[0].ID)
	}
	if len(notices) == 0 {
		t.Fatalf("expected backfill notice")
	}
}

func TestCollectArticlesForRequestLocalBackfillFiltersBySelectedSource(t *testing.T) {
	payload := strings.Join([]string{
		`{"id":"a1","source_id":"moneycontrol","source_name":"Moneycontrol","url":"https://example.com/a1","title":"A1","summary":"S1","body":"B1","language":"en","region":"india","published_at":"2026-02-22T10:30:00Z"}`,
		`{"id":"a2","source_id":"bbc-business","source_name":"BBC","url":"https://example.com/a2","title":"A2","summary":"S2","body":"B2","language":"en","region":"global","published_at":"2026-02-22T10:40:00Z"}`,
	}, "\n")
	path := filepath.Join(t.TempDir(), "articles.jsonl")
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatalf("write backfill file: %v", err)
	}

	req := core.RunRequest{
		BackfillMode:     "local_file",
		BackfillFilePath: path,
		BackfillFormat:   "jsonl",
	}
	sources := []config.Source{{ID: "moneycontrol", Kind: config.SourceKindRSS, Enabled: true}}

	articles, _, err := engine.CollectArticlesForRequest(context.Background(), nil, sources, req)
	if err != nil {
		t.Fatalf("collect backfill articles: %v", err)
	}
	if len(articles) != 1 {
		t.Fatalf("expected only selected-source article, got %d", len(articles))
	}
	if articles[0].SourceID != "moneycontrol" {
		t.Fatalf("expected selected source moneycontrol, got %s", articles[0].SourceID)
	}
}

type collectorBackfillAdapterStub struct {
	mode     string
	articles []core.Article
	notices  []string
	err      error
	called   bool
}

func (a *collectorBackfillAdapterStub) Mode() string {
	return a.mode
}

func (a *collectorBackfillAdapterStub) Collect(_ context.Context, _ core.RunRequest, _ []config.Source) ([]core.Article, []string, error) {
	a.called = true
	if a.err != nil {
		return nil, nil, a.err
	}
	return append([]core.Article{}, a.articles...), append([]string{}, a.notices...), nil
}

func TestCollectArticlesForRequestWithBackfillUsesRegisteredAdapter(t *testing.T) {
	adapter := &collectorBackfillAdapterStub{
		mode:     "archive_http",
		articles: []core.Article{{ID: "a1", SourceID: "moneycontrol"}},
		notices:  []string{"archive adapter used"},
	}
	req := core.RunRequest{BackfillMode: "archive_http"}
	registry := engine.NewBackfillAdapterRegistry(adapter)

	articles, notices, err := engine.CollectArticlesForRequestWithBackfill(context.Background(), nil, nil, req, registry)
	if err != nil {
		t.Fatalf("collect via adapter registry: %v", err)
	}
	if !adapter.called {
		t.Fatalf("expected registered adapter to be called")
	}
	if len(articles) != 1 || articles[0].ID != "a1" {
		t.Fatalf("unexpected adapter articles: %+v", articles)
	}
	if len(notices) != 1 || notices[0] != "archive adapter used" {
		t.Fatalf("unexpected adapter notices: %+v", notices)
	}
}

func TestCollectArticlesForRequestWithBackfillReturnsErrorForUnknownMode(t *testing.T) {
	req := core.RunRequest{BackfillMode: "archive_http"}

	_, _, err := engine.CollectArticlesForRequestWithBackfill(context.Background(), nil, nil, req, engine.NewBackfillAdapterRegistry())
	if err == nil {
		t.Fatalf("expected unsupported mode error")
	}
	if !strings.Contains(err.Error(), "unsupported backfill_mode") {
		t.Fatalf("expected unsupported backfill_mode error, got %v", err)
	}
}

func TestCollectArticlesForRequestWithBackfillFallsBackToFetcherPath(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	fetcher := collectorStubFetcher{articles: []core.Article{{ID: "a1", SourceID: "moneycontrol", PublishedAt: now}}}
	sources := []config.Source{{ID: "moneycontrol", Kind: config.SourceKindRSS, Enabled: true}}
	req := core.RunRequest{DateFrom: now.Add(-time.Hour), DateTo: now.Add(time.Hour)}

	articles, _, err := engine.CollectArticlesForRequestWithBackfill(context.Background(), &fetcher, sources, req, engine.NewBackfillAdapterRegistry())
	if err != nil {
		t.Fatalf("collect via fetcher path: %v", err)
	}
	if len(articles) != 1 {
		t.Fatalf("expected fetcher article, got %d", len(articles))
	}
	if fetcher.callsBySource["moneycontrol"] != 1 {
		t.Fatalf("expected fetcher to run once, got %d", fetcher.callsBySource["moneycontrol"])
	}
}

func TestNewDefaultBackfillAdapterRegistryRegistersLocalAndArchiveHTTP(t *testing.T) {
	registry := engine.NewDefaultBackfillAdapterRegistry()

	local, ok := registry.Lookup("local_file")
	if !ok {
		t.Fatalf("expected local_file adapter to be registered")
	}
	if local.Mode() != "local_file" {
		t.Fatalf("unexpected local adapter mode %q", local.Mode())
	}

	archive, ok := registry.Lookup("archive_http")
	if !ok {
		t.Fatalf("expected archive_http adapter to be registered")
	}
	if archive.Mode() != "archive_http" {
		t.Fatalf("unexpected archive adapter mode %q", archive.Mode())
	}
}

func TestCollectArticlesForRequestWithBackfillArchiveHTTPRequiresURL(t *testing.T) {
	registry := engine.NewDefaultBackfillAdapterRegistry()
	req := core.RunRequest{BackfillMode: "archive_http"}

	_, _, err := engine.CollectArticlesForRequestWithBackfill(context.Background(), nil, nil, req, registry)
	if err == nil {
		t.Fatalf("expected archive_http URL validation error")
	}
	if !strings.Contains(err.Error(), "backfill_url is required") {
		t.Fatalf("expected backfill_url required error, got %v", err)
	}
}

func TestCollectArticlesForRequestWithBackfillArchiveHTTPSuccess(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)
	from := now.Add(-2 * time.Hour)
	to := now.Add(-30 * time.Minute)

	payload := strings.Join([]string{
		`{"id":"a1","source_id":"moneycontrol","source_name":"Moneycontrol","url":"https://example.com/a1","title":"A1","summary":"S1","body":"B1","language":"en","region":"india","published_at":"2026-02-22T10:30:00Z"}`,
		`{"id":"a2","source_id":"bbc-business","source_name":"BBC","url":"https://example.com/a2","title":"A2","summary":"S2","body":"B2","language":"en","region":"global","published_at":"2026-02-22T10:40:00Z"}`,
	}, "\n")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	req := core.RunRequest{
		BackfillMode:   "archive_http",
		BackfillURL:    server.URL,
		BackfillFormat: "jsonl",
		DateFrom:       from,
		DateTo:         to,
	}
	sources := []config.Source{{ID: "moneycontrol", Kind: config.SourceKindRSS, Enabled: true}}

	articles, notices, err := engine.CollectArticlesForRequestWithBackfill(context.Background(), nil, sources, req, engine.NewDefaultBackfillAdapterRegistry())
	if err != nil {
		t.Fatalf("collect archive_http backfill articles: %v", err)
	}
	if len(articles) != 1 {
		t.Fatalf("expected 1 archive article after source/date filters, got %d", len(articles))
	}
	if articles[0].ID != "a1" {
		t.Fatalf("expected article a1, got %s", articles[0].ID)
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "archive_http") {
		t.Fatalf("expected archive notice, got %+v", notices)
	}
}
