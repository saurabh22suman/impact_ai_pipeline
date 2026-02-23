package ingest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

type stubSourceFetcher struct {
	articles   []core.Article
	err        error
	calls      int
	lastSource config.Source
}

func (s *stubSourceFetcher) Fetch(_ context.Context, source config.Source) ([]core.Article, error) {
	s.calls++
	s.lastSource = source
	if s.err != nil {
		return nil, s.err
	}
	return s.articles, nil
}

func TestRouterDispatchesBySourceKind(t *testing.T) {
	rss := &stubSourceFetcher{articles: []core.Article{{Title: "rss article"}}}
	direct := &stubSourceFetcher{articles: []core.Article{{Title: "direct article"}}}
	pulse := &stubSourceFetcher{articles: []core.Article{{Title: "pulse article"}}}
	router := NewRouterFetcher(rss, direct, pulse)

	rssArticles, rssNotices, err := router.FetchWithNotices(context.Background(), config.Source{ID: "rss-source", Kind: config.SourceKindRSS})
	if err != nil {
		t.Fatalf("fetch rss source: %v", err)
	}
	if len(rssNotices) != 0 {
		t.Fatalf("expected no notices for direct rss success, got %v", rssNotices)
	}
	if len(rssArticles) != 1 || rssArticles[0].Title != "rss article" {
		t.Fatalf("unexpected rss articles: %+v", rssArticles)
	}
	if rss.calls != 1 {
		t.Fatalf("expected rss fetcher called once, got %d", rss.calls)
	}
	if direct.calls != 0 {
		t.Fatalf("expected direct fetcher not called for rss source, got %d", direct.calls)
	}
	if pulse.calls != 0 {
		t.Fatalf("expected pulse fetcher not called for rss source, got %d", pulse.calls)
	}

	directArticles, directNotices, err := router.FetchWithNotices(context.Background(), config.Source{ID: "direct-source", Kind: config.SourceKindDirect})
	if err != nil {
		t.Fatalf("fetch direct source: %v", err)
	}
	if len(directNotices) != 0 {
		t.Fatalf("expected no notices for direct success, got %v", directNotices)
	}
	if len(directArticles) != 1 || directArticles[0].Title != "direct article" {
		t.Fatalf("unexpected direct articles: %+v", directArticles)
	}
	if direct.calls != 1 {
		t.Fatalf("expected direct fetcher called once, got %d", direct.calls)
	}
	if pulse.calls != 0 {
		t.Fatalf("expected pulse fetcher not called for direct source, got %d", pulse.calls)
	}
}

func TestRouterDispatchesPulseKind(t *testing.T) {
	rss := &stubSourceFetcher{}
	direct := &stubSourceFetcher{}
	pulse := &stubSourceFetcher{articles: []core.Article{{Title: "pulse article"}}}
	router := NewRouterFetcher(rss, direct, pulse)

	articles, notices, err := router.FetchWithNotices(context.Background(), config.Source{ID: "zerodha-pulse", Kind: config.SourceKindPulse})
	if err != nil {
		t.Fatalf("fetch pulse source: %v", err)
	}
	if len(notices) != 0 {
		t.Fatalf("expected no notices for pulse success, got %v", notices)
	}
	if len(articles) != 1 || articles[0].Title != "pulse article" {
		t.Fatalf("unexpected pulse articles: %+v", articles)
	}
	if pulse.calls != 1 {
		t.Fatalf("expected pulse fetcher called once, got %d", pulse.calls)
	}
	if rss.calls != 0 || direct.calls != 0 {
		t.Fatalf("expected rss/direct not called for pulse, got rss=%d direct=%d", rss.calls, direct.calls)
	}
}

func TestRouterRejectsUnknownKind(t *testing.T) {
	router := NewRouterFetcher(&stubSourceFetcher{}, &stubSourceFetcher{}, &stubSourceFetcher{})
	_, _, err := router.FetchWithNotices(context.Background(), config.Source{ID: "source-1", Kind: "atom"})
	if err == nil {
		t.Fatalf("expected error for unknown source kind")
	}
	if !strings.Contains(err.Error(), "unsupported source kind") {
		t.Fatalf("expected unsupported kind error, got %v", err)
	}
}

func TestRouterFallsBackToDirectWhenRSSFails(t *testing.T) {
	rss := &stubSourceFetcher{err: errors.New("rss fetch failed")}
	direct := &stubSourceFetcher{articles: []core.Article{{Title: "from direct fallback"}}}
	router := NewRouterFetcher(rss, direct, &stubSourceFetcher{})

	articles, notices, err := router.FetchWithNotices(context.Background(), config.Source{ID: "source-1", Kind: config.SourceKindRSS, CrawlFallback: true})
	if err != nil {
		t.Fatalf("fetch with fallback: %v", err)
	}
	if len(articles) != 1 || articles[0].Title != "from direct fallback" {
		t.Fatalf("unexpected fallback articles: %+v", articles)
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "used direct fallback") {
		t.Fatalf("expected fallback success notice, got %v", notices)
	}
	if rss.calls != 1 || direct.calls != 1 {
		t.Fatalf("expected both fetchers called once, rss=%d direct=%d", rss.calls, direct.calls)
	}
}

func TestRouterReportsFallbackFailure(t *testing.T) {
	rss := &stubSourceFetcher{err: errors.New("rss fetch failed")}
	direct := &stubSourceFetcher{err: errors.New("direct fetch failed")}
	router := NewRouterFetcher(rss, direct, &stubSourceFetcher{})

	_, notices, err := router.FetchWithNotices(context.Background(), config.Source{ID: "source-1", Kind: config.SourceKindRSS, CrawlFallback: true})
	if err == nil {
		t.Fatalf("expected error when rss and direct fallback both fail")
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "direct fallback failed") {
		t.Fatalf("expected fallback failure notice, got %v", notices)
	}
	if !strings.Contains(err.Error(), "rss fetch failed") || !strings.Contains(err.Error(), "direct fallback failed") {
		t.Fatalf("expected combined failure details, got %v", err)
	}
}
