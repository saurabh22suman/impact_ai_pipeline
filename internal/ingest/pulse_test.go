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
