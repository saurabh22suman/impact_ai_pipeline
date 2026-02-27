package ingest

import (
	"context"
	"fmt"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

type SourceFetcher interface {
	Fetch(ctx context.Context, source config.Source) ([]core.Article, error)
}

type RouterFetcher struct {
	rssFetcher    SourceFetcher
	directFetcher SourceFetcher
	pulseFetcher  SourceFetcher
}

func NewRouterFetcher(rssFetcher, directFetcher, pulseFetcher SourceFetcher) *RouterFetcher {
	return &RouterFetcher{
		rssFetcher:    rssFetcher,
		directFetcher: directFetcher,
		pulseFetcher:  pulseFetcher,
	}
}

func (r *RouterFetcher) Fetch(ctx context.Context, source config.Source) ([]core.Article, error) {
	articles, _, err := r.FetchWithNotices(ctx, source)
	return articles, err
}

func (r *RouterFetcher) FetchWithNotices(ctx context.Context, source config.Source) ([]core.Article, []string, error) {
	kind := config.NormalizeSourceKind(source.Kind)
	source.Kind = kind

	switch kind {
	case config.SourceKindRSS:
		if r.rssFetcher == nil {
			return nil, nil, fmt.Errorf("rss fetcher is nil")
		}
		articles, err := r.rssFetcher.Fetch(ctx, source)
		if err == nil {
			return articles, nil, nil
		}
		if !source.CrawlFallback {
			return nil, nil, err
		}
		if r.directFetcher == nil {
			return nil, []string{fmt.Sprintf("source %s rss fetch failed; direct fallback failed", source.ID)}, fmt.Errorf("source %s rss fetch failed: %w; direct fallback failed: direct fetcher is nil", source.ID, err)
		}

		fallbackSource := source
		fallbackSource.Kind = config.SourceKindDirect
		fallbackArticles, fallbackErr := r.directFetcher.Fetch(ctx, fallbackSource)
		if fallbackErr != nil {
			return nil, []string{fmt.Sprintf("source %s rss fetch failed; direct fallback failed", source.ID)}, fmt.Errorf("source %s rss fetch failed: %w; direct fallback failed: %v", source.ID, err, fallbackErr)
		}
		return fallbackArticles, []string{fmt.Sprintf("source %s rss fetch failed; used direct fallback", source.ID)}, nil
	case config.SourceKindDirect:
		if r.directFetcher == nil {
			return nil, nil, fmt.Errorf("direct fetcher is nil")
		}
		articles, err := r.directFetcher.Fetch(ctx, source)
		return articles, nil, err
	case config.SourceKindPulse:
		if r.pulseFetcher == nil {
			return nil, nil, fmt.Errorf("pulse fetcher is nil")
		}
		articles, err := r.pulseFetcher.Fetch(ctx, source)
		return articles, nil, err
	default:
		return nil, nil, fmt.Errorf("unsupported source kind %q", source.Kind)
	}
}
