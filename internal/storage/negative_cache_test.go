package storage

import (
	"testing"
)

func TestGetNegativeCachePolicy(t *testing.T) {
	testCases := []struct {
		errorType     NegativeCacheErrorType
		expectError   bool
		expectedTTL   int
		expectWarmup  bool
	}{
		{SymbolNotFound, false, 60, false},
		{BackendUnavailable, false, 15, false},
		{WorkspaceNotReady, false, 10, true},
		{Timeout, false, 5, false},
		{IndexNotFound, false, 60, false},
		{ParseError, false, 60, false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.errorType), func(t *testing.T) {
			policy, err := GetNegativeCachePolicy(tc.errorType)

			if tc.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}

				if policy.TTLSeconds != tc.expectedTTL {
					t.Errorf("Expected TTL %d, got %d", tc.expectedTTL, policy.TTLSeconds)
				}

				if policy.TriggerWarmup != tc.expectWarmup {
					t.Errorf("Expected TriggerWarmup %v, got %v", tc.expectWarmup, policy.TriggerWarmup)
				}
			}
		})
	}
}

func TestGetNegativeCachePolicyUnknown(t *testing.T) {
	_, err := GetNegativeCachePolicy("unknown-error")
	if err == nil {
		t.Error("Expected error for unknown error type")
	}
}

func TestGetNegativeCacheTTL(t *testing.T) {
	testCases := []struct {
		errorType   NegativeCacheErrorType
		expectedTTL int
	}{
		{SymbolNotFound, 60},
		{BackendUnavailable, 15},
		{WorkspaceNotReady, 10},
		{Timeout, 5},
		{IndexNotFound, 60},
		{ParseError, 60},
	}

	for _, tc := range testCases {
		t.Run(string(tc.errorType), func(t *testing.T) {
			ttl := GetNegativeCacheTTL(tc.errorType)
			if ttl != tc.expectedTTL {
				t.Errorf("Expected TTL %d, got %d", tc.expectedTTL, ttl)
			}
		})
	}
}

func TestGetNegativeCacheTTLUnknown(t *testing.T) {
	ttl := GetNegativeCacheTTL("unknown-error")
	// Should default to 60 seconds for unknown error types
	if ttl != 60 {
		t.Errorf("Expected default TTL 60, got %d", ttl)
	}
}

func TestShouldTriggerWarmup(t *testing.T) {
	testCases := []struct {
		errorType     NegativeCacheErrorType
		expectWarmup  bool
	}{
		{SymbolNotFound, false},
		{BackendUnavailable, false},
		{WorkspaceNotReady, true},
		{Timeout, false},
		{IndexNotFound, false},
		{ParseError, false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.errorType), func(t *testing.T) {
			warmup := ShouldTriggerWarmup(tc.errorType)
			if warmup != tc.expectWarmup {
				t.Errorf("Expected warmup %v, got %v", tc.expectWarmup, warmup)
			}
		})
	}
}

func TestShouldTriggerWarmupUnknown(t *testing.T) {
	warmup := ShouldTriggerWarmup("unknown-error")
	// Should return false for unknown error types
	if warmup {
		t.Error("Expected false for unknown error type")
	}
}

func TestNegativeCacheErrorTypeValues(t *testing.T) {
	// Verify the string values match expected constants
	if SymbolNotFound != "symbol-not-found" {
		t.Errorf("SymbolNotFound should be 'symbol-not-found', got '%s'", SymbolNotFound)
	}
	if BackendUnavailable != "backend-unavailable" {
		t.Errorf("BackendUnavailable should be 'backend-unavailable', got '%s'", BackendUnavailable)
	}
	if WorkspaceNotReady != "workspace-not-ready" {
		t.Errorf("WorkspaceNotReady should be 'workspace-not-ready', got '%s'", WorkspaceNotReady)
	}
	if Timeout != "timeout" {
		t.Errorf("Timeout should be 'timeout', got '%s'", Timeout)
	}
	if IndexNotFound != "index-not-found" {
		t.Errorf("IndexNotFound should be 'index-not-found', got '%s'", IndexNotFound)
	}
	if ParseError != "parse-error" {
		t.Errorf("ParseError should be 'parse-error', got '%s'", ParseError)
	}
}

func TestNegativeCachePolicyDescriptions(t *testing.T) {
	// Verify all policies have descriptions
	errorTypes := []NegativeCacheErrorType{
		SymbolNotFound,
		BackendUnavailable,
		WorkspaceNotReady,
		Timeout,
		IndexNotFound,
		ParseError,
	}

	for _, errorType := range errorTypes {
		policy, err := GetNegativeCachePolicy(errorType)
		if err != nil {
			t.Errorf("Unexpected error for %s: %v", errorType, err)
			continue
		}

		if policy.Description == "" {
			t.Errorf("Policy for %s should have a description", errorType)
		}
	}
}

func TestNegativeCacheManagerInvalidateError(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer teardownTestDB(t, db, tmpDir)

	cache := NewCache(db)
	manager := NewNegativeCacheManager(cache)

	// Cache an error
	err := manager.CacheError("test-key", SymbolNotFound, "Symbol not found", "state-1")
	if err != nil {
		t.Fatalf("Failed to cache error: %v", err)
	}

	// Verify it exists
	entry, err := manager.CheckError("test-key", "state-1")
	if err != nil {
		t.Fatalf("Failed to check error: %v", err)
	}
	if entry == nil {
		t.Fatal("Expected error to be cached")
	}

	// Invalidate it
	err = manager.InvalidateError("test-key")
	if err != nil {
		t.Fatalf("Failed to invalidate error: %v", err)
	}

	// Verify it's gone
	entry, err = manager.CheckError("test-key", "state-1")
	if err != nil {
		t.Fatalf("Failed to check error after invalidation: %v", err)
	}
	if entry != nil {
		t.Error("Expected error to be invalidated")
	}
}

