# AI Impact Scrapper

Lightweight internal stack for running the pipeline on a VPS with:
- a static frontend for operators,
- one API/engine runtime as the source of truth,
- a scheduler that triggers runs through the API,
- file-backed persistent outputs (no database required).

## Architecture

Docker Compose runs 3 services:

1. **frontend** (nginx)
   - Serves `frontend/` static UI.
   - Reverse proxies `/api/*` to the engine API.

2. **engine** (Go API)
   - Runs `cmd/api`.
   - Accepts run requests and keeps in-memory run index for the running process.
   - Persists run artifacts to mounted volume path `/data/outputs`.

3. **scheduler** (Go scheduler)
   - Runs `cmd/scheduler`.
   - Uses `SCHEDULER_TIMEZONE` + `SCHEDULER_DAILY_TIME`.
   - Calls `POST /v1/runs` on engine (`SCHEDULER_API_BASE_URL`) instead of executing pipeline in-process.

Persistent data is stored in Docker volume **`engine_outputs`**, mounted at `/data/outputs` in the engine container.

## Quick start

1. Copy env file:

```bash
cp .env.example .env
```

2. Start services:

```bash
docker compose up -d --build
```

3. Open UI:

- `http://<server-ip>:${FRONTEND_PORT}`
- default from `.env.example`: `http://<server-ip>:18080`

4. Verify health through frontend proxy:

```bash
curl http://localhost:${FRONTEND_PORT}/api/health
```

## Output layout (file mode)

With `STORAGE_MODE=file` and `FILE_STORE_DIR=/data/outputs`, run files are written under:

```text
/data/outputs/
  runs/
    <run-id>/
      run.json
      request.json
      events.json
      feature_rows.json
      exports/
        events.jsonl
        features.csv
        events.toon.jsonl
```

## Scheduling in IST

Defaults in `.env.example`:

- `SCHEDULER_TIMEZONE=Asia/Kolkata`
- `SCHEDULER_DAILY_TIME=09:15`
- `SCHEDULER_API_BASE_URL=http://engine:8080`

To change schedule:
1. edit `.env`,
2. restart scheduler:

```bash
docker compose up -d scheduler
```

## API endpoints used by the UI

- `GET /api/health` -> engine `GET /health`
- `GET /api/v1/config`
- `GET /api/v1/sources`
- `POST /api/v1/sources`
- `GET /api/v1/bhavcopy/download?date=YYYY-MM-DD`
- `GET /api/v1/runs`
- `POST /api/v1/runs`
- `GET /api/v1/runs/{id}`
- `GET /api/v1/runs/{id}/export?format=jsonl|csv|toon`

### `/api/v1/config` additions

`GET /api/v1/config` now includes:
- `entities_effective` (count, backward compatible)
- `entities` (array of enabled effective entities), each with:
  - `id`, `symbol`, `name`, `aliases`, `exchange`, `sector`, `type`

### Source management API

`POST /api/v1/sources` request:

```json
{
  "id": "moneycontrol-tech",
  "name": "Moneycontrol Technology",
  "kind": "rss",
  "url": "https://example.com/feed.xml",
  "region": "india",
  "language": "en",
  "enabled": true,
  "crawl_fallback": true
}
```

Behavior:
- validates required fields and URL shape,
- validates `kind` (`rss` or `direct`),
- persists source into `configs/sources.yaml` atomically,
- reloads runtime config so the source appears immediately.

### Bhavcopy download endpoint

`GET /api/v1/bhavcopy/download?date=YYYY-MM-DD`
- Downloads NSE CM bhavcopy zip for the provided trade date.
- Enforces T+2 availability window (returns 400 if requested too early).

## NSE universe sync from bhavcopy

Use sync command:

```bash
go run ./cmd/entities-sync --date 2026-02-20 --config-dir ./configs
```

Behavior:
- fetches bhavcopy archive for the date,
- parses + normalizes equity rows,
- upserts into `configs/entities.custom.yaml` by symbol,
- preserves existing aliases where possible,
- ensures index entities `NIFTIT` and `NIFTBANK` are present,
- enforces T+2 availability rule before fetching.

## UI updates

Frontend now includes:
- source add form,
- searchable multi-selects for **sources** and **entities**,
- select-all / clear-all actions for each selector,
- fixed-height scrollable option lists,
- bhavcopy download form with trade-date picker and T+2 validation messaging.

## Entity selection semantics

Run behavior:
- empty `entities` list => all entities,
- explicit `entities` list with unknown symbol/name/alias => `400 Bad Request` (no fallback to all).

## VPS deployment notes

- Expose only `FRONTEND_PORT` publicly.
- Keep engine/scheduler internal to Docker network.
- Services use `restart: unless-stopped`.
- Back up the `engine_outputs` volume regularly.

Example backup command:

```bash
docker run --rm -v engine_outputs:/src -v "$PWD":/backup alpine \
  tar czf /backup/engine_outputs_backup.tgz -C /src .
```

## Troubleshooting

### Engine health

```bash
docker compose logs engine --tail=200
curl http://localhost:${FRONTEND_PORT}/api/health
```

### Scheduler not creating runs

```bash
docker compose logs scheduler --tail=200
```

Look for:
- `scheduler next_run=...`
- `scheduler run triggered run_id=...`

### Missing output files

1. Check run exists in UI table.
2. Inspect engine volume contents:

```bash
docker run --rm -v engine_outputs:/data alpine ls -R /data/runs
```

3. Check engine logs for write/export errors:

```bash
docker compose logs engine --tail=200
```

## Testing

```bash
go test ./cmd/api -v
go test ./tests/unit -v
go test ./...
go test ./... -race
go test ./cmd/scheduler -run FuzzParseDailyTime -fuzz=FuzzParseDailyTime -fuzztime=2s
```
