package unit

import (
	"context"
	"errors"
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