func TestNegativeCacheManagerInvalidateAllErrors(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer teardownTestDB(t, db, tmpDir)

	cache := NewCache(db)
	manager := NewNegativeCacheManager(cache)

	// Cache multiple errors
	errors := []struct {
		key       string
		errorType NegativeCacheErrorType
		message   string
	}{
		{"key-1", SymbolNotFound, "Symbol 1 not found"},
		{"key-2", BackendUnavailable, "Backend unavailable"},
		{"key-3", Timeout, "Request timed out"},
	}

	for _, e := range errors {
		err := manager.CacheError(e.key, e.errorType, e.message, "state-1")
		if err != nil {
			t.Fatalf("Failed to cache error %s: %v", e.key, err)
		}
	}

	// Verify all exist
	for _, e := range errors {
		entry, err := manager.CheckError(e.key, "state-1")
		if err != nil {
			t.Fatalf("Failed to check error %s: %v", e.key, err)
		}
		if entry == nil {
			t.Fatalf("Expected error %s to be cached", e.key)
		}
	}

	// Invalidate all
	err := manager.InvalidateAllErrors()
	if err != nil {
		t.Fatalf("Failed to invalidate all errors: %v", err)
	}

	// Verify all are gone
	for _, e := range errors {
		entry, err := manager.CheckError(e.key, "state-1")
		if err != nil {
			t.Fatalf("Failed to check error %s after invalidation: %v", e.key, err)
		}
		if entry != nil {
			t.Errorf("Expected error %s to be invalidated", e.key)
		}
	}
}

func TestNegativeCacheManagerWorkspaceNotReadyTrigger(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer teardownTestDB(t, db, tmpDir)

	cache := NewCache(db)
	manager := NewNegativeCacheManager(cache)

	// Cache a WorkspaceNotReady error (should trigger warmup)
	err := manager.CacheError("workspace-key", WorkspaceNotReady, "Workspace not initialized", "state-1")
	if err != nil {
		t.Fatalf("Failed to cache error: %v", err)
	}

	// Verify it was cached
	entry, err := manager.CheckError("workspace-key", "state-1")
	if err != nil {
		t.Fatalf("Failed to check error: %v", err)
	}

	if entry == nil {
		t.Fatal("Expected error to be cached")
	}

	if entry.ErrorType != string(WorkspaceNotReady) {
		t.Errorf("Expected error type '%s', got '%s'", WorkspaceNotReady, entry.ErrorType)
	}
}

func TestCacheStatsByErrorType(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer teardownTestDB(t, db, tmpDir)

	cache := NewCache(db)
	manager := NewNegativeCacheManager(cache)

	// Cache errors of different types
	testErrors := []struct {
		key       string
		errorType NegativeCacheErrorType
	}{
		{"sym-1", SymbolNotFound},
		{"sym-2", SymbolNotFound},
		{"sym-3", SymbolNotFound},
		{"backend-1", BackendUnavailable},
		{"backend-2", BackendUnavailable},
		{"timeout-1", Timeout},
	}

	for _, te := range testErrors {
		err := manager.CacheError(te.key, te.errorType, "test message", "state-1")
		if err != nil {
			t.Fatalf("Failed to cache error %s: %v", te.key, err)
		}
	}

	// Get stats
	stats, err := manager.GetErrorStats()
	if err != nil {
		t.Fatalf("Failed to get error stats: %v", err)
	}

	// Verify counts
	if stats[string(SymbolNotFound)] != 3 {
		t.Errorf("Expected 3 SymbolNotFound errors, got %d", stats[string(SymbolNotFound)])
	}
	if stats[string(BackendUnavailable)] != 2 {
		t.Errorf("Expected 2 BackendUnavailable errors, got %d", stats[string(BackendUnavailable)])
	}
	if stats[string(Timeout)] != 1 {
		t.Errorf("Expected 1 Timeout error, got %d", stats[string(Timeout)])
	}
}

func TestNegativeCacheStateIsolation(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer teardownTestDB(t, db, tmpDir)

	cache := NewCache(db)
	manager := NewNegativeCacheManager(cache)

	// Cache an error with state-1
	err := manager.CacheError("key-1", SymbolNotFound, "Not found", "state-1")
	if err != nil {
		t.Fatalf("Failed to cache error: %v", err)
	}

	// Check with state-1 - should find it
	entry, err := manager.CheckError("key-1", "state-1")
	if err != nil {
		t.Fatalf("Failed to check error: %v", err)
	}
	if entry == nil {
		t.Error("Expected error to be found with state-1")
	}

	// Check with state-2 - should not find it (different state)
	entry, err = manager.CheckError("key-1", "state-2")
	if err != nil {
		t.Fatalf("Failed to check error: %v", err)
	}
	if entry != nil {
		t.Error("Expected error NOT to be found with state-2")
	}
}

// BenchmarkNegativeCacheTTLLookup benchmarks TTL lookup
func BenchmarkNegativeCacheTTLLookup(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = GetNegativeCacheTTL(SymbolNotFound)
	}
}

// BenchmarkShouldTriggerWarmup benchmarks warmup check
func BenchmarkShouldTriggerWarmup(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = ShouldTriggerWarmup(WorkspaceNotReady)
	}
}
