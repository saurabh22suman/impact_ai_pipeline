package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	store := &PostgresStore{db: db}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *PostgresStore) migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS runs (
			run_id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			started_at TIMESTAMPTZ NOT NULL,
			finished_at TIMESTAMPTZ NOT NULL,
			config_version TEXT NOT NULL,
			profile TEXT NOT NULL,
			input_tokens BIGINT NOT NULL,
			output_tokens BIGINT NOT NULL,
			estimated_cost DOUBLE PRECISION NOT NULL,
			failure_reason TEXT NOT NULL DEFAULT '',
			artifact_counts_json JSONB NOT NULL,
			result_json JSONB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS run_events (
			run_id TEXT NOT NULL,
			idx INT NOT NULL,
			event_json JSONB NOT NULL,
			PRIMARY KEY (run_id, idx)
		)`,
		`CREATE TABLE IF NOT EXISTS run_features (
			run_id TEXT NOT NULL,
			idx INT NOT NULL,
			feature_json JSONB NOT NULL,
			PRIMARY KEY (run_id, idx)
		)`,
		`CREATE TABLE IF NOT EXISTS artifacts (
			artifact_key TEXT PRIMARY KEY,
			payload BYTEA NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	}

	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate postgres: %w", err)
		}
	}
	return nil
}

func (s *PostgresStore) SaveRun(ctx context.Context, result core.RunResult) error {
	artifactCounts, err := json.Marshal(result.ArtifactCounts)
	if err != nil {
		return err
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}

	query := `
	INSERT INTO runs (
		run_id, status, created_at, started_at, finished_at,
		config_version, profile, input_tokens, output_tokens,
		estimated_cost, failure_reason, artifact_counts_json, result_json
	) VALUES (
		$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13
	)
	ON CONFLICT (run_id) DO UPDATE SET
		status = EXCLUDED.status,
		created_at = EXCLUDED.created_at,
		started_at = EXCLUDED.started_at,
		finished_at = EXCLUDED.finished_at,
		config_version = EXCLUDED.config_version,
		profile = EXCLUDED.profile,
		input_tokens = EXCLUDED.input_tokens,
		output_tokens = EXCLUDED.output_tokens,
		estimated_cost = EXCLUDED.estimated_cost,
		failure_reason = EXCLUDED.failure_reason,
		artifact_counts_json = EXCLUDED.artifact_counts_json,
		result_json = EXCLUDED.result_json
	`
	_, err = s.db.ExecContext(ctx, query,
		result.RunID,
		string(result.Status),
		result.CreatedAt,
		result.StartedAt,
		result.FinishedAt,
		result.ConfigVersion,
		result.Profile,
		result.InputTokens,
		result.OutputTokens,
		result.EstimatedCost,
		result.FailureReason,
		artifactCounts,
		resultJSON,
	)
	return err
}

func (s *PostgresStore) GetRun(ctx context.Context, runID string) (core.RunResult, bool) {
	row := s.db.QueryRowContext(ctx, `SELECT result_json FROM runs WHERE run_id = $1`, runID)
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if err == sql.ErrNoRows {
			return core.RunResult{}, false
		}
		return core.RunResult{}, false
	}
	var result core.RunResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return core.RunResult{}, false
	}
	return result, true
}

func (s *PostgresStore) ListRuns(ctx context.Context) []core.RunResult {
	rows, err := s.db.QueryContext(ctx, `SELECT result_json FROM runs ORDER BY created_at DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := make([]core.RunResult, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			continue
		}
		var result core.RunResult
		if err := json.Unmarshal(payload, &result); err != nil {
			continue
		}
		out = append(out, result)
	}
	return out
}

func (s *PostgresStore) SaveEvents(ctx context.Context, runID string, events []core.MarketAlignedEvent) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM run_events WHERE run_id = $1`, runID); err != nil {
		return err
	}

	for idx, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO run_events (run_id, idx, event_json) VALUES ($1, $2, $3)`,
			runID, idx, payload,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *PostgresStore) GetEvents(ctx context.Context, runID string) []core.MarketAlignedEvent {
	rows, err := s.db.QueryContext(ctx, `SELECT event_json FROM run_events WHERE run_id = $1 ORDER BY idx`, runID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := make([]core.MarketAlignedEvent, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			continue
		}
		var event core.MarketAlignedEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			continue
		}
		out = append(out, event)
	}
	return out
}

func (s *PostgresStore) SaveFeatureRows(ctx context.Context, runID string, rowsData []core.FeatureRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM run_features WHERE run_id = $1`, runID); err != nil {
		return err
	}

	for idx, row := range rowsData {
		payload, err := json.Marshal(row)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO run_features (run_id, idx, feature_json) VALUES ($1, $2, $3)`,
			runID, idx, payload,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *PostgresStore) GetFeatureRows(ctx context.Context, runID string) []core.FeatureRow {
	rows, err := s.db.QueryContext(ctx, `SELECT feature_json FROM run_features WHERE run_id = $1 ORDER BY idx`, runID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := make([]core.FeatureRow, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			continue
		}
		var row core.FeatureRow
		if err := json.Unmarshal(payload, &row); err != nil {
			continue
		}
		out = append(out, row)
	}
	return out
}

func (s *PostgresStore) Put(ctx context.Context, key string, payload []byte) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO artifacts (artifact_key, payload, updated_at) VALUES ($1,$2,NOW())
		 ON CONFLICT (artifact_key) DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`,
		key, payload,
	)
	return err
}

func (s *PostgresStore) Get(ctx context.Context, key string) ([]byte, bool) {
	row := s.db.QueryRowContext(ctx, `SELECT payload FROM artifacts WHERE artifact_key = $1`, key)
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if err == sql.ErrNoRows {
			return nil, false
		}
		return nil, false
	}
	return payload, true
}

func (s *PostgresStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
