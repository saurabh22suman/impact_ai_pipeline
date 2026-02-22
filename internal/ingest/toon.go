package ingest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

type TOONRecord struct {
	ID         string `json:"id"`
	SourceID   string `json:"source_id"`
	SourceName string `json:"source_name"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	Body       string `json:"body"`
	Language   string `json:"language"`
	Region     string `json:"region"`
	Published  string `json:"published_at"`
}

func ParseTOON(payload []byte) ([]core.Article, error) {
	// Practical compatibility mode:
	// - Accept line-delimited JSON objects (TOON-like streams)
	// - Accept JSON array payload
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil, nil
	}

	if trimmed[0] == '[' {
		var records []TOONRecord
		if err := json.Unmarshal(trimmed, &records); err != nil {
			return nil, fmt.Errorf("parse toon array: %w", err)
		}
		return recordsToArticles(records)
	}

	scanner := bufio.NewScanner(bytes.NewReader(trimmed))
	records := make([]TOONRecord, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec TOONRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("parse toon line: %w", err)
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return recordsToArticles(records)
}

func recordsToArticles(records []TOONRecord) ([]core.Article, error) {
	articles := make([]core.Article, 0, len(records))
	for _, rec := range records {
		ts, err := time.Parse(time.RFC3339, rec.Published)
		if err != nil {
			ts = time.Now().UTC()
		}
		articles = append(articles, core.Article{
			ID:          rec.ID,
			SourceID:    rec.SourceID,
			SourceName:  rec.SourceName,
			URL:         rec.URL,
			Title:       rec.Title,
			Summary:     rec.Summary,
			Body:        rec.Body,
			Language:    rec.Language,
			Region:      rec.Region,
			PublishedAt: ts.UTC(),
			IngestedAt:  time.Now().UTC(),
		})
	}
	return articles, nil
}
