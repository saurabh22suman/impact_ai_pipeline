package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

type BackfillAdapter interface {
	Mode() string
	Collect(ctx context.Context, req core.RunRequest, sources []config.Source) ([]core.Article, []string, error)
}

type BackfillAdapterRegistry struct {
	byMode map[string]BackfillAdapter
}

func NewBackfillAdapterRegistry(adapters ...BackfillAdapter) BackfillAdapterRegistry {
	registry := BackfillAdapterRegistry{byMode: map[string]BackfillAdapter{}}
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		mode := strings.ToLower(strings.TrimSpace(adapter.Mode()))
		if mode == "" {
			continue
		}
		registry.byMode[mode] = adapter
	}
	return registry
}

func NewDefaultBackfillAdapterRegistry() BackfillAdapterRegistry {
	return NewBackfillAdapterRegistry(
		LocalFileBackfillAdapter{},
		ArchiveHTTPBackfillAdapter{},
	)
}

func NewBackfillAdapterRegistryWithHTTPClient(client *http.Client) BackfillAdapterRegistry {
	return NewBackfillAdapterRegistry(
		LocalFileBackfillAdapter{},
		ArchiveHTTPBackfillAdapter{HTTPClient: client},
	)
}

func (r BackfillAdapterRegistry) Lookup(mode string) (BackfillAdapter, bool) {
	key := strings.ToLower(strings.TrimSpace(mode))
	if key == "" {
		return nil, false
	}
	adapter, ok := r.byMode[key]
	return adapter, ok
}

func CollectArticlesForRequestWithBackfill(ctx context.Context, fetcher ArticleFetcher, sources []config.Source, req core.RunRequest, registry BackfillAdapterRegistry) ([]core.Article, []string, error) {
	mode := strings.TrimSpace(req.BackfillMode)
	if mode != "" {
		adapter, ok := registry.Lookup(mode)
		if !ok {
			return nil, nil, fmt.Errorf("unsupported backfill_mode %q", req.BackfillMode)
		}
		return adapter.Collect(ctx, req, sources)
	}
	return CollectArticles(ctx, fetcher, sources, req.DateFrom, req.DateTo)
}

type LocalFileBackfillAdapter struct{}

func (LocalFileBackfillAdapter) Mode() string {
	return "local_file"
}

func (LocalFileBackfillAdapter) Collect(_ context.Context, req core.RunRequest, sources []config.Source) ([]core.Article, []string, error) {
	articles, err := loadLocalBackfillArticles(req.BackfillFilePath, req.BackfillFormat)
	if err != nil {
		return nil, nil, err
	}
	filtered := filterBySelectedSourcesAndWindow(articles, sources, req.DateFrom, req.DateTo)
	notice := fmt.Sprintf("backfill local file loaded: mode=%s format=%s file=%s", strings.TrimSpace(req.BackfillMode), normalizeBackfillFormat(req.BackfillFormat), strings.TrimSpace(req.BackfillFilePath))
	return filtered, []string{notice}, nil
}

type ArchiveHTTPBackfillAdapter struct {
	HTTPClient *http.Client
}

func (a ArchiveHTTPBackfillAdapter) Mode() string {
	return "archive_http"
}

func (a ArchiveHTTPBackfillAdapter) Collect(ctx context.Context, req core.RunRequest, sources []config.Source) ([]core.Article, []string, error) {
	targetURL := strings.TrimSpace(req.BackfillURL)
	if targetURL == "" {
		return nil, nil, fmt.Errorf("backfill_url is required when backfill_mode=archive_http")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("build archive_http request: %w", err)
	}

	client := a.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch archive_http payload: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode >= http.StatusBadRequest {
		return nil, nil, fmt.Errorf("archive_http returned status %d", response.StatusCode)
	}

	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read archive_http payload: %w", err)
	}

	articles, err := parseBackfillArticles(payload, normalizeBackfillFormat(req.BackfillFormat))
	if err != nil {
		return nil, nil, err
	}

	filtered := filterBySelectedSourcesAndWindow(articles, sources, req.DateFrom, req.DateTo)
	notice := fmt.Sprintf("backfill archive_http loaded: url=%s format=%s", targetURL, normalizeBackfillFormat(req.BackfillFormat))
	return filtered, []string{notice}, nil
}
