package ingest

import (
	"strings"

	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

type DedupeEngine struct{}

func NewDedupeEngine() DedupeEngine {
	return DedupeEngine{}
}

func (d DedupeEngine) Filter(articles []core.Article) []core.Article {
	seen := map[string]struct{}{}
	result := make([]core.Article, 0, len(articles))

	for _, article := range articles {
		key := strings.TrimSpace(article.CanonicalHash)
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(article.URL + "|" + article.Title))
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, article)
	}
	return result
}
