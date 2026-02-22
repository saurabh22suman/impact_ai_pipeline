package providers

import (
	"context"
	"strings"
)

type StubProvider struct {
	provider string
	model    string
}

func NewStubProvider(provider, model string) *StubProvider {
	return &StubProvider{provider: provider, model: model}
}

func (s *StubProvider) Name() string {
	return s.provider
}

func (s *StubProvider) Model() string {
	return s.model
}

func (s *StubProvider) ClassifySentiment(_ context.Context, req ClassificationRequest) (SentimentResult, error) {
	text := strings.ToLower(req.Text)
	score := 0.0
	label := "neutral"

	positiveTokens := []string{"beat", "growth", "strong", "upgrade", "bullish", "surge"}
	negativeTokens := []string{"miss", "downgrade", "weak", "selloff", "delay", "bearish"}

	for _, tok := range positiveTokens {
		if strings.Contains(text, tok) {
			score += 0.2
		}
	}
	for _, tok := range negativeTokens {
		if strings.Contains(text, tok) {
			score -= 0.2
		}
	}

	if score > 0.1 {
		label = "positive"
	}
	if score < -0.1 {
		label = "negative"
	}

	return SentimentResult{
		Label:        label,
		Score:        score,
		InputTokens:  estimateTokens(req.Text),
		OutputTokens: 30,
	}, nil
}

func (s *StubProvider) TagFactors(_ context.Context, req ClassificationRequest) (FactorResult, error) {
	text := strings.ToLower(req.Text)
	tags := []FactorTag{}

	appendTag := func(id, name, category string, score float64) {
		tags = append(tags, FactorTag{
			FactorID:   id,
			Name:       name,
			Category:   category,
			Score:      score,
			MatchedBy:  "stub_keyword",
			LLMRefined: false,
		})
	}

	if strings.Contains(text, "capex") || strings.Contains(text, "gpu") || strings.Contains(text, "datacenter") {
		appendTag("ai-capex", "AI Capex", "investment", 0.8)
	}
	if strings.Contains(text, "regulation") || strings.Contains(text, "compliance") {
		appendTag("ai-regulation", "AI Regulation", "policy", 0.7)
	}
	if strings.Contains(text, "adoption") || strings.Contains(text, "demand") || strings.Contains(text, "deal") {
		appendTag("ai-demand", "AI Demand Signal", "demand", 0.75)
	}
	if strings.Contains(text, "cost") || strings.Contains(text, "margin") {
		appendTag("ai-cost-pressure", "AI Cost Pressure", "margin", 0.72)
	}
	if strings.Contains(text, "hiring") || strings.Contains(text, "talent") || strings.Contains(text, "attrition") {
		appendTag("ai-talent", "AI Talent Dynamics", "labor", 0.65)
	}

	return FactorResult{
		Tags:         tags,
		InputTokens:  estimateTokens(req.Text),
		OutputTokens: 80,
	}, nil
}

func (s *StubProvider) DisambiguateEntity(_ context.Context, req ClassificationRequest) (EntityDisambiguationResult, error) {
	return EntityDisambiguationResult{
		Matches:      []EntityMatch{},
		InputTokens:  estimateTokens(req.Text),
		OutputTokens: 40,
	}, nil
}

func estimateTokens(text string) int {
	if text == "" {
		return 1
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return 1
	}
	return len(words) + len(words)/3
}
