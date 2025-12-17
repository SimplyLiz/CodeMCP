package lsp

import (
	"context"
	"testing"
	"time"

	"ckb/internal/config"
	"ckb/internal/logging"
)

// TestLspSupervisorCreation tests basic supervisor creation
func TestLspSupervisorCreation(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	if supervisor == nil {
		t.Fatal("Failed to create LSP supervisor")
	}

	// Check initial state
	if supervisor.GetProcessCount() != 0 {
		t.Errorf("Expected 0 processes, got %d", supervisor.GetProcessCount())
	}

	if !supervisor.HasCapacity() {
		t.Error("Expected supervisor to have capacity initially")
	}

	// Cleanup
	_ = supervisor.Shutdown()
}

// TestProcessLifecycle tests process state transitions
func TestProcessLifecycle(t *testing.T) {
	proc := NewLspProcess("typescript", "/tmp/test")

	// Check initial state
	if proc.GetState() != StateStarting {
		t.Errorf("Expected StateStarting, got %v", proc.GetState())
	}

	// Test state changes
	proc.SetState(StateInitializing)
	if proc.GetState() != StateInitializing {
		t.Errorf("Expected StateInitializing, got %v", proc.GetState())
	}

	proc.SetState(StateReady)
	if !proc.IsHealthy() {
		t.Error("Expected process to be healthy when ready")
	}

	// Test failure tracking
	proc.RecordFailure()
	if proc.GetConsecutiveFailures() != 1 {
		t.Errorf("Expected 1 failure, got %d", proc.GetConsecutiveFailures())
	}

	proc.RecordSuccess()
	if proc.GetConsecutiveFailures() != 0 {
		t.Errorf("Expected failures to be reset, got %d", proc.GetConsecutiveFailures())
	}

	// Cleanup
	_ = proc.Shutdown()
}

// TestBackoffCalculation tests exponential backoff
func TestBackoffCalculation(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	// Test backoff calculation
	testCases := []struct {
		restartCount int
		expectedMin  time.Duration
		expectedMax  time.Duration
	}{
		{0, 1 * time.Second, 1 * time.Second},
		{1, 1 * time.Second, 1 * time.Second},
		{2, 2 * time.Second, 2 * time.Second},
		{3, 4 * time.Second, 4 * time.Second},
		{4, 8 * time.Second, 8 * time.Second},
		{10, 30 * time.Second, 30 * time.Second}, // Capped at max
	}

	for _, tc := range testCases {
		backoff := supervisor.computeBackoff(tc.restartCount)
		if backoff < tc.expectedMin || backoff > tc.expectedMax {
			t.Errorf("Backoff for restart %d: expected %v-%v, got %v",
				tc.restartCount, tc.expectedMin, tc.expectedMax, backoff)
		}
	}
}

// TestQueueManagement tests request queue operations
func TestQueueManagement(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LspSupervisor.QueueSizePerLanguage = 5
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	// Check queue size
	queueSize := supervisor.getQueueSize("typescript")
	if queueSize != 0 {
		t.Errorf("Expected empty queue, got size %d", queueSize)
	}

	// Test reject fast logic
	if supervisor.RejectFast("typescript") {
		t.Error("Expected not to reject when queue is empty")
	}
}

// TestLspAdapter tests the adapter implementation
func TestLspAdapter(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	adapter := NewLspAdapter(supervisor, "typescript", logger)

	// Test backend interface methods
	if adapter.ID() != "lsp" {
		t.Errorf("Expected backend ID 'lsp', got '%s'", adapter.ID())
	}

	if adapter.Priority() != 3 {
		t.Errorf("Expected priority 3, got %d", adapter.Priority())
	}

	// LSP should be available if configured
	if !adapter.IsAvailable() {
		t.Error("Expected LSP to be available")
	}

	// Test capabilities
	caps := adapter.Capabilities()
	if len(caps) == 0 {
		t.Error("Expected some capabilities")
	}
}

