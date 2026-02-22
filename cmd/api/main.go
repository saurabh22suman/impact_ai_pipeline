package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
	"github.com/soloengine/ai-impact-scrapper/internal/engine"
	"github.com/soloengine/ai-impact-scrapper/internal/nse"
	"github.com/soloengine/ai-impact-scrapper/internal/sourceadmin"
)

type server struct {
	rt          *engine.Runtime
	sourceAdmin *sourceadmin.Service
}

func main() {
	rt, err := engine.Bootstrap()
	if err != nil {
		log.Fatalf("bootstrap failed: %v", err)
	}
	defer func() {
		if err := rt.Close(); err != nil {
			log.Printf("runtime close error: %v", err)
		}
	}()

	cfgDir := strings.TrimSpace(rt.ConfigDir)
	if cfgDir == "" {
		cfgDir = os.Getenv("CONFIG_DIR")
	}
	if strings.TrimSpace(cfgDir) == "" {
		cfgDir = filepath.Join(".", "configs")
	}

	srv := &server{rt: rt, sourceAdmin: sourceadmin.NewService(cfgDir)}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", srv.handleHealth)
	mux.HandleFunc("/v1/config", srv.handleConfig)
	mux.HandleFunc("/v1/sources", srv.handleSources)
	mux.HandleFunc("/v1/runs", srv.handleRuns)
	mux.HandleFunc("/v1/runs/", srv.handleRunByID)
	mux.HandleFunc("/v1/bhavcopy/download", srv.handleBhavcopyDownload)

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("api listening on :%s", port)
	if err := http.ListenAndServe(":"+port, loggingMiddleware(mux)); err != nil {
		log.Fatalf("api server exited: %v", err)
	}
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	enabledSources := s.rt.Config.EnabledSources()
	sources := make([]map[string]any, 0, len(enabledSources))
	for _, source := range enabledSources {
		sources = append(sources, map[string]any{
			"id":             source.ID,
			"name":           source.Name,
			"region":         source.Region,
			"language":       source.Language,
			"kind":           config.NormalizeSourceKind(source.Kind),
			"crawl_fallback": source.CrawlFallback,
		})
	}

	effectiveEntities := s.rt.Config.EffectiveEntities()
	entities := make([]map[string]any, 0, len(effectiveEntities))
	for _, entity := range effectiveEntities {
		aliases := make([]string, len(entity.Aliases))
		copy(aliases, entity.Aliases)
		entities = append(entities, map[string]any{
			"id":       entity.ID,
			"symbol":   entity.Symbol,
			"name":     entity.Name,
			"aliases":  aliases,
			"exchange": entity.Exchange,
			"sector":   entity.Sector,
			"type":     config.NormalizeEntityType(entity.Type),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"config_version":     s.rt.Config.ConfigVersion,
		"sources_enabled":    len(enabledSources),
		"sources":            sources,
		"entities_effective": len(effectiveEntities),
		"entities":           entities,
		"factors":            len(s.rt.Config.Factors.Factors),
		"providers_enabled":  len(s.rt.Config.EnabledProviders()),
		"profiles":           s.rt.Config.Pipelines.Profiles,
	})
}

func (s *server) handleSources(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sources, err := s.sourceAdmin.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sources": sources})
	case http.MethodPost:
		s.handleSourceCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *server) handleSourceCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		Kind          string `json:"kind"`
		URL           string `json:"url"`
		Region        string `json:"region"`
		Language      string `json:"language"`
		Enabled       bool   `json:"enabled"`
		CrawlFallback bool   `json:"crawl_fallback"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	created, err := s.sourceAdmin.Create(sourceadmin.CreateSourceInput{
		ID:            req.ID,
		Name:          req.Name,
		Kind:          req.Kind,
		URL:           req.URL,
		Region:        req.Region,
		Language:      req.Language,
		Enabled:       req.Enabled,
		CrawlFallback: req.CrawlFallback,
	})
	if err != nil {
		if sourceadmin.IsValidationError(err) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	reloaded, err := config.Load(s.rt.ConfigDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("source created but config reload failed: %v", err), http.StatusInternalServerError)
		return
	}
	s.rt.Config = reloaded
	s.rt.Service = engine.NewService(reloaded, s.rt.Store)

	writeJSON(w, http.StatusCreated, map[string]any{
		"source": created,
	})
}

func (s *server) handleRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleRunCreate(w, r)
	case http.MethodGet:
		runs := s.rt.Service.ListRuns(r.Context())
		writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *server) handleRunCreate(w http.ResponseWriter, r *http.Request) {
	var req core.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.DateFrom.IsZero() {
		req.DateFrom = time.Now().UTC().Add(-24 * time.Hour)
	}
	if req.DateTo.IsZero() {
		req.DateTo = time.Now().UTC()
	}
	if req.PipelineProfile == "" {
		req.PipelineProfile = s.rt.Config.Pipelines.DefaultProfile
	}

	sources, err := engine.ResolveSources(s.rt.Config, req.Sources)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	articles, notices, err := engine.CollectArticles(r.Context(), s.rt.Fetcher, sources, req.DateFrom, req.DateTo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	result, err := s.rt.Service.Run(r.Context(), req, articles)
	if err != nil {
		if errors.Is(err, engine.ErrInvalidEntitySelection) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"run":     result,
		"notices": notices,
	})
}

func (s *server) handleRunByID(w http.ResponseWriter, r *http.Request) {
	id, tail, err := parseRunPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if tail == "" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		run, ok := s.rt.Service.GetRun(r.Context(), id)
		if !ok {
			http.Error(w, "run not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, run)
		return
	}

	if tail != "export" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "jsonl"
	}

	var payload []byte
	switch format {
	case "jsonl":
		payload, err = s.rt.Service.ExportJSONL(r.Context(), id)
	case "csv":
		payload, err = s.rt.Service.ExportCSV(r.Context(), id)
	case "toon":
		payload, err = s.rt.Service.ExportTOON(r.Context(), id)
	default:
		http.Error(w, "unsupported format", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	ct := "application/octet-stream"
	switch format {
	case "jsonl":
		ct = "application/x-ndjson"
	case "csv":
		ct = "text/csv"
	case "toon":
		ct = "application/json"
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *server) handleBhavcopyDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	dateParam := strings.TrimSpace(r.URL.Query().Get("date"))
	if dateParam == "" {
		http.Error(w, "date query parameter is required (YYYY-MM-DD)", http.StatusBadRequest)
		return
	}
	requestedDate, err := time.Parse("2006-01-02", dateParam)
	if err != nil {
		http.Error(w, "invalid date format; expected YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	requestedDate = requestedDate.UTC()
	availableOn := requestedDate.AddDate(0, 0, 2)
	nowUTC := time.Now().UTC()
	if availableOn.After(nowUTC) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":          "bhavcopy is available only after T+2",
			"requested_date": requestedDate.Format("2006-01-02"),
			"available_on":   availableOn.Format(time.RFC3339),
			"now":            nowUTC.Format(time.RFC3339),
		})
		return
	}

	archive, filename, err := nse.FetchBhavcopyArchive(r.Context(), http.DefaultClient, requestedDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", bhavcopyContentType(filename))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(archive)
}

func bhavcopyContentType(filename string) string {
	if strings.HasSuffix(strings.ToLower(strings.TrimSpace(filename)), ".csv") {
		return "text/csv"
	}
	return "application/zip"
}

func parseRunPath(path string) (id, tail string, err error) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 || parts[0] != "v1" || parts[1] != "runs" {
		return "", "", errors.New("invalid run path")
	}
	id = parts[2]
	if id == "" {
		return "", "", errors.New("run id is required")
	}
	if len(parts) > 3 {
		tail = parts[3]
	}
	return id, tail, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(started))
	})
}
