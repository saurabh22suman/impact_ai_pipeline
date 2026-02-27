# NiftyIT Alias + LLM-First Enrichment Design

## Context
The requested run input (`entities=["NiftyIT","Infy"]`) was failing because entity selection used strict normalized matching and the configured NIFTIT aliases did not include the compact token `NiftyIT`.

Pipeline behavior was also rule-gated before enrichment, which prevented broad LLM execution. The requirement is to trigger LLM on all collected articles and keep rule-based scoring as secondary/fallback behavior.

## Approved Design

### 1) Alias fix
- Add `"NiftyIT"` alias to `nse-index-niftit` in `configs/entities.custom.yaml`.
- Preserve existing aliases: `"NIFTIT"`, `"Nifty IT"`.

### 2) LLM-first enrichment flow
- Remove pre-enrichment relevance hard filter from `internal/engine/service.go`.
- Enrich every deduped article via `Enricher.EnrichArticle`.
- Keep relevance score and factor/entity scoring in event metadata for analytics.

### 3) Reduce rule dominance
- In `internal/enrich/enricher.go`, call provider router whenever available (not only on ambiguity window).
- Keep deterministic outputs as fallback if provider call fails.
- Keep `provider=model=rules` fallback behavior only for provider failure paths.

### 4) Provider priority for execution
- Update `configs/providers.yaml` fallback chain to prioritize non-MIMO stubs in this environment:
  - `openai:gpt-4o-mini`
  - `deepseek:deepseek-chat`
  - `anthropic:claude-sonnet-4.6`
  - `mimo:mimo-v2-flash`
  - `mimo:mimo-v2-synthetic`

## Tests added/updated (TDD)

### New tests
- `tests/unit/service_entity_selection_test.go`
  - `TestServiceRunAcceptsNiftyITAliasWithoutSpace`
- `tests/unit/enricher_behavior_test.go`
  - `TestEnricherAlwaysUsesRouterWhenAvailable`

### Updated tests
- `tests/integration/service_pipeline_test.go`
  - Renamed and changed expectation:
  - `TestServiceRunReturnsCompletedRunWithEventsForLowSignalArticles`
  - Low-signal article must still emit event (no prefilter drop)

## Verification evidence

### Automated tests
- `go test ./tests/unit -run 'TestEnricherAlwaysUsesRouterWhenAvailable|TestServiceRunAcceptsNiftyITAliasWithoutSpace' -count=1` → PASS
- `go test ./tests/integration -run 'TestServiceRunReturnsCompletedRunWithEventsForLowSignalArticles|TestServiceRunProducesProvenancedOutputs' -count=1` → PASS
- `go test ./tests/unit ./tests/integration -count=1` → PASS
- `go test ./... -count=1` → PASS

### Runtime checks
- Restarted engine container to load config/code.
- Verified `/api/v1/config` exposes NIFTIT aliases including `NiftyIT`.
- Re-ran source checks (2026-01-01 to now) with entities `["NiftyIT","Infy"]`.

## Post-change run findings (source-by-source)
- All sources now accept the requested entity pair (no 400 alias error).
- LLM provider metadata appears on emitted events (`openai:gpt-4o-mini`, with fallback to `deepseek:deepseek-chat` in some runs).
- `events_output` now equals collected article count for non-empty sources due to removal of prefilter gating.
- `economic-times-markets` produced INFY/NIFTIT entity hits.
- `moneycontrol-markets` and `zerodha-pulse` still returned zero collected articles in this window, so zero events.

## Changed files
- `configs/entities.custom.yaml`
- `configs/providers.yaml`
- `internal/enrich/enricher.go`
- `internal/engine/service.go`
- `tests/integration/service_pipeline_test.go`
- `tests/unit/enricher_behavior_test.go` (new)
- `tests/unit/service_entity_selection_test.go` (new)
