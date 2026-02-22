package enrich

import (
	"context"
	"strings"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/enrich/providers"
	"github.com/soloengine/ai-impact-scrapper/internal/ingest"
)

type Enricher struct {
	mapper    EntityMapper
	router    *ProviderRouter
	relevance ingest.RelevanceGate
}

func NewEnricher(entities []config.Entity, router *ProviderRouter) *Enricher {
	return &Enricher{
		mapper:    NewEntityMapper(entities),
		router:    router,
		relevance: ingest.NewRelevanceGate(),
	}
}

func (e *Enricher) EnrichArticle(ctx context.Context, baseMeta core.RunMetadata, article core.Article, factors []config.Factor, entities []config.Entity, profile config.PipelineProfile) (core.EnrichedEvent, error) {
	text := strings.TrimSpace(strings.Join([]string{article.Title, article.Summary, article.Body}, " "))
	entityMatches := e.mapper.Map(article)
	deterministicFactors := deterministicFactorTags(text, factors)
	sentimentLabel, sentimentScore := deterministicSentiment(text)
	relevanceScore := e.relevance.Score(article, factors, entities)
	needsLLM := e.relevance.NeedsLLMRefinement(relevanceScore, profile.NoveltyThreshold, profile.AmbiguityThreshold)

	meta := baseMeta
	tags := deterministicFactors

	if needsLLM && e.router != nil {
		routed, err := e.router.Enrich(ctx, providers.ClassificationRequest{Text: text})
		if err == nil {
			sentimentLabel = routed.Sentiment.Label
			sentimentScore = routed.Sentiment.Score
			tags = mergeFactorTags(tags, providerTagsToCore(routed.Factors.Tags))
			meta.Provider = routed.Provider
			meta.Model = routed.Model
			meta.InputTokens = routed.InputTokens
			meta.OutputTokens = routed.OutputTokens
			meta.EstimatedCostUS = routed.EstimatedCost
		}
	}

	if meta.Provider == "" {
		meta.Provider = "rules"
	}
	if meta.Model == "" {
		meta.Model = "rules"
	}

	return core.EnrichedEvent{
		Metadata:           meta,
		Article:            article,
		Entities:           entityMatches,
		Factors:            tags,
		SentimentLabel:     sentimentLabel,
		SentimentScore:     sentimentScore,
		RelevanceScore:     relevanceScore,
		NeedsLLMRefinement: needsLLM,
	}, nil
}

func deterministicSentiment(text string) (string, float64) {
	lower := strings.ToLower(text)
	score := 0.0

	for _, token := range []string{"growth", "strong", "beat", "upside", "upgrade", "surge"} {
		if strings.Contains(lower, token) {
			score += 0.15
		}
	}
	for _, token := range []string{"weak", "miss", "downside", "downgrade", "delay", "cut"} {
		if strings.Contains(lower, token) {
			score -= 0.15
		}
	}

	label := "neutral"
	if score > 0.1 {
		label = "positive"
	}
	if score < -0.1 {
		label = "negative"
	}
	return label, score
}

func deterministicFactorTags(text string, factors []config.Factor) []core.FactorTag {
	lower := strings.ToLower(text)
	out := make([]core.FactorTag, 0)

	for _, factor := range factors {
		hits := 0
		for _, keyword := range factor.Keywords {
			if keyword != "" && strings.Contains(lower, strings.ToLower(keyword)) {
				hits++
			}
		}
		if hits == 0 {
			continue
		}
		score := float64(hits) / float64(len(factor.Keywords))
		if score > 1 {
			score = 1
		}
		out = append(out, core.FactorTag{
			FactorID:   factor.ID,
			Name:       factor.Name,
			Category:   factor.Category,
			Score:      score * factor.Weight,
			MatchedBy:  "rule_keyword",
			LLMRefined: false,
		})
	}

	return out
}

func providerTagsToCore(tags []providers.FactorTag) []core.FactorTag {
	out := make([]core.FactorTag, 0, len(tags))
	for _, tag := range tags {
		out = append(out, core.FactorTag{
			FactorID:   tag.FactorID,
			Name:       tag.Name,
			Category:   tag.Category,
			Score:      tag.Score,
			MatchedBy:  tag.MatchedBy,
			LLMRefined: tag.LLMRefined,
		})
	}
	return out
}

func mergeFactorTags(base, add []core.FactorTag) []core.FactorTag {
	merged := make([]core.FactorTag, 0, len(base)+len(add))
	seen := map[string]int{}

	for _, tag := range base {
		seen[tag.FactorID] = len(merged)
		merged = append(merged, tag)
	}

	for _, tag := range add {
		if idx, ok := seen[tag.FactorID]; ok {
			if tag.Score > merged[idx].Score {
				merged[idx] = tag
			}
			continue
		}
		seen[tag.FactorID] = len(merged)
		merged = append(merged, tag)
	}
	return merged
}
