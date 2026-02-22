package providers

import "context"

type ClassificationRequest struct {
	Text string
}

type SentimentResult struct {
	Label        string
	Score        float64
	InputTokens  int
	OutputTokens int
}

type FactorTag struct {
	FactorID   string
	Name       string
	Category   string
	Score      float64
	MatchedBy  string
	LLMRefined bool
}

type FactorResult struct {
	Tags         []FactorTag
	InputTokens  int
	OutputTokens int
}

type EntityMatch struct {
	EntityID   string
	Symbol     string
	Name       string
	Confidence float64
	Method     string
}

type EntityDisambiguationResult struct {
	Matches      []EntityMatch
	InputTokens  int
	OutputTokens int
}

type ProviderClient interface {
	Name() string
	Model() string
	ClassifySentiment(ctx context.Context, req ClassificationRequest) (SentimentResult, error)
	TagFactors(ctx context.Context, req ClassificationRequest) (FactorResult, error)
	DisambiguateEntity(ctx context.Context, req ClassificationRequest) (EntityDisambiguationResult, error)
}
