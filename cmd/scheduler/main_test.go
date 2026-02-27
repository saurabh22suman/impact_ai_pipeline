package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/soloengine/ai-impact-scrapper/internal/core"
)

func TestParseDailyTime(t *testing.T) {
	h, m, err := parseDailyTime("09:15")
	if err != nil {
		t.Fatalf("parseDailyTime returned error: %v", err)
	}
	if h != 9 || m != 15 {
		t.Fatalf("expected 09:15, got %02d:%02d", h, m)
	}
}

func TestParseDailyTimeRejectsInvalidValues(t *testing.T) {
	cases := []string{"", "9:15", "24:00", "10:60", "abcd", "10-15"}
	for _, tc := range cases {
		if _, _, err := parseDailyTime(tc); err == nil {
			t.Fatalf("expected parseDailyTime(%q) to fail", tc)
		}
	}
}

func TestNextDailyRunUsesConfiguredTimezone(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	now := time.Date(2026, 2, 22, 8, 0, 0, 0, loc)
	next := nextDailyRun(now, loc, 9, 15)
	want := time.Date(2026, 2, 22, 9, 15, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("unexpected next run: got %s want %s", next, want)
	}
}

func TestNextDailyRunRollsToNextDayAfterScheduledTime(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	now := time.Date(2026, 2, 22, 20, 0, 0, 0, loc)
	next := nextDailyRun(now, loc, 9, 15)
	want := time.Date(2026, 2, 23, 9, 15, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("unexpected next run: got %s want %s", next, want)
	}
}

func TestTriggerRunSuccess(t *testing.T) {
	var (
		received         core.RunRequest
		receivedMethod   string
		receivedPath     string
		receivedType     string
		decodeRequestErr error
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		receivedType = r.Header.Get("Content-Type")
		decodeRequestErr = json.NewDecoder(r.Body).Decode(&received)

		writeJSON, err := json.Marshal(map[string]any{
			"run": map[string]any{"run_id": "run-000123"},
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(writeJSON)
	}))
	defer srv.Close()

	req := core.RunRequest{
		PipelineProfile:    "cost_optimized",
		Sources:            []string{"source_a", "source_b"},
		DateFrom:           time.Date(2026, 2, 22, 6, 0, 0, 0, time.UTC),
		DateTo:             time.Date(2026, 2, 22, 8, 0, 0, 0, time.UTC),
	}

	runID, err := triggerRun(context.Background(), srv.Client(), srv.URL, req)
	if err != nil {
		t.Fatalf("triggerRun returned error: %v", err)
	}
	if runID != "run-000123" {
		t.Fatalf("unexpected run id: got %q", runID)
	}
	if receivedMethod != http.MethodPost {
		t.Fatalf("unexpected method: %s", receivedMethod)
	}
	if receivedPath != "/v1/runs" {
		t.Fatalf("unexpected path: %s", receivedPath)
	}
	if !strings.Contains(receivedType, "application/json") {
		t.Fatalf("unexpected content type: %s", receivedType)
	}
	if decodeRequestErr != nil {
		t.Fatalf("decode request: %v", decodeRequestErr)
	}
	if received.PipelineProfile != req.PipelineProfile {
		t.Fatalf("unexpected profile: got %q want %q", received.PipelineProfile, req.PipelineProfile)
	}
	if len(received.Sources) != len(req.Sources) || received.Sources[0] != req.Sources[0] || received.Sources[1] != req.Sources[1] {
		t.Fatalf("unexpected sources: got %#v want %#v", received.Sources, req.Sources)
	}
	if !received.DateFrom.Equal(req.DateFrom) || !received.DateTo.Equal(req.DateTo) {
		t.Fatalf("unexpected date window: got %s..%s want %s..%s", received.DateFrom, received.DateTo, req.DateFrom, req.DateTo)
	}
}

func TestTriggerRunReturnsErrorOnNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream timeout", http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := triggerRun(context.Background(), srv.Client(), srv.URL, core.RunRequest{})
	if err == nil {
		t.Fatalf("expected triggerRun to fail for non-200 response")
	}
	if !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTriggerRunReturnsErrorOnMalformedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"run":{}}`)); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	_, err := triggerRun(context.Background(), srv.Client(), srv.URL, core.RunRequest{})
	if err == nil {
		t.Fatalf("expected triggerRun to fail when run_id is missing")
	}
	if !strings.Contains(err.Error(), "missing run_id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func FuzzParseDailyTime(f *testing.F) {
	seedInputs := []string{"09:15", "00:00", "23:59", "9:15", "24:00", "aa:bb", ""}
	for _, input := range seedInputs {
		f.Add(input)
	}

	f.Fuzz(func(t *testing.T, input string) {
		hour, minute, err := parseDailyTime(input)
		if err == nil {
			if hour < 0 || hour > 23 {
				t.Fatalf("hour out of range for %q: %d", input, hour)
			}
			if minute < 0 || minute > 59 {
				t.Fatalf("minute out of range for %q: %d", input, minute)
			}
		}
	})
}
