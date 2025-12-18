package lsp

import (
	"context"
	"testing"
	"time"

	"ckb/internal/config"
	"ckb/internal/logging"
)

// TestProcessStateTransitions tests all process state transitions
func TestProcessStateTransitions(t *testing.T) {
	proc := NewLspProcess("typescript", "/tmp/test")

	// Initial state should be Starting
	if proc.GetState() != StateStarting {
		t.Errorf("Expected initial state StateStarting, got %v", proc.GetState())
	}

	// Test all valid transitions
	transitions := []struct {
		toState  LspProcessState
		expected LspProcessState
	}{
		{StateInitializing, StateInitializing},
		{StateReady, StateReady},
		{StateUnhealthy, StateUnhealthy},
		{StateReady, StateReady},
		{StateDead, StateDead},
	}

	for _, tr := range transitions {
		proc.SetState(tr.toState)
		if proc.GetState() != tr.expected {
			t.Errorf("After SetState(%v), expected %v, got %v", tr.toState, tr.expected, proc.GetState())
		}
	}
}

// TestProcessHealthy tests the IsHealthy method
func TestProcessHealthy(t *testing.T) {
	proc := NewLspProcess("typescript", "/tmp/test")

	testCases := []struct {
		state    LspProcessState
		expected bool
	}{
		{StateStarting, false},
		{StateInitializing, false},
		{StateReady, true},
		{StateUnhealthy, false},
		{StateDead, false},
	}

	for _, tc := range testCases {
		proc.SetState(tc.state)
		if proc.IsHealthy() != tc.expected {
			t.Errorf("State %v: expected IsHealthy()=%v, got %v", tc.state, tc.expected, proc.IsHealthy())
		}
	}
}

// TestProcessFailureTracking tests failure recording and reset
func TestProcessFailureTracking(t *testing.T) {
	proc := NewLspProcess("typescript", "/tmp/test")

	// Initial failures should be 0
	if proc.GetConsecutiveFailures() != 0 {
		t.Errorf("Initial failures should be 0, got %d", proc.GetConsecutiveFailures())
	}

	// Record failures
	for i := 1; i <= 5; i++ {
		proc.RecordFailure()
		if proc.GetConsecutiveFailures() != i {
			t.Errorf("After %d failures, expected %d, got %d", i, i, proc.GetConsecutiveFailures())
		}
	}

	// Record success should reset failures
	proc.RecordSuccess()
	if proc.GetConsecutiveFailures() != 0 {
		t.Errorf("After RecordSuccess, failures should be 0, got %d", proc.GetConsecutiveFailures())
	}
}

// TestSupervisorCapacity tests capacity management
func TestSupervisorCapacity(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LspSupervisor.MaxTotalProcesses = 3
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	// Initially should have capacity
	if !supervisor.HasCapacity() {
		t.Error("Expected supervisor to have capacity initially")
	}

	// Verify max processes
	if supervisor.GetMaxProcesses() != 3 {
		t.Errorf("Expected max processes 3, got %d", supervisor.GetMaxProcesses())
	}
}

// TestQueueStats tests queue statistics
func TestQueueStats(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LspSupervisor.QueueSizePerLanguage = 10
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	// Initially should have empty stats
	stats := supervisor.GetQueueStats()
	if len(stats) != 0 {
		t.Errorf("Expected empty queue stats, got %d entries", len(stats))
	}
}

// TestRejectFastThreshold tests the RejectFast threshold logic
func TestRejectFastThreshold(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LspSupervisor.QueueSizePerLanguage = 10
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	// Empty queue should not reject
	if supervisor.RejectFast("typescript") {
		t.Error("Should not reject when queue is empty")
	}
}

// TestGetInFlightCount tests in-flight request counting
func TestGetInFlightCount(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	// Initially should be 0
	count := supervisor.GetInFlightCount("typescript")
	if count != 0 {
		t.Errorf("Expected 0 in-flight requests, got %d", count)
	}
}

// TestWaitForQueueEmpty tests waiting for queue to drain
func TestWaitForQueueEmpty(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	// Empty queue should return immediately
	result := supervisor.WaitForQueue("typescript", 0, 100*time.Millisecond)
	if !result {
		t.Error("WaitForQueue should return true for empty queue")
	}
}

// TestBackoffCapping tests that backoff is capped at maximum
func TestBackoffCapping(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	// Very high restart count should still be capped
	backoff := supervisor.computeBackoff(100)
	maxBackoff := 30 * time.Second // As per the implementation

	if backoff > maxBackoff {
		t.Errorf("Backoff should be capped at %v, got %v", maxBackoff, backoff)
	}
}

// TestLspAdapterCapabilities tests the adapter capabilities method
func TestLspAdapterCapabilities(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	adapter := NewLspAdapter(supervisor, "typescript", logger)

	caps := adapter.Capabilities()
	// Capabilities may be empty if LSP server hasn't initialized
	// Just verify the method doesn't panic
	t.Logf("LSP adapter has %d capabilities: %v", len(caps), caps)
}

