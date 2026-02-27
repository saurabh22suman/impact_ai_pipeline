package engine

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/ingest"
)

type ArticleFetcher interface {
	Fetch(ctx context.Context, source config.Source) ([]core.Article, error)
}

type ArticleFetcherWithNotices interface {
	ArticleFetcher
	FetchWithNotices(ctx context.Context, source config.Source) ([]core.Article, []string, error)
}

func ResolveSources(cfg config.AppConfig, requested []string) ([]config.Source, error) {
	enabled := cfg.EnabledSources()
	if len(requested) == 0 {
		return enabled, nil
	}

	byID := map[string]config.Source{}
	for _, source := range enabled {
		byID[strings.ToLower(source.ID)] = source
	}

	out := make([]config.Source, 0, len(requested))
	for _, req := range requested {
		key := strings.ToLower(strings.TrimSpace(req))
		source, ok := byID[key]
		if !ok {
			return nil, fmt.Errorf("unknown or disabled source %q", req)
		}
		out = append(out, source)
	}
	return out, nil
}

func CollectArticles(ctx context.Context, fetcher ArticleFetcher, sources []config.Source, from, to time.Time) ([]core.Article, []string, error) {
	if fetcher == nil {
		return nil, nil, fmt.Errorf("fetcher is nil")
	}

	notices := make([]string, 0)
	all := make([]core.Article, 0)
	for _, source := range sources {
		var (
			articles      []core.Article
			sourceNotices []string
			err           error
		)
		if withNotices, ok := fetcher.(ArticleFetcherWithNotices); ok {
			articles, sourceNotices, err = withNotices.FetchWithNotices(ctx, source)
			notices = append(notices, sourceNotices...)
		} else {
			articles, err = fetcher.Fetch(ctx, source)
		}
		if err != nil {
			return nil, notices, fmt.Errorf("fetch source %s: %w", source.ID, err)
		}
		for _, article := range articles {
			if !from.IsZero() && article.PublishedAt.Before(from) {
				continue
			}
			if !to.IsZero() && article.PublishedAt.After(to) {
				continue
			}
			all = append(all, article)
		}
	}
	return all, notices, nil
}

func CollectArticlesForRequest(ctx context.Context, fetcher ArticleFetcher, sources []config.Source, req core.RunRequest) ([]core.Article, []string, error) {
	registry := NewDefaultBackfillAdapterRegistry()
	return CollectArticlesForRequestWithBackfill(ctx, fetcher, sources, req, registry)
}

func loadLocalBackfillArticles(path string, format string) ([]core.Article, error) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return nil, fmt.Errorf("backfill_file_path is required when backfill_mode=local_file")
	}
	payload, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("read backfill file %s: %w", cleanPath, err)
	}
	if len(payload) == 0 {
		return nil, nil
	}
	return parseBackfillArticles(payload, normalizeBackfillFormat(format))
}

func normalizeBackfillFormat(format string) string {
	trimmed := strings.ToLower(strings.TrimSpace(format))
	if trimmed == "" {
		return "toon"
	}
	return trimmed
}

func parseBackfillArticles(payload []byte, format string) ([]core.Article, error) {
	switch format {
	case "toon", "json", "jsonl":
		articles, err := ingest.ParseTOON(payload)
		if err != nil {
			return nil, fmt.Errorf("parse backfill %s: %w", format, err)
		}
		return articles, nil
	default:
		return nil, fmt.Errorf("unsupported backfill_format %q", format)
	}
}

func filterBySelectedSourcesAndWindow(articles []core.Article, sources []config.Source, from, to time.Time) []core.Article {
	allowed := map[string]struct{}{}
	for _, source := range sources {
		id := strings.ToLower(strings.TrimSpace(source.ID))
		if id == "" {
			continue
		}
		allowed[id] = struct{}{}
	}

	out := make([]core.Article, 0, len(articles))
	for _, article := range articles {
		sourceID := strings.ToLower(strings.TrimSpace(article.SourceID))
		if len(allowed) > 0 {
			if _, ok := allowed[sourceID]; !ok {
				continue
			}
		}
		if !from.IsZero() && article.PublishedAt.Before(from) {
			continue
		}
		if !to.IsZero() && article.PublishedAt.After(to) {
			continue
		}
		out = append(out, article)
	}
	return out
}
