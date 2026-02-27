# Pulse Source + Entity Matching + UI Metadata Cleanup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add Zerodha Pulse as a dedicated selectable ingestion pipeline (with publisher crawling + browser fallback hook), improve entity mapping precision/recall, and remove `config_version` from user-facing UI/TOON output while preserving internal provenance.

**Architecture:** Keep internal run/config provenance untouched in core/storage, but remove user-facing exposure points. Introduce a new `pulse` source kind with a dedicated fetcher (`listing -> publisher -> readability`) and route it via the existing ingest router only when Pulse is selected/enabled. Unify entity matching logic into a shared matcher utility used by both `EntityMapper` and `RelevanceGate` to avoid inconsistent recall/precision behavior.

**Tech Stack:** Go 1.24, net/http, goquery, go-readability, optional chromedp fallback hook, vanilla HTML/JS frontend, Go unit/integration tests.

---

### Task 1: Remove `config_version` from user-facing UI and TOON output

**Files:**
- Modify: `frontend/index.html`
- Modify: `frontend/app.js`
- Modify: `internal/export/exporter.go`
- Test: `tests/unit/export_eval_test.go`

**Step 1: Write the failing test**

In `tests/unit/export_eval_test.go` inside `TestExporterOutputsJSONLCSVTOON`, add:

```go
if strings.Contains(toonStr, `"config_version"`) {
	t.Fatalf("toon must not expose config_version")
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
go test ./tests/unit -run TestExporterOutputsJSONLCSVTOON -count=1
```

Expected: FAIL with message containing `toon must not expose config_version`.

**Step 3: Write minimal implementation**

1) Remove config card from `frontend/index.html`:

```html
<!-- remove this card -->
<article class="card">
  <h2>Config</h2>
  <p id="config-version">Loading...</p>
</article>
```

2) Remove UI binding/error assignment from `frontend/app.js`:

```js
// remove both lines
// elements.configVersion.textContent = config.config_version || "-";
// elements.configVersion.textContent = "Failed to load";
```

3) Remove TOON field from `internal/export/exporter.go`:

```go
// remove from struct
ConfigVersion string `json:"config_version"`

// remove assignment
ConfigVersion: event.Event.Metadata.ConfigVersion,
```

**Step 4: Run test to verify it passes**

Run:
```bash
go test ./tests/unit -run TestExporterOutputsJSONLCSVTOON -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add tests/unit/export_eval_test.go internal/export/exporter.go frontend/index.html frontend/app.js
git commit -m "chore: remove user-facing config version from UI and toon export"
```

---

### Task 2: Add `pulse` source kind to config/admin/frontend validation paths

**Files:**
- Modify: `internal/config/types.go`
- Modify: `internal/config/loader.go`
- Modify: `internal/sourceadmin/service.go`
- Modify: `frontend/index.html`
- Test: `tests/unit/config_loader_test.go`
- Test: `tests/unit/source_admin_test.go`

**Step 1: Write failing tests**

Add in `tests/unit/config_loader_test.go`:

```go
func TestLoadConfigAcceptsPulseSourceKind(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: pulse\n    name: Pulse\n    kind: pulse\n    url: https://pulse.zerodha.com/\n    region: india\n    language: en\n    enabled: true\n    crawl_fallback: false\n")

	if _, err := config.Load(dir); err != nil {
		t.Fatalf("expected pulse source kind to be valid, got %v", err)
	}
}
```

Add in `tests/unit/source_admin_test.go`:

```go
func TestSourceAdminCreateAcceptsPulseKind(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "sources.yaml", "version: v1\nsources:\n  - id: base\n    name: Base\n    kind: rss\n    url: https://example.com/rss\n    region: global\n    language: en\n    enabled: true\n    crawl_fallback: false\n")

	svc := sourceadmin.NewService(dir)
	created, err := svc.Create(sourceadmin.CreateSourceInput{
		ID: "zerodha-pulse", Name: "Zerodha Pulse", Kind: "pulse",
		URL: "https://pulse.zerodha.com/", Region: "india", Language: "en", Enabled: true,
	})
	if err != nil {
		t.Fatalf("expected pulse kind create to pass: %v", err)
	}
	if created.Kind != "pulse" {
		t.Fatalf("expected pulse kind, got %q", created.Kind)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
go test ./tests/unit -run 'TestLoadConfigAcceptsPulseSourceKind|TestSourceAdminCreateAcceptsPulseKind' -count=1
```

Expected: FAIL with `unsupported kind "pulse"`.

**Step 3: Write minimal implementation**

In `internal/config/types.go`:

```go
const (
	SourceKindRSS    = "rss"
	SourceKindDirect = "direct"
	SourceKindPulse  = "pulse"
)
```

