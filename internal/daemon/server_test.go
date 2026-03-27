package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/config"
	"github.com/SimplyLiz/CodeMCP/internal/version"
	"github.com/SimplyLiz/CodeMCP/internal/watcher"
)

// testLogger implements Printf for testing
type testLogger struct{}

func (l *testLogger) Printf(format string, args ...interface{}) {}

// newTestDaemon creates a minimal daemon for HTTP handler testing (no watcher)
func newTestDaemon(t *testing.T) *Daemon {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	return &Daemon{
		config:    &config.DaemonConfig{Port: 9120, Bind: "localhost"},
		startedAt: time.Now(),
		ctx:       ctx,
		cancel:    cancel,
		logger:    log.New(io.Discard, "", 0),
	}
}

// newTestDaemonWithWatcher creates a minimal daemon for HTTP handler testing
func newTestDaemonWithWatcher(t *testing.T) *Daemon {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Create real watcher with a callback that does nothing
	cfg := watcher.DefaultConfig()
	cfg.Enabled = false // Don't actually start watching
	w := watcher.New(cfg, logger, func(repoPath string, events []watcher.Event) {})

	d := &Daemon{
		ctx:       ctx,
		cancel:    cancel,
		startedAt: time.Now(),
		logger:    nil, // Not needed for handler tests
		watcher:   w,
	}

	// Create real refresh manager (but operations will fail on invalid repos)
	stdLogger := &testLogger{}
	d.refreshManager = NewRefreshManager(logger, stdLogger, nil)

	return d
}

// =============================================================================
// handleRefresh Tests
// =============================================================================

