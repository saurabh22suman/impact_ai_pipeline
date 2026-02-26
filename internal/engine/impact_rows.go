package engine

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

const niftyITImpactGroupID = "nifty-it-impact"

type impactModeConfig struct {
	Enabled              bool
	ParentSymbols        map[string]struct{}
	ChildSymbols         map[string]struct{}
	ChildSymbolsByParent map[string]map[string]struct{}
	rolesBySymbol        map[string]string
}

func newImpactModeConfig(groups []config.EntityGroup, requested []config.Entity, all []config.Entity) impactModeConfig {
	groupParents := map[string]struct{}{}
	groupChildren := map[string]struct{}{}
	childSymbolsByParent := map[string]map[string]struct{}{}
	for _, group := range groups {
		if !strings.EqualFold(strings.TrimSpace(group.ID), niftyITImpactGroupID) {
			continue
		}
		parent := strings.ToUpper(strings.TrimSpace(group.ParentSymbol))
		if parent == "" {
			continue
		}
		groupParents[parent] = struct{}{}
		if _, ok := childSymbolsByParent[parent]; !ok {
			childSymbolsByParent[parent] = map[string]struct{}{}
		}
		for _, childRaw := range group.ChildSymbols {
			child := strings.ToUpper(strings.TrimSpace(childRaw))
			if child == "" {
				continue
			}
			groupChildren[child] = struct{}{}
			childSymbolsByParent[parent][child] = struct{}{}
		}
	}

	enabled := false
	requestedParents := map[string]struct{}{}
	for _, entity := range requested {
		symbol := strings.ToUpper(strings.TrimSpace(entity.Symbol))
		if symbol == "" {
			continue
		}
		if _, ok := groupParents[symbol]; ok {
			enabled = true
			requestedParents[symbol] = struct{}{}
		}
	}
	if !enabled {
		return impactModeConfig{}
	}

	roles := map[string]string{}
	for _, entity := range all {
		symbol := strings.ToUpper(strings.TrimSpace(entity.Symbol))
		if symbol == "" {
			continue
		}
		roles[symbol] = strings.ToLower(strings.TrimSpace(entity.Role))
	}

	requestedChildUnion := map[string]struct{}{}
	requestedChildByParent := map[string]map[string]struct{}{}
	for parent := range requestedParents {
		childrenForParent := childSymbolsByParent[parent]
		if len(childrenForParent) == 0 {
			continue
		}
		requestedChildByParent[parent] = map[string]struct{}{}
		for child := range childrenForParent {
			requestedChildUnion[child] = struct{}{}
			requestedChildByParent[parent][child] = struct{}{}
		}
	}

	return impactModeConfig{
		Enabled:              true,
		ParentSymbols:        requestedParents,
		ChildSymbols:         requestedChildUnion,
		ChildSymbolsByParent: requestedChildByParent,
		rolesBySymbol:        roles,
	}
}

