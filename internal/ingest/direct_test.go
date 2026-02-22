package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
)

func TestDirectFetcherDiscoversAndExtractsArticles(t *testing.T) {
	now := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)

	mux := http.NewServeMux()
	mux.HandleFunc("/listing", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
			<html><body>
				<a href="/article-1">First</a>
				<a href="https://external.example.com/article-2">External</a>
			</body></html>
		`))
	})
	mux.HandleFunc("/article-1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
			<html>
			<head><title>AI demand rises</title></head>
			<body>
				<article>
					<p>Chip demand is rising across enterprise workloads.</p>
				</article>
			</body>
			</html>
		`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	fetcher := NewDirectFetcher(server.Client())
	fetcher.nowFn = func() time.Time { return now }

	articles, err := fetcher.Fetch(context.Background(), config.Source{
		ID:       "source-1",
		Name:     "Source One",
		Kind:     config.SourceKindDirect,
		URL:      server.URL + "/listing",
		Region:   "global",
		Language: "en",
	})
	if err != nil {
		t.Fatalf("fetch direct: %v", err)
	}

	if len(articles) != 1 {
		t.Fatalf("expected one same-host article, got %d", len(articles))
	}
	article := articles[0]
	if article.URL != server.URL+"/article-1" {
		t.Fatalf("unexpected article URL: %s", article.URL)
	}
	if article.Title == "" {
		t.Fatalf("expected extracted title")
	}
	if article.Body == "" {
		t.Fatalf("expected extracted body")
	}
	if article.SourceID != "source-1" || article.SourceName != "Source One" {
		t.Fatalf("unexpected source metadata: %+v", article)
	}
	if article.Language != "en" || article.Region != "global" {
		t.Fatalf("unexpected locale metadata: %+v", article)
	}
	if article.CanonicalHash == "" {
		t.Fatalf("expected canonical hash")
	}
	if article.IngestedAt != now {
		t.Fatalf("expected ingested_at=%s, got %s", now, article.IngestedAt)
	}
}

func TestDirectFetcherRespectsPageCap(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/listing", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
			<html><body>
				<a href="/article-1">One</a>
				<a href="/article-2">Two</a>
			</body></html>
		`))
	})
	mux.HandleFunc("/article-1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>One</title></head><body><article><p>Body one</p></article></body></html>`))
	})
	mux.HandleFunc("/article-2", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><head><title>Two</title></head><body><article><p>Body two</p></article></body></html>`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	fetcher := NewDirectFetcher(server.Client())
	fetcher.maxPagesPerSource = 1

	articles, err := fetcher.Fetch(context.Background(), config.Source{
		ID:       "source-1",
		Name:     "Source One",
		Kind:     config.SourceKindDirect,
		URL:      server.URL + "/listing",
		Region:   "global",
		Language: "en",
	})
	if err != nil {
		t.Fatalf("fetch direct with cap: %v", err)
	}
	if len(articles) != 1 {
		t.Fatalf("expected one article due to cap, got %d", len(articles))
	}
}

func TestDirectFetcherRejectsUnsupportedKind(t *testing.T) {
	fetcher := NewDirectFetcher(&http.Client{Timeout: 200 * time.Millisecond})
	_, err := fetcher.Fetch(context.Background(), config.Source{Kind: config.SourceKindRSS})
	if err == nil {
		t.Fatalf("expected unsupported kind error")
	}
}

func TestDirectFetcherFailsOnListingTooLarge(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/listing", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("0123456789ABCDEFGHIJ"))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	fetcher := NewDirectFetcher(server.Client())
	fetcher.maxResponseBytes = 8

	_, err := fetcher.Fetch(context.Background(), config.Source{
		ID:       "source-1",
		Name:     "Source One",
		Kind:     config.SourceKindDirect,
		URL:      server.URL + "/listing",
		Region:   "global",
		Language: "en",
	})
	if err == nil {
		t.Fatalf("expected response size error")
	}
}

func TestDirectFetcherSkipsNonHTTPLinks(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/listing", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
			<html><body>
				<a href="mailto:news@example.com">Mail</a>
				<a href="javascript:void(0)">JS</a>
			</body></html>
		`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	fetcher := NewDirectFetcher(server.Client())
	articles, err := fetcher.Fetch(context.Background(), config.Source{
		ID:       "source-1",
		Name:     "Source One",
		Kind:     config.SourceKindDirect,
		URL:      server.URL + "/listing",
		Region:   "global",
		Language: "en",
	})
	if err != nil {
		t.Fatalf("fetch direct non-http links: %v", err)
	}
	if len(articles) != 0 {
		t.Fatalf("expected no extracted articles, got %d", len(articles))
	}
}