In `internal/config/loader.go` validation switch:

```go
switch source.Kind {
case SourceKindRSS, SourceKindDirect, SourceKindPulse:
default:
	return fmt.Errorf("source %s has unsupported kind %q", source.ID, source.Kind)
}
```

In `internal/sourceadmin/service.go` validation switch:

```go
switch source.Kind {
case config.SourceKindRSS, config.SourceKindDirect, config.SourceKindPulse:
default:
	return config.Source{}, ValidationError{Message: fmt.Sprintf("unsupported kind %q", source.Kind)}
}
```

In `frontend/index.html` source kind select:

```html
<option value="pulse">pulse</option>
```

**Step 4: Run tests to verify they pass**

Run:
```bash
go test ./tests/unit -run 'TestLoadConfigAcceptsPulseSourceKind|TestSourceAdminCreateAcceptsPulseKind' -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/config/types.go internal/config/loader.go internal/sourceadmin/service.go frontend/index.html tests/unit/config_loader_test.go tests/unit/source_admin_test.go
git commit -m "feat: add pulse source kind across config and source admin"
```

---

### Task 3: Implement dedicated Pulse fetcher (listing -> publisher crawl)

**Files:**
- Create: `internal/ingest/pulse.go`
- Test: `internal/ingest/pulse_test.go`

**Step 1: Write failing tests**

Create `internal/ingest/pulse_test.go` with:

```go
func TestPulseFetcherFollowsPublisherLinks(t *testing.T) { /* listing has pulse + publisher links, expect publisher article extracted */ }
func TestPulseFetcherSkipsShareLinks(t *testing.T) { /* listing includes twitter/facebook share URLs, expect 0 articles */ }
func TestPulseFetcherRejectsUnsupportedKind(t *testing.T) { /* kind!=pulse => error */ }
```

Use `httptest.Server` and verify:
- extracted article URL is publisher URL,
- title/body non-empty,
- source metadata preserved,
- canonical hash populated.

**Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/ingest -run TestPulseFetcher -count=1
```

Expected: FAIL (missing `NewPulseFetcher` / missing behavior).

**Step 3: Write minimal implementation**

Create `internal/ingest/pulse.go`:

```go
type PulseFetcher struct {
	client            HTTPClient
	nowFn             func() time.Time
	maxPagesPerSource int
	maxResponseBytes  int64
	browserFallback   BrowserContentExtractor
}

func NewPulseFetcher(client HTTPClient, browserFallback BrowserContentExtractor) *PulseFetcher { /* init defaults */ }
func (f *PulseFetcher) Fetch(ctx context.Context, source config.Source) ([]core.Article, error) { /* pulse-only flow */ }
```

Core logic in `Fetch`:
1. Validate `source.Kind == pulse`.
2. Fetch listing HTML with response cap.
3. Discover candidate publisher links from anchors.
4. Filter out non-http(s), empty, duplicate, and share/social/tracker URLs.
5. Fetch each publisher URL and extract text using readability.
6. Build `core.Article` records using publisher URL + source metadata.

**Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/ingest -run TestPulseFetcher -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/ingest/pulse.go internal/ingest/pulse_test.go
git commit -m "feat: add dedicated pulse fetcher for listing and publisher crawl"
```

---

### Task 4: Add browser fallback hook for thin/failed extraction

**Files:**
- Create: `internal/ingest/browser_fallback.go`
- Modify: `internal/ingest/pulse.go`
- Modify: `internal/ingest/pulse_test.go`

**Step 1: Write failing test**

Add in `internal/ingest/pulse_test.go`:

```go
func TestPulseFetcherUsesBrowserFallbackForThinContent(t *testing.T) {
	// publisher body intentionally empty/too short
	// fake fallback extractor returns richer content
	// expect fallback output used in article body/summary
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
go test ./internal/ingest -run TestPulseFetcherUsesBrowserFallbackForThinContent -count=1
```

Expected: FAIL (fallback not called).

**Step 3: Write minimal implementation**

Create `internal/ingest/browser_fallback.go`:

```go
type BrowserContentExtractor interface {
	Extract(ctx context.Context, pageURL string) (title, summary, body string, err error)
}

type NoopBrowserExtractor struct{}
func (NoopBrowserExtractor) Extract(context.Context, string) (string, string, string, error) {
	return "", "", "", fmt.Errorf("browser fallback not configured")
}
```

In `internal/ingest/pulse.go`, after readability extraction:

