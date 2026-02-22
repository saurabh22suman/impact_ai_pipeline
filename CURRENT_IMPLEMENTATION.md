# Current Implementation Handoff

_Last updated: 2026-02-22_

## 1) Implemented Architecture

### 1.1 Runtime + service wiring
- Runtime bootstrap loads config and initializes storage via env-driven mode selection in `internal/engine/bootstrap.go:24`.
- Storage mode resolution is implemented in `internal/storage/env.go:11`:
  - `STORAGE_MODE=memory` -> in-memory store.
  - default (empty) -> persistent store.

### 1.2 API service (UI/API tier)
- API entrypoint is implemented in `cmd/api/main.go:20`.
- Registered endpoints:
  - `GET /health` (`cmd/api/main.go:49`)
  - `GET /v1/config` (`cmd/api/main.go:57`)
  - `GET|POST /v1/runs` (`cmd/api/main.go:73`)
  - `GET /v1/runs/{id}` (`cmd/api/main.go:126`)
  - `GET /v1/runs/{id}/export?format=jsonl|csv|toon` (`cmd/api/main.go:147`)
- Run execution flow:
  - source resolution -> article collection -> pipeline run (`cmd/api/main.go:102`, `cmd/api/main.go:108`, `cmd/api/main.go:114`).

### 1.3 Worker + scheduler
- Worker periodic pipeline execution implemented in `cmd/worker/main.go:15`.
- Scheduler tick loop scaffold implemented in `cmd/scheduler/main.go:11`.

## 2) Config-driven Foundation

### 2.1 YAML configs loaded and validated
- Config loader: `internal/config/loader.go:13`.
- Schema/types: `internal/config/types.go:10`.
- Validation includes:
  - enabled sources/providers,
  - default profile existence,
  - fallback chain consistency (`internal/config/loader.go:85`).

### 2.2 Config files present
- `configs/sources.yaml`
- `configs/entities.niftyit.yaml`
- `configs/entities.custom.yaml`
- `configs/factors.yaml`
- `configs/providers.yaml`
- `configs/pipelines.yaml`

## 3) Engine Pipeline

### 3.1 Orchestration and output generation
- Main pipeline service implemented in `internal/engine/service.go:18`.
- Steps implemented:
  1. Dedupe (`internal/engine/service.go:63`, `internal/ingest/dedupe.go:15`)
  2. Relevance scoring/gating (`internal/engine/service.go:69`, `internal/ingest/relevance.go:16`)
  3. Enrichment (`internal/engine/service.go:74`, `internal/enrich/enricher.go:27`)
  4. Market session alignment (`internal/engine/service.go:79`, `internal/market/session.go:11`)
  5. Feature row generation (`internal/engine/service.go:86`, `internal/engine/service.go:186`)
  6. Persist run/events/features (`internal/engine/service.go:113`)

### 3.2 Ingestion
- RSS fetcher implemented in `internal/ingest/rss.go:22`.
- Source collection and date filtering implemented in `internal/engine/collector.go:40`.
- TOON compatibility parser implemented in `internal/ingest/toon.go:27`.

### 3.3 Enrichment and provider routing
- Entity mapping implemented in `internal/enrich/entity_mapper.go:17`.
- Deterministic sentiment/factor tagging + optional LLM routing implemented in `internal/enrich/enricher.go:27`.
- Provider router with fallback chain, budget checks, and circuit breaker implemented in `internal/enrich/router.go:23`.
- Provider adapters currently route to stub behavior:
  - `internal/enrich/providers/anthropic.go:3`
  - `internal/enrich/providers/openai.go:3`
  - `internal/enrich/providers/gemini.go:3`
  - `internal/enrich/providers/deepseek.go:3`
  - `internal/enrich/providers/mimo.go:3`
  - core stub logic in `internal/enrich/providers/stub_provider.go:8`.

## 4) Export + Evaluation

### 4.1 Export formats
- Exporter implemented in `internal/export/exporter.go:14`.
- Supported formats:
  - JSONL (`internal/export/exporter.go:20`)
  - CSV (`internal/export/exporter.go:31`)
  - TOON-compatible line-delimited JSON (`internal/export/exporter.go:70`)

### 4.2 Eval utility
- Purged walk-forward fold builder implemented in `internal/eval/eval.go:17`.

