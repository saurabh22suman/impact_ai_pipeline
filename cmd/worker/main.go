package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/engine"
)

func main() {
	rt, err := engine.Bootstrap()
	if err != nil {
		log.Fatalf("worker bootstrap failed: %v", err)
	}
	defer func() {
		if err := rt.Close(); err != nil {
			log.Printf("runtime close error: %v", err)
		}
	}()

	profile := os.Getenv("WORKER_PROFILE")
	if profile == "" {
		profile = rt.Config.Pipelines.DefaultProfile
	}
	sourceIDs := splitCSV(os.Getenv("WORKER_SOURCES"))

	log.Printf("worker started profile=%s", profile)
	runOnce(rt, profile, sourceIDs)
}

func runOnce(rt *engine.Runtime, profile string, sourceIDs []string) {
	ctx := context.Background()
	now := time.Now().UTC()
	req := core.RunRequest{
		PipelineProfile: profile,
		DateFrom:        now.Add(-2 * time.Hour),
		DateTo:          now,
		Sources:         sourceIDs,
	}

	sources, err := engine.ResolveSources(rt.Config, req.Sources)
	if err != nil {
		log.Printf("worker source resolution failed: %v", err)
		return
	}

	articles, notices, err := engine.CollectArticles(ctx, rt.Fetcher, sources, req.DateFrom, req.DateTo)
	if err != nil {
		log.Printf("worker collection failed: %v", err)
		return
	}
	for _, notice := range notices {
		log.Printf("worker notice: %s", notice)
	}

	result, err := rt.Service.Run(ctx, req, articles)
	if err != nil {
		log.Printf("worker run failed: %v", err)
		return
	}
	log.Printf("worker run completed run_id=%s events=%d features=%d", result.RunID, len(result.Events), len(result.FeatureRows))
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
