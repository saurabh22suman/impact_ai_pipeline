# NIFTY IT Parent-Child Impact Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add permanent parent-child impact analysis for NIFTY IT so one article can emit multiple `(parent, child)` rows with per-pair sentiment and the exact CSV contract requested by the user.

**Architecture:** Extend config with entity roles plus an explicit impact group (`nifty-it-impact`), then keep ingestion/enrichment intact and change feature-row generation in `engine.Service` to produce parent/child pair rows. Pair-level sentiment is computed by calling the existing enrich router with pair-scoped entity context. CSV export is switched to the required 10-column business format while keeping internal run provenance fields in storage models.

**Tech Stack:** Go 1.24, YAML config loader (`gopkg.in/yaml.v3`), existing enrich provider router, existing file/memory storage, Go unit/integration tests.

---

### Task 1: Add config schema for entity roles + impact groups

**Files:**
- Modify: `internal/config/types.go`
- Modify: `internal/config/loader.go`
- Test: `tests/unit/config_loader_test.go`

**Step 1: Write the failing tests**

Add tests in `tests/unit/config_loader_test.go`:

```go
func TestLoadConfigParsesEntityGroups(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "entities.custom.yaml", `version: v1
entities:
  - id: ai-openai
    symbol: OPENAI
    name: OpenAI
    aliases: [OpenAI, ChatGPT]
    exchange: GLOBAL
    sector: AI
    type: equity
    role: child
    enabled: true
`)
	mustWrite(t, dir, "entity_groups.yaml", `version: v1
groups:
  - id: nifty-it-impact
    parents: [TCS]
    children: [OPENAI]
`)

	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.EntityGroups.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(cfg.EntityGroups.Groups))
	}
}

func TestLoadConfigRejectsEntityGroupUnknownSymbol(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "entity_groups.yaml", `version: v1
groups:
  - id: nifty-it-impact
    parents: [TCS]
    children: [UNKNOWN_CHILD]
`)

	_, err := config.Load(dir)
	if err == nil {
		t.Fatalf("expected unknown symbol validation error")
	}
}

func TestLoadConfigRejectsEntityGroupRoleMismatch(t *testing.T) {
	dir := t.TempDir()
	writeBaseConfigFiles(t, dir)
	mustWrite(t, dir, "entities.custom.yaml", `version: v1
entities:
  - id: ai-openai
    symbol: OPENAI
    name: OpenAI
    aliases: [OpenAI]
    exchange: GLOBAL
    sector: AI
    type: equity
    role: parent
    enabled: true
`)
	mustWrite(t, dir, "entity_groups.yaml", `version: v1
groups:
  - id: nifty-it-impact
    parents: [TCS]
    children: [OPENAI]
`)

	_, err := config.Load(dir)
	if err == nil {
		t.Fatalf("expected role mismatch validation error")
	}
}
```

Also update `writeBaseConfigFiles` to include:

```go
mustWrite(t, dir, "entity_groups.yaml", "version: v1\ngroups: []\n")
```

**Step 2: Run tests to verify they fail**

Run:
```bash
go test ./tests/unit -run 'TestLoadConfigParsesEntityGroups|TestLoadConfigRejectsEntityGroupUnknownSymbol|TestLoadConfigRejectsEntityGroupRoleMismatch' -count=1
```

Expected: FAIL (missing `EntityGroups` schema/validation).

**Step 3: Write minimal implementation**

In `internal/config/types.go`, add role/group types and app config field:

```go
const (
	EntityRoleParent = "parent"
	EntityRoleChild  = "child"
)

type Entity struct {
	ID       string   `yaml:"id"`
	Symbol   string   `yaml:"symbol"`
	Name     string   `yaml:"name"`
	Aliases  []string `yaml:"aliases"`
	Exchange string   `yaml:"exchange"`
	Sector   string   `yaml:"sector"`
	Type     string   `yaml:"type"`
	Role     string   `yaml:"role"`
	Enabled  bool     `yaml:"enabled"`
}

type EntityGroup struct {
	ID       string   `yaml:"id"`
	Parents  []string `yaml:"parents"`
	Children []string `yaml:"children"`
}

type EntityGroupsFile struct {
	Version string        `yaml:"version"`
	Groups  []EntityGroup `yaml:"groups"`
}
```

