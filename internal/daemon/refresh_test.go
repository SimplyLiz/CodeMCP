package daemon

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"ckb/internal/logging"
)

// mockLogger implements the Printf interface for testing
type mockLogger struct {
	messages []string
	mu       sync.Mutex
}

func (m *mockLogger) Printf(format string, args ...interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, format)
}

func TestNewRefreshManager(t *testing.T) {
	logger := logging.NewLogger(logging.Config{
		Level:  logging.InfoLevel,
		Format: logging.HumanFormat,
	})
	stdLogger := &mockLogger{}

	rm := NewRefreshManager(logger, stdLogger, nil)

	if rm == nil {
		t.Fatal("NewRefreshManager returned nil")
	}
	if rm.pending == nil {
		t.Error("pending map should be initialized")
	}
	if rm.logger != logger {
		t.Error("logger not set correctly")
	}
	if rm.stdLogger != stdLogger {
		t.Error("stdLogger not set correctly")
	}
}

func TestRefreshManager_HasPendingRefresh_Empty(t *testing.T) {
	rm := NewRefreshManager(nil, &mockLogger{}, nil)

	if rm.HasPendingRefresh("/some/repo") {
		t.Error("expected no pending refresh for unknown repo")
	}
}

func TestRefreshManager_PendingState(t *testing.T) {
	rm := NewRefreshManager(nil, &mockLogger{}, nil)
	repoPath := "/test/repo"

	// Initially not pending
	if rm.HasPendingRefresh(repoPath) {
		t.Error("expected no pending refresh initially")
	}

	// Mark as pending
	rm.markPending(repoPath)
	if !rm.HasPendingRefresh(repoPath) {
		t.Error("expected pending after markPending")
	}

	// Clear pending
	rm.clearPending(repoPath)
	if rm.HasPendingRefresh(repoPath) {
		t.Error("expected not pending after clearPending")
	}
}

func TestRefreshManager_PendingState_MultipleRepos(t *testing.T) {
	rm := NewRefreshManager(nil, &mockLogger{}, nil)
	repo1 := "/test/repo1"
	repo2 := "/test/repo2"

	rm.markPending(repo1)
	rm.markPending(repo2)

	if !rm.HasPendingRefresh(repo1) {
		t.Error("expected repo1 to be pending")
	}
	if !rm.HasPendingRefresh(repo2) {
		t.Error("expected repo2 to be pending")
	}

	rm.clearPending(repo1)

	if rm.HasPendingRefresh(repo1) {
		t.Error("expected repo1 to not be pending after clear")
	}
	if !rm.HasPendingRefresh(repo2) {
		t.Error("expected repo2 to still be pending")
	}
}

func TestRefreshManager_PendingState_Concurrent(t *testing.T) {
	rm := NewRefreshManager(nil, &mockLogger{}, nil)
	repoPath := "/test/repo"

	var wg sync.WaitGroup

	// Concurrently mark and check pending state
	for i := 0; i < 100; i++ {
		wg.Add(2)

		go func() {
			defer wg.Done()
			rm.markPending(repoPath)
		}()

		go func() {
			defer wg.Done()
			rm.HasPendingRefresh(repoPath)
		}()
	}

	wg.Wait()

	// After all concurrent operations, should still be pending
	if !rm.HasPendingRefresh(repoPath) {
		t.Error("expected pending after concurrent marks")
	}
}

func TestRefreshManager_ClearPending_Idempotent(t *testing.T) {
	rm := NewRefreshManager(nil, &mockLogger{}, nil)
	repoPath := "/test/repo"

	// Clear without marking first - should not panic
	rm.clearPending(repoPath)

	if rm.HasPendingRefresh(repoPath) {
		t.Error("expected not pending after clearing non-existent")
	}

	// Clear again
	rm.clearPending(repoPath)

	if rm.HasPendingRefresh(repoPath) {
		t.Error("expected still not pending after second clear")
	}
}

