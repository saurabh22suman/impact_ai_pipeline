package storage

import (
	"context"
	"sync"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

type RunRepository interface {
	SaveRun(ctx context.Context, result core.RunResult) error
	GetRun(ctx context.Context, runID string) (core.RunResult, bool)
	ListRuns(ctx context.Context) []core.RunResult
}

type EventRepository interface {
	SaveEvents(ctx context.Context, runID string, events []core.MarketAlignedEvent) error
	GetEvents(ctx context.Context, runID string) []core.MarketAlignedEvent
}

type FeatureRepository interface {
	SaveFeatureRows(ctx context.Context, runID string, rows []core.FeatureRow) error
	GetFeatureRows(ctx context.Context, runID string) []core.FeatureRow
}

type ArtifactStore interface {
	Put(ctx context.Context, key string, payload []byte) error
	Get(ctx context.Context, key string) ([]byte, bool)
}

type EngineStore interface {
	RunRepository
	EventRepository
	FeatureRepository
	ArtifactStore
}

type InMemoryStore struct {
	mu        sync.RWMutex
	runs      map[string]core.RunResult
	events    map[string][]core.MarketAlignedEvent
	features  map[string][]core.FeatureRow
	artifacts map[string][]byte
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		runs:      map[string]core.RunResult{},
		events:    map[string][]core.MarketAlignedEvent{},
		features:  map[string][]core.FeatureRow{},
		artifacts: map[string][]byte{},
	}
}

func (s *InMemoryStore) SaveRun(_ context.Context, result core.RunResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if result.CreatedAt.IsZero() {
		result.CreatedAt = time.Now().UTC()
	}
	s.runs[result.RunID] = result
	return nil
}

func (s *InMemoryStore) GetRun(_ context.Context, runID string) (core.RunResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.runs[runID]
	return r, ok
}

func (s *InMemoryStore) ListRuns(_ context.Context) []core.RunResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]core.RunResult, 0, len(s.runs))
	for _, run := range s.runs {
		out = append(out, run)
	}
	return out
}

func (s *InMemoryStore) SaveEvents(_ context.Context, runID string, events []core.MarketAlignedEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events[runID] = append([]core.MarketAlignedEvent{}, events...)
	return nil
}

func (s *InMemoryStore) GetEvents(_ context.Context, runID string) []core.MarketAlignedEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data := s.events[runID]
	return append([]core.MarketAlignedEvent{}, data...)
}

func (s *InMemoryStore) SaveFeatureRows(_ context.Context, runID string, rows []core.FeatureRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.features[runID] = append([]core.FeatureRow{}, rows...)
	return nil
}

func (s *InMemoryStore) GetFeatureRows(_ context.Context, runID string) []core.FeatureRow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data := s.features[runID]
	return append([]core.FeatureRow{}, data...)
}

func (s *InMemoryStore) Put(_ context.Context, key string, payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifacts[key] = append([]byte{}, payload...)
	return nil
}

func (s *InMemoryStore) Get(_ context.Context, key string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	payload, ok := s.artifacts[key]
	if !ok {
		return nil, false
	}
	return append([]byte{}, payload...), true
}