Add normalization helper in `types.go`:

```go
func NormalizeEntityRole(raw string) string {
	role := strings.ToLower(strings.TrimSpace(raw))
	switch role {
	case EntityRoleParent, EntityRoleChild:
		return role
	default:
		return ""
	}
}
```

In `internal/config/loader.go`:
- load `entity_groups.yaml`
- normalize entity roles for default/custom entities
- validate group symbol resolution + role correctness against `cfg.EffectiveEntities()`

Use optional load fallback:

```go
entityGroups := EntityGroupsFile{Version: "v1", Groups: []EntityGroup{}}
if groups, err := loadYAML[EntityGroupsFile](filepath.Join(configDir, "entity_groups.yaml")); err == nil {
	entityGroups = groups
} else if !errors.Is(err, fs.ErrNotExist) {
	return AppConfig{}, err
}
```

**Step 4: Run tests to verify they pass**

Run:
```bash
go test ./tests/unit -run 'TestLoadConfigParsesEntityGroups|TestLoadConfigRejectsEntityGroupUnknownSymbol|TestLoadConfigRejectsEntityGroupRoleMismatch|TestLoadConfigIncludesNiftyITByDefault' -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/config/types.go internal/config/loader.go tests/unit/config_loader_test.go
git commit -m "feat: add entity roles and impact-group config validation"
```

---

### Task 2: Seed default NIFTY IT parent/child config data

**Files:**
- Modify: `configs/entities.niftyit.yaml`
- Modify: `configs/entities.custom.yaml`
- Create: `configs/entity_groups.yaml`
- Test: `tests/unit/config_loader_test.go`

**Step 1: Write the failing test**

Add in `tests/unit/config_loader_test.go`:

```go
func TestDefaultConfigDefinesNiftyITImpactGroup(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if len(cfg.EntityGroups.Groups) == 0 {
		t.Fatalf("expected at least one entity group")
	}

	found := false
	for _, g := range cfg.EntityGroups.Groups {
		if g.ID == "nifty-it-impact" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected nifty-it-impact group in default config")
	}
}
```

**Step 2: Run test to verify it fails**

Run:
```bash
go test ./tests/unit -run TestDefaultConfigDefinesNiftyITImpactGroup -count=1
```

Expected: FAIL (`nifty-it-impact` not present yet).

**Step 3: Write minimal implementation**

1) In `configs/entities.niftyit.yaml`, set all 10 constituents to `role: parent`.

Example pattern (apply to all 10):

```yaml
- id: nse-infy
  symbol: INFY
  name: Infosys Limited
  aliases: ["Infosys", "INFY", "Infosys Ltd"]
  exchange: NSE
  sector: IT
  type: equity
  role: parent
  enabled: true
```

2) In `configs/entities.custom.yaml`, add child entities with `role: child` for:
- OPENAI
- GEMINI
- ANTHROPIC
- DEEPSEEK
- BYTEDANCE
- XAI
- GEOPOLITICS
- GOLD_SILVER_COPPER
- SEMICONDUCTOR
- NVIDIA
- ECONOMICS
- ENERGY_ELECTRICITY
- AI
- DRAM
- CHIPS
- INDIA_US_RELATIONS

3) Create `configs/entity_groups.yaml`:

```yaml
version: "2026-02-26"
groups:
  - id: nifty-it-impact
    parents: [TCS, INFY, HCLTECH, WIPRO, TECHM, LTIM, MPHASIS, COFORGE, OFSS, PERSISTENT]
    children: [OPENAI, GEMINI, ANTHROPIC, DEEPSEEK, BYTEDANCE, XAI, GEOPOLITICS, GOLD_SILVER_COPPER, SEMICONDUCTOR, NVIDIA, ECONOMICS, ENERGY_ELECTRICITY, AI, DRAM, CHIPS, INDIA_US_RELATIONS]
```

**Step 4: Run tests to verify they pass**