func TestRefreshResult_Fields(t *testing.T) {
	result := &RefreshResult{
		RepoPath:     "/test/repo",
		Type:         "incremental",
		Success:      true,
		Duration:     5 * time.Second,
		FilesChanged: 10,
	}

	if result.RepoPath != "/test/repo" {
		t.Errorf("expected RepoPath='/test/repo', got %q", result.RepoPath)
	}
	if result.Type != "incremental" {
		t.Errorf("expected Type='incremental', got %q", result.Type)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if result.Duration != 5*time.Second {
		t.Errorf("expected Duration=5s, got %v", result.Duration)
	}
	if result.FilesChanged != 10 {
		t.Errorf("expected FilesChanged=10, got %d", result.FilesChanged)
	}
}

func TestRefreshResult_Error(t *testing.T) {
	result := &RefreshResult{
		RepoPath: "/test/repo",
		Type:     "full",
		Success:  false,
		Error:    "indexer failed: exit status 1",
	}

	if result.Success {
		t.Error("expected Success=false for error result")
	}
	if result.Error == "" {
		t.Error("expected Error to be set")
	}
}

func TestRefreshManager_RunIncrementalRefresh_InvalidRepo(t *testing.T) {
	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel, // Suppress info logs
		Format: logging.HumanFormat,
	})
	stdLogger := &mockLogger{}

	rm := NewRefreshManager(logger, stdLogger, nil)

	ctx := context.Background()
	result := rm.RunIncrementalRefresh(ctx, "/nonexistent/repo/path")

	if result.Success {
		t.Error("expected failure for nonexistent repo")
	}
	if result.Error == "" {
		t.Error("expected error message for nonexistent repo")
	}
	if result.RepoPath != "/nonexistent/repo/path" {
		t.Errorf("expected RepoPath to be set correctly")
	}
	if result.Type != "incremental" {
		t.Errorf("expected Type='incremental', got %q", result.Type)
	}

	// Should not leave pending state after completion
	if rm.HasPendingRefresh("/nonexistent/repo/path") {
		t.Error("pending state should be cleared after refresh completes")
	}
}

func TestRefreshManager_RunFullReindex_InvalidRepo(t *testing.T) {
	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.HumanFormat,
	})
	stdLogger := &mockLogger{}

	rm := NewRefreshManager(logger, stdLogger, nil)

	ctx := context.Background()
	result := rm.RunFullReindex(ctx, "/nonexistent/repo/path")

	if result.Success {
		t.Error("expected failure for nonexistent repo")
	}
	if result.Error == "" {
		t.Error("expected error message for nonexistent repo")
	}
	if result.Type != "full" {
		t.Errorf("expected Type='full', got %q", result.Type)
	}
}

func TestRefreshManager_RunFullReindex_Cancelled(t *testing.T) {
	// Note: Context cancellation check in RunFullReindex happens after
	// language detection, so we test that failure modes work correctly.
	// For an empty dir, it will fail at language detection first.
	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.HumanFormat,
	})
	stdLogger := &mockLogger{}

	rm := NewRefreshManager(logger, stdLogger, nil)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := rm.RunFullReindex(ctx, t.TempDir())

	// Should fail - either due to language detection (empty dir) or cancellation
	if result.Success {
		t.Error("expected failure")
	}
	// Error should be non-empty
	if result.Error == "" {
		t.Error("expected error message to be set")
	}
}

func TestRefreshManager_EmitWebhookEvent_NilManager(t *testing.T) {
	stdLogger := &mockLogger{}
	rm := NewRefreshManager(nil, stdLogger, nil)

	// Should not panic with nil webhook manager
	rm.emitWebhookEvent("test.event", "/test/repo", map[string]interface{}{
		"key": "value",
	})

	// No error expected
}

func TestRefreshManager_EmitWebhookEvent_WithData(t *testing.T) {
	stdLogger := &mockLogger{}
	rm := NewRefreshManager(nil, stdLogger, nil)

	// Test with various data types
	rm.emitWebhookEvent("index.updated", "/test/repo", map[string]interface{}{
		"type":         "incremental",
		"filesChanged": 5,
		"duration":     "1.5s",
		"nested": map[string]interface{}{
			"key": "value",
		},
	})

	// No panic expected
}