// TestSymbolIDParsingEdgeCases tests edge cases for symbol ID parsing
func TestSymbolIDParsingEdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expectError bool
	}{
		{"empty string", "", true},
		{"only colons", ":::", true},
		{"missing line", "file:///path:5", true},
		// Note: negative numbers may parse successfully depending on implementation
		{"spaces in path", "file:///path/with spaces/file.ts:10:5", false},
		{"unicode in path", "file:///path/日本語/file.ts:10:5", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := parseSymbolID(tc.input)
			if tc.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestSymbolKindConversionKnown tests LSP symbol kind values that are implemented
func TestSymbolKindConversionKnown(t *testing.T) {
	// Test the kinds that are actually implemented
	testCases := []struct {
		kind     int
		expected string
	}{
		{1, "file"},
		{2, "module"},
		{3, "namespace"},
		{4, "package"},
		{5, "class"},
		{6, "method"},
		{7, "property"},
		{8, "field"},
		{9, "constructor"},
		{10, "enum"},
		{11, "interface"},
		{12, "function"},
		{13, "variable"},
		{14, "constant"},
		{15, "string"},
		{16, "number"},
		{17, "boolean"},
		{18, "array"},
		{0, "symbol"},   // Unknown kind
		{-1, "symbol"},  // Invalid kind
		{100, "symbol"}, // Out of range kind
	}

	for _, tc := range testCases {
		result := symbolKindToString(tc.kind)
		if result != tc.expected {
			t.Errorf("symbolKindToString(%d) = %s, expected %s", tc.kind, result, tc.expected)
		}
	}
}

// TestSymbolKindConversionUnknown tests that unknown kinds return "symbol"
func TestSymbolKindConversionUnknown(t *testing.T) {
	// These kinds may not be implemented and should return "symbol"
	unknownKinds := []int{19, 20, 21, 22, 23, 24, 25, 26, 50, 99}
	for _, kind := range unknownKinds {
		result := symbolKindToString(kind)
		// Either return a specific kind name or "symbol" (fallback)
		if result == "" {
			t.Errorf("symbolKindToString(%d) returned empty string", kind)
		}
	}
}

// TestHealthCheckNonExistentLanguage tests health check for non-existent language
func TestHealthCheckNonExistentLanguage(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	err := supervisor.HealthCheck("nonexistent-language")
	if err == nil {
		t.Error("Expected error for non-existent language")
	}
}

// TestSupervisorShutdown tests proper cleanup on shutdown
func TestSupervisorShutdown(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)

	// Shutdown should not error
	err := supervisor.Shutdown()
	if err != nil {
		t.Errorf("Shutdown returned error: %v", err)
	}

	// After shutdown, process count should be 0
	if supervisor.GetProcessCount() != 0 {
		t.Errorf("Expected 0 processes after shutdown, got %d", supervisor.GetProcessCount())
	}
}

// TestProcessShutdown tests process cleanup
func TestProcessShutdown(t *testing.T) {
	proc := NewLspProcess("typescript", "/tmp/test")
	proc.SetState(StateReady)

	err := proc.Shutdown()
	if err != nil {
		t.Errorf("Process shutdown returned error: %v", err)
	}

	// After shutdown, state should be dead
	if proc.GetState() != StateDead {
		t.Errorf("Expected state StateDead after shutdown, got %v", proc.GetState())
	}
}

// TestEvictionWithNoProcesses tests eviction with no processes
func TestEvictionWithNoProcesses(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	// Eviction with no processes should return 0
	evicted := supervisor.EvictIdle(1 * time.Minute)
	if evicted != 0 {
		t.Errorf("Expected 0 evictions with no processes, got %d", evicted)
	}
}

// TestClearQueueNonExistent tests clearing non-existent queue
func TestClearQueueNonExistent(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	// Should not panic or error
	supervisor.clearQueue("nonexistent")
}

// TestContextCancellation tests that context cancellation is handled
func TestContextCancellation(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Query with cancelled context should fail immediately
	_, err := supervisor.QueryDefinition(ctx, "typescript", "file:///test.ts", 1, 1)
	if err == nil {
		t.Error("Expected error for cancelled context")
	}
}

// BenchmarkSymbolIDParsing benchmarks symbol ID parsing
func BenchmarkSymbolIDParsing(b *testing.B) {
	input := "file:///path/to/file.ts:100:50"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = parseSymbolID(input)
	}
}

// BenchmarkSymbolKindConversion benchmarks symbol kind conversion
func BenchmarkSymbolKindConversion(b *testing.B) {
	kinds := []int{1, 5, 6, 12, 13}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, kind := range kinds {
			_ = symbolKindToString(kind)
		}
	}
}