Run:
```bash
go test ./tests/unit -run 'TestDefaultConfigDefinesNiftyITImpactGroup|TestLoadConfigIncludesNiftyITByDefault' -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add configs/entities.niftyit.yaml configs/entities.custom.yaml configs/entity_groups.yaml tests/unit/config_loader_test.go
git commit -m "feat: configure nifty-it parent child impact universe"
```

---

### Task 3: Implement impact row expansion (parent required, child optional)

**Files:**
- Modify: `internal/engine/service.go`
- Create: `internal/engine/impact_rows.go`
- Modify: `internal/core/schemas.go`
- Create: `tests/unit/service_parent_child_impact_test.go`

**Step 1: Write failing tests**

Create `tests/unit/service_parent_child_impact_test.go` with:

```go
func TestServiceRunBuildsParentChildCrossProductRows(t *testing.T) {
	// req.Entities selects parent only; service must still augment with configured children.
	// Article mentions INFY + OpenAI + Gemini => expect 2 rows:
	// (INFY, OPENAI), (INFY, GEMINI)
}

func TestServiceRunBuildsParentOnlyRowsWithNAChild(t *testing.T) {
	// Article mentions INFY but no child keywords => expect 1 row with ChildEntity == "N/A".
}

func TestServiceRunSkipsArticlesWithoutParentMatchInImpactMode(t *testing.T) {
	// Article mentions OpenAI only => expect 0 feature rows when parent required.
}
```

Use `cfg := config.Load(...)`, override providers to stub openai:

```go
cfg.Providers = config.ProvidersFile{
	Defaults:      config.ProviderDefaults{PromptVersion: "v1"},
	Providers:     []config.Provider{{Name: "openai", Model: "gpt-4o-mini", Enabled: true, PricePer1KInput: 0.1, PricePer1KOutput: 0.1}},
	FallbackChain: []string{"openai:gpt-4o-mini"},
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
go test ./tests/unit -run 'TestServiceRunBuildsParentChildCrossProductRows|TestServiceRunBuildsParentOnlyRowsWithNAChild|TestServiceRunSkipsArticlesWithoutParentMatchInImpactMode' -count=1
```

Expected: FAIL (legacy `buildFeatureRows` has no parent/child matrix).

**Step 3: Write minimal implementation**

1) Extend `core.FeatureRow` in `internal/core/schemas.go` with export-facing fields:

```go
NewsSource      string  `json:"news_source"`
URL             string  `json:"url"`
ParentEntity    string  `json:"parent_entity"`
ChildEntity     string  `json:"child_entity"`
SentimentDisplay string `json:"sentiment_display"`
Weight          float64 `json:"weight"`
ConfidenceScore float64 `json:"confidence_score"`
Summary         string  `json:"summary"`
```

2) In `internal/engine/service.go`, detect impact mode and augment match universe:
- impact mode active when `nifty-it-impact` group exists and request includes at least one parent symbol
- augment selected entities with group children so child matching works even when user requested only parents

3) Create `internal/engine/impact_rows.go` with helpers:

```go
func splitParentChildMatches(matches []core.EntityMatch, group config.EntityGroup, roles map[string]string) (parents []core.EntityMatch, children []core.EntityMatch)
func buildParentChildPairs(parents, children []core.EntityMatch) []pair
```

Rules:
- no parent => no rows
- parent + no child => one row/parent with `ChildEntity = "N/A"`
- parent + children => Cartesian product rows

**Step 4: Run tests to verify they pass**

Run:
```bash
go test ./tests/unit -run 'TestServiceRunBuildsParentChildCrossProductRows|TestServiceRunBuildsParentOnlyRowsWithNAChild|TestServiceRunSkipsArticlesWithoutParentMatchInImpactMode|TestServiceRunFiltersEntitiesToRequestedSet' -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/core/schemas.go internal/engine/service.go internal/engine/impact_rows.go tests/unit/service_parent_child_impact_test.go
git commit -m "feat: expand feature rows into parent child impact pairs"
```

---

### Task 4: Add per-pair sentiment analysis + row-level scoring

