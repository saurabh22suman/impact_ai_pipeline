package ingest

import (
	"regexp"
	"strings"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/entitymatch"
)

type RelevanceGate struct{}

type RelevanceBreakdown struct {
	RelevanceScore float64
	FactorScore    float64
	EntityScore    float64
}

func NewRelevanceGate() RelevanceGate {
	return RelevanceGate{}
}

func (r RelevanceGate) Score(article core.Article, factors []config.Factor, entities []config.Entity) float64 {
	return r.ScoreBreakdown(article, factors, entities).RelevanceScore
}

func (r RelevanceGate) ScoreBreakdown(article core.Article, factors []config.Factor, entities []config.Entity) RelevanceBreakdown {
	text := strings.ToLower(strings.Join([]string{article.Title, article.Summary, article.Body}, " "))
	if text == "" {
		return RelevanceBreakdown{}
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
		if matched, _, _ := entitymatch.MatchEntity(text, entity); matched {
			entityHits++
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

	return RelevanceBreakdown{
		RelevanceScore: 0.65*factorScore + 0.35*entityScore,
		FactorScore:    factorScore,
		EntityScore:    entityScore,
	}
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
