package unit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/enrich"
	"github.com/soloengine/ai-impact-scrapper/internal/enrich/providers"
)

func TestRouterSelectsProviderFromFallbackChain(t *testing.T) {
	server := newMimoServer(t, statusPlan{
		http.StatusOK,
		http.StatusOK,
	}, func(_ int, call int) string {
		switch call {
		case 1:
			return `{"label":"positive","score":0.7}`
		case 2:
			return `{"tags":[{"factor_id":"ai-demand","name":"AI Demand","category":"demand","score":0.8,"matched_by":"llm","llm_refined":true}]}`
		default:
			return `{"matches":[]}`
		}
	})
	defer server.Close()

	t.Setenv("MIMO_API_KEY", "test-key")
	t.Setenv("MIMO_BASE_URL", server.URL)
	t.Setenv("MIMO_TIMEOUT_SECONDS", "20")
	t.Setenv("MIMO_MODEL", "")

	cfg := config.ProvidersFile{
		Defaults: config.ProviderDefaults{
			CircuitBreakerFailures: 2,
			CircuitBreakerSeconds:  60,
		},
		Providers: []config.Provider{
			{Name: "mimo", Model: "mimo-v2-synthetic", Enabled: true, PricePer1KInput: 0.1, PricePer1KOutput: 0.1},
			{Name: "gemini", Model: "gemini-2.0-flash", Enabled: true, PricePer1KInput: 0.2, PricePer1KOutput: 0.2},
		},
		FallbackChain:          []string{"mimo:mimo-v2-synthetic", "gemini:gemini-2.0-flash"},
		PerRunTokenBudget:      100000,
		PerProviderTokenBudget: 100000,
	}

	r := enrich.NewProviderRouter(cfg)
	out, err := r.Enrich(context.Background(), providers.ClassificationRequest{Text: "AI demand growth"})
	if err != nil {
		t.Fatalf("enrich failed: %v", err)
	}
	if out.Provider != "mimo" {
		t.Fatalf("expected mimo as first fallback provider, got %s", out.Provider)
	}
	if out.InputTokens <= 0 {
		t.Fatalf("expected positive token usage")
	}
}

func TestRouterRespectsProviderBudget(t *testing.T) {
	server := newMimoServer(t, statusPlan{
		http.StatusOK,
		http.StatusOK,
	}, func(_ int, call int) string {
		switch call {
		case 1:
			return `{"label":"neutral","score":0.0}`
		case 2:
			return `{"tags":[]}`
		default:
			return `{"matches":[]}`
		}
	})
	defer server.Close()

	t.Setenv("MIMO_API_KEY", "test-key")
	t.Setenv("MIMO_BASE_URL", server.URL)
	t.Setenv("MIMO_TIMEOUT_SECONDS", "20")
	t.Setenv("MIMO_MODEL", "")

	cfg := config.ProvidersFile{
		Defaults: config.ProviderDefaults{
			CircuitBreakerFailures: 1,
			CircuitBreakerSeconds:  60,
		},
		Providers: []config.Provider{
			{Name: "mimo", Model: "mimo-v2-synthetic", Enabled: true, PricePer1KInput: 0.1, PricePer1KOutput: 0.1},
		},
		FallbackChain:           []string{"mimo:mimo-v2-synthetic"},
		PerRunTokenBudget:       2,
		PerProviderTokenBudget:  2,
		PerRunCostBudgetUSD:     0.00001,
		PerProviderCostBudgetUS: 0.00001,
	}

	r := enrich.NewProviderRouter(cfg)
	_, _ = r.Enrich(context.Background(), providers.ClassificationRequest{Text: "ai growth"})
	_, err := r.Enrich(context.Background(), providers.ClassificationRequest{Text: "ai growth again"})
	if err == nil {
		t.Fatalf("expected budget exhaustion error")
	}
}