**Files:**
- Modify: `internal/enrich/enricher.go`
- Modify: `internal/engine/impact_rows.go`
- Modify: `internal/engine/service.go`
- Create: `tests/unit/enricher_pair_sentiment_test.go`
- Modify: `tests/unit/service_parent_child_impact_test.go`

**Step 1: Write failing tests**

Create `tests/unit/enricher_pair_sentiment_test.go`:

```go
func TestEnricherClassifyPairSentimentUsesRouter(t *testing.T) {
	// use mimo test server returning {"label":"positive","score":0.61}
	// call ClassifyPairSentiment(... parent INFY, child OPENAI)
	// expect Label=positive Score=0.61 and token accounting > 0
}
```

Add assertion in `tests/unit/service_parent_child_impact_test.go`:

```go
if !strings.Contains(row.SentimentDisplay, "(") {
	t.Fatalf("expected sentiment display format label (score), got %q", row.SentimentDisplay)
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
go test ./tests/unit -run 'TestEnricherClassifyPairSentimentUsesRouter|TestServiceRunBuildsParentChildCrossProductRows' -count=1
```

Expected: FAIL (`ClassifyPairSentiment` missing / sentiment display not populated).

**Step 3: Write minimal implementation**

In `internal/enrich/enricher.go` add:

```go
type PairSentimentResult struct {
	Label        string
	Score        float64
	InputTokens  int
	OutputTokens int
	EstimatedCost float64
}

func (e *Enricher) ClassifyPairSentiment(ctx context.Context, article core.Article, pair []config.Entity) (PairSentimentResult, error) {
	text := strings.TrimSpace(strings.Join([]string{article.Title, article.Summary, article.Body}, " "))
	if e.router == nil {
		label, score := deterministicSentiment(text)
		return PairSentimentResult{Label: label, Score: score}, nil
	}
	entityCtx := make([]providers.EntityContext, 0, len(pair))
	for _, ent := range pair {
		entityCtx = append(entityCtx, providers.EntityContext{Symbol: ent.Symbol, Name: ent.Name, Aliases: append([]string{}, ent.Aliases...)})
	}
	routed, err := e.router.Enrich(ctx, providers.ClassificationRequest{Text: text, Entities: entityCtx})
	if err != nil {
		return PairSentimentResult{}, err
	}
	return PairSentimentResult{
		Label: routed.Sentiment.Label,
		Score: routed.Sentiment.Score,
		InputTokens: routed.InputTokens,
		OutputTokens: routed.OutputTokens,
		EstimatedCost: routed.EstimatedCost,
	}, nil
}
```

In `internal/engine/impact_rows.go` (or service helper):
- call `ClassifyPairSentiment` per generated pair row
- set `SentimentDisplay = fmt.Sprintf("%s (%.2f)", label, score)`
- `Weight`: normalized parent confidence (sum of parent confidences per article)
- `ConfidenceScore`: `(parentConfidence + childConfidence)/2`, or parent confidence if child is `N/A`
- row token/cost allocation:
  - `baseShare = eventMeta / pairCount`
  - `row = baseShare + pairCallUsage`

**Step 4: Run tests to verify they pass**

Run:
```bash
go test ./tests/unit -run 'TestEnricherClassifyPairSentimentUsesRouter|TestServiceRunBuildsParentChildCrossProductRows|TestServiceRunBuildsParentOnlyRowsWithNAChild' -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/enrich/enricher.go internal/engine/impact_rows.go internal/engine/service.go tests/unit/enricher_pair_sentiment_test.go tests/unit/service_parent_child_impact_test.go
git commit -m "feat: add per pair sentiment scoring for impact rows"
```

---

### Task 5: Replace CSV export with required 10-column business format

**Files:**
- Modify: `internal/export/exporter.go`
- Modify: `tests/unit/export_eval_test.go`
- Modify: `tests/unit/engine_export_test.go`
- Modify: `tests/integration/service_file_output_test.go`

**Step 1: Write failing tests**

In `tests/unit/export_eval_test.go`, update CSV expectations:

