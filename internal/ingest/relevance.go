package ingest

import (
	"regexp"
	"strings"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

type RelevanceGate struct{}

func NewRelevanceGate() RelevanceGate {
	return RelevanceGate{}
}

func (r RelevanceGate) Score(article core.Article, factors []config.Factor, entities []config.Entity) float64 {
	text := strings.ToLower(strings.Join([]string{article.Title, article.Summary, article.Body}, " "))
	if text == "" {
		return 0
	}

	factorHits := 0
	factorTotal := 0
	for _, factor := range factors {
		for _, keyword := range factor.Keywords {
			factorTotal++
			if keyword != "" && containsTerm(text, keyword) {
				factorHits++
			}
		}
	}

	entityHits := 0
	for _, entity := range entities {
		if containsTerm(text, entity.Name) {
			entityHits++
			continue
		}
		for _, alias := range entity.Aliases {
			if alias != "" && containsTerm(text, alias) {
				entityHits++
				break
			}
		}
	}

	factorScore := 0.0
	if factorTotal > 0 {
		factorScore = float64(factorHits) / float64(factorTotal)
	}
	entityScore := 0.0
	if len(entities) > 0 {
		entityScore = float64(entityHits) / float64(len(entities))
	}

	return 0.65*factorScore + 0.35*entityScore
}

func containsTerm(text, term string) bool {
	normalizedTerm := strings.ToLower(strings.TrimSpace(term))
	if normalizedTerm == "" {
		return false
	}

	pattern := `(^|[^a-z0-9])` + regexp.QuoteMeta(normalizedTerm) + `([^a-z0-9]|$)`
	return regexp.MustCompile(pattern).MatchString(text)
}

func (r RelevanceGate) NeedsLLMRefinement(relevanceScore, noveltyThreshold, ambiguityThreshold float64) bool {
	return relevanceScore >= ambiguityThreshold && relevanceScore < noveltyThreshold
}