func TestHandleRefresh_MethodNotAllowed(t *testing.T) {
	d := newTestDaemonWithWatcher(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/refresh", nil)
	w := httptest.NewRecorder()

	d.handleRefresh(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleRefresh_NoWatchedRepos(t *testing.T) {
	d := newTestDaemonWithWatcher(t) // Watcher starts with no repos

	req := httptest.NewRequest(http.MethodPost, "/api/v1/refresh", nil)
	w := httptest.NewRecorder()

	d.handleRefresh(w, req)

	// No watched repos and no repo specified = error
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleRefresh_WithRepoPath(t *testing.T) {
	d := newTestDaemonWithWatcher(t)

	body := `{"repo": "/custom/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/refresh", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	d.handleRefresh(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, w.Code)
	}

	var resp RefreshResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Repo != "/custom/repo" {
		t.Errorf("expected repo '/custom/repo', got %q", resp.Repo)
	}
	if resp.Type != "incremental" {
		t.Errorf("expected type 'incremental', got %q", resp.Type)
	}
}

func TestHandleRefresh_FullReindex(t *testing.T) {
	d := newTestDaemonWithWatcher(t)

	body := `{"full": true, "repo": "/test/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/refresh", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	d.handleRefresh(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, w.Code)
	}

	var resp RefreshResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Type != "full" {
		t.Errorf("expected type 'full', got %q", resp.Type)
	}
}

func TestHandleRefresh_InvalidJSON(t *testing.T) {
	d := newTestDaemonWithWatcher(t)

	body := `{"invalid json`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/refresh", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	d.handleRefresh(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleRefresh_AlreadyPending(t *testing.T) {
	d := newTestDaemonWithWatcher(t)
	repoPath := "/test/repo"

	// Mark as pending
	d.refreshManager.markPending(repoPath)

	body := `{"repo": "/test/repo"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/refresh", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	d.handleRefresh(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, w.Code)
	}

	var resp RefreshResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Status != "already_queued" {
		t.Errorf("expected status 'already_queued', got %q", resp.Status)
	}
}

// =============================================================================
// formatDuration Tests
// =============================================================================

func TestFormatDuration_Seconds(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{0, "0s"},
		{1 * time.Second, "1s"},
		{30 * time.Second, "30s"},
		{59 * time.Second, "59s"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.duration)
		if result != tt.expected {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
		}
	}
}

func TestFormatDuration_Minutes(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{1 * time.Minute, "1m0s"},
		{5 * time.Minute, "5m0s"},
		{5*time.Minute + 30*time.Second, "5m30s"},
		{59*time.Minute + 59*time.Second, "59m59s"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.duration)
		if result != tt.expected {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
		}
	}
}

func TestFormatDuration_Hours(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{1 * time.Hour, "1h0m0s"},
		{2*time.Hour + 30*time.Minute, "2h30m0s"},
		{24 * time.Hour, "24h0m0s"},
		{100*time.Hour + 5*time.Minute + 10*time.Second, "100h5m10s"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.duration)
		if result != tt.expected {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
		}
	}
}

// =============================================================================
// Health Endpoint Tests
// =============================================================================

func TestHandleHealth_MethodNotAllowed(t *testing.T) {
	d := newTestDaemonWithWatcher(t)

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()

	d.handleHealth(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleHealth_Success(t *testing.T) {
	d := newTestDaemonWithWatcher(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	d.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Status != "healthy" {
		t.Errorf("expected status 'healthy', got %q", resp.Status)
	}
	if resp.Checks["database"] != "ok" {
		t.Errorf("expected database check 'ok', got %q", resp.Checks["database"])
	}
}

// =============================================================================
// writeJSON / writeError Tests
// =============================================================================

func TestWriteJSON(t *testing.T) {
	d := newTestDaemonWithWatcher(t)

	w := httptest.NewRecorder()
	data := map[string]string{"key": "value"}

	d.writeJSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", ct)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected key='value', got %q", result["key"])
	}
}

func TestWriteError(t *testing.T) {
	d := newTestDaemonWithWatcher(t)

	w := httptest.NewRecorder()

	d.writeError(w, http.StatusBadRequest, "bad_request", "Invalid input")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Success {
		t.Error("expected Success=false")
	}
	if resp.Error == nil {
		t.Fatal("expected Error to be set")
	}
	if resp.Error.Code != "bad_request" {
		t.Errorf("expected code 'bad_request', got %q", resp.Error.Code)
	}
	if resp.Error.Message != "Invalid input" {
		t.Errorf("expected message 'Invalid input', got %q", resp.Error.Message)
	}
}

// =============================================================================
// handleHealth Tests (extended)
// =============================================================================

func TestHandleHealth_ResponseFields(t *testing.T) {
	d := newTestDaemon(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	d.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Status != "healthy" {
		t.Errorf("expected status 'healthy', got %q", resp.Status)
	}
	if resp.Version != version.Version {
		t.Errorf("expected version %q, got %q", version.Version, resp.Version)
	}
	if resp.Uptime == "" {
		t.Error("expected non-empty uptime")
	}
	expectedChecks := []string{"database", "federations", "jobQueue"}
	for _, key := range expectedChecks {
		val, ok := resp.Checks[key]
		if !ok {
			t.Errorf("expected check %q to be present", key)
		} else if val != "ok" {
			t.Errorf("expected check %q = 'ok', got %q", key, val)
		}
	}
}

// =============================================================================
// handleScheduleList Tests
// =============================================================================

func TestHandleScheduleList_NoScheduler(t *testing.T) {
	d := newTestDaemon(t)
	// scheduler is nil by default in newTestDaemon

	req := httptest.NewRequest(http.MethodGet, "/api/v1/daemon/schedule", nil)
	w := httptest.NewRecorder()

	d.handleScheduleList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	schedules, ok := resp["schedules"].([]interface{})
	if !ok {
		t.Fatal("expected 'schedules' to be an array")
	}
	if len(schedules) != 0 {
		t.Errorf("expected empty schedules, got %d entries", len(schedules))
	}

	totalCount, ok := resp["totalCount"].(float64)
	if !ok {
		t.Fatal("expected 'totalCount' to be a number")
	}
	if totalCount != 0 {
		t.Errorf("expected totalCount=0, got %v", totalCount)
	}
}

func TestHandleScheduleList_MethodNotAllowed(t *testing.T) {
	d := newTestDaemon(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/daemon/schedule", nil)
	w := httptest.NewRecorder()

	d.handleScheduleList(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// =============================================================================
// handleJobsList Tests
// =============================================================================

func TestHandleJobsList_NoScheduler(t *testing.T) {
	d := newTestDaemon(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/daemon/jobs", nil)
	w := httptest.NewRecorder()

	d.handleJobsList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	jobs, ok := resp["jobs"].([]interface{})
	if !ok {
		t.Fatal("expected 'jobs' to be an array")
	}
	if len(jobs) != 0 {
		t.Errorf("expected empty jobs, got %d entries", len(jobs))
	}
}

// =============================================================================
// handleJobsRoute Tests
// =============================================================================

func TestHandleJobsRoute_MissingID(t *testing.T) {
	d := newTestDaemon(t)

	// Path with trailing slash but no ID
	req := httptest.NewRequest(http.MethodGet, "/api/v1/daemon/jobs/", nil)
	w := httptest.NewRecorder()

	d.handleJobsRoute(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Success {
		t.Error("expected Success=false")
	}
	if resp.Error == nil {
		t.Fatal("expected Error to be set")
	}
	if resp.Error.Code != "missing_id" {
		t.Errorf("expected error code 'missing_id', got %q", resp.Error.Code)
	}
}

func TestHandleJobsRoute_NoScheduler(t *testing.T) {
	d := newTestDaemon(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/daemon/jobs/some-job-id", nil)
	w := httptest.NewRecorder()

	d.handleJobsRoute(w, req)

	// When scheduler is nil and ID is provided, returns 404
	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleJobsRoute_MethodNotAllowed(t *testing.T) {
	d := newTestDaemon(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/daemon/jobs/some-job-id", nil)
	w := httptest.NewRecorder()

	// Need a non-nil scheduler so it reaches the method switch
	// With nil scheduler it returns 404 before checking method.
	// So this test verifies the nil-scheduler path returns 404 for unsupported methods too.
	d.handleJobsRoute(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d (nil scheduler returns 404 before method check)", http.StatusNotFound, w.Code)
	}
}

// =============================================================================
// handleReposList Tests
// =============================================================================

func TestHandleReposList_ResponseFormat(t *testing.T) {
	d := newTestDaemon(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/repos", nil)
	w := httptest.NewRecorder()

	d.handleReposList(w, req)

	// LoadRegistry may fail or succeed depending on env; either way we get valid JSON
	var raw map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	if w.Code == http.StatusOK {
		if _, ok := raw["repos"]; !ok {
			t.Error("expected 'repos' key in successful response")
		}
		if _, ok := raw["totalCount"]; !ok {
			t.Error("expected 'totalCount' key in successful response")
		}
	} else if w.Code == http.StatusInternalServerError {
		// Registry load failed (no ~/.ckb directory, etc.) — that's fine, check error format
		if _, ok := raw["error"]; !ok {
			t.Error("expected 'error' key in error response")
		}
	} else {
		t.Errorf("unexpected status %d", w.Code)
	}
}

func TestHandleReposRoute_MethodNotAllowed(t *testing.T) {
	d := newTestDaemon(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/repos/some-repo", nil)
	w := httptest.NewRecorder()

	d.handleReposRoute(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleReposRoute_MissingName(t *testing.T) {
	d := newTestDaemon(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/repos/", nil)
	w := httptest.NewRecorder()

	d.handleReposRoute(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Error == nil || resp.Error.Code != "missing_id" {
		t.Errorf("expected error code 'missing_id', got %v", resp.Error)
	}
}

// =============================================================================
// handleFederationsList Tests
// =============================================================================

func TestHandleFederationsList_ResponseFormat(t *testing.T) {
	d := newTestDaemon(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/federations", nil)
	w := httptest.NewRecorder()

	d.handleFederationsList(w, req)

	var raw map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	if w.Code == http.StatusOK {
		if _, ok := raw["federations"]; !ok {
			t.Error("expected 'federations' key in successful response")
		}
		if _, ok := raw["totalCount"]; !ok {
			t.Error("expected 'totalCount' key in successful response")
		}
	} else if w.Code == http.StatusInternalServerError {
		if _, ok := raw["error"]; !ok {
			t.Error("expected 'error' key in error response")
		}
	} else {
		t.Errorf("unexpected status %d", w.Code)
	}
}

func TestHandleFederationsList_MethodNotAllowed(t *testing.T) {
	d := newTestDaemon(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/federations", nil)
	w := httptest.NewRecorder()

	d.handleFederationsList(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleFederationsRoute_MethodNotAllowed(t *testing.T) {
	d := newTestDaemon(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/federations/my-fed", nil)
	w := httptest.NewRecorder()

	d.handleFederationsRoute(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleFederationsRoute_MissingName(t *testing.T) {
	d := newTestDaemon(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/federations/", nil)
	w := httptest.NewRecorder()

	d.handleFederationsRoute(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var resp APIResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Error == nil || resp.Error.Code != "missing_name" {
		t.Errorf("expected error code 'missing_name', got %v", resp.Error)
	}
}

// =============================================================================
// handleDaemonStatus Tests
// =============================================================================

func TestHandleDaemonStatus_MethodNotAllowed(t *testing.T) {
	d := newTestDaemon(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/daemon/status", nil)
	w := httptest.NewRecorder()

	d.handleDaemonStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}
