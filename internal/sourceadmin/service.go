package sourceadmin

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"gopkg.in/yaml.v3"
)

var (
	ErrValidation   = errors.New("source validation")
	sourceIDPattern = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
)

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

func (e ValidationError) Unwrap() error {
	return ErrValidation
}

func IsValidationError(err error) bool {
	return errors.Is(err, ErrValidation)
}

type CreateSourceInput struct {
	ID            string
	Name          string
	Kind          string
	URL           string
	Region        string
	Language      string
	Enabled       bool
	CrawlFallback bool
}

type Service struct {
	configDir string
	now       func() time.Time
	mu        sync.Mutex
}

func NewService(configDir string) *Service {
	return &Service{
		configDir: strings.TrimSpace(configDir),
		now:       func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) List() ([]config.Source, error) {
	sourcesFile, _, err := s.loadSourcesFile()
	if err != nil {
		return nil, err
	}
	out := make([]config.Source, len(sourcesFile.Sources))
	copy(out, sourcesFile.Sources)
	return out, nil
}

func (s *Service) Create(input CreateSourceInput) (config.Source, error) {
	candidate, err := normalizeAndValidate(input)
	if err != nil {
		return config.Source{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sourcesFile, originalBytes, err := s.loadSourcesFile()
	if err != nil {
		return config.Source{}, err
	}

	for _, existing := range sourcesFile.Sources {
		if strings.EqualFold(strings.TrimSpace(existing.ID), candidate.ID) {
			return config.Source{}, ValidationError{Message: fmt.Sprintf("source id %q already exists", candidate.ID)}
		}
	}

	sourcesFile.Sources = append(sourcesFile.Sources, candidate)
	if strings.TrimSpace(sourcesFile.Version) == "" {
		sourcesFile.Version = s.now().Format("2006-01-02")
	}

	payload, err := yaml.Marshal(sourcesFile)
	if err != nil {
		return config.Source{}, fmt.Errorf("marshal sources.yaml: %w", err)
	}

	sourcesPath := filepath.Join(s.configDir, "sources.yaml")
	if err := writeFileAtomically(sourcesPath, payload); err != nil {
		return config.Source{}, err
	}

	if _, err := config.Load(s.configDir); err != nil {
		_ = writeFileAtomically(sourcesPath, originalBytes)
		return config.Source{}, fmt.Errorf("source persisted but config reload failed: %w", err)
	}

	return candidate, nil
}

func normalizeAndValidate(input CreateSourceInput) (config.Source, error) {
	source := config.Source{
		ID:            strings.TrimSpace(input.ID),
		Name:          strings.TrimSpace(input.Name),
		Kind:          config.NormalizeSourceKind(input.Kind),
		URL:           strings.TrimSpace(input.URL),
		Region:        strings.TrimSpace(input.Region),
		Language:      strings.TrimSpace(input.Language),
		Enabled:       input.Enabled,
		CrawlFallback: input.CrawlFallback,
	}

	if source.ID == "" || source.Name == "" || source.URL == "" || source.Region == "" || source.Language == "" {
		return config.Source{}, ValidationError{Message: "id, name, url, region, and language are required"}
	}
	if !sourceIDPattern.MatchString(source.ID) {
		return config.Source{}, ValidationError{Message: "id must contain only letters, numbers, dot, underscore, or hyphen"}
	}
	if _, err := url.ParseRequestURI(source.URL); err != nil {
		return config.Source{}, ValidationError{Message: "url must be a valid absolute URL"}
	}

	switch source.Kind {
	case config.SourceKindRSS, config.SourceKindDirect:
	default:
		return config.Source{}, ValidationError{Message: fmt.Sprintf("unsupported kind %q", source.Kind)}
	}

	return source, nil
}

func (s *Service) loadSourcesFile() (config.SourcesFile, []byte, error) {
	if strings.TrimSpace(s.configDir) == "" {
		return config.SourcesFile{}, nil, fmt.Errorf("config directory is required")
	}
	path := filepath.Join(s.configDir, "sources.yaml")
	payload, err := os.ReadFile(path)
	if err != nil {
		return config.SourcesFile{}, nil, fmt.Errorf("read sources.yaml: %w", err)
	}
	var sourcesFile config.SourcesFile
	if err := yaml.Unmarshal(payload, &sourcesFile); err != nil {
		return config.SourcesFile{}, nil, fmt.Errorf("parse sources.yaml: %w", err)
	}
	for i := range sourcesFile.Sources {
		sourcesFile.Sources[i].Kind = config.NormalizeSourceKind(sourcesFile.Sources[i].Kind)
	}
	return sourcesFile, payload, nil
}

func writeFileAtomically(path string, payload []byte) error {
	dir := filepath.Dir(path)
	name := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, name+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file for %s: %w", path, err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file for %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
