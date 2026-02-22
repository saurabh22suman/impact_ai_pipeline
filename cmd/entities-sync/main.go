package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/nse"
	"gopkg.in/yaml.v3"
)

func main() {
	var (
		dateRaw    string
		configDir  string
		urlPattern string
	)
	flag.StringVar(&dateRaw, "date", "", "bhavcopy date in YYYY-MM-DD (required)")
	flag.StringVar(&configDir, "config-dir", strings.TrimSpace(os.Getenv("CONFIG_DIR")), "config directory path")
	flag.StringVar(&urlPattern, "url-pattern", strings.TrimSpace(os.Getenv("NSE_BHAVCOPY_URL_PATTERN")), "optional bhavcopy URL pattern with %s for date YYYYMMDD")
	flag.Parse()

	if strings.TrimSpace(configDir) == "" {
		configDir = filepath.Join(".", "configs")
	}
	if strings.TrimSpace(dateRaw) == "" {
		log.Fatal("--date is required (YYYY-MM-DD)")
	}

	bhavcopyDate, err := time.Parse("2006-01-02", dateRaw)
	if err != nil {
		log.Fatalf("invalid --date: %v", err)
	}
	bhavcopyDate = bhavcopyDate.UTC()

	availableAfter := bhavcopyDate.AddDate(0, 0, 2)
	if time.Now().UTC().Before(availableAfter) {
		log.Fatalf("bhavcopy for %s is available only after T+2 (%s)", bhavcopyDate.Format("2006-01-02"), availableAfter.Format(time.RFC3339))
	}

	cfg, err := config.Load(configDir)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	archive, _, err := nse.FetchBhavcopyArchiveWithPattern(ctx, nil, bhavcopyDate, urlPattern)
	if err != nil {
		log.Fatalf("fetch bhavcopy archive: %v", err)
	}

	rows, err := nse.ParseBhavcopyRows(archive)
	if err != nil {
		log.Fatalf("parse bhavcopy archive: %v", err)
	}

	normalized := nse.NormalizeBhavcopyRows(rows, cfg.EntitiesCustom.Entities)

	customFile := config.EntitiesFile{
		Version:  time.Now().UTC().Format("2006-01-02"),
		Entities: normalized,
	}
	payload, err := yaml.Marshal(customFile)
	if err != nil {
		log.Fatalf("marshal entities.custom.yaml: %v", err)
	}

	target := filepath.Join(configDir, "entities.custom.yaml")
	if err := os.WriteFile(target, payload, 0o644); err != nil {
		log.Fatalf("write entities.custom.yaml: %v", err)
	}

	if _, err := config.Load(configDir); err != nil {
		log.Fatalf("reload config after sync: %v", err)
	}

	fmt.Printf("synced %d entities into %s for %s\n", len(normalized), target, bhavcopyDate.Format("2006-01-02"))
}
