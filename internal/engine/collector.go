package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
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
