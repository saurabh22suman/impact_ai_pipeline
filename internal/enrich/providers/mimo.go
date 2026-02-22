package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type mimoProvider struct {
	model   string
	baseURL string
	apiKey  string
	client  *http.Client
	initErr error
}

type mimoChatRequest struct {
	Model          string             `json:"model"`
	Messages       []mimoChatMessage  `json:"messages"`
	Temperature    float64            `json:"temperature"`
	ResponseFormat mimoResponseFormat `json:"response_format"`
}

type mimoChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type mimoResponseFormat struct {
	Type string `json:"type"`
}

type mimoChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type mimoTokenUsage struct {
	inputTokens  int
	outputTokens int
}

func NewMimoProvider(model string) ProviderClient {
	resolvedModel := strings.TrimSpace(os.Getenv("MIMO_MODEL"))
	if resolvedModel == "" {
		resolvedModel = model
	}

	provider := &mimoProvider{
		model:   resolvedModel,
		baseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("MIMO_BASE_URL")), "/"),
		apiKey:  strings.TrimSpace(os.Getenv("MIMO_API_KEY")),
		client:  &http.Client{Timeout: 20 * time.Second},
	}

	if timeoutRaw := strings.TrimSpace(os.Getenv("MIMO_TIMEOUT_SECONDS")); timeoutRaw != "" {
		seconds, err := strconv.Atoi(timeoutRaw)
		if err != nil || seconds <= 0 {
			provider.initErr = NewFatalProviderError(ErrProviderConfig, fmt.Sprintf("invalid MIMO_TIMEOUT_SECONDS value %q", timeoutRaw))
			return provider
		}
		provider.client.Timeout = time.Duration(seconds) * time.Second
	}

	missing := make([]string, 0, 2)
	if provider.apiKey == "" {
		missing = append(missing, "MIMO_API_KEY")
	}
	if provider.baseURL == "" {
		missing = append(missing, "MIMO_BASE_URL")
	}
	if len(missing) > 0 {
		provider.initErr = NewFatalProviderError(ErrProviderConfig, fmt.Sprintf("missing required env vars: %s", strings.Join(missing, ", ")))
	}

	return provider
}

func (m *mimoProvider) Name() string {
	return "mimo"
}

func (m *mimoProvider) Model() string {
	return m.model
}

func (m *mimoProvider) ClassifySentiment(ctx context.Context, req ClassificationRequest) (SentimentResult, error) {
	if err := m.ready(); err != nil {
		return SentimentResult{}, err
	}

	systemPrompt := `Classify financial sentiment for AI-impact market intelligence. Return ONLY strict JSON matching this schema: {"label":"positive|neutral|negative","score":number}. score must be between -1.0 and 1.0.`
	content, usage, err := m.chatCompletion(ctx, systemPrompt, req.Text)
	if err != nil {
		return SentimentResult{}, err
	}

	var payload struct {
		Label string  `json:"label"`
		Score float64 `json:"score"`
	}
	if err := decodeStrictJSON(content, &payload); err != nil {
		return SentimentResult{}, fmt.Errorf("parse sentiment payload: %w", err)
	}

	payload.Label = strings.ToLower(strings.TrimSpace(payload.Label))
	switch payload.Label {
	case "positive", "neutral", "negative":
	default:
		return SentimentResult{}, fmt.Errorf("invalid sentiment label %q", payload.Label)
	}

	if payload.Score < -1 || payload.Score > 1 {
		return SentimentResult{}, fmt.Errorf("sentiment score must be between -1 and 1, got %f", payload.Score)
	}

	inputTokens, outputTokens := usage.resolve(req.Text, content)
	return SentimentResult{
		Label:        payload.Label,
		Score:        payload.Score,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}, nil
}

func (m *mimoProvider) TagFactors(ctx context.Context, req ClassificationRequest) (FactorResult, error) {
	if err := m.ready(); err != nil {
		return FactorResult{}, err
	}

	systemPrompt := `Extract AI-impact market factors from financial text. Return ONLY strict JSON matching this schema: {"tags":[{"factor_id":"string","name":"string","category":"string","score":number,"matched_by":"llm","llm_refined":true|false}]}. Use an empty tags array when no factor is present.`
	content, usage, err := m.chatCompletion(ctx, systemPrompt, req.Text)
	if err != nil {
		return FactorResult{}, err
	}

	var payload struct {
		Tags []struct {
			FactorID   string  `json:"factor_id"`
			Name       string  `json:"name"`
			Category   string  `json:"category"`
			Score      float64 `json:"score"`
			MatchedBy  string  `json:"matched_by"`
			LLMRefined bool    `json:"llm_refined"`
		} `json:"tags"`
	}
	if err := decodeStrictJSON(content, &payload); err != nil {
		return FactorResult{}, fmt.Errorf("parse factors payload: %w", err)
	}

	tags := make([]FactorTag, 0, len(payload.Tags))
	for _, tag := range payload.Tags {
		tags = append(tags, FactorTag{
			FactorID:   tag.FactorID,
			Name:       tag.Name,
			Category:   tag.Category,
			Score:      tag.Score,
			MatchedBy:  tag.MatchedBy,
			LLMRefined: tag.LLMRefined,
		})
	}

	inputTokens, outputTokens := usage.resolve(req.Text, content)
	return FactorResult{
		Tags:         tags,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}, nil
}

