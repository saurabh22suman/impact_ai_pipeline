package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"

	_ "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

type ClickHouseStore struct {
	db *sql.DB
}

func NewClickHouseStore(ctx context.Context, host, port, database, username, password string) (*ClickHouseStore, error) {
	if host == "" {
		host = "clickhouse"
	}
	if port == "" {
		port = "8123"
	}
	if database == "" {
		database = "default"
	}

	dsn := fmt.Sprintf("clickhouse://%s:%s/%s", host, port, database)
	if username != "" {
		q := make(url.Values)
		q.Set("username", username)
		q.Set("password", password)
		dsn += "?" + q.Encode()
	}

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, fmt.Errorf("open clickhouse: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping clickhouse: %w", err)
	}

	store := &ClickHouseStore{db: db}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *ClickHouseStore) migrate(ctx context.Context) error {
	stmt := `
	CREATE TABLE IF NOT EXISTS run_features (
		run_id String,
		idx UInt32,
		feature_json String
	)
	ENGINE = MergeTree
	ORDER BY (run_id, idx)
	`
	if _, err := s.db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("migrate clickhouse: %w", err)
	}
	return nil
}

func (s *ClickHouseStore) SaveFeatureRows(ctx context.Context, runID string, rows []core.FeatureRow) error {
	if len(rows) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO run_features (run_id, idx, feature_json) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for idx, row := range rows {
		payload, err := json.Marshal(row)
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, runID, idx, string(payload)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *ClickHouseStore) GetFeatureRows(ctx context.Context, runID string) []core.FeatureRow {
	rows, err := s.db.QueryContext(ctx, `SELECT feature_json FROM run_features WHERE run_id = ? ORDER BY idx`, runID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	out := make([]core.FeatureRow, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			continue
		}
		var row core.FeatureRow
		if err := json.Unmarshal([]byte(payload), &row); err != nil {
			continue
		}
		out = append(out, row)
	}
	return out
}

func (s *ClickHouseStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