func TestRouterStopsFallbackOnFatalProviderError(t *testing.T) {
	t.Setenv("MIMO_API_KEY", "")
	t.Setenv("MIMO_BASE_URL", "")
	t.Setenv("MIMO_TIMEOUT_SECONDS", "20")
	t.Setenv("MIMO_MODEL", "")

	cfg := config.ProvidersFile{
		Defaults: config.ProviderDefaults{
			CircuitBreakerFailures: 2,
			CircuitBreakerSeconds:  60,
		},
		Providers: []config.Provider{
			{Name: "mimo", Model: "mimo-v2-synthetic", Enabled: true, PricePer1KInput: 0.1, PricePer1KOutput: 0.1},
			{Name: "gemini", Model: "gemini-2.0-flash", Enabled: true, PricePer1KInput: 0.2, PricePer1KOutput: 0.2},
		},
		FallbackChain:          []string{"mimo:mimo-v2-synthetic", "gemini:gemini-2.0-flash"},
		PerRunTokenBudget:      100000,
		PerProviderTokenBudget: 100000,
	}

	r := enrich.NewProviderRouter(cfg)
	_, err := r.Enrich(context.Background(), providers.ClassificationRequest{Text: "AI demand growth"})
	if err == nil {
		t.Fatalf("expected fatal provider error")
	}
	if !providers.IsFatalProviderError(err) {
		t.Fatalf("expected fatal provider error, got %v", err)
	}
	if !errors.Is(err, providers.ErrProviderConfig) {
		t.Fatalf("expected provider config error, got %v", err)
	}
}

func TestRouterStopsFallbackOnFatalAuthError(t *testing.T) {
	server := newMimoServer(t, statusPlan{http.StatusUnauthorized}, func(_ int, _ int) string {
		return `{"label":"positive","score":0.8}`
	})
	defer server.Close()

	t.Setenv("MIMO_API_KEY", "test-key")
	t.Setenv("MIMO_BASE_URL", server.URL)
	t.Setenv("MIMO_TIMEOUT_SECONDS", "20")
	t.Setenv("MIMO_MODEL", "")

	cfg := config.ProvidersFile{
		Defaults: config.ProviderDefaults{
			CircuitBreakerFailures: 2,
			CircuitBreakerSeconds:  60,
		},
		Providers: []config.Provider{
			{Name: "mimo", Model: "mimo-v2-synthetic", Enabled: true, PricePer1KInput: 0.1, PricePer1KOutput: 0.1},
			{Name: "gemini", Model: "gemini-2.0-flash", Enabled: true, PricePer1KInput: 0.2, PricePer1KOutput: 0.2},
		},
		FallbackChain:          []string{"mimo:mimo-v2-synthetic", "gemini:gemini-2.0-flash"},
		PerRunTokenBudget:      100000,
		PerProviderTokenBudget: 100000,
	}

	r := enrich.NewProviderRouter(cfg)
	_, err := r.Enrich(context.Background(), providers.ClassificationRequest{Text: "AI demand growth"})
	if err == nil {
		t.Fatalf("expected fatal auth error")
	}
	if !providers.IsFatalProviderError(err) {
		t.Fatalf("expected fatal provider error, got %v", err)
	}
	if !errors.Is(err, providers.ErrProviderAuth) {
		t.Fatalf("expected provider auth error, got %v", err)
	}
}

type statusPlan []int

func (p statusPlan) forCall(call int) int {
	if len(p) == 0 {
		return http.StatusOK
	}
	idx := call - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(p) {
		return p[len(p)-1]
	}
	return p[idx]
}

func newMimoServer(t *testing.T, plan statusPlan, contentFn func(status int, call int) string) *httptest.Server {
	t.Helper()

	var calls atomic.Int32

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected /chat/completions path, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("expected bearer auth header, got %q", got)
		}

		call := int(calls.Add(1))
		status := plan.forCall(call)
		if status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = fmt.Fprintf(w, `{"error":{"message":"status %d"}}`, status)
			return
		}

		content := contentFn(status, call)
		if content == "" {
			content = `{}`
		}
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}],"usage":{"prompt_tokens":6,"completion_tokens":4}}`, content)
	}))
}
