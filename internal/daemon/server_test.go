package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ckb/internal/watcher"
)

// testLogger implements Printf for testing
type testLogger struct{}

func (l *testLogger) Printf(format string, args ...interface{}) {}

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
