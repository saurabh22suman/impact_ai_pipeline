package enrich

import (
	"strings"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
)

type EntityMapper struct {
	entities []config.Entity
}

func NewEntityMapper(entities []config.Entity) EntityMapper {
	return EntityMapper{entities: entities}
}

func (m EntityMapper) Map(article Article) []EntityMatch {
	text := strings.ToLower(strings.Join([]string{article.Title, article.Summary, article.Body}, " "))
	matches := make([]EntityMatch, 0)
	seen := map[string]struct{}{}

	for _, entity := range m.entities {
		if !entity.Enabled {
			continue
		}
		confidence := 0.0
		if strings.Contains(text, strings.ToLower(entity.Name)) {
			confidence = 0.95
		}
		if strings.Contains(text, strings.ToLower(entity.Symbol)) && confidence < 0.90 {
			confidence = 0.9
		}
		for _, alias := range entity.Aliases {
			if alias != "" && strings.Contains(text, strings.ToLower(alias)) && confidence < 0.85 {
				confidence = 0.85
			}
		}
		if confidence == 0 {
			continue
		}
		if _, ok := seen[entity.ID]; ok {
			continue
		}
		seen[entity.ID] = struct{}{}
		matches = append(matches, EntityMatch{
			EntityID:   entity.ID,
			Symbol:     entity.Symbol,
			Name:       entity.Name,
			Confidence: confidence,
			Method:     "keyword_match",
		})
	}

	return matches
}
