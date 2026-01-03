package backends

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// mockBackend is a test double for Backend interface
type mockBackend struct {
	id           BackendID
	available    bool
	healthy      bool
	capabilities []string
	priority     int

	// For tracking calls
	mu          sync.Mutex
	searchCalls int
	refCalls    int
	symbolCalls int

	// Configurable responses
	symbolResult     *SymbolResult
	searchResult     *SearchResult
	referencesResult *ReferencesResult
	err              error
	delay            time.Duration
	closed           bool
}

func newMockBackend(id BackendID) *mockBackend {
	return &mockBackend{
		id:           id,
		available:    true,
		healthy:      true,
		capabilities: []string{"symbol-search", "find-references", "symbol-info", "goto-definition"},
		priority:     1,
	}
}

func (m *mockBackend) ID() BackendID {
	return m.id
}

func (m *mockBackend) IsAvailable() bool {
	return m.available
}

func (m *mockBackend) IsHealthy() bool {
	return m.healthy
}

func (m *mockBackend) Capabilities() []string {
	return m.capabilities
}

func (m *mockBackend) Priority() int {
	return m.priority
}

func (m *mockBackend) GetSymbol(ctx context.Context, id string) (*SymbolResult, error) {
	m.mu.Lock()
	m.symbolCalls++
	m.mu.Unlock()

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.err != nil {
		return nil, m.err
	}
	if m.symbolResult != nil {
		return m.symbolResult, nil
	}
	return &SymbolResult{
		StableID: id,
		Name:     "TestSymbol",
		Kind:     "function",
		Completeness: CompletenessInfo{
			Score: 1.0,
		},
	}, nil
}

func (m *mockBackend) SearchSymbols(ctx context.Context, query string, opts SearchOptions) (*SearchResult, error) {
	m.mu.Lock()
	m.searchCalls++
	m.mu.Unlock()

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.err != nil {
		return nil, m.err
	}
	if m.searchResult != nil {
		return m.searchResult, nil
	}
	return &SearchResult{
		Symbols: []SymbolResult{
			{StableID: "test:sym:1", Name: query, Kind: "function"},
		},
		TotalMatches: 1,
		Completeness: CompletenessInfo{
			Score: 1.0,
		},
	}, nil
}

func (m *mockBackend) FindReferences(ctx context.Context, symbolID string, opts RefOptions) (*ReferencesResult, error) {
	m.mu.Lock()
	m.refCalls++
	m.mu.Unlock()

	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if m.err != nil {
		return nil, m.err
	}
	if m.referencesResult != nil {
		return m.referencesResult, nil
	}
	return &ReferencesResult{
		References: []Reference{
			{Location: Location{Path: "test.go", Line: 10}, Kind: "call"},
		},
		TotalReferences: 1,
		Completeness: CompletenessInfo{
			Score: 1.0,
		},
	}, nil
}

func (m *mockBackend) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

//nolint:unused // test helper for future use
func (m *mockBackend) getCalls() (symbol, search, ref int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.symbolCalls, m.searchCalls, m.refCalls
}

func createTestOrchestrator(t *testing.T) *Orchestrator {
	t.Helper()
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	policy := DefaultQueryPolicy()
	return NewOrchestrator(policy, logger)
}

func TestNewOrchestrator(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	policy := DefaultQueryPolicy()

	orch := NewOrchestrator(policy, logger)

	if orch == nil {
		t.Fatal("Expected non-nil orchestrator")
	}
	if orch.backends == nil {
		t.Error("Expected backends map to be initialized")
	}
	if orch.policy != policy {
		t.Error("Expected policy to be set")
	}
	if orch.limiter == nil {
		t.Error("Expected limiter to be initialized")
	}
	if orch.ladder == nil {
		t.Error("Expected ladder to be initialized")
	}
	if orch.preferFirstMerger == nil {
		t.Error("Expected preferFirstMerger to be initialized")
	}
	if orch.unionMerger == nil {
		t.Error("Expected unionMerger to be initialized")
	}
}

