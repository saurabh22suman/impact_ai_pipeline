package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

type FileStore struct {
	mu       sync.RWMutex
	rootDir  string
	runsDir  string
	runs     map[string]core.RunResult
	events   map[string][]core.MarketAlignedEvent
	features map[string][]core.FeatureRow
}

func NewFileStore(rootDir string) (*FileStore, error) {
	cleanRoot := strings.TrimSpace(rootDir)
	if cleanRoot == "" {
		return nil, fmt.Errorf("file store root directory is required")
	}
	if err := os.MkdirAll(cleanRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create file store root: %w", err)
	}
	runsDir := filepath.Join(cleanRoot, "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create runs directory: %w", err)
	}
	return &FileStore{
		rootDir:  cleanRoot,
		runsDir:  runsDir,
		runs:     map[string]core.RunResult{},
		events:   map[string][]core.MarketAlignedEvent{},
		features: map[string][]core.FeatureRow{},
	}, nil
}

func (s *FileStore) SaveRun(_ context.Context, result core.RunResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if result.CreatedAt.IsZero() {
		result.CreatedAt = time.Now().UTC()
	}
	s.runs[result.RunID] = result
	if err := s.writeJSONFile(filepath.Join(s.runDir(result.RunID), "run.json"), result); err != nil {
		return err
	}
	return nil
}

func (s *FileStore) GetRun(_ context.Context, runID string) (core.RunResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.runs[runID]
	return r, ok
}

func (s *FileStore) ListRuns(_ context.Context) []core.RunResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]core.RunResult, 0, len(s.runs))
	for _, run := range s.runs {
		out = append(out, run)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (s *FileStore) SaveEvents(_ context.Context, runID string, events []core.MarketAlignedEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := append([]core.MarketAlignedEvent{}, events...)
	s.events[runID] = cloned
	if err := s.writeJSONFile(filepath.Join(s.runDir(runID), "events.json"), cloned); err != nil {
		return err
	}
	return nil
}

func (s *FileStore) GetEvents(_ context.Context, runID string) []core.MarketAlignedEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data := s.events[runID]
	return append([]core.MarketAlignedEvent{}, data...)
}

func (s *FileStore) SaveFeatureRows(_ context.Context, runID string, rows []core.FeatureRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := append([]core.FeatureRow{}, rows...)
	s.features[runID] = cloned
	if err := s.writeJSONFile(filepath.Join(s.runDir(runID), "feature_rows.json"), cloned); err != nil {
		return err
	}
	return nil
}

func (s *FileStore) GetFeatureRows(_ context.Context, runID string) []core.FeatureRow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data := s.features[runID]
	return append([]core.FeatureRow{}, data...)
}

func (s *FileStore) Put(_ context.Context, key string, payload []byte) error {
	path, err := s.artifactPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func (s *FileStore) Get(_ context.Context, key string) ([]byte, bool) {
	path, err := s.artifactPath(key)
	if err != nil {
		return nil, false
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return payload, true
}

func (s *FileStore) SaveRunRequest(runID string, req core.RunRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeJSONFile(filepath.Join(s.runDir(runID), "request.json"), req)
}

func (s *FileStore) RootDir() string {
	return s.rootDir
}

func (s *FileStore) runDir(runID string) string {
	return filepath.Join(s.runsDir, runID)
}

func (s *FileStore) artifactPath(key string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || clean == string(filepath.Separator) || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("invalid artifact key %q", key)
	}
	return filepath.Join(s.rootDir, clean), nil
}

func (s *FileStore) writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}
