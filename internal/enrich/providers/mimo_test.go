package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMimoClassifySentimentSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected /chat/completions, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("expected bearer auth header, got %q", got)
		}

		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request payload: %v", err)
		}
		if payload.Model != "mimo-v2-synthetic" {
			t.Fatalf("expected model mimo-v2-synthetic, got %s", payload.Model)
		}

		responseContent := `{"label":"positive","score":0.73}`
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}],"usage":{"prompt_tokens":12,"completion_tokens":8}}`, responseContent)
	}))
	defer server.Close()

	t.Setenv("MIMO_API_KEY", "test-key")
	t.Setenv("MIMO_BASE_URL", server.URL)
	t.Setenv("MIMO_TIMEOUT_SECONDS", "20")
	t.Setenv("MIMO_MODEL", "")

	provider := NewMimoProvider("mimo-v2-synthetic")
	result, err := provider.ClassifySentiment(context.Background(), ClassificationRequest{Text: "AI demand is strong"})
	if err != nil {
		t.Fatalf("classify sentiment: %v", err)
	}

	if result.Label != "positive" {
		t.Fatalf("expected positive label, got %s", result.Label)
	}
	if result.Score != 0.73 {
		t.Fatalf("expected score 0.73, got %f", result.Score)
	}
	if result.InputTokens != 12 || result.OutputTokens != 8 {
		t.Fatalf("expected token usage from API response (12/8), got %d/%d", result.InputTokens, result.OutputTokens)
	}
}

func TestMimoTagFactorsSuccess(t *testing.T) {
	responseContent := `{"tags":[{"factor_id":"ai-demand","name":"AI Demand Signal","category":"demand","score":0.81,"matched_by":"llm","llm_refined":true}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, responseContent)
	}))
	defer server.Close()

	t.Setenv("MIMO_API_KEY", "test-key")
	t.Setenv("MIMO_BASE_URL", server.URL)
	t.Setenv("MIMO_TIMEOUT_SECONDS", "20")
	t.Setenv("MIMO_MODEL", "")

	provider := NewMimoProvider("mimo-v2-synthetic")
	input := "AI demand and enterprise adoption continue to grow"
	result, err := provider.TagFactors(context.Background(), ClassificationRequest{Text: input})
	if err != nil {
		t.Fatalf("tag factors: %v", err)
	}

	if len(result.Tags) != 1 {
		t.Fatalf("expected 1 factor tag, got %d", len(result.Tags))
	}
	if result.Tags[0].FactorID != "ai-demand" {
		t.Fatalf("expected ai-demand factor id, got %s", result.Tags[0].FactorID)
	}
	if result.InputTokens != estimateTokens(input) {
		t.Fatalf("expected fallback input tokens %d, got %d", estimateTokens(input), result.InputTokens)
	}
	if result.OutputTokens != estimateTokens(responseContent) {
		t.Fatalf("expected fallback output tokens %d, got %d", estimateTokens(responseContent), result.OutputTokens)
	}
}

func TestMimoDisambiguateEntitySuccess(t *testing.T) {
	responseContent := `{"matches":[{"entity_id":"msft","symbol":"MSFT","name":"Microsoft","confidence":0.92,"method":"llm_disambiguation"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}],"usage":{"prompt_tokens":7,"completion_tokens":6}}`, responseContent)
	}))
	defer server.Close()

	t.Setenv("MIMO_API_KEY", "test-key")
	t.Setenv("MIMO_BASE_URL", server.URL)
	t.Setenv("MIMO_TIMEOUT_SECONDS", "20")
	t.Setenv("MIMO_MODEL", "")

	provider := NewMimoProvider("mimo-v2-synthetic")
	result, err := provider.DisambiguateEntity(context.Background(), ClassificationRequest{Text: "Microsoft announced new Azure AI contracts"})
	if err != nil {
		t.Fatalf("disambiguate entity: %v", err)
	}

	if len(result.Matches) != 1 {
		t.Fatalf("expected 1 entity match, got %d", len(result.Matches))
	}
	if result.Matches[0].EntityID != "msft" {
		t.Fatalf("expected entity id msft, got %s", result.Matches[0].EntityID)
	}
	if result.InputTokens != 7 || result.OutputTokens != 6 {
		t.Fatalf("expected token usage from API response (7/6), got %d/%d", result.InputTokens, result.OutputTokens)
	}
}

func TestMimoHandlesNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream failed"}}`))
	}))
	defer server.Close()

	t.Setenv("MIMO_API_KEY", "test-key")
	t.Setenv("MIMO_BASE_URL", server.URL)
	t.Setenv("MIMO_TIMEOUT_SECONDS", "20")
	t.Setenv("MIMO_MODEL", "")

	provider := NewMimoProvider("mimo-v2-synthetic")
	_, err := provider.ClassifySentiment(context.Background(), ClassificationRequest{Text: "AI demand"})
	if err == nil {
		t.Fatalf("expected non-200 error")
	}
	if IsFatalProviderError(err) {
		t.Fatalf("expected non-200 error to be non-fatal")
	}
}

func TestMimoHandlesMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		responseContent := `not-valid-json`
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, responseContent)
	}))
	defer server.Close()

	t.Setenv("MIMO_API_KEY", "test-key")
	t.Setenv("MIMO_BASE_URL", server.URL)
	t.Setenv("MIMO_TIMEOUT_SECONDS", "20")
	t.Setenv("MIMO_MODEL", "")

	provider := NewMimoProvider("mimo-v2-synthetic")
	_, err := provider.TagFactors(context.Background(), ClassificationRequest{Text: "AI demand"})
	if err == nil {
		t.Fatalf("expected malformed JSON parse error")
	}
	if IsFatalProviderError(err) {
		t.Fatalf("expected malformed JSON parse error to be non-fatal")
	}
}

func TestMimoHandlesTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(250 * time.Millisecond)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"label\":\"neutral\",\"score\":0.0}"}}]}`))
	}))
	defer server.Close()

	t.Setenv("MIMO_API_KEY", "test-key")
	t.Setenv("MIMO_BASE_URL", server.URL)
	t.Setenv("MIMO_TIMEOUT_SECONDS", "20")
	t.Setenv("MIMO_MODEL", "")

	provider := NewMimoProvider("mimo-v2-synthetic")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := provider.ClassifySentiment(ctx, ClassificationRequest{Text: "AI demand"})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if IsFatalProviderError(err) {
		t.Fatalf("expected timeout error to be non-fatal")
	}
}

func TestMimoFatalConfigError(t *testing.T) {
	t.Setenv("MIMO_API_KEY", "")
	t.Setenv("MIMO_BASE_URL", "")
	t.Setenv("MIMO_TIMEOUT_SECONDS", "20")
	t.Setenv("MIMO_MODEL", "")

	provider := NewMimoProvider("mimo-v2-synthetic")
	_, err := provider.ClassifySentiment(context.Background(), ClassificationRequest{Text: "AI demand"})
	if err == nil {
		t.Fatalf("expected fatal config error")
	}
	if !IsFatalProviderError(err) {
		t.Fatalf("expected fatal config error type")
	}
}

func TestMimoFatalAuthErrors(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":{"message":"auth failed"}}`))
			}))
			defer server.Close()

			t.Setenv("MIMO_API_KEY", "test-key")
			t.Setenv("MIMO_BASE_URL", server.URL)
			t.Setenv("MIMO_TIMEOUT_SECONDS", "20")
			t.Setenv("MIMO_MODEL", "")

			provider := NewMimoProvider("mimo-v2-synthetic")
			_, err := provider.ClassifySentiment(context.Background(), ClassificationRequest{Text: "AI demand"})
			if err == nil {
				t.Fatalf("expected fatal auth error")
			}
			if !IsFatalProviderError(err) {
				t.Fatalf("expected fatal error type for auth failures")
			}
		})
	}
}

func TestMimoModelOverride(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if payload.Model != "mimo-v2-flash" {
			t.Fatalf("expected MIMO_MODEL override mimo-v2-flash, got %s", payload.Model)
		}
		responseContent := `{"label":"neutral","score":0.0}`
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, responseContent)
	}))
	defer server.Close()

	t.Setenv("MIMO_API_KEY", "test-key")
	t.Setenv("MIMO_BASE_URL", server.URL)
	t.Setenv("MIMO_TIMEOUT_SECONDS", "20")
	t.Setenv("MIMO_MODEL", "mimo-v2-flash")

	provider := NewMimoProvider("mimo-v2-synthetic")
	_, err := provider.ClassifySentiment(context.Background(), ClassificationRequest{Text: "AI demand"})
	if err != nil {
		t.Fatalf("expected success with model override, got %v", err)
	}

	if provider.Model() != "mimo-v2-flash" {
		t.Fatalf("expected model accessor to return mimo-v2-flash, got %s", provider.Model())
	}
}
