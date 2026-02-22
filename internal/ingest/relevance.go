package ingest

import (
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
			if keyword != "" && strings.Contains(text, strings.ToLower(keyword)) {
				factorHits++
			}
		}
	}

	entityHits := 0
	for _, entity := range entities {
		if strings.Contains(text, strings.ToLower(entity.Name)) {
			entityHits++
			continue
		}
		for _, alias := range entity.Aliases {
			if alias != "" && strings.Contains(text, strings.ToLower(alias)) {
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

func (r RelevanceGate) NeedsLLMRefinement(relevanceScore, noveltyThreshold, ambiguityThreshold float64) bool {
	return relevanceScore >= ambiguityThreshold && relevanceScore < noveltyThreshold
}
