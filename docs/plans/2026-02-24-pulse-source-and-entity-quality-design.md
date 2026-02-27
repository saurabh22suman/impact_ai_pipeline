# Pulse Source + Entity Quality Design

Date: 2026-02-24
Status: Approved

## Context

The pipeline should:

1. Remove `config_version` from user-facing outputs (UI and TOON export) while preserving internal traceability.
2. Add `https://pulse.zerodha.com/` as a built-in source.
3. Use a dedicated Pulse ingestion path that crawls publisher pages for full article content when Pulse is selected.
4. Improve entity coverage and mapping quality (balanced recall + precision).
5. Allow browser fallback for JS-heavy article pages only when HTTP parsing is insufficient.

## Goals

- Keep internal auditability (`config_version`) for storage and provenance internals.
- Improve source coverage by integrating Pulse listing + publisher crawl.
- Reduce false negatives and false positives in entity detection.
- Keep existing RSS/direct ingestion behavior stable.

## Non-Goals

- Removing `config_version` from internal DB schema/models.
- Rewriting the entire enrichment/provider pipeline.
- Replacing readability extraction globally.

## Current-State Findings

- UI displays config version via `frontend/index.html` and `frontend/app.js` (`config.config_version`).
- CSV exports already exclude `config_version` in current headers.
- TOON export includes `config_version` field.
- Direct crawler currently restricts links to same host (`internal/ingest/direct.go`), which prevents Pulse → publisher crawling.
- Entity matching currently relies on naive substring checks in mapper and relevance gate.

## Selected Approach

### 1) User-facing `config_version` removal (internal retention)

- Remove config version card/value from frontend summary UI.
- Remove `config_version` from TOON export payload.
- Keep internal fields intact in run metadata/storage/models.

### 2) Dedicated Pulse ingestion path

Add a new source kind: `pulse`.

Flow for pulse source:

1. Fetch Pulse listing page.
2. Extract outbound publisher article URLs and listing metadata.
3. Filter links:
   - Allow only `http/https`.
   - Exclude social/share/tracker URLs.
   - Deduplicate.
   - Cap per-source page count.
4. Fetch publisher pages over HTTP.
5. Extract title/body/summary using readability.
6. If extraction quality is insufficient, attempt browser fallback.
7. Build `core.Article` and continue through existing dedupe/relevance/enrichment flow.

### 3) Browser fallback (selective)

- HTTP-first always.
- Browser rendering invoked only for failed/low-content extractions.
- Keep fallback bounded by limits (timeouts/page cap).

### 4) Entity matching precision+recall upgrade

Unify matching logic with a normalized matching utility:

- Case/punctuation/whitespace normalization.
- Boundary-aware matching for names/symbols/aliases.
- Confidence tiers by match type.
- Ambiguity-aware behavior (downweight weak ambiguous alias-only matches).

Use this utility in both:

- `EntityMapper.Map`
- `RelevanceGate.ScoreBreakdown`

to avoid inconsistency between detection and relevance scoring.

## Architecture Changes

### Config and source admin

- Add `SourceKindPulse` constant.
- Allow `pulse` in source creation validation.
- Add Pulse source in `configs/sources.yaml`.

### Ingest routing

- Extend ingest router to dispatch pulse kind to Pulse fetcher.
- Preserve existing behavior for rss/direct kinds.

### New pulse fetcher component

Responsibilities:

- Listing discovery from pulse homepage.
- Outbound publisher URL extraction and sanitization.
- Publisher article fetching + extraction.
- Selective browser fallback.

### Frontend/UI

- Remove config version display card and associated value assignment.

### Export

- Remove TOON `config_version` field from schema and row mapping.

## Safeguards

- Max pages/articles per source.
- Max response size limit.
- Request timeouts through context.
- Skip-on-error for individual publisher links.
- Per-source notice logging on listing failure/high skip rates/fallback usage.

## Data Contracts

- Internal run metadata remains unchanged.
- Run/API contracts should remain backward compatible unless explicitly changed in frontend.
- TOON output contract changes by removing `config_version` user-facing field.

## Testing Strategy

### Unit tests

1. `pulse` kind normalization/validation and routing.
2. Pulse listing URL extraction and filtering.
3. Pulse publisher extraction + fallback invocation behavior.
4. Entity matching normalization + boundary tests.
5. Relevance gate entity scoring alignment with matcher utility.
6. TOON export no longer emits `config_version`.

### Integration tests

1. Selected source includes Pulse → pulse path contributes articles/events.
2. Selected source excludes Pulse → pulse fetcher not used.
3. Mixed source runs remain stable.
4. Entity-focused pipeline fixture shows improved balanced recall+precision.

### UI/E2E checks

- Summary loads without config version card/value.
- Export downloads still function for CSV/raw/multi-focus/TOON.

## Acceptance Criteria

1. Pulse exists as built-in source in `configs/sources.yaml`.
2. Selecting Pulse triggers dedicated pulse crawling of publisher content.
3. Non-selected Pulse source does not execute pulse fetching.
4. Browser fallback is available and only used when HTTP extraction is insufficient.
5. Entity recall/precision improve on fixture tests with no major regression.
6. `config_version` removed from UI and TOON export but retained internally.

## Risks and Mitigations

- **Publisher site anti-bot/rate limits**: keep bounded retries and partial-success behavior.
- **Noise from aggregator links**: strict URL filtering and dedupe.
- **Entity false positives**: boundary-aware + ambiguity-aware scoring.
- **Contract drift in TOON**: update export tests to lock expected fields.
