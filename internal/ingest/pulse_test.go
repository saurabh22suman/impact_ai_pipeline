package ingest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
)

type stubBrowserExtractor struct {
	title string
	summary string
	body string
	err error
	calls []string
}

func (s *stubBrowserExtractor) Extract(_ context.Context, pageURL string) (string, string, string, error) {
	s.calls = append(s.calls, pageURL)
	if s.err != nil {
		return "", "", "", s.err
	}
	return s.title, s.summary, s.body, nil
}

type stubHTTPResponse struct {
	status int
	body   string
}

type stubHTTPClient struct {
	responses map[string]stubHTTPResponse
	calls     []string
}

func (s *stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
	target := req.URL.String()
	s.calls = append(s.calls, target)

	resp, ok := s.responses[target]
	if !ok {
		return nil, fmt.Errorf("unexpected request: %s", target)
	}

	return &http.Response{
		StatusCode: resp.status,
		Body:       io.NopCloser(strings.NewReader(resp.body)),
		Header:     make(http.Header),
	}, nil
}

func TestPulseFetcherFollowsPublisherLinks(t *testing.T) {
	now := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	listingURL := "https://pulse.example.com/listing"
	publisherURL := "https://publisher.example.com/story-1"

	client := &stubHTTPClient{responses: map[string]stubHTTPResponse{
		listingURL: {
			status: http.StatusOK,
			body: `<html><body>
				<a href="https://twitter.com/intent/tweet?url=https://publisher.example.com/story-1">Tweet</a>
				<a href="` + publisherURL + `">Publisher Story</a>
			</body></html>`,
		},
		publisherURL: {
			status: http.StatusOK,
			body: `<html>
				<head><title>Pulse Publisher Story</title></head>
				<body><article><p>Publisher article body content.</p></article></body>
			</html>`,
		},
	}}

	fetcher := NewPulseFetcher(client, nil)
	fetcher.nowFn = func() time.Time { return now }

	articles, err := fetcher.Fetch(context.Background(), config.Source{
		ID:       "pulse-1",
		Name:     "Pulse Feed",
		Kind:     config.SourceKindPulse,
		URL:      listingURL,
		Region:   "india",
		Language: "en",
	})
	if err != nil {
		t.Fatalf("fetch pulse: %v", err)
	}

	if len(articles) != 1 {
		t.Fatalf("expected 1 article, got %d", len(articles))
	}
	article := articles[0]
	if article.URL != publisherURL {
		t.Fatalf("unexpected article URL: %s", article.URL)
	}
	if article.SourceID != "pulse-1" || article.SourceName != "Pulse Feed" {
		t.Fatalf("unexpected source metadata: %+v", article)
	}
	if article.Language != "en" || article.Region != "india" {
		t.Fatalf("unexpected locale metadata: %+v", article)
	}
	if article.Title == "" || article.Body == "" || article.CanonicalHash == "" {
		t.Fatalf("expected extracted title/body/hash, got %+v", article)
	}
	if article.IngestedAt != now {
		t.Fatalf("expected ingested_at=%s, got %s", now, article.IngestedAt)
	}
}

func TestPulseFetcherSkipsShareLinks(t *testing.T) {
	listingURL := "https://pulse.example.com/listing"
	publisherURL := "https://publisher.example.com/story-2"
	twitterShareURL := "https://twitter.com/intent/tweet?url=https://publisher.example.com/story-2"
	facebookShareURL := "https://www.facebook.com/sharer/sharer.php?u=https://publisher.example.com/story-2"

	client := &stubHTTPClient{responses: map[string]stubHTTPResponse{
		listingURL: {
			status: http.StatusOK,
			body: `<html><body>
				<a href="` + twitterShareURL + `">Tweet</a>
				<a href="` + facebookShareURL + `">Share</a>
				<a href="` + publisherURL + `">Publisher Story</a>
			</body></html>`,
		},
		publisherURL: {
			status: http.StatusOK,
			body: `<html>
				<head><title>Publisher Story Two</title></head>
				<body><article><p>Publisher article body content two.</p></article></body>
			</html>`,
		},
	}}

	fetcher := NewPulseFetcher(client, nil)
	articles, err := fetcher.Fetch(context.Background(), config.Source{
		ID:       "pulse-2",
		Name:     "Pulse Feed",
		Kind:     config.SourceKindPulse,
		URL:      listingURL,
		Region:   "india",
		Language: "en",
	})
	if err != nil {
		t.Fatalf("fetch pulse: %v", err)
	}

	if len(articles) != 1 {
		t.Fatalf("expected 1 article, got %d", len(articles))
	}

	if containsString(client.calls, twitterShareURL) {
		t.Fatalf("expected twitter share URL to be skipped, calls=%v", client.calls)
	}
	if containsString(client.calls, facebookShareURL) {
		t.Fatalf("expected facebook share URL to be skipped, calls=%v", client.calls)
	}
}

