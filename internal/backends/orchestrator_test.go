package backends

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"ckb/internal/logging"
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