func (s *Service) buildFeatureRows(ctx context.Context, event core.MarketAlignedEvent, impact impactModeConfig) []core.FeatureRow {
	if impact.Enabled {
		rows := s.buildImpactFeatureRows(ctx, event, impact)
		if rows == nil {
			return []core.FeatureRow{}
		}
		return rows
	}

	factorIDs := make([]string, 0, len(event.Event.Factors))
	for _, factor := range event.Event.Factors {
		factorIDs = append(factorIDs, factor.FactorID)
	}

	if len(event.Event.Entities) == 0 {
		return []core.FeatureRow{{
			RunID:            event.Event.Metadata.RunID,
			ConfigVersion:    event.Event.Metadata.ConfigVersion,
			PipelineProfile:  event.Event.Metadata.PipelineProfile,
			Provider:         event.Event.Metadata.Provider,
			Model:            event.Event.Metadata.Model,
			PromptVersion:    event.Event.Metadata.PromptVersion,
			ArticleID:        event.Event.Article.ID,
			Symbol:           "UNKNOWN",
			SessionDate:      event.Session.SessionDate,
			SessionLabel:     event.Session.SessionLabel,
			SentimentScore:   event.Event.SentimentScore,
			RelevanceScore:   event.Event.RelevanceScore,
			FactorVector:     factorIDs,
			InputTokens:      event.Event.Metadata.InputTokens,
			OutputTokens:     event.Event.Metadata.OutputTokens,
			EstimatedCostUS:  event.Event.Metadata.EstimatedCostUS,
			NewsSource:       event.Event.Article.SourceName,
			URL:              event.Event.Article.URL,
			ParentEntity:     "N/A",
			ChildEntity:      "N/A",
			SentimentDisplay: formatBaseSentimentDisplay(event.Event.SentimentLabel),
			Weight:           1.0,
			ConfidenceScore:  0,
			Summary:          featureSummary(event.Event.Article),
		}}
	}

	rows := make([]core.FeatureRow, 0, len(event.Event.Entities))
	for _, entity := range event.Event.Entities {
		rows = append(rows, core.FeatureRow{
			RunID:            event.Event.Metadata.RunID,
			ConfigVersion:    event.Event.Metadata.ConfigVersion,
			PipelineProfile:  event.Event.Metadata.PipelineProfile,
			Provider:         event.Event.Metadata.Provider,
			Model:            event.Event.Metadata.Model,
			PromptVersion:    event.Event.Metadata.PromptVersion,
			ArticleID:        event.Event.Article.ID,
			Symbol:           entity.Symbol,
			SessionDate:      event.Session.SessionDate,
			SessionLabel:     event.Session.SessionLabel,
			SentimentScore:   event.Event.SentimentScore,
			RelevanceScore:   event.Event.RelevanceScore,
			FactorVector:     factorIDs,
			InputTokens:      event.Event.Metadata.InputTokens,
			OutputTokens:     event.Event.Metadata.OutputTokens,
			EstimatedCostUS:  event.Event.Metadata.EstimatedCostUS,
			NewsSource:       event.Event.Article.SourceName,
			URL:              event.Event.Article.URL,
			ParentEntity:     entity.Symbol,
			ChildEntity:      "N/A",
			SentimentDisplay: formatBaseSentimentDisplay(event.Event.SentimentLabel),
			Weight:           1.0,
			ConfidenceScore:  entity.Confidence,
			Summary:          featureSummary(event.Event.Article),
		})
	}
	return rows
}

type impactPairSpec struct {
	parent core.EntityMatch
	child  *core.EntityMatch
}

