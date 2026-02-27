package unit

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/sourceadmin"
)

func TestSourceAdminCreatePersistsSource(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: base\n    name: Base\n    kind: rss\n    url: https://example.com/rss\n    region: global\n    language: en\n    enabled: true\n    crawl_fallback: false\n")

	svc := sourceadmin.NewService(dir)
	created, err := svc.Create(sourceadmin.CreateSourceInput{
		ID:            "new-source",
		Name:          "New Source",
		Kind:          "rss",
		URL:           "https://example.com/new.xml",
		Region:        "india",
		Language:      "en",
		Enabled:       true,
		CrawlFallback: true,
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if created.ID != "new-source" || !created.CrawlFallback {
		t.Fatalf("unexpected create result: %+v", created)
	}

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("reload config after create: %v", err)
	}
	found := false
	for _, src := range cfg.Sources.Sources {
		if src.ID == "new-source" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected source persisted in sources.yaml")
	}
}

func TestSourceAdminCreateRejectsInvalidInput(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: base\n    name: Base\n    kind: rss\n    url: https://example.com/rss\n    region: global\n    language: en\n    enabled: true\n    crawl_fallback: false\n")

	svc := sourceadmin.NewService(dir)
	_, err := svc.Create(sourceadmin.CreateSourceInput{
		ID:       "",
		Name:     "Missing ID",
		Kind:     "rss",
		URL:      "https://example.com/new.xml",
		Region:   "india",
		Language: "en",
		Enabled:  true,
	})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !errors.Is(err, sourceadmin.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestSourceAdminCreateAcceptsPulseKind(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: base\n    name: Base\n    kind: rss\n    url: https://example.com/rss\n    region: global\n    language: en\n    enabled: true\n    crawl_fallback: false\n")

	svc := sourceadmin.NewService(dir)
	created, err := svc.Create(sourceadmin.CreateSourceInput{
		ID:       "pulse-source",
		Name:     "Pulse Source",
		Kind:     "pulse",
		URL:      "https://example.com/pulse",
		Region:   "global",
		Language: "en",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if created.Kind != config.SourceKindPulse {
		t.Fatalf("expected source kind %q, got %q", config.SourceKindPulse, created.Kind)
	}
}

func TestSourceAdminCreateRejectsDuplicateID(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: base\n    name: Base\n    kind: rss\n    url: https://example.com/rss\n    region: global\n    language: en\n    enabled: true\n    crawl_fallback: false\n")

	svc := sourceadmin.NewService(dir)
	_, err := svc.Create(sourceadmin.CreateSourceInput{
		ID:       "BASE",
		Name:     "Duplicate",
		Kind:     "rss",
		URL:      "https://example.com/dup.xml",
		Region:   "india",
		Language: "en",
		Enabled:  true,
	})
	if err == nil {
		t.Fatalf("expected duplicate validation error")
	}
	if !errors.Is(err, sourceadmin.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestSourceAdminListReturnsConfiguredSources(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: base\n    name: Base\n    kind: rss\n    url: https://example.com/rss\n    region: global\n    language: en\n    enabled: true\n    crawl_fallback: false\n")

	svc := sourceadmin.NewService(dir)
	sources, err := svc.List()
	if err != nil {
		t.Fatalf("list sources: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	if sources[0].ID != "base" {
		t.Fatalf("unexpected source: %+v", sources[0])
	}
}

func TestSourceAdminCreatePreservesConfigLoadability(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: base\n    name: Base\n    kind: rss\n    url: https://example.com/rss\n    region: global\n    language: en\n    enabled: true\n    crawl_fallback: false\n")

	svc := sourceadmin.NewService(dir)
	_, err := svc.Create(sourceadmin.CreateSourceInput{
		ID:       "live-source",
		Name:     "Live Source",
		Kind:     "direct",
		URL:      "https://example.com/live",
		Region:   "global",
		Language: "en",
		Enabled:  true,
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	if _, err := config.Load(filepath.Clean(dir)); err != nil {
		t.Fatalf("config should remain loadable: %v", err)
	}
}