func (m *mimoProvider) DisambiguateEntity(ctx context.Context, req ClassificationRequest) (EntityDisambiguationResult, error) {
	if err := m.ready(); err != nil {
		return EntityDisambiguationResult{}, err
	}

	systemPrompt := `Disambiguate mentioned companies/entities in financial text. Return ONLY strict JSON matching this schema: {"matches":[{"entity_id":"string","symbol":"string","name":"string","confidence":number,"method":"string"}]}. Use an empty matches array when none can be resolved.`
	content, usage, err := m.chatCompletion(ctx, systemPrompt, req.Text)
	if err != nil {
		return EntityDisambiguationResult{}, err
	}

	var payload struct {
		Matches []struct {
			EntityID   string  `json:"entity_id"`
			Symbol     string  `json:"symbol"`
			Name       string  `json:"name"`
			Confidence float64 `json:"confidence"`
			Method     string  `json:"method"`
		} `json:"matches"`
	}
	if err := decodeStrictJSON(content, &payload); err != nil {
		return EntityDisambiguationResult{}, fmt.Errorf("parse entity payload: %w", err)
	}

	matches := make([]EntityMatch, 0, len(payload.Matches))
	for _, match := range payload.Matches {
		matches = append(matches, EntityMatch{
			EntityID:   match.EntityID,
			Symbol:     match.Symbol,
			Name:       match.Name,
			Confidence: match.Confidence,
			Method:     match.Method,
		})
	}

	inputTokens, outputTokens := usage.resolve(req.Text, content)
	return EntityDisambiguationResult{
		Matches:      matches,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}, nil
}

func (m *mimoProvider) ready() error {
	if m.initErr != nil {
		return m.initErr
	}
	return nil
}

func (m *mimoProvider) chatCompletion(ctx context.Context, systemPrompt, userText string) (string, mimoTokenUsage, error) {
	payload := mimoChatRequest{
		Model: m.model,
		Messages: []mimoChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userText},
		},
		Temperature:    0,
		ResponseFormat: mimoResponseFormat{Type: "json_object"},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", mimoTokenUsage{}, fmt.Errorf("marshal mimo request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", mimoTokenUsage{}, fmt.Errorf("build mimo request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+m.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return "", mimoTokenUsage{}, fmt.Errorf("mimo request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", mimoTokenUsage{}, fmt.Errorf("read mimo response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", mimoTokenUsage{}, NewFatalProviderError(ErrProviderAuth, fmt.Sprintf("mimo authentication failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes))))
	}
	if resp.StatusCode != http.StatusOK {
		return "", mimoTokenUsage{}, fmt.Errorf("mimo API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
	}

	var decoded mimoChatResponse
	if err := json.Unmarshal(respBytes, &decoded); err != nil {
		return "", mimoTokenUsage{}, fmt.Errorf("decode mimo response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return "", mimoTokenUsage{}, fmt.Errorf("mimo response missing choices")
	}

	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	if content == "" {
		return "", mimoTokenUsage{}, fmt.Errorf("mimo response missing message content")
	}

	usage := mimoTokenUsage{}
	if decoded.Usage != nil {
		usage.inputTokens = decoded.Usage.PromptTokens
		usage.outputTokens = decoded.Usage.CompletionTokens
	}

	return content, usage, nil
}

func (u mimoTokenUsage) resolve(input, output string) (int, int) {
	inputTokens := u.inputTokens
	outputTokens := u.outputTokens

	if inputTokens <= 0 {
		inputTokens = estimateTokens(input)
	}
	if outputTokens <= 0 {
		outputTokens = estimateTokens(output)
	}
	return inputTokens, outputTokens
}

func decodeStrictJSON(raw string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("unexpected trailing content")
	}
	return nil
}