## 5) Storage Implementation (persistent storage done)

### 5.1 Storage interfaces + memory mode
- Store interfaces (`RunRepository`, `EventRepository`, `FeatureRepository`, `ArtifactStore`, `EngineStore`) in `internal/storage/storage.go:11`.
- In-memory store with runs/events/features/artifacts in `internal/storage/storage.go:39`.

### 5.2 Postgres store
- Postgres client + migrations in `internal/storage/postgres.go:18` and `internal/storage/postgres.go:40`.
- Tables created:
  - `runs`
  - `run_events`
  - `run_features`
  - `artifacts` (`internal/storage/postgres.go:42`, `internal/storage/postgres.go:57`, `internal/storage/postgres.go:63`, `internal/storage/postgres.go:69`)

### 5.3 ClickHouse store
- ClickHouse client and migration in `internal/storage/clickhouse.go:18`.
- Feature table: `run_features` MergeTree (`internal/storage/clickhouse.go:56`).

### 5.4 MinIO store
- Bucket init + object put/get implemented in `internal/storage/minio.go:19`.

### 5.5 Persistent composite store
- Persistent composition in `internal/storage/persistent.go:11`:
  - runs/events -> Postgres
  - feature writes -> Postgres + ClickHouse (`internal/storage/persistent.go:64`)
  - feature reads -> ClickHouse first, Postgres fallback (`internal/storage/persistent.go:71`)
  - artifacts -> MinIO (or Postgres fallback if artifact store nil) (`internal/storage/persistent.go:79`)

## 6) Infra and Runtime Config

### 6.1 Docker compose
- Services configured: `api`, `worker`, `scheduler`, `postgres`, `clickhouse`, `minio` in `docker-compose.yml:1`.
- Go runtime image set to `golang:1.25` for app services (`docker-compose.yml:3`, `docker-compose.yml:18`, `docker-compose.yml:33`).

### 6.2 Environment defaults
- `.env.example` includes Postgres/ClickHouse/MinIO/app ports (`.env.example:1`).
- ClickHouse env default port is currently `8123` (`.env.example:8`).

## 7) Tests Implemented

- Unit tests:
  - `tests/unit/config_loader_test.go`
  - `tests/unit/ingest_relevance_test.go`
  - `tests/unit/enrich_router_test.go`
  - `tests/unit/export_eval_test.go`
  - `tests/unit/engine_export_test.go` (CSV reads from feature repository)
- Integration tests:
  - `tests/integration/service_pipeline_test.go`
  - `tests/integration/storage_memory_test.go`
- E2E smoke tests:
  - `tests/e2e/api_smoke_test.go`

Current local test status:
- `go test ./...` passes.

## 8) Claude Governance Setup

- Settings and hooks configured in `.claude/settings.json:1`.
- Hook scripts:
  - `.claude/hooks/block-destructive.sh`
  - `.claude/hooks/enforce-go-quality-gates.sh`
- Skills and agents scaffolded under:
  - `.claude/skills/*`
  - `.claude/agents/*`

## 9) Current Runtime Blocker (important for next agent)

API and worker are failing during bootstrap in persistent mode due ClickHouse handshake mismatch:
- Error observed in container logs:
  - `ping clickhouse: [handshake] unexpected packet [72] from server`
- Where this occurs:
  - ClickHouse ping call in `internal/storage/clickhouse.go:41`
- Related runtime wiring:
  - Persistent store initialization in `internal/storage/env.go:33`
- Current compose/env values point ClickHouse to port `8123` (`docker-compose.yml:63`, `.env.example:8`).

This is the primary unresolved issue blocking full persistent runtime verification.

## 10) Immediate Next Steps for Next Agent

1. Fix ClickHouse connection protocol/port mismatch so API/worker bootstrap succeeds.
2. Re-run `docker compose up -d api worker scheduler` and verify API health on `:18080`.
3. Execute one run via `POST /v1/runs` and verify persistence across:
   - Postgres (`runs`, `run_events`, `run_features`)
   - ClickHouse (`run_features`)
   - MinIO artifact path (if raw artifact writes are triggered).
4. Add an integration test for persistent mode against dockerized services (currently only memory-mode storage integration test exists).

---

This document reflects implementation completed so far and the current blocker for seamless handoff.