func (s *Service) buildImpactFeatureRows(ctx context.Context, event core.MarketAlignedEvent, impact impactModeConfig) []core.FeatureRow {
	parents := make([]core.EntityMatch, 0)
	childrenBySymbol := map[string]core.EntityMatch{}
	for _, match := range event.Event.Entities {
		symbol := strings.ToUpper(strings.TrimSpace(match.Symbol))
		if symbol == "" {
			continue
		}
		if _, ok := impact.ParentSymbols[symbol]; ok {
			parents = append(parents, match)
			continue
		}
		if _, ok := impact.ChildSymbols[symbol]; ok {
			childrenBySymbol[symbol] = match
			continue
		}
		role := impact.rolesBySymbol[symbol]
		if role == config.EntityRoleParent {
			if _, ok := impact.ParentSymbols[symbol]; ok {
				parents = append(parents, match)
			}
		}
		if role == config.EntityRoleChild {
			if _, ok := impact.ChildSymbols[symbol]; ok {
				childrenBySymbol[symbol] = match
			}
		}
	}

	if len(parents) == 0 {
		return nil
	}

	sort.Slice(parents, func(i, j int) bool {
		return strings.ToUpper(parents[i].Symbol) < strings.ToUpper(parents[j].Symbol)
	})

	factorIDs := make([]string, 0, len(event.Event.Factors))
	for _, factor := range event.Event.Factors {
		factorIDs = append(factorIDs, factor.FactorID)
	}

	pairSpecs := make([]impactPairSpec, 0, len(parents))
	for _, parent := range parents {
		parentSymbol := strings.ToUpper(strings.TrimSpace(parent.Symbol))
		allowedChildren := impact.ChildSymbolsByParent[parentSymbol]
		if len(allowedChildren) == 0 {
			pairSpecs = append(pairSpecs, impactPairSpec{parent: parent})
			continue
		}

		matchedChildren := make([]core.EntityMatch, 0, len(allowedChildren))
		for childSymbol := range allowedChildren {
			child, ok := childrenBySymbol[childSymbol]
			if !ok {
				continue
			}
			matchedChildren = append(matchedChildren, child)
		}
		if len(matchedChildren) == 0 {
			pairSpecs = append(pairSpecs, impactPairSpec{parent: parent})
			continue
		}

		sort.Slice(matchedChildren, func(i, j int) bool {
			return strings.ToUpper(matchedChildren[i].Symbol) < strings.ToUpper(matchedChildren[j].Symbol)
		})
		for _, child := range matchedChildren {
			childCopy := child
			pairSpecs = append(pairSpecs, impactPairSpec{parent: parent, child: &childCopy})
		}
	}

	if len(pairSpecs) == 0 {
		return nil
	}

	type evaluatedPair struct {
		pair      impactPairSpec
		sentiment pairSentimentResult
		rawWeight float64
	}

	evaluatedPairs := make([]evaluatedPair, 0, len(pairSpecs))
	totalRawWeight := 0.0
	for _, pair := range pairSpecs {
		pairSentiment := s.classifyPairSentiment(ctx, event, pair)
		rawWeight := pairRawWeight(pair, pairSentiment.Score)
		totalRawWeight += rawWeight
		evaluatedPairs = append(evaluatedPairs, evaluatedPair{
			pair:      pair,
			sentiment: pairSentiment,
			rawWeight: rawWeight,
		})
	}
	if totalRawWeight <= 0 {
		totalRawWeight = float64(len(evaluatedPairs))
		for i := range evaluatedPairs {
			evaluatedPairs[i].rawWeight = 1.0
		}
	}

	rows := make([]core.FeatureRow, 0, len(pairSpecs))
	for idx, evaluated := range evaluatedPairs {
		pair := evaluated.pair
		pairSentiment := evaluated.sentiment
		baseInput := allocateInt(event.Event.Metadata.InputTokens, len(pairSpecs), idx)
		baseOutput := allocateInt(event.Event.Metadata.OutputTokens, len(pairSpecs), idx)
		baseCost := 0.0
		if len(pairSpecs) > 0 {
			baseCost = event.Event.Metadata.EstimatedCostUS / float64(len(pairSpecs))
		}

		childEntity := "N/A"
		entityConfidence := pair.parent.Confidence
		if pair.child != nil {
			childEntity = pair.child.Symbol
			entityConfidence = (pair.parent.Confidence + pair.child.Confidence) / 2.0
		}

		confidence := pairConfidence(entityConfidence, pairSentiment.Score)
		provider := "mixed"
		model := "mixed"

		pairProvider := strings.TrimSpace(pairSentiment.Provider)
		pairModel := strings.TrimSpace(pairSentiment.Model)
		baseProvider := strings.TrimSpace(event.Event.Metadata.Provider)
		baseModel := strings.TrimSpace(event.Event.Metadata.Model)

		if pairProvider == "" {
			pairProvider = baseProvider
		}
		if pairModel == "" {
			pairModel = baseModel
		}

		if pairProvider != "" && strings.EqualFold(pairProvider, baseProvider) {
			provider = pairProvider
		}
		if pairModel != "" && strings.EqualFold(pairModel, baseModel) {
			model = pairModel
		}

		rows = append(rows, core.FeatureRow{
			RunID:            event.Event.Metadata.RunID,
			ConfigVersion:    event.Event.Metadata.ConfigVersion,
			PipelineProfile:  event.Event.Metadata.PipelineProfile,
			Provider:         provider,
			Model:            model,
			PromptVersion:    event.Event.Metadata.PromptVersion,
			ArticleID:        event.Event.Article.ID,
			Symbol:           pair.parent.Symbol,
			SessionDate:      event.Session.SessionDate,
			SessionLabel:     event.Session.SessionLabel,
			SentimentScore:   pairSentiment.Score,
			RelevanceScore:   event.Event.RelevanceScore,
			FactorVector:     factorIDs,
			InputTokens:      baseInput + pairSentiment.InputTokens,
			OutputTokens:     baseOutput + pairSentiment.OutputTokens,
			EstimatedCostUS:  baseCost + pairSentiment.EstimatedCostUS,
			NewsSource:       event.Event.Article.SourceName,
			URL:              event.Event.Article.URL,
			ParentEntity:     pair.parent.Symbol,
			ChildEntity:      childEntity,
			SentimentDisplay: formatPairSentimentDisplay(pairSentiment.Label, pairSentiment.Score),
			Weight:           evaluated.rawWeight / totalRawWeight,
			ConfidenceScore:  confidence,
			Summary:          featureSummary(event.Event.Article),
		})
	}

	return rows
}

