package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	readability "github.com/go-shiori/go-readability"
	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

type BrowserContentExtractor interface {
	Extract(ctx context.Context, pageURL string) (string, error)
}

type PulseFetcher struct {
	client            HTTPClient
	browserFallback   BrowserContentExtractor
	nowFn             func() time.Time
	maxPagesPerSource int
	maxResponseBytes  int64
}

func NewPulseFetcher(client HTTPClient, browserFallback BrowserContentExtractor) *PulseFetcher {
	return &PulseFetcher{
		client:            client,
		browserFallback:   browserFallback,
		nowFn:             func() time.Time { return time.Now().UTC() },
		maxPagesPerSource: 20,
		maxResponseBytes:  2 * 1024 * 1024,
	}
}

func (f *PulseFetcher) Fetch(ctx context.Context, source config.Source) ([]core.Article, error) {
	source.Kind = config.NormalizeSourceKind(source.Kind)
	if source.Kind != config.SourceKindPulse {
		return nil, fmt.Errorf("unsupported source kind %q", source.Kind)
	}

	listingURL, err := url.Parse(strings.TrimSpace(source.URL))
	if err != nil {
		return nil, fmt.Errorf("parse listing url: %w", err)
	}
	if listingURL.Scheme != "http" && listingURL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported listing url scheme %q", listingURL.Scheme)
	}

	listingBody, err := f.fetchLimited(ctx, listingURL.String())
	if err != nil {
		return nil, err
	}
	links, err := f.discoverPublisherLinks(listingBody, listingURL)
	if err != nil {
		return nil, err
	}

	if f.maxPagesPerSource > 0 && len(links) > f.maxPagesPerSource {
		links = links[:f.maxPagesPerSource]
	}

	articles := make([]core.Article, 0, len(links))
	for _, link := range links {
		articleBody, err := f.fetchLimited(ctx, link)
		if err != nil {
			continue
		}
		articleURL, err := url.Parse(link)
		if err != nil {
			continue
		}
		parsed, err := readability.FromReader(bytes.NewReader(articleBody), articleURL)
		if err != nil {
			continue
		}

		title := strings.TrimSpace(parsed.Title)
		body := strings.TrimSpace(parsed.TextContent)
		summary := strings.TrimSpace(parsed.Excerpt)
		if title == "" && body == "" {
			continue
		}
		if summary == "" {
			summary = truncate(body, 280)
		}

		hash := sha256.Sum256([]byte(strings.ToLower(title + "|" + link + "|" + summary)))
		now := f.nowFn()
		articles = append(articles, core.Article{
			ID:            link,
			SourceID:      source.ID,
			SourceName:    source.Name,
			URL:           link,
			Title:         title,
			Summary:       summary,
			Body:          body,
			Language:      source.Language,
			Region:        source.Region,
			PublishedAt:   now,
			IngestedAt:    now,
			CanonicalHash: hex.EncodeToString(hash[:]),
		})
	}

	return articles, nil
}

func (f *PulseFetcher) fetchLimited(ctx context.Context, target string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch %s returned status %d", target, resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, f.maxResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > f.maxResponseBytes {
		return nil, fmt.Errorf("response body exceeded max size for %s", target)
	}
	return body, nil
}

func (f *PulseFetcher) discoverPublisherLinks(listing []byte, listingURL *url.URL) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(listing))
	if err != nil {
		return nil, fmt.Errorf("parse listing html: %w", err)
	}

	seen := map[string]struct{}{}
	out := make([]string, 0)
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok {
			return
		}
		href = strings.TrimSpace(href)
		if href == "" {
			return
		}

		target, err := listingURL.Parse(href)
		if err != nil {
			return
		}
		if target.Scheme != "http" && target.Scheme != "https" {
			return
		}
		if isShareSocialTrackerURL(target) {
			return
		}

		normalized := normalizeURL(target)
		if normalized == "" {
			return
		}
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	})

	return out, nil
}

func normalizeURL(raw *url.URL) string {
	if raw == nil {
		return ""
	}
	normalized := *raw
	normalized.Fragment = ""
	return normalized.String()
}

func isShareSocialTrackerURL(target *url.URL) bool {
	if target == nil {
		return true
	}
	host := strings.ToLower(target.Hostname())
	path := strings.ToLower(target.Path)
	query := strings.ToLower(target.RawQuery)

	socialHosts := []string{
		"twitter.com", "x.com", "facebook.com", "linkedin.com", "instagram.com",
		"t.me", "telegram.me", "reddit.com", "pinterest.com", "whatsapp.com",
	}
	for _, socialHost := range socialHosts {
		if host == socialHost || strings.HasSuffix(host, "."+socialHost) {
			return true
		}
	}

	if strings.Contains(path, "/intent/") || strings.Contains(path, "/sharer") {
		return true
	}

	trackerHostMarkers := []string{"doubleclick", "googlesyndication", "adservice", "analytics", "tracker", "track"}
	for _, marker := range trackerHostMarkers {
		if strings.Contains(host, marker) {
			return true
		}
	}

	trackerQueryMarkers := []string{"utm_", "fbclid", "gclid", "mc_cid", "mc_eid"}
	for _, marker := range trackerQueryMarkers {
		if strings.Contains(query, marker) {
			return true
		}
	}

	return false
}
