package engine

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/ingest"
	"github.com/soloengine/ai-impact-scrapper/internal/storage"
)

type Runtime struct {
	Config    config.AppConfig
	ConfigDir string
	Service   *Service
	Fetcher   ArticleFetcher
	Store     storage.EngineStore
	closeFn   func() error
}

func Bootstrap() (*Runtime, error) {
	cfgDir := os.Getenv("CONFIG_DIR")
	if cfgDir == "" {
		cfgDir = filepath.Join(".", "configs")
	}

	cfg, err := config.Load(cfgDir)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, cleanup, err := storage.NewStoreFromEnv(ctx)
	if err != nil {
		return nil, fmt.Errorf("init storage: %w", err)
	}

	service := NewService(cfg, store)
	httpClient := &http.Client{Timeout: 20 * time.Second}
	rssFetcher := ingest.NewRSSFetcher(httpClient)
	directFetcher := ingest.NewDirectFetcher(httpClient)
	pulseFetcher := ingest.NewPulseFetcher(httpClient, ingest.NoopBrowserExtractor{})
	fetcher := ingest.NewRouterFetcher(rssFetcher, directFetcher, pulseFetcher)

	return &Runtime{
		Config:    cfg,
		ConfigDir: cfgDir,
		Service:   service,
		Fetcher:   fetcher,
		Store:     store,
		closeFn:   cleanup,
	}, nil
}

func (r *Runtime) Close() error {
	if r == nil || r.closeFn == nil {
		return nil
	}
	return r.closeFn()
}