// TestEviction tests LRU eviction
func TestEviction(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LspSupervisor.MaxTotalProcesses = 2
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	// Check capacity
	if supervisor.GetMaxProcesses() != 2 {
		t.Errorf("Expected max 2 processes, got %d", supervisor.GetMaxProcesses())
	}

	// Test idle eviction
	evicted := supervisor.EvictIdle(1 * time.Hour)
	if evicted != 0 {
		t.Errorf("Expected 0 evictions (no processes), got %d", evicted)
	}
}

// TestSymbolIDParsing tests symbol ID parsing
func TestSymbolIDParsing(t *testing.T) {
	testCases := []struct {
		input        string
		expectError  bool
		expectedURI  string
		expectedLine int
		expectedChar int
	}{
		{
			"file:///path/to/file.ts:10:5",
			false,
			"file:///path/to/file.ts",
			10,
			5,
		},
		{
			"file:///path:with:colons.ts:20:15",
			false,
			"file:///path:with:colons.ts",
			20,
			15,
		},
		{
			"invalid",
			true,
			"",
			0,
			0,
		},
		{
			"file:///noposition",
			true,
			"",
			0,
			0,
		},
	}

	for _, tc := range testCases {
		uri, line, char, err := parseSymbolID(tc.input)
		if tc.expectError {
			if err == nil {
				t.Errorf("Expected error for input '%s', got none", tc.input)
			}
		} else {
			if err != nil {
				t.Errorf("Unexpected error for input '%s': %v", tc.input, err)
			}
			if uri != tc.expectedURI {
				t.Errorf("Expected URI '%s', got '%s'", tc.expectedURI, uri)
			}
			if line != tc.expectedLine {
				t.Errorf("Expected line %d, got %d", tc.expectedLine, line)
			}
			if char != tc.expectedChar {
				t.Errorf("Expected char %d, got %d", tc.expectedChar, char)
			}
		}
	}
}

// TestSymbolKindConversion tests LSP symbol kind to string conversion
func TestSymbolKindConversion(t *testing.T) {
	testCases := []struct {
		kind     int
		expected string
	}{
		{1, "file"},
		{5, "class"},
		{6, "method"},
		{12, "function"},
		{13, "variable"},
		{999, "symbol"}, // Unknown kind
	}

	for _, tc := range testCases {
		result := symbolKindToString(tc.kind)
		if result != tc.expected {
			t.Errorf("Expected '%s' for kind %d, got '%s'", tc.expected, tc.kind, result)
		}
	}
}

// TestHealthChecking tests health check logic
func TestHealthChecking(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	// Get health status for non-existent process
	status := supervisor.GetHealthStatus()
	if len(status) != 0 {
		t.Errorf("Expected empty status map, got %d entries", len(status))
	}

	// Test health check on non-existent process
	err := supervisor.HealthCheck("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent process")
	}
}

// BenchmarkBackoffCalculation benchmarks backoff calculation
func BenchmarkBackoffCalculation(b *testing.B) {
	cfg := config.DefaultConfig()
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		supervisor.computeBackoff(i % 10)
	}
}

// Example of using the LSP supervisor
func ExampleLspSupervisor() {
	// Create configuration
	cfg := config.DefaultConfig()

	// Create logger
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	// Create supervisor
	supervisor := NewLspSupervisor(cfg, logger)
	defer func() { _ = supervisor.Shutdown() }()

	// Start a TypeScript LSP server
	if err := supervisor.StartServer("typescript"); err != nil {
		logger.Error("Failed to start TypeScript LSP", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// Query for definitions
	ctx := context.Background()
	result, err := supervisor.QueryDefinition(
		ctx,
		"typescript",
		"file:///path/to/file.ts",
		10, // line
		5,  // character
	)

	if err != nil {
		logger.Error("Query failed", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	logger.Info("Query succeeded", map[string]interface{}{
		"result": result,
	})
}