func TestRefreshManager_RunIncrementalRefresh_SetsType(t *testing.T) {
	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.HumanFormat,
	})
	stdLogger := &mockLogger{}

	rm := NewRefreshManager(logger, stdLogger, nil)

	ctx := context.Background()
	result := rm.RunIncrementalRefresh(ctx, "/nonexistent/repo/path")

	// Even on failure, type should be set correctly
	if result.Type != "incremental" {
		t.Errorf("expected Type='incremental', got %q", result.Type)
	}
}

func TestRefreshManager_RunFullReindex_SetsType(t *testing.T) {
	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.HumanFormat,
	})
	stdLogger := &mockLogger{}

	rm := NewRefreshManager(logger, stdLogger, nil)

	ctx := context.Background()
	result := rm.RunFullReindex(ctx, "/nonexistent/repo/path")

	// Even on failure, type should be set correctly
	if result.Type != "full" {
		t.Errorf("expected Type='full', got %q", result.Type)
	}
}

func TestRefreshManager_RunIncrementalRefresh_SetsDuration(t *testing.T) {
	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.HumanFormat,
	})
	stdLogger := &mockLogger{}

	rm := NewRefreshManager(logger, stdLogger, nil)

	ctx := context.Background()
	result := rm.RunIncrementalRefresh(ctx, "/nonexistent/repo/path")

	// Duration should be positive even on failure
	if result.Duration <= 0 {
		t.Error("expected positive Duration")
	}
}

func TestRefreshManager_RunFullReindex_SetsDuration(t *testing.T) {
	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.HumanFormat,
	})
	stdLogger := &mockLogger{}

	rm := NewRefreshManager(logger, stdLogger, nil)

	ctx := context.Background()
	result := rm.RunFullReindex(ctx, "/nonexistent/repo/path")

	// Duration should be positive even on failure
	if result.Duration <= 0 {
		t.Error("expected positive Duration")
	}
}

func TestRefreshManager_PendingClearedAfterRefresh(t *testing.T) {
	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.HumanFormat,
	})
	stdLogger := &mockLogger{}

	rm := NewRefreshManager(logger, stdLogger, nil)
	repoPath := "/nonexistent/repo/path"

	// Run refresh (will fail but should still clear pending)
	ctx := context.Background()
	rm.RunIncrementalRefresh(ctx, repoPath)

	// Pending should be cleared after refresh completes
	if rm.HasPendingRefresh(repoPath) {
		t.Error("pending state should be cleared after refresh completes")
	}
}

func TestRefreshResult_JSONMarshaling(t *testing.T) {
	result := &RefreshResult{
		RepoPath:     "/test/repo",
		Type:         "incremental",
		Success:      true,
		Duration:     2 * time.Second,
		FilesChanged: 5,
	}

	// Marshal should work
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal should work
	var decoded RefreshResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.RepoPath != result.RepoPath {
		t.Errorf("expected RepoPath=%q, got %q", result.RepoPath, decoded.RepoPath)
	}
	if decoded.Type != result.Type {
		t.Errorf("expected Type=%q, got %q", result.Type, decoded.Type)
	}
	if !decoded.Success {
		t.Error("expected Success=true")
	}
	if decoded.FilesChanged != result.FilesChanged {
		t.Errorf("expected FilesChanged=%d, got %d", result.FilesChanged, decoded.FilesChanged)
	}
}

func TestRefreshResult_JSONMarshaling_WithError(t *testing.T) {
	result := &RefreshResult{
		RepoPath: "/test/repo",
		Type:     "full",
		Success:  false,
		Duration: 500 * time.Millisecond,
		Error:    "indexer not found",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded RefreshResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Success {
		t.Error("expected Success=false")
	}
	if decoded.Error != "indexer not found" {
		t.Errorf("expected Error='indexer not found', got %q", decoded.Error)
	}
}

func TestMockLogger_Concurrent(t *testing.T) {
	logger := &mockLogger{}
	var wg sync.WaitGroup

	// Concurrent logging should not panic
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			logger.Printf("message %d", n)
		}(i)
	}

	wg.Wait()

	logger.mu.Lock()
	count := len(logger.messages)
	logger.mu.Unlock()

	if count != 100 {
		t.Errorf("expected 100 messages, got %d", count)
	}
}