```go
if (body == "" || len(body) < 200) && f.browserFallback != nil {
	ft, fs, fb, ferr := f.browserFallback.Extract(ctx, link)
	if ferr == nil && strings.TrimSpace(fb) != "" {
		if strings.TrimSpace(ft) != "" { title = strings.TrimSpace(ft) }
		if strings.TrimSpace(fs) != "" { summary = strings.TrimSpace(fs) }
		body = strings.TrimSpace(fb)
	}
}
```

**Step 4: Run test to verify it passes**

Run:
```bash
go test ./internal/ingest -run TestPulseFetcherUsesBrowserFallbackForThinContent -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/ingest/browser_fallback.go internal/ingest/pulse.go internal/ingest/pulse_test.go
git commit -m "feat: add optional browser fallback hook for pulse extraction"
```

---

### Task 5: Wire pulse fetcher into router/bootstrap and ensure selection behavior

**Files:**
- Modify: `internal/ingest/router.go`
- Modify: `internal/ingest/router_test.go`
- Modify: `internal/engine/bootstrap.go`
- Test: `tests/unit/collector_test.go`

**Step 1: Write failing tests**

1) Update/add router test in `internal/ingest/router_test.go`:

```go
func TestRouterDispatchesPulseKind(t *testing.T) {
	rss := &stubSourceFetcher{}
	direct := &stubSourceFetcher{}
	pulse := &stubSourceFetcher{articles: []core.Article{{Title: "pulse article"}}}
	router := NewRouterFetcher(rss, direct, pulse)

	articles, notices, err := router.FetchWithNotices(context.Background(), config.Source{ID: "zerodha-pulse", Kind: config.SourceKindPulse})
	// expect pulse called once, rss/direct not called
}
```

2) Add selection behavior test in `tests/unit/collector_test.go`:

```go
func TestCollectArticlesOnlyFetchesSelectedSources(t *testing.T) {
	// sources list contains pulse + rss
	// selected list resolves only rss
	// collect and assert pulse fetch path not called
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/ingest ./tests/unit -run 'TestRouterDispatchesPulseKind|TestCollectArticlesOnlyFetchesSelectedSources' -count=1
```

Expected: FAIL (router signature/dispatch not updated).

**Step 3: Write minimal implementation**

In `internal/ingest/router.go`:

```go
type RouterFetcher struct {
	rssFetcher    SourceFetcher
	directFetcher SourceFetcher
	pulseFetcher  SourceFetcher
}

func NewRouterFetcher(rssFetcher, directFetcher, pulseFetcher SourceFetcher) *RouterFetcher { /* ... */ }

case config.SourceKindPulse:
	if r.pulseFetcher == nil { return nil, nil, fmt.Errorf("pulse fetcher is nil") }
	articles, err := r.pulseFetcher.Fetch(ctx, source)
	return articles, nil, err
```

In `internal/engine/bootstrap.go`:

```go
pulseFetcher := ingest.NewPulseFetcher(httpClient, ingest.NoopBrowserExtractor{})
fetcher := ingest.NewRouterFetcher(rssFetcher, directFetcher, pulseFetcher)
```

(If env-gated browser extractor is added later, swap `NoopBrowserExtractor{}` via config/env.)

**Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/ingest ./tests/unit -run 'TestRouterDispatchesPulseKind|TestCollectArticlesOnlyFetchesSelectedSources' -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/ingest/router.go internal/ingest/router_test.go internal/engine/bootstrap.go tests/unit/collector_test.go
git commit -m "feat: route pulse sources through dedicated ingest pipeline"
```

---

### Task 6: Add Pulse source to default sources config

**Files:**
- Modify: `configs/sources.yaml`
- Test: `tests/unit/config_loader_test.go`

**Step 1: Write failing test**

Add in `tests/unit/config_loader_test.go`:

```go
func TestDefaultConfigIncludesZerodhaPulseSource(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs"))
	if err != nil { t.Fatalf("load config: %v", err) }

	found := false
	for _, s := range cfg.Sources.Sources {
		if s.ID == "zerodha-pulse" {
			found = true
			if s.Kind != config.SourceKindPulse {
				t.Fatalf("expected pulse kind, got %q", s.Kind)
			}
		}
	}
	if !found { t.Fatalf("expected zerodha-pulse in default sources") }
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
go test ./tests/unit -run TestDefaultConfigIncludesZerodhaPulseSource -count=1
```

Expected: FAIL (`expected zerodha-pulse in default sources`).

**Step 3: Write minimal implementation**

Add source entry in `configs/sources.yaml`:

```yaml
- id: zerodha-pulse
  name: Zerodha Pulse
  kind: pulse
  url: https://pulse.zerodha.com/
  region: india
  language: en
  enabled: true
  crawl_fallback: false
```

**Step 4: Run test to verify it passes**

Run:
```bash
go test ./tests/unit -run TestDefaultConfigIncludesZerodhaPulseSource -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add configs/sources.yaml tests/unit/config_loader_test.go
git commit -m "feat: add zerodha pulse to default enabled sources"
```

---

### Task 7: Unify entity matching logic for mapper + relevance gate

**Files:**
- Create: `internal/entitymatch/matcher.go`
- Modify: `internal/enrich/entity_mapper.go`
- Modify: `internal/ingest/relevance.go`
- Test: `tests/unit/ingest_relevance_test.go`
- Create Test: `tests/unit/entity_mapper_test.go`

**Step 1: Write failing tests**

1) Add in `tests/unit/ingest_relevance_test.go`:

```go
func TestRelevanceGateDoesNotMatchSymbolInsideLargerWord(t *testing.T) {
	gate := ingest.NewRelevanceGate()
	article := core.Article{Title: "Infinity demand rises", Summary: "", Body: ""}
	entities := []config.Entity{{Name: "Infosys", Symbol: "INFY", Aliases: []string{"INFY"}, Enabled: true}}
	score := gate.Score(article, nil, entities)
	if score >= 0.4 { t.Fatalf("expected no INFY entity hit from Infinity, got %.4f", score) }
}
```

2) Create `tests/unit/entity_mapper_test.go`:

```go
func TestEntityMapperUsesBoundaryAwareMatching(t *testing.T) {
	mapper := enrich.NewEntityMapper([]config.Entity{{ID:"nse-infy", Name:"Infosys", Symbol:"INFY", Aliases:[]string{"INFY"}, Enabled:true}})
	matches := mapper.Map(core.Article{Title:"Infinity outlook improves"})
	if len(matches) != 0 { t.Fatalf("expected no match, got %+v", matches) }
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
go test ./tests/unit -run 'TestRelevanceGateDoesNotMatchSymbolInsideLargerWord|TestEntityMapperUsesBoundaryAwareMatching' -count=1
```

Expected: FAIL due substring-based matching.

**Step 3: Write minimal implementation**

Create `internal/entitymatch/matcher.go`:

```go
package entitymatch

// Normalize text (lowercase, punctuation->space, collapse spaces)
// Boundary-aware ContainsTerm(text, needle)
// MatchEntity(text, entity) -> (matched bool, confidence float64, method string)
```

Use it in `internal/enrich/entity_mapper.go`:

```go
matched, confidence, method := entitymatch.MatchEntity(text, entity)
if matched { /* append match */ }
```

Use same matcher in `internal/ingest/relevance.go` for entity hit detection:

```go
if matched, _, _ := entitymatch.MatchEntity(text, entity); matched { entityMatched = true; break }
```

**Step 4: Run tests to verify they pass**

Run:
```bash
go test ./tests/unit -run 'TestRelevanceGateDoesNotMatchSymbolInsideLargerWord|TestEntityMapperUsesBoundaryAwareMatching|TestRelevanceGateScoresExpectedRange' -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/entitymatch/matcher.go internal/enrich/entity_mapper.go internal/ingest/relevance.go tests/unit/ingest_relevance_test.go tests/unit/entity_mapper_test.go
git commit -m "feat: unify boundary-aware entity matching across mapper and relevance"
```

---

### Task 8: Full verification + regression sweep

**Files:**
- Verify only (no new files expected)

**Step 1: Run targeted package tests first**

Run:
```bash
go test ./internal/ingest ./internal/config ./internal/sourceadmin ./internal/export ./tests/unit ./tests/integration ./cmd/api -count=1
```

Expected: PASS for all touched domains.

**Step 2: Run full test suite**

Run:
```bash
go test ./... -count=1
```

Expected: PASS.

**Step 3: Manual smoke checks**

Run app and verify:
- UI no longer shows config card/value.
- Source form supports `pulse` kind.
- Run with `sources=["zerodha-pulse"]` returns notices/articles without routing errors.
- Run with non-pulse sources does not trigger pulse notices/calls.

**Step 4: Final verification checklist (@superpowers:verification-before-completion)**

- Confirm no unexpected `config_version` exposure in user-facing outputs.
- Confirm pulse ingestion path only runs when pulse selected/enabled.
- Confirm entity mapping improvements did not break existing relevance thresholds.

**Step 5: Final commit (if verification-only changes were made)**

```bash
git status
# if any final fixups were needed:
# git add <files>
# git commit -m "test: finalize pulse pipeline and entity matching verification"
```

---

## Execution Notes

- Use **@superpowers:test-driven-development** discipline inside each task (red -> green -> refactor).
- Keep commits scoped to one task at a time (frequent small commits).
- Do not remove internal provenance fields from core/storage models.
- Keep YAGNI: only implement browser fallback hook behavior needed for Pulse path.