```go
for _, header := range []string{"Index", "News Source", "URL", "Parent entity", "Child Entity", "Sentiment", "Weight", "Confidence Score", "Cost", "Summary"} {
	if !strings.Contains(csvStr, header) {
		t.Fatalf("csv missing expected header %s", header)
	}
}
if strings.Contains(csvStr, "run_id") {
	t.Fatalf("business csv should not expose run_id column")
}
```

Add summary truncation test:

```go
func TestExporterTruncatesSummaryToTenWords(t *testing.T) {
	// summary has >10 words
	// expect exported summary cell to contain only first 10 words
}
```

In `tests/integration/service_file_output_test.go`, replace header assertion:

```go
if !strings.Contains(csvText, "Index,News Source,URL,Parent entity,Child Entity,Sentiment,Weight,Confidence Score,Cost,Summary") {
	t.Fatalf("csv export missing expected business header")
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
go test ./tests/unit ./tests/integration -run 'TestExporterOutputsJSONLCSVTOON|TestExporterTruncatesSummaryToTenWords|TestServiceRunWritesFileOutputsWithMockData|TestExportCSVReadsFeatureRepository' -count=1
```

Expected: FAIL (exporter still outputs legacy schema).

**Step 3: Write minimal implementation**

In `internal/export/exporter.go`, replace CSV headers and row mapping:

```go
headers := []string{"Index", "News Source", "URL", "Parent entity", "Child Entity", "Sentiment", "Weight", "Confidence Score", "Cost", "Summary"}
```

Write each row as:

```go
rec := []string{
	strconv.Itoa(idx + 1),
	row.NewsSource,
	row.URL,
	row.ParentEntity,
	row.ChildEntity,
	row.SentimentDisplay,
	fmt.Sprintf("%.6f", row.Weight),
	fmt.Sprintf("%.6f", row.ConfidenceScore),
	fmt.Sprintf("%.6f", row.EstimatedCostUS),
	truncateWords(row.Summary, 10),
}
```

Add helper:

```go
func truncateWords(text string, max int) string {
	words := strings.Fields(strings.TrimSpace(text))
	if len(words) <= max {
		return strings.Join(words, " ")
	}
	return strings.Join(words[:max], " ")
}
```

**Step 4: Run tests to verify they pass**

Run:
```bash
go test ./tests/unit ./tests/integration -run 'TestExporterOutputsJSONLCSVTOON|TestExporterTruncatesSummaryToTenWords|TestServiceRunWritesFileOutputsWithMockData|TestExportCSVReadsFeatureRepository' -count=1
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/export/exporter.go tests/unit/export_eval_test.go tests/unit/engine_export_test.go tests/integration/service_file_output_test.go
git commit -m "feat: export parent child impact csv format"
```

---

### Task 6: Regression sweep for service behavior + compatibility

**Files:**
- Verify only; fix only if failing tests identify regressions

**Step 1: Run focused service/config/enrich tests**

Run:
```bash
go test ./tests/unit -run 'TestServiceRun|TestLoadConfig|TestEnricher|TestExporter|TestCollectArticles' -count=1
```

Expected: PASS.

**Step 2: Run integration tests for pipeline outputs**

Run:
```bash
go test ./tests/integration -count=1
```

Expected: PASS.

**Step 3: Run full repository tests**

Run:
```bash
go test ./... -count=1
```

Expected: PASS.

**Step 4: Final verification checklist (@superpowers:verification-before-completion)**

- Confirm parent-required logic in impact mode skips child-only articles
- Confirm parent-only articles emit `Child Entity = N/A`
- Confirm multi-parent + multi-child article emits Cartesian product rows
- Confirm sentiment format is `label (score)`
- Confirm summary column never exceeds 10 words

**Step 5: Commit only if final fixups were required**

```bash
git status
# if required:
# git add <fixup files>
# git commit -m "test: finalize parent child impact regression fixes"
```

---

## Execution Notes

- Use **@superpowers:test-driven-development** discipline in every task (red -> green -> refactor).
- Keep changes DRY/YAGNI: avoid introducing new APIs or UI surface area.
- Keep commits task-scoped and small.
- Preserve internal provenance models even though the CSV is business-oriented.