func (s *Service) classifyPairSentiment(ctx context.Context, event core.MarketAlignedEvent, pair impactPairSpec) pairSentimentResult {
	if s != nil && s.enricher != nil {
		result := s.enricher.ClassifyPairSentiment(ctx, event.Event.Article, pair.parent, pair.child)
		return pairSentimentResult(result)
	}

	label, score := fallbackPairSentiment(event.Event.Article, pair.parent, pair.child)
	return pairSentimentResult{
		Provider: "rules",
		Model:    "rules",
		Label:    label,
		Score:    score,
	}
}

type pairSentimentResult struct {
	Provider        string
	Model           string
	Label           string
	Score           float64
	InputTokens     int
	OutputTokens    int
	EstimatedCostUS float64
}

func fallbackPairSentiment(article core.Article, parent core.EntityMatch, child *core.EntityMatch) (string, float64) {
	childSymbol := "N/A"
	if child != nil {
		childSymbol = child.Symbol
	}
	text := strings.Join([]string{article.Title, article.Summary, article.Body, parent.Symbol, childSymbol}, " ")
	return deterministicSentimentFallback(text)
}

func deterministicSentimentFallback(text string) (string, float64) {
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

func pairRawWeight(pair impactPairSpec, sentimentScore float64) float64 {
	parentConfidence := pair.parent.Confidence
	if parentConfidence < 0 {
		parentConfidence = 0
	}
	childConfidence := 0.0
	if pair.child != nil {
		childConfidence = pair.child.Confidence
		if childConfidence < 0 {
			childConfidence = 0
		}
	}
	magnitude := math.Abs(sentimentScore)
	if magnitude > 1 {
		magnitude = 1
	}
	raw := parentConfidence + 0.5*childConfidence + 0.1*magnitude
	if raw <= 0 {
		return 1.0
	}
	return raw
}

func pairConfidence(entityConfidence float64, sentimentScore float64) float64 {
	magnitude := math.Abs(sentimentScore)
	if magnitude > 1 {
		magnitude = 1
	}
	confidence := entityConfidence * (0.7 + 0.3*magnitude)
	if confidence < 0 {
		return 0
	}
	if confidence > 1 {
		return 1
	}
	return confidence
}

func allocateInt(total, parts, idx int) int {
	if parts <= 0 || total <= 0 {
		return 0
	}
	base := total / parts
	remainder := total % parts
	if idx < remainder {
		return base + 1
	}
	return base
}

func formatBaseSentimentDisplay(label string) string {
	cleanLabel := strings.TrimSpace(label)
	if cleanLabel == "" {
		cleanLabel = "neutral"
	}
	return cleanLabel
}

func formatPairSentimentDisplay(label string, score float64) string {
	cleanLabel := strings.TrimSpace(label)
	if cleanLabel == "" {
		cleanLabel = "neutral"
	}
	return fmt.Sprintf("%s (%.2f)", cleanLabel, score)
}

func featureSummary(article core.Article) string {
	summary := strings.TrimSpace(article.Summary)
	if summary != "" {
		return summary
	}
	return strings.TrimSpace(article.Title)
}
