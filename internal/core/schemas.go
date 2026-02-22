package core

import "time"

type RunRequest struct {
	Entities        []string  `json:"entities"`
	Sources         []string  `json:"sources"`
	DateFrom        time.Time `json:"date_from"`
	DateTo          time.Time `json:"date_to"`
	RawDataToggle   bool      `json:"raw_data_toggle"`
	PipelineProfile string    `json:"pipeline_profile"`
}

type RunStatus string

const (
	RunStatusQueued      RunStatus = "queued"
	RunStatusRunning     RunStatus = "running"
	RunStatusCompleted   RunStatus = "completed"
	RunStatusFailed      RunStatus = "failed"
	RunStatusCancelled   RunStatus = "cancelled"
	DefaultPromptVersion           = "v1"
)

type RunMetadata struct {
	RunID           string    `json:"run_id"`
	ConfigVersion   string    `json:"config_version"`
	PipelineProfile string    `json:"pipeline_profile"`
	Provider        string    `json:"provider"`
	Model           string    `json:"model"`
	PromptVersion   string    `json:"prompt_version"`
	InputTokens     int       `json:"input_tokens"`
	OutputTokens    int       `json:"output_tokens"`
	EstimatedCostUS float64   `json:"estimated_cost_usd"`
	CreatedAt       time.Time `json:"created_at"`
}

type Article struct {
	ID              string    `json:"id"`
	SourceID        string    `json:"source_id"`
	SourceName      string    `json:"source_name"`
	URL             string    `json:"url"`
	Title           string    `json:"title"`
	Summary         string    `json:"summary"`
	Body            string    `json:"body"`
	Language        string    `json:"language"`
	Region          string    `json:"region"`
	PublishedAt     time.Time `json:"published_at"`
	IngestedAt      time.Time `json:"ingested_at"`
	CanonicalHash   string    `json:"canonical_hash"`
	RawArtifactPath string    `json:"raw_artifact_path,omitempty"`
}

type EntityMatch struct {
	EntityID   string  `json:"entity_id"`
	Symbol     string  `json:"symbol"`
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
	Method     string  `json:"method"`
}

type FactorTag struct {
	FactorID   string  `json:"factor_id"`
	Name       string  `json:"name"`
	Category   string  `json:"category"`
	Score      float64 `json:"score"`
	MatchedBy  string  `json:"matched_by"`
	LLMRefined bool    `json:"llm_refined"`
}

type EnrichedEvent struct {
	Metadata           RunMetadata   `json:"metadata"`
	Article            Article       `json:"article"`
	Entities           []EntityMatch `json:"entities"`
	Factors            []FactorTag   `json:"factors"`
	SentimentLabel     string        `json:"sentiment_label"`
	SentimentScore     float64       `json:"sentiment_score"`
	RelevanceScore     float64       `json:"relevance_score"`
	NeedsLLMRefinement bool          `json:"needs_llm_refinement"`
}

type MarketSession struct {
	Exchange      string    `json:"exchange"`
	SessionDate   time.Time `json:"session_date"`
	SessionLabel  string    `json:"session_label"`
	PreOpen       bool      `json:"pre_open"`
	DuringSession bool      `json:"during_session"`
	PostClose     bool      `json:"post_close"`
}

type MarketAlignedEvent struct {
	Event         EnrichedEvent `json:"event"`
	Session       MarketSession `json:"session"`
	LabelWindowTo time.Time     `json:"label_window_to"`
}

type FeatureRow struct {
	RunID           string    `json:"run_id"`
	ConfigVersion   string    `json:"config_version"`
	PipelineProfile string    `json:"pipeline_profile"`
	Provider        string    `json:"provider"`
	Model           string    `json:"model"`
	PromptVersion   string    `json:"prompt_version"`
	ArticleID       string    `json:"article_id"`
	Symbol          string    `json:"symbol"`
	SessionDate     time.Time `json:"session_date"`
	SessionLabel    string    `json:"session_label"`
	SentimentScore  float64   `json:"sentiment_score"`
	RelevanceScore  float64   `json:"relevance_score"`
	FactorVector    []string  `json:"factor_vector"`
	InputTokens     int       `json:"input_tokens"`
	OutputTokens    int       `json:"output_tokens"`
	EstimatedCostUS float64   `json:"estimated_cost_usd"`
}

type RunResult struct {
	RunID          string               `json:"run_id"`
	Status         RunStatus            `json:"status"`
	CreatedAt      time.Time            `json:"created_at"`
	StartedAt      time.Time            `json:"started_at"`
	FinishedAt     time.Time            `json:"finished_at"`
	ConfigVersion  string               `json:"config_version"`
	Profile        string               `json:"profile"`
	Events         []MarketAlignedEvent `json:"events"`
	FeatureRows    []FeatureRow         `json:"feature_rows"`
	InputTokens    int                  `json:"input_tokens"`
	OutputTokens   int                  `json:"output_tokens"`
	EstimatedCost  float64              `json:"estimated_cost_usd"`
	FailureReason  string               `json:"failure_reason,omitempty"`
	ArtifactCounts map[string]int       `json:"artifact_counts,omitempty"`
}

type ProviderUsage struct {
	Provider      string  `json:"provider"`
	Model         string  `json:"model"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	EstimatedCost float64 `json:"estimated_cost_usd"`
}

type DatasetProvenance struct {
	RunID           string `json:"run_id"`
	ConfigVersion   string `json:"config_version"`
	PipelineProfile string `json:"pipeline_profile"`
	Provider        string `json:"provider"`
	Model           string `json:"model"`
	PromptVersion   string `json:"prompt_version"`
}