func TestRegisterBackend(t *testing.T) {
	orch := createTestOrchestrator(t)
	backend := newMockBackend(BackendSCIP)

	orch.RegisterBackend(backend)

	retrieved, ok := orch.GetBackend(BackendSCIP)
	if !ok {
		t.Fatal("Expected backend to be registered")
	}
	if retrieved.ID() != BackendSCIP {
		t.Errorf("Expected backend ID %s, got %s", BackendSCIP, retrieved.ID())
	}
}

func TestUnregisterBackend(t *testing.T) {
	orch := createTestOrchestrator(t)
	backend := newMockBackend(BackendSCIP)

	orch.RegisterBackend(backend)
	orch.UnregisterBackend(BackendSCIP)

	_, ok := orch.GetBackend(BackendSCIP)
	if ok {
		t.Error("Expected backend to be unregistered")
	}
}

func TestGetAvailableBackends(t *testing.T) {
	orch := createTestOrchestrator(t)

	// Add available and unavailable backends
	available := newMockBackend(BackendSCIP)
	available.available = true

	unavailable := newMockBackend(BackendLSP)
	unavailable.available = false

	orch.RegisterBackend(available)
	orch.RegisterBackend(unavailable)

	backends := orch.GetAvailableBackends()

	if len(backends) != 1 {
		t.Errorf("Expected 1 available backend, got %d", len(backends))
	}
	if backends[0] != BackendSCIP {
		t.Errorf("Expected available backend to be %s, got %s", BackendSCIP, backends[0])
	}
}

func TestGetSymbolBackend(t *testing.T) {
	orch := createTestOrchestrator(t)
	backend := newMockBackend(BackendSCIP)

	orch.RegisterBackend(backend)

	symbolBackend, ok := orch.GetSymbolBackend(BackendSCIP)
	if !ok {
		t.Fatal("Expected to get symbol backend")
	}
	if symbolBackend == nil {
		t.Error("Expected non-nil symbol backend")
	}

	// Test non-existent backend
	_, ok = orch.GetSymbolBackend("nonexistent")
	if ok {
		t.Error("Expected false for non-existent backend")
	}
}

func TestQueryNoBackends(t *testing.T) {
	orch := createTestOrchestrator(t)

	_, err := orch.Query(context.Background(), QueryRequest{
		Type:  QueryTypeSearch,
		Query: "test",
	})

	if err == nil {
		t.Fatal("Expected error when no backends available")
	}
}

func TestQueryAllBackendsFail(t *testing.T) {
	orch := createTestOrchestrator(t)

	// Add a backend that returns errors
	failingBackend := newMockBackend(BackendSCIP)
	failingBackend.err = fmt.Errorf("backend error")

	orch.RegisterBackend(failingBackend)

	_, err := orch.Query(context.Background(), QueryRequest{
		Type:  QueryTypeSearch,
		Query: "test",
	})

	if err == nil {
		t.Fatal("Expected error when all backends fail")
	}
}

