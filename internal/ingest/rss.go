package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type RSSFetcher struct {
	client HTTPClient
	nowFn  func() time.Time
}

func NewRSSFetcher(client HTTPClient) *RSSFetcher {
	return &RSSFetcher{
		client: client,
		nowFn:  func() time.Time { return time.Now().UTC() },
	}
}

func (f *RSSFetcher) Fetch(ctx context.Context, source config.Source) ([]core.Article, error) {
	source.Kind = config.NormalizeSourceKind(source.Kind)
	if source.Kind != config.SourceKindRSS {
		return nil, fmt.Errorf("unsupported source kind %q", source.Kind)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch rss %s returned status %d", source.URL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	parsed, err := parseRSS(body)
	if err != nil {
		return nil, err
	}

	articles := make([]core.Article, 0, len(parsed.Channel.Items))
	for _, item := range parsed.Channel.Items {
		publishedAt := parsePubDate(item.PubDate, f.nowFn)
		title := strings.TrimSpace(item.Title)
		url := strings.TrimSpace(item.Link)
		summary := strings.TrimSpace(item.Description)
		hash := sha256.Sum256([]byte(strings.ToLower(title + "|" + url + "|" + summary)))
		articles = append(articles, core.Article{
			ID:            item.GUID,
			SourceID:      source.ID,
			SourceName:    source.Name,
			URL:           url,
			Title:         title,
			Summary:       summary,
			Body:          "",
			Language:      source.Language,
			Region:        source.Region,
			PublishedAt:   publishedAt,
			IngestedAt:    f.nowFn(),
			CanonicalHash: hex.EncodeToString(hash[:]),
		})
	}

	return articles, nil
}

type rssDocument struct {
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			PubDate     string `xml:"pubDate"`
			GUID        string `xml:"guid"`
		} `xml:"item"`
	} `xml:"channel"`
}

func parseRSS(payload []byte) (rssDocument, error) {
	var doc rssDocument
	if err := xml.Unmarshal(payload, &doc); err != nil {
		return rssDocument{}, fmt.Errorf("parse rss: %w", err)
	}
	return doc, nil
}

func parsePubDate(raw string, nowFn func() time.Time) time.Time {
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC3339,
		time.RFC822,
	}
	cleaned := strings.TrimSpace(raw)
	for _, layout := range formats {
		ts, err := time.Parse(layout, cleaned)
		if err == nil {
			return ts.UTC()
		}
	}
	return nowFn().UTC()
}
