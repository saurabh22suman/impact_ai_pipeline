package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/config"
	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

func main() {
	cfgDir := strings.TrimSpace(os.Getenv("CONFIG_DIR"))
	if cfgDir == "" {
		cfgDir = filepath.Join(".", "configs")
	}
	cfg, err := config.Load(cfgDir)
	if err != nil {
		log.Fatalf("scheduler config load failed: %v", err)
	}

	profile := strings.TrimSpace(os.Getenv("SCHEDULER_PROFILE"))
	if profile == "" {
		profile = cfg.Pipelines.DefaultProfile
	}
	sourceIDs := splitCSV(os.Getenv("SCHEDULER_SOURCES"))

	locName := strings.TrimSpace(os.Getenv("SCHEDULER_TIMEZONE"))
	if locName == "" {
		locName = "Asia/Kolkata"
	}
	loc, err := time.LoadLocation(locName)
	if err != nil {
		log.Fatalf("invalid scheduler timezone %q: %v", locName, err)
	}

	timeRaw := strings.TrimSpace(os.Getenv("SCHEDULER_DAILY_TIME"))
	if timeRaw == "" {
		timeRaw = "09:15"
	}
	hour, minute, err := parseDailyTime(timeRaw)
	if err != nil {
		log.Fatalf("invalid SCHEDULER_DAILY_TIME %q: %v", timeRaw, err)
	}

	apiBaseURL := strings.TrimSpace(os.Getenv("SCHEDULER_API_BASE_URL"))
	if apiBaseURL == "" {
		apiBaseURL = "http://engine:8080"
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	log.Printf("scheduler started profile=%s timezone=%s daily_time=%s api_base_url=%s", profile, locName, timeRaw, apiBaseURL)
	for {
		now := time.Now().In(loc)
		next := nextDailyRun(now, loc, hour, minute)
		wait := time.Until(next)
		if wait > 0 {
			log.Printf("scheduler next_run=%s", next.Format(time.RFC3339))
			time.Sleep(wait)
		}
		runOnce(httpClient, apiBaseURL, profile, sourceIDs)
	}
}

func parseDailyTime(raw string) (int, int, error) {
	if len(raw) != 5 || raw[2] != ':' {
		return 0, 0, fmt.Errorf("expected HH:MM")
	}
	hour, err := parseTwoDigits(raw[0], raw[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid hour")
	}
	minute, err := parseTwoDigits(raw[3], raw[4])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid minute")
	}
	if hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("hour out of range")
	}
	if minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("minute out of range")
	}
	return hour, minute, nil
}

func parseTwoDigits(a, b byte) (int, error) {
	if a < '0' || a > '9' || b < '0' || b > '9' {
		return 0, fmt.Errorf("not a two digit number")
	}
	return int(a-'0')*10 + int(b-'0'), nil
}

func nextDailyRun(now time.Time, loc *time.Location, hour, minute int) time.Time {
	now = now.In(loc)
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, loc)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func runOnce(client *http.Client, apiBaseURL, profile string, sourceIDs []string) {
	ctx := context.Background()
	now := time.Now().UTC()
	req := core.RunRequest{
		PipelineProfile: profile,
		DateFrom:        now.Add(-2 * time.Hour),
		DateTo:          now,
		Sources:         sourceIDs,
	}

	runID, err := triggerRun(ctx, client, apiBaseURL, req)
	if err != nil {
		log.Printf("scheduler trigger failed: %v", err)
		return
	}
	log.Printf("scheduler run triggered run_id=%s profile=%s", runID, profile)
}

func triggerRun(ctx context.Context, client *http.Client, apiBaseURL string, req core.RunRequest) (string, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	endpoint := strings.TrimRight(strings.TrimSpace(apiBaseURL), "/") + "/v1/runs"

	payload, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal run request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build scheduler API request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("call scheduler API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("scheduler API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded struct {
		Run struct {
			RunID string `json:"run_id"`
		} `json:"run"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode scheduler API response: %w", err)
	}

	runID := strings.TrimSpace(decoded.Run.RunID)
	if runID == "" {
		return "", fmt.Errorf("malformed scheduler API response: missing run_id")
	}
	return runID, nil
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