func TestQuerySymbol(t *testing.T) {
	orch := createTestOrchestrator(t)
	backend := newMockBackend(BackendSCIP)
	backend.symbolResult = &SymbolResult{
		StableID: "ckb:test:sym:abc",
		Name:     "TestFunc",
		Kind:     "function",
		Completeness: CompletenessInfo{
			Score: 0.9,
		},
	}

	orch.RegisterBackend(backend)

	result, err := orch.Query(context.Background(), QueryRequest{
		Type:     QueryTypeSymbol,
		SymbolID: "ckb:test:sym:abc",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	symbolResult, ok := result.Data.(*SymbolResult)
	if !ok {
		t.Fatalf("Expected SymbolResult, got %T", result.Data)
	}
	if symbolResult.Name != "TestFunc" {
		t.Errorf("Expected symbol name TestFunc, got %s", symbolResult.Name)
	}
}

func TestQuerySearch(t *testing.T) {
	orch := createTestOrchestrator(t)
	backend := newMockBackend(BackendSCIP)
	backend.searchResult = &SearchResult{
		Symbols: []SymbolResult{
			{StableID: "sym1", Name: "Foo", Kind: "function"},
			{StableID: "sym2", Name: "FooBar", Kind: "function"},
		},
		TotalMatches: 2,
		Completeness: CompletenessInfo{
			Score: 1.0,
		},
	}

	orch.RegisterBackend(backend)

	result, err := orch.Query(context.Background(), QueryRequest{
		Type:  QueryTypeSearch,
		Query: "Foo",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	searchResult, ok := result.Data.(*SearchResult)
	if !ok {
		t.Fatalf("Expected SearchResult, got %T", result.Data)
	}
	if len(searchResult.Symbols) != 2 {
		t.Errorf("Expected 2 symbols, got %d", len(searchResult.Symbols))
	}
}

func TestQueryReferences(t *testing.T) {
	orch := createTestOrchestrator(t)
	backend := newMockBackend(BackendSCIP)
	backend.referencesResult = &ReferencesResult{
		References: []Reference{
			{Location: Location{Path: "main.go", Line: 10}, Kind: "call"},
			{Location: Location{Path: "test.go", Line: 20}, Kind: "read"},
		},
		TotalReferences: 2,
		Completeness: CompletenessInfo{
			Score: 1.0,
		},
	}

	orch.RegisterBackend(backend)

	result, err := orch.Query(context.Background(), QueryRequest{
		Type:     QueryTypeReferences,
		SymbolID: "ckb:test:sym:abc",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	refResult, ok := result.Data.(*ReferencesResult)
	if !ok {
		t.Fatalf("Expected ReferencesResult, got %T", result.Data)
	}
	if len(refResult.References) != 2 {
		t.Errorf("Expected 2 references, got %d", len(refResult.References))
	}
}

func TestQueryWithUnionMergeMode(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	policy := DefaultQueryPolicy()
	policy.MergeMode = MergeModeUnion

	orch := NewOrchestrator(policy, logger)

	backend1 := newMockBackend(BackendSCIP)
	backend1.searchResult = &SearchResult{
		Symbols: []SymbolResult{
			{StableID: "sym1", Name: "Foo", Kind: "function"},
		},
		TotalMatches: 1,
		Completeness: CompletenessInfo{Score: 1.0},
	}

	backend2 := newMockBackend(BackendLSP)
	backend2.priority = 2
	backend2.searchResult = &SearchResult{
		Symbols: []SymbolResult{
			{StableID: "sym2", Name: "Bar", Kind: "function"},
		},
		TotalMatches: 1,
		Completeness: CompletenessInfo{Score: 1.0},
	}

	orch.RegisterBackend(backend1)
	orch.RegisterBackend(backend2)

	result, err := orch.Query(context.Background(), QueryRequest{
		Type:  QueryTypeSearch,
		Query: "test",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.Provenance.MergeMode != MergeModeUnion {
		t.Errorf("Expected union merge mode in provenance, got %s", result.Provenance.MergeMode)
	}
}

func TestQueryContributions(t *testing.T) {
	orch := createTestOrchestrator(t)

	backend := newMockBackend(BackendSCIP)
	backend.searchResult = &SearchResult{
		Symbols: []SymbolResult{
			{StableID: "sym1", Name: "Foo", Kind: "function"},
		},
		TotalMatches: 1,
		Completeness: CompletenessInfo{Score: 1.0},
	}

	orch.RegisterBackend(backend)

	result, err := orch.Query(context.Background(), QueryRequest{
		Type:  QueryTypeSearch,
		Query: "Foo",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(result.Contributions) == 0 {
		t.Fatal("Expected at least one contribution")
	}

	found := false
	for _, contrib := range result.Contributions {
		if contrib.BackendID == BackendSCIP {
			found = true
			if !contrib.WasUsed {
				t.Error("Expected SCIP contribution to be marked as used")
			}
			if contrib.ItemCount != 1 {
				t.Errorf("Expected item count of 1, got %d", contrib.ItemCount)
			}
		}
	}
	if !found {
		t.Error("Expected SCIP backend in contributions")
	}
}

func TestQueryWithContext(t *testing.T) {
	orch := createTestOrchestrator(t)

	backend := newMockBackend(BackendSCIP)
	backend.delay = 500 * time.Millisecond

	orch.RegisterBackend(backend)

	// Create a context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := orch.Query(ctx, QueryRequest{
		Type:  QueryTypeSearch,
		Query: "test",
	})

	// Should fail due to all backends timing out
	if err == nil {
		t.Error("Expected error due to context timeout")
	}
}

func TestShutdown(t *testing.T) {
	orch := createTestOrchestrator(t)

	backend := newMockBackend(BackendSCIP)
	orch.RegisterBackend(backend)

	err := orch.Shutdown()
	if err != nil {
		t.Fatalf("Unexpected shutdown error: %v", err)
	}

	if !backend.closed {
		t.Error("Expected backend to be closed")
	}

	// Backends should be cleared
	backends := orch.GetAvailableBackends()
	if len(backends) != 0 {
		t.Errorf("Expected no backends after shutdown, got %d", len(backends))
	}
}

func TestIsHealthy(t *testing.T) {
	orch := createTestOrchestrator(t)

	// With no backends, should be healthy
	if !orch.IsHealthy() {
		t.Error("Expected healthy with no backends")
	}

	// Add healthy backend
	healthy := newMockBackend(BackendSCIP)
	healthy.healthy = true
	orch.RegisterBackend(healthy)

	if !orch.IsHealthy() {
		t.Error("Expected healthy with all healthy backends")
	}

	// Add unhealthy backend
	unhealthy := newMockBackend(BackendLSP)
	unhealthy.healthy = false
	orch.RegisterBackend(unhealthy)

	if orch.IsHealthy() {
		t.Error("Expected unhealthy when any backend is unhealthy")
	}
}

func TestConcurrentRegistration(t *testing.T) {
	orch := createTestOrchestrator(t)

	var wg sync.WaitGroup
	numGoroutines := 10

	// Concurrently register backends
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			backend := newMockBackend(BackendID(fmt.Sprintf("backend-%d", i)))
			orch.RegisterBackend(backend)
		}(i)
	}

	wg.Wait()

	// All should be registered
	available := orch.GetAvailableBackends()
	if len(available) != numGoroutines {
		t.Errorf("Expected %d backends, got %d", numGoroutines, len(available))
	}
}

func TestQueryDuration(t *testing.T) {
	orch := createTestOrchestrator(t)

	backend := newMockBackend(BackendSCIP)
	backend.delay = 10 * time.Millisecond

	orch.RegisterBackend(backend)

	result, err := orch.Query(context.Background(), QueryRequest{
		Type:  QueryTypeSearch,
		Query: "test",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.TotalDurationMs < 10 {
		t.Errorf("Expected duration >= 10ms, got %d", result.TotalDurationMs)
	}
}

func TestQueryWithSearchOptions(t *testing.T) {
	orch := createTestOrchestrator(t)
	backend := newMockBackend(BackendSCIP)

	orch.RegisterBackend(backend)

	_, err := orch.Query(context.Background(), QueryRequest{
		Type:  QueryTypeSearch,
		Query: "test",
		SearchOpts: &SearchOptions{
			MaxResults:   50,
			IncludeTests: true,
			Kind:         []string{"function", "method"},
		},
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestQueryWithRefOptions(t *testing.T) {
	orch := createTestOrchestrator(t)
	backend := newMockBackend(BackendSCIP)

	orch.RegisterBackend(backend)

	_, err := orch.Query(context.Background(), QueryRequest{
		Type:     QueryTypeReferences,
		SymbolID: "test:sym:1",
		RefOpts: &RefOptions{
			MaxResults:         100,
			IncludeTests:       true,
			IncludeDeclaration: true,
		},
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
}

func TestBuildContributionsWithErrors(t *testing.T) {
	orch := createTestOrchestrator(t)

	results := []BackendResult{
		{
			BackendID:  BackendSCIP,
			Error:      nil,
			Data:       &SearchResult{Symbols: []SymbolResult{{StableID: "sym1"}}},
			DurationMs: 50,
		},
		{
			BackendID:  BackendLSP,
			Error:      fmt.Errorf("connection timeout"),
			DurationMs: 5000,
		},
	}

	contributions := orch.buildContributions(results)

	if len(contributions) != 2 {
		t.Fatalf("Expected 2 contributions, got %d", len(contributions))
	}

	// First should be successful
	if contributions[0].BackendID != BackendSCIP {
		t.Errorf("Expected first contribution from SCIP, got %s", contributions[0].BackendID)
	}
	if !contributions[0].WasUsed {
		t.Error("Expected SCIP contribution to be used")
	}
	if contributions[0].Error != "" {
		t.Errorf("Expected no error for SCIP, got %s", contributions[0].Error)
	}

	// Second should show error
	if contributions[1].BackendID != BackendLSP {
		t.Errorf("Expected second contribution from LSP, got %s", contributions[1].BackendID)
	}
	if contributions[1].WasUsed {
		t.Error("Expected LSP contribution not to be used")
	}
	if contributions[1].Error != "connection timeout" {
		t.Errorf("Expected timeout error, got %s", contributions[1].Error)
	}
}

func TestBuildContributionsItemCount(t *testing.T) {
	orch := createTestOrchestrator(t)

	testCases := []struct {
		name          string
		data          interface{}
		expectedCount int
	}{
		{
			name: "symbol result",
			data: &SymbolResult{
				StableID: "sym1",
				Name:     "Test",
			},
			expectedCount: 1,
		},
		{
			name: "search result with multiple symbols",
			data: &SearchResult{
				Symbols: []SymbolResult{
					{StableID: "sym1"},
					{StableID: "sym2"},
					{StableID: "sym3"},
				},
			},
			expectedCount: 3,
		},
		{
			name: "references result with multiple refs",
			data: &ReferencesResult{
				References: []Reference{
					{SymbolID: "ref1"},
					{SymbolID: "ref2"},
				},
			},
			expectedCount: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			results := []BackendResult{
				{
					BackendID: BackendSCIP,
					Data:      tc.data,
				},
			}

			contributions := orch.buildContributions(results)
			if contributions[0].ItemCount != tc.expectedCount {
				t.Errorf("Expected item count %d, got %d", tc.expectedCount, contributions[0].ItemCount)
			}
		})
	}
}

// =============================================================================
// Backend Fallback Integration Tests
// =============================================================================
// These tests verify the orchestrator's parallel query behavior with multiple backends.
// In prefer-first mode: only primary backend is queried (no fallback on failure)
// In union mode: all backends are queried in parallel, results merged

func TestFallback_UnionMode_SCIPFails_LSPSucceeds(t *testing.T) {
	// Scenario: Union mode - SCIP fails, LSP succeeds
	// Expected: Should return LSP results (both queried in parallel)
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	policy := DefaultQueryPolicy()
	policy.MergeMode = MergeModeUnion

	orch := NewOrchestrator(policy, logger)

	// Primary backend fails
	scipBackend := newMockBackend(BackendSCIP)
	scipBackend.priority = 1
	scipBackend.err = fmt.Errorf("SCIP index not found")

	// Secondary backend succeeds
	lspBackend := newMockBackend(BackendLSP)
	lspBackend.priority = 2
	lspBackend.searchResult = &SearchResult{
		Symbols: []SymbolResult{
			{StableID: "lsp:sym:1", Name: "FallbackFunc", Kind: "function"},
		},
		TotalMatches: 1,
		Completeness: CompletenessInfo{
			Score: 0.8,
		},
	}

	orch.RegisterBackend(scipBackend)
	orch.RegisterBackend(lspBackend)

	result, err := orch.Query(context.Background(), QueryRequest{
		Type:  QueryTypeSearch,
		Query: "Fallback",
	})

	if err != nil {
		t.Fatalf("Expected query to succeed with LSP, got error: %v", err)
	}

	// Verify we got LSP results
	searchResult, ok := result.Data.(*SearchResult)
	if !ok {
		t.Fatalf("Expected SearchResult, got %T", result.Data)
	}
	if len(searchResult.Symbols) != 1 {
		t.Errorf("Expected 1 symbol from LSP, got %d", len(searchResult.Symbols))
	}
	if searchResult.Symbols[0].Name != "FallbackFunc" {
		t.Errorf("Expected FallbackFunc, got %s", searchResult.Symbols[0].Name)
	}

	// Verify contributions track both backends
	scipContrib := findContribution(result.Contributions, BackendSCIP)
	lspContrib := findContribution(result.Contributions, BackendLSP)

	if scipContrib == nil {
		t.Error("Expected SCIP in contributions")
	} else {
		if scipContrib.WasUsed {
			t.Error("SCIP contribution should not be marked as used (it failed)")
		}
		if scipContrib.Error == "" {
			t.Error("SCIP contribution should have error message")
		}
	}

	if lspContrib == nil {
		t.Error("Expected LSP in contributions")
	} else {
		if !lspContrib.WasUsed {
			t.Error("LSP contribution should be marked as used")
		}
		if lspContrib.ItemCount != 1 {
			t.Errorf("Expected LSP item count of 1, got %d", lspContrib.ItemCount)
		}
	}
}

func TestFallback_PreferFirst_PrimaryFails_NoFallback(t *testing.T) {
	// Scenario: Prefer-first mode - only primary is queried, no fallback on failure
	// Expected: Query fails since primary failed and no fallback is attempted
	orch := createTestOrchestrator(t)

	scipBackend := newMockBackend(BackendSCIP)
	scipBackend.priority = 1
	scipBackend.err = fmt.Errorf("SCIP index not found")

	// LSP is registered but won't be queried in prefer-first mode
	lspBackend := newMockBackend(BackendLSP)
	lspBackend.priority = 2
	lspBackend.searchResult = &SearchResult{
		Symbols: []SymbolResult{
			{StableID: "lsp:sym:1", Name: "FallbackFunc", Kind: "function"},
		},
		TotalMatches: 1,
	}

	orch.RegisterBackend(scipBackend)
	orch.RegisterBackend(lspBackend)

	_, err := orch.Query(context.Background(), QueryRequest{
		Type:  QueryTypeSearch,
		Query: "Fallback",
	})

	// In prefer-first mode, only primary is queried - if it fails, the query fails
	if err == nil {
		t.Error("Expected error in prefer-first mode when primary fails")
	}
}

func TestFallback_UnionMode_SCIPTimeout_LSPSucceeds(t *testing.T) {
	// Scenario: Union mode - SCIP times out, LSP responds in time
	// Expected: Should return LSP results (both queried in parallel)
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	policy := DefaultQueryPolicy()
	policy.MergeMode = MergeModeUnion

	orch := NewOrchestrator(policy, logger)

	// SCIP backend with long delay (will be canceled by context)
	scipBackend := newMockBackend(BackendSCIP)
	scipBackend.priority = 1
	scipBackend.delay = 500 * time.Millisecond

	// LSP backend responds quickly
	lspBackend := newMockBackend(BackendLSP)
	lspBackend.priority = 2
	lspBackend.delay = 10 * time.Millisecond
	lspBackend.searchResult = &SearchResult{
		Symbols: []SymbolResult{
			{StableID: "lsp:sym:fast", Name: "FastResponse", Kind: "function"},
		},
		TotalMatches: 1,
		Completeness: CompletenessInfo{Score: 0.7},
	}

	orch.RegisterBackend(scipBackend)
	orch.RegisterBackend(lspBackend)

	// Use context with timeout shorter than SCIP delay but longer than LSP
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := orch.Query(ctx, QueryRequest{
		Type:  QueryTypeSearch,
		Query: "Fast",
	})

	// Both backends were queried in parallel, SCIP was canceled, LSP succeeded
	if err != nil {
		t.Fatalf("Expected LSP to succeed within timeout, got error: %v", err)
	}

	searchResult, ok := result.Data.(*SearchResult)
	if !ok {
		t.Fatalf("Expected SearchResult, got %T", result.Data)
	}
	if len(searchResult.Symbols) == 0 {
		t.Error("Expected at least one symbol from LSP")
	}
}

func TestFallback_UnionMode_MultipleBackends_PartialFailure(t *testing.T) {
	// Scenario: Union mode - SCIP fails, LSP fails, Git heuristics succeeds
	// Expected: Should return heuristic results (all queried in parallel)
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	policy := DefaultQueryPolicy()
	policy.MergeMode = MergeModeUnion

	orch := NewOrchestrator(policy, logger)

	// SCIP fails
	scipBackend := newMockBackend(BackendSCIP)
	scipBackend.priority = 1
	scipBackend.err = fmt.Errorf("SCIP unavailable")

	// LSP also fails
	lspBackend := newMockBackend(BackendLSP)
	lspBackend.priority = 2
	lspBackend.err = fmt.Errorf("LSP connection refused")

	// Git heuristics succeeds
	gitBackend := newMockBackend(BackendGit)
	gitBackend.priority = 3
	gitBackend.searchResult = &SearchResult{
		Symbols: []SymbolResult{
			{StableID: "git:heuristic:1", Name: "HeuristicMatch", Kind: "unknown"},
		},
		TotalMatches: 1,
		Completeness: CompletenessInfo{
			Score: 0.3, // Lower confidence from heuristics
		},
	}

	orch.RegisterBackend(scipBackend)
	orch.RegisterBackend(lspBackend)
	orch.RegisterBackend(gitBackend)

	result, err := orch.Query(context.Background(), QueryRequest{
		Type:  QueryTypeSearch,
		Query: "Heuristic",
	})

	if err != nil {
		t.Fatalf("Expected query to succeed with Git, got error: %v", err)
	}

	searchResult, ok := result.Data.(*SearchResult)
	if !ok {
		t.Fatalf("Expected SearchResult, got %T", result.Data)
	}
	if len(searchResult.Symbols) != 1 || searchResult.Symbols[0].Name != "HeuristicMatch" {
		t.Error("Expected heuristic result from Git backend")
	}

	// All three backends should be in contributions
	if len(result.Contributions) < 3 {
		t.Errorf("Expected 3 contributions, got %d", len(result.Contributions))
	}
}

func TestFallback_PartialResults_PreferFirst(t *testing.T) {
	// Scenario: Both backends succeed, prefer-first mode
	// Expected: Should return SCIP results, LSP ignored
	orch := createTestOrchestrator(t)

	scipBackend := newMockBackend(BackendSCIP)
	scipBackend.priority = 1
	scipBackend.searchResult = &SearchResult{
		Symbols: []SymbolResult{
			{StableID: "scip:sym:1", Name: "SCIPResult", Kind: "function"},
		},
		TotalMatches: 1,
		Completeness: CompletenessInfo{Score: 1.0},
	}

	lspBackend := newMockBackend(BackendLSP)
	lspBackend.priority = 2
	lspBackend.searchResult = &SearchResult{
		Symbols: []SymbolResult{
			{StableID: "lsp:sym:1", Name: "LSPResult", Kind: "function"},
		},
		TotalMatches: 1,
		Completeness: CompletenessInfo{Score: 0.8},
	}

	orch.RegisterBackend(scipBackend)
	orch.RegisterBackend(lspBackend)

	result, err := orch.Query(context.Background(), QueryRequest{
		Type:  QueryTypeSearch,
		Query: "Result",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	searchResult, ok := result.Data.(*SearchResult)
	if !ok {
		t.Fatalf("Expected SearchResult, got %T", result.Data)
	}
	// In prefer-first mode, should only have SCIP results
	if len(searchResult.Symbols) != 1 {
		t.Errorf("Expected 1 symbol in prefer-first mode, got %d", len(searchResult.Symbols))
	}
	if searchResult.Symbols[0].Name != "SCIPResult" {
		t.Errorf("Expected SCIPResult in prefer-first mode, got %s", searchResult.Symbols[0].Name)
	}
}

func TestFallback_UnionMode_GetSymbol_PrimaryFails(t *testing.T) {
	// Scenario: Union mode - GetSymbol on SCIP fails, LSP succeeds
	// Expected: Should return LSP symbol info (both queried in parallel)
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	policy := DefaultQueryPolicy()
	policy.MergeMode = MergeModeUnion

	orch := NewOrchestrator(policy, logger)

	scipBackend := newMockBackend(BackendSCIP)
	scipBackend.priority = 1
	scipBackend.err = fmt.Errorf("symbol not indexed")

	lspBackend := newMockBackend(BackendLSP)
	lspBackend.priority = 2
	lspBackend.symbolResult = &SymbolResult{
		StableID: "lsp:sym:fallback",
		Name:     "FallbackSymbol",
		Kind:     "variable",
		Completeness: CompletenessInfo{
			Score: 0.7,
		},
	}

	orch.RegisterBackend(scipBackend)
	orch.RegisterBackend(lspBackend)

	result, err := orch.Query(context.Background(), QueryRequest{
		Type:     QueryTypeSymbol,
		SymbolID: "ckb:test:sym:xyz",
	})

	if err != nil {
		t.Fatalf("Expected GetSymbol to succeed with LSP, got error: %v", err)
	}

	symbolResult, ok := result.Data.(*SymbolResult)
	if !ok {
		t.Fatalf("Expected SymbolResult, got %T", result.Data)
	}
	if symbolResult.Name != "FallbackSymbol" {
		t.Errorf("Expected FallbackSymbol from LSP, got %s", symbolResult.Name)
	}
}

func TestFallback_UnionMode_FindReferences_PrimaryFails(t *testing.T) {
	// Scenario: Union mode - FindReferences on SCIP fails, LSP succeeds
	// Expected: Should return LSP references (both queried in parallel)
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	policy := DefaultQueryPolicy()
	policy.MergeMode = MergeModeUnion

	orch := NewOrchestrator(policy, logger)

	scipBackend := newMockBackend(BackendSCIP)
	scipBackend.priority = 1
	scipBackend.err = fmt.Errorf("references not available")

	lspBackend := newMockBackend(BackendLSP)
	lspBackend.priority = 2
	lspBackend.referencesResult = &ReferencesResult{
		References: []Reference{
			{Location: Location{Path: "fallback.go", Line: 42}, Kind: "call"},
			{Location: Location{Path: "fallback.go", Line: 99}, Kind: "read"},
		},
		TotalReferences: 2,
		Completeness: CompletenessInfo{
			Score: 0.6,
		},
	}

	orch.RegisterBackend(scipBackend)
	orch.RegisterBackend(lspBackend)

	result, err := orch.Query(context.Background(), QueryRequest{
		Type:     QueryTypeReferences,
		SymbolID: "ckb:test:sym:ref",
	})

	if err != nil {
		t.Fatalf("Expected FindReferences to succeed with LSP, got error: %v", err)
	}

	refResult, ok := result.Data.(*ReferencesResult)
	if !ok {
		t.Fatalf("Expected ReferencesResult, got %T", result.Data)
	}
	if len(refResult.References) != 2 {
		t.Errorf("Expected 2 references from LSP, got %d", len(refResult.References))
	}
	if refResult.References[0].Location.Path != "fallback.go" {
		t.Errorf("Expected fallback.go, got %s", refResult.References[0].Location.Path)
	}
}

func TestFallback_EmptyResult_TriggersFallback(t *testing.T) {
	// Scenario: SCIP returns empty result (not error), should still try LSP
	// Expected: In union mode, both are queried; in prefer-first, empty from primary is accepted
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	policy := DefaultQueryPolicy()
	policy.MergeMode = MergeModeUnion

	orch := NewOrchestrator(policy, logger)

	scipBackend := newMockBackend(BackendSCIP)
	scipBackend.priority = 1
	scipBackend.searchResult = &SearchResult{
		Symbols:      []SymbolResult{}, // Empty but not error
		TotalMatches: 0,
		Completeness: CompletenessInfo{Score: 1.0},
	}

	lspBackend := newMockBackend(BackendLSP)
	lspBackend.priority = 2
	lspBackend.searchResult = &SearchResult{
		Symbols: []SymbolResult{
			{StableID: "lsp:sym:found", Name: "FoundByLSP", Kind: "function"},
		},
		TotalMatches: 1,
		Completeness: CompletenessInfo{Score: 0.8},
	}

	orch.RegisterBackend(scipBackend)
	orch.RegisterBackend(lspBackend)

	result, err := orch.Query(context.Background(), QueryRequest{
		Type:  QueryTypeSearch,
		Query: "Found",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	searchResult, ok := result.Data.(*SearchResult)
	if !ok {
		t.Fatalf("Expected SearchResult, got %T", result.Data)
	}
	// In union mode, we should get LSP's result even though SCIP returned empty
	if len(searchResult.Symbols) < 1 {
		t.Error("Expected at least one symbol from union of empty SCIP + LSP")
	}
}

// Helper function to find a contribution by backend ID
func findContribution(contributions []BackendContribution, backendID BackendID) *BackendContribution {
	for i := range contributions {
		if contributions[i].BackendID == backendID {
			return &contributions[i]
		}
	}
	return nil
}
