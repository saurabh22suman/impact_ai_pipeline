package storage

import (
	"context"
	"fmt"
	"sort"

	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

type PersistentStore struct {
	postgres   *PostgresStore
	clickhouse *ClickHouseStore
	artifacts  ArtifactStore
}

func NewPersistentStore(postgres *PostgresStore, clickhouse *ClickHouseStore, artifacts ArtifactStore) (*PersistentStore, error) {
	if postgres == nil {
		return nil, fmt.Errorf("postgres store is required")
	}
	if clickhouse == nil {
		return nil, fmt.Errorf("clickhouse store is required")
	}
	if artifacts == nil {
		artifacts = postgres
	}
	return &PersistentStore{
		postgres:   postgres,
		clickhouse: clickhouse,
		artifacts:  artifacts,
	}, nil
}

func (s *PersistentStore) SaveRun(ctx context.Context, result core.RunResult) error {
	return s.postgres.SaveRun(ctx, result)
}

func (s *PersistentStore) GetRun(ctx context.Context, runID string) (core.RunResult, bool) {
	result, ok := s.postgres.GetRun(ctx, runID)
	if !ok {
		return core.RunResult{}, false
	}
	result.FeatureRows = s.GetFeatureRows(ctx, runID)
	result.Events = s.GetEvents(ctx, runID)
	return result, true
}

func (s *PersistentStore) ListRuns(ctx context.Context) []core.RunResult {
	runs := s.postgres.ListRuns(ctx)
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].CreatedAt.After(runs[j].CreatedAt)
	})
	return runs
}

func (s *PersistentStore) SaveEvents(ctx context.Context, runID string, events []core.MarketAlignedEvent) error {
	return s.postgres.SaveEvents(ctx, runID, events)
}

func (s *PersistentStore) GetEvents(ctx context.Context, runID string) []core.MarketAlignedEvent {
	return s.postgres.GetEvents(ctx, runID)
}

func (s *PersistentStore) SaveFeatureRows(ctx context.Context, runID string, rows []core.FeatureRow) error {
	if err := s.postgres.SaveFeatureRows(ctx, runID, rows); err != nil {
		return err
	}
	return s.clickhouse.SaveFeatureRows(ctx, runID, rows)
}

func (s *PersistentStore) GetFeatureRows(ctx context.Context, runID string) []core.FeatureRow {
	rows := s.clickhouse.GetFeatureRows(ctx, runID)
	if len(rows) > 0 {
		return rows
	}
	return s.postgres.GetFeatureRows(ctx, runID)
}

func (s *PersistentStore) Put(ctx context.Context, key string, payload []byte) error {
	return s.artifacts.Put(ctx, key, payload)
}

func (s *PersistentStore) Get(ctx context.Context, key string) ([]byte, bool) {
	return s.artifacts.Get(ctx, key)
}

func (s *PersistentStore) Close() error {
	var firstErr error
	if s.clickhouse != nil {
		if err := s.clickhouse.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.postgres != nil {
		if err := s.postgres.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
