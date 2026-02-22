package enrich

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/enrich/providers"
)

type RouterResult struct {
	Provider      string
	Model         string
	Sentiment     providers.SentimentResult
	Factors       providers.FactorResult
	InputTokens   int
	OutputTokens  int
	EstimatedCost float64
}

type ProviderRouter struct {
	cfg       config.ProvidersFile
	clients   map[string]providers.ProviderClient
	priceMap  map[string]config.Provider
	failures  map[string]int
	openUntil map[string]time.Time
	usage     map[string]usageState
	nowFn     func() time.Time
	mu        sync.Mutex
}

type usageState struct {
	inputTokens  int
	outputTokens int
	costUSD      float64
}

func NewProviderRouter(cfg config.ProvidersFile) *ProviderRouter {
	clients := map[string]providers.ProviderClient{}
	priceMap := map[string]config.Provider{}

	for _, providerCfg := range cfg.Providers {
		if !providerCfg.Enabled {
			continue
		}
		key := providerCfg.Name + ":" + providerCfg.Model
		clients[key] = buildProviderClient(providerCfg)
		priceMap[key] = providerCfg
	}

	return &ProviderRouter{
		cfg:       cfg,
		clients:   clients,
		priceMap:  priceMap,
		failures:  map[string]int{},
		openUntil: map[string]time.Time{},
		usage:     map[string]usageState{},
		nowFn:     func() time.Time { return time.Now().UTC() },
	}
}

func buildProviderClient(providerCfg config.Provider) providers.ProviderClient {
	switch providerCfg.Name {
	case "anthropic":
		return providers.NewAnthropicProvider(providerCfg.Model)
	case "openai":
		return providers.NewOpenAIProvider(providerCfg.Model)
	case "gemini":
		return providers.NewGeminiProvider(providerCfg.Model)
	case "deepseek":
		return providers.NewDeepSeekProvider(providerCfg.Model)
	case "mimo":
		return providers.NewMimoProvider(providerCfg.Model)
	default:
		return providers.NewStubProvider(providerCfg.Name, providerCfg.Model)
	}
}

func (r *ProviderRouter) Enrich(ctx context.Context, req providers.ClassificationRequest) (RouterResult, error) {
	chain := r.cfg.FallbackChain
	if len(chain) == 0 {
		for key := range r.clients {
			chain = append(chain, key)
		}
	}

	if len(chain) == 0 {
		return RouterResult{}, fmt.Errorf("no providers configured")
	}

	var lastErr error
	for _, key := range chain {
		if !r.allowed(key) {
			continue
		}
		client, ok := r.clients[key]
		if !ok {
			continue
		}

		sentiment, err := client.ClassifySentiment(ctx, req)
		if err != nil {
			if providers.IsFatalProviderError(err) {
				return RouterResult{}, err
			}
			r.recordFailure(key)
			lastErr = err
			continue
		}

		factors, err := client.TagFactors(ctx, req)
		if err != nil {
			if providers.IsFatalProviderError(err) {
				return RouterResult{}, err
			}
			r.recordFailure(key)
			lastErr = err
			continue
		}

		r.resetFailure(key)
		price := r.priceMap[key]
		inputTokens := sentiment.InputTokens + factors.InputTokens
		outputTokens := sentiment.OutputTokens + factors.OutputTokens
		cost := estimateCost(price, inputTokens, outputTokens)
		r.recordUsage(key, inputTokens, outputTokens, cost)

		return RouterResult{
			Provider:      client.Name(),
			Model:         client.Model(),
			Sentiment:     sentiment,
			Factors:       factors,
			InputTokens:   inputTokens,
			OutputTokens:  outputTokens,
			EstimatedCost: cost,
		}, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no provider available due to budget or circuit breaker")
	}
	return RouterResult{}, lastErr
}

func estimateCost(price config.Provider, inputTokens, outputTokens int) float64 {
	return (float64(inputTokens)/1000.0)*price.PricePer1KInput + (float64(outputTokens)/1000.0)*price.PricePer1KOutput
}

func (r *ProviderRouter) allowed(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.nowFn()
	if until, ok := r.openUntil[key]; ok && now.Before(until) {
		return false
	}

	usage := r.usage[key]
	if r.cfg.PerProviderTokenBudget > 0 && usage.inputTokens+usage.outputTokens >= r.cfg.PerProviderTokenBudget {
		return false
	}
	if r.cfg.PerProviderCostBudgetUS > 0 && usage.costUSD >= r.cfg.PerProviderCostBudgetUS {
		return false
	}

	var totalTokens int
	var totalCost float64
	for _, st := range r.usage {
		totalTokens += st.inputTokens + st.outputTokens
		totalCost += st.costUSD
	}
	if r.cfg.PerRunTokenBudget > 0 && totalTokens >= r.cfg.PerRunTokenBudget {
		return false
	}
	if r.cfg.PerRunCostBudgetUSD > 0 && totalCost >= r.cfg.PerRunCostBudgetUSD {
		return false
	}
	return true
}

func (r *ProviderRouter) recordFailure(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures[key]++
	if r.failures[key] >= r.cfg.Defaults.CircuitBreakerFailures {
		r.openUntil[key] = r.nowFn().Add(time.Duration(r.cfg.Defaults.CircuitBreakerSeconds) * time.Second)
		r.failures[key] = 0
	}
}

func (r *ProviderRouter) resetFailure(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures[key] = 0
	delete(r.openUntil, key)
}

func (r *ProviderRouter) recordUsage(key string, inputTokens, outputTokens int, cost float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	usage := r.usage[key]
	usage.inputTokens += inputTokens
	usage.outputTokens += outputTokens
	usage.costUSD += cost
	r.usage[key] = usage
}
