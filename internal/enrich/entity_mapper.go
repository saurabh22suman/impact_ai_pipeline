package enrich

import (
	"strings"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/entitymatch"
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
		matched, confidence, method := entitymatch.MatchEntity(text, entity)
		if !matched {
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
			Method:     method,
		})
	}

	return matches
}