func TestPulseFetcherPreservesPublisherLinksWithTrackingParams(t *testing.T) {
	listingURL := "https://pulse.example.com/listing"
	publisherURLWithTracking := "https://publisher.example.com/story-3?utm_source=pulse&utm_medium=social&fbclid=abc123"
	normalizedPublisherURL := "https://publisher.example.com/story-3"

	client := &stubHTTPClient{responses: map[string]stubHTTPResponse{
		listingURL: {
			status: http.StatusOK,
			body: `<html><body>
				<a href="` + publisherURLWithTracking + `">Publisher Story</a>
			</body></html>`,
		},
		normalizedPublisherURL: {
			status: http.StatusOK,
			body: `<html>
				<head><title>Publisher Story Three</title></head>
				<body><article><p>Publisher article body content three.</p></article></body>
			</html>`,
		},
	}}

	fetcher := NewPulseFetcher(client, nil)
	articles, err := fetcher.Fetch(context.Background(), config.Source{
		ID:       "pulse-3",
		Name:     "Pulse Feed",
		Kind:     config.SourceKindPulse,
		URL:      listingURL,
		Region:   "india",
		Language: "en",
	})
	if err != nil {
		t.Fatalf("fetch pulse: %v", err)
	}

	if len(articles) != 1 {
		t.Fatalf("expected 1 article, got %d", len(articles))
	}
	if articles[0].URL != normalizedPublisherURL {
		t.Fatalf("expected normalized publisher URL %s, got %s", normalizedPublisherURL, articles[0].URL)
	}
	if !containsString(client.calls, normalizedPublisherURL) {
		t.Fatalf("expected fetch call to normalized publisher URL, calls=%v", client.calls)
	}
	if containsString(client.calls, publisherURLWithTracking) {
		t.Fatalf("expected tracking-parameter URL to be normalized before fetch, calls=%v", client.calls)
	}
}

func TestPulseFetcherUsesBrowserFallbackForThinContent(t *testing.T) {
	cases := []struct {
		name               string
		publisherHTML      string
		expectedTitle      string
		expectedSummary    string
		expectedBody       string
		expectedCallCount  int
		expectedArticleCnt int
	}{
		{
			name: "thin readability body",
			publisherHTML: `<html>
				<head><title>Thin Article</title></head>
				<body><article><p>short body</p></article></body>
			</html>`,
			expectedTitle:      "Browser Extracted Title",
			expectedSummary:    "Browser extracted summary",
			expectedBody:       "Browser extracted body with sufficiently rich content to replace thin readability text.",
			expectedCallCount:  1,
			expectedArticleCnt: 1,
		},
		{
			name:               "empty readability content",
			publisherHTML:      `<html><body></body></html>`,
			expectedTitle:      "Browser Extracted Title",
			expectedSummary:    "Browser extracted summary",
			expectedBody:       "Browser extracted body with sufficiently rich content to replace thin readability text.",
			expectedCallCount:  1,
			expectedArticleCnt: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			listingURL := "https://pulse.example.com/listing"
			publisherURL := "https://publisher.example.com/story-thin"

			client := &stubHTTPClient{responses: map[string]stubHTTPResponse{
				listingURL: {
					status: http.StatusOK,
					body: `<html><body><a href="` + publisherURL + `">Publisher Story</a></body></html>`,
				},
				publisherURL: {
					status: http.StatusOK,
					body:   tc.publisherHTML,
				},
			}}

			fallback := &stubBrowserExtractor{
				title:   "Browser Extracted Title",
				summary: "Browser extracted summary",
				body:    "Browser extracted body with sufficiently rich content to replace thin readability text.",
			}

			fetcher := NewPulseFetcher(client, fallback)
			articles, err := fetcher.Fetch(context.Background(), config.Source{
				ID:       "pulse-fallback",
				Name:     "Pulse Feed",
				Kind:     config.SourceKindPulse,
				URL:      listingURL,
				Region:   "india",
				Language: "en",
			})
			if err != nil {
				t.Fatalf("fetch pulse: %v", err)
			}

			if len(articles) != tc.expectedArticleCnt {
				t.Fatalf("expected %d articles, got %d", tc.expectedArticleCnt, len(articles))
			}
			if len(fallback.calls) != tc.expectedCallCount || (tc.expectedCallCount > 0 && fallback.calls[0] != publisherURL) {
				t.Fatalf("expected browser fallback called %d times for %s, calls=%v", tc.expectedCallCount, publisherURL, fallback.calls)
			}
			if tc.expectedArticleCnt == 0 {
				return
			}
			if articles[0].Title != tc.expectedTitle {
				t.Fatalf("expected fallback title %q, got %q", tc.expectedTitle, articles[0].Title)
			}
			if articles[0].Summary != tc.expectedSummary {
				t.Fatalf("expected fallback summary %q, got %q", tc.expectedSummary, articles[0].Summary)
			}
			if articles[0].Body != tc.expectedBody {
				t.Fatalf("expected fallback body %q, got %q", tc.expectedBody, articles[0].Body)
			}
		})
	}
}

func TestPulseFetcherRejectsUnsupportedKind(t *testing.T) {
	fetcher := NewPulseFetcher(&http.Client{Timeout: 200 * time.Millisecond}, nil)
	_, err := fetcher.Fetch(context.Background(), config.Source{Kind: config.SourceKindRSS})
	if err == nil {
		t.Fatalf("expected unsupported kind error")
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
