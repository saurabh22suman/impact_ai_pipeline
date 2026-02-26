package engine

import (
	"fmt"
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

func buildFeatureRows(event core.MarketAlignedEvent, impact impactModeConfig) []core.FeatureRow {
	if impact.Enabled {
		rows := buildImpactFeatureRows(event, impact)
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
			SentimentDisplay: formatSentimentDisplay(event.Event.SentimentLabel, event.Event.SentimentScore),
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
			SentimentDisplay: formatSentimentDisplay(event.Event.SentimentLabel, event.Event.SentimentScore),
			Weight:           1.0,
			ConfidenceScore:  entity.Confidence,
			Summary:          featureSummary(event.Event.Article),
		})
	}
	return rows
}

func buildImpactFeatureRows(event core.MarketAlignedEvent, impact impactModeConfig) []core.FeatureRow {
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

	rows := make([]core.FeatureRow, 0, len(parents))
	for _, parent := range parents {
		parentSymbol := strings.ToUpper(strings.TrimSpace(parent.Symbol))
		allowedChildren := impact.ChildSymbolsByParent[parentSymbol]
		if len(allowedChildren) == 0 {
			rows = append(rows, impactFeatureRow(event, factorIDs, parent, nil))
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
			rows = append(rows, impactFeatureRow(event, factorIDs, parent, nil))
			continue
		}

		sort.Slice(matchedChildren, func(i, j int) bool {
			return strings.ToUpper(matchedChildren[i].Symbol) < strings.ToUpper(matchedChildren[j].Symbol)
		})
		for _, child := range matchedChildren {
			childCopy := child
			rows = append(rows, impactFeatureRow(event, factorIDs, parent, &childCopy))
		}
	}
	return rows
}

func impactFeatureRow(event core.MarketAlignedEvent, factorIDs []string, parent core.EntityMatch, child *core.EntityMatch) core.FeatureRow {
	confidence := parent.Confidence
	childEntity := "N/A"
	if child != nil {
		childEntity = child.Symbol
		confidence = (parent.Confidence + child.Confidence) / 2.0
	}
	return core.FeatureRow{
		RunID:            event.Event.Metadata.RunID,
		ConfigVersion:    event.Event.Metadata.ConfigVersion,
		PipelineProfile:  event.Event.Metadata.PipelineProfile,
		Provider:         event.Event.Metadata.Provider,
		Model:            event.Event.Metadata.Model,
		PromptVersion:    event.Event.Metadata.PromptVersion,
		ArticleID:        event.Event.Article.ID,
		Symbol:           parent.Symbol,
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
		ParentEntity:     parent.Symbol,
		ChildEntity:      childEntity,
		SentimentDisplay: formatSentimentDisplay(event.Event.SentimentLabel, event.Event.SentimentScore),
		Weight:           1.0,
		ConfidenceScore:  confidence,
		Summary:          featureSummary(event.Event.Article),
	}
}

func formatSentimentDisplay(label string, score float64) string {
	cleanLabel := strings.TrimSpace(label)
	if cleanLabel == "" {
		cleanLabel = "neutral"
	}
	return fmt.Sprintf("%s %.2f", cleanLabel, score)
}

func featureSummary(article core.Article) string {
	summary := strings.TrimSpace(article.Summary)
	if summary != "" {
		return summary
	}
	return strings.TrimSpace(article.Title)
}
