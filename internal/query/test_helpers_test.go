package query

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ckb/internal/backends"
	"ckb/internal/config"
	"ckb/internal/logging"
	"ckb/internal/storage"
)

// mockSCIPAdapter is a configurable mock for the SCIP backend adapter.
// It implements the backends.SymbolBackend interface for testing.
type mockSCIPAdapter struct {
	mu sync.Mutex

	// Availability
	available bool
	healthy   bool

	// Configurable responses
	symbolResult     *backends.SymbolResult
	searchResult     *backends.SearchResult
	referencesResult *backends.ReferencesResult
	err              error
	delay            time.Duration

	// Call tracking
	getSymbolCalls      int
	searchSymbolsCalls  int
	findReferencesCalls int

	// Captured arguments for verification
	lastSymbolID   string
	lastQuery      string
	lastSearchOpts backends.SearchOptions
	lastRefOpts    backends.RefOptions
}

func newMockSCIPAdapter() *mockSCIPAdapter {
	return &mockSCIPAdapter{
		available: true,
		healthy:   true,
	}
}

func (m *mockSCIPAdapter) ID() backends.BackendID {
	return backends.BackendSCIP
}

func (m *mockSCIPAdapter) IsAvailable() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.available
}

func (m *mockSCIPAdapter) IsHealthy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.healthy
}

func (m *mockSCIPAdapter) Capabilities() []string {
	return []string{"symbol-search", "find-references", "symbol-info", "call-graph"}
}

func (m *mockSCIPAdapter) Priority() int {
	return 1
}

func (m *mockSCIPAdapter) GetSymbol(ctx context.Context, id string) (*backends.SymbolResult, error) {
	m.mu.Lock()
	m.getSymbolCalls++
	m.lastSymbolID = id
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
	return &backends.SymbolResult{
		StableID: id,
		Name:     "MockSymbol",
		Kind:     "function",
		Completeness: backends.CompletenessInfo{
			Score: 1.0,
		},
	}, nil
}

func (m *mockSCIPAdapter) SearchSymbols(ctx context.Context, query string, opts backends.SearchOptions) (*backends.SearchResult, error) {
	m.mu.Lock()
	m.searchSymbolsCalls++
	m.lastQuery = query
	m.lastSearchOpts = opts
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
	return &backends.SearchResult{
		Symbols: []backends.SymbolResult{
			{StableID: "mock:sym:1", Name: query, Kind: "function"},
		},
		TotalMatches: 1,
		Completeness: backends.CompletenessInfo{Score: 1.0},
	}, nil
}

func (m *mockSCIPAdapter) FindReferences(ctx context.Context, symbolID string, opts backends.RefOptions) (*backends.ReferencesResult, error) {
	m.mu.Lock()
	m.findReferencesCalls++
	m.lastSymbolID = symbolID
	m.lastRefOpts = opts
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
	return &backends.ReferencesResult{
		References: []backends.Reference{
			{Location: backends.Location{Path: "test.go", Line: 10}, Kind: "call"},
		},
		TotalReferences: 1,
		Completeness:    backends.CompletenessInfo{Score: 1.0},
	}, nil
}

func (m *mockSCIPAdapter) Close() error {
	return nil
}

// Helper methods for test assertions

func (m *mockSCIPAdapter) GetSymbolCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getSymbolCalls
}

func (m *mockSCIPAdapter) SearchSymbolsCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.searchSymbolsCalls
}

func (m *mockSCIPAdapter) FindReferencesCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.findReferencesCalls
}

func (m *mockSCIPAdapter) LastSymbolID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastSymbolID
}

func (m *mockSCIPAdapter) LastQuery() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastQuery
}

// Reset clears all call counts and captured arguments
func (m *mockSCIPAdapter) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getSymbolCalls = 0
	m.searchSymbolsCalls = 0
	m.findReferencesCalls = 0
	m.lastSymbolID = ""
	m.lastQuery = ""
	m.err = nil
}

// mockQueryCache is a configurable mock for the query cache.
type mockQueryCache struct {
	mu sync.Mutex

	// Storage
	cache map[string]interface{}

	// Behavior
	disabled bool
	getErr   error
	setErr   error

	// Tracking
	getCalls int
	setCalls int
	hits     int
	misses   int
}

func newMockQueryCache() *mockQueryCache {
	return &mockQueryCache{
		cache: make(map[string]interface{}),
	}
}

func (c *mockQueryCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getCalls++

	if c.disabled {
		c.misses++
		return nil, false
	}

	val, ok := c.cache[key]
	if ok {
		c.hits++
	} else {
		c.misses++
	}
	return val, ok
}

func (c *mockQueryCache) Set(key string, value interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setCalls++

	if c.setErr != nil {
		return c.setErr
	}
	if !c.disabled {
		c.cache[key] = value
	}
	return nil
}

func (c *mockQueryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, key)
}

func (c *mockQueryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]interface{})
}

func (c *mockQueryCache) Stats() (hits, misses, getCalls, setCalls int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses, c.getCalls, c.setCalls
}

// testEngineWithMocks creates a test engine with injectable mock dependencies.
// This allows testing specific code paths by controlling backend behavior.
type testEngineBuilder struct {
	t          *testing.T
	tmpDir     string
	db         *storage.DB
	logger     *logging.Logger
	cfg        *config.Config
	scipMock   *mockSCIPAdapter
	cacheMock  *mockQueryCache
	cleanupFns []func()
}

func newTestEngineBuilder(t *testing.T) *testEngineBuilder {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "ckb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	ckbDir := filepath.Join(tmpDir, ".ckb")
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create .ckb dir: %v", err)
	}

	logger := logging.NewLogger(logging.Config{
		Format: logging.JSONFormat,
		Level:  logging.ErrorLevel,
	})

	db, err := storage.Open(tmpDir, logger)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create test db: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.RepoRoot = tmpDir

	return &testEngineBuilder{
		t:        t,
		tmpDir:   tmpDir,
		db:       db,
		logger:   logger,
		cfg:      cfg,
		cleanupFns: []func(){
			func() { db.Close() },
			func() { os.RemoveAll(tmpDir) },
		},
	}
}

func (b *testEngineBuilder) WithSCIPMock(mock *mockSCIPAdapter) *testEngineBuilder {
	b.scipMock = mock
	return b
}

func (b *testEngineBuilder) WithCacheMock(mock *mockQueryCache) *testEngineBuilder {
	b.cacheMock = mock
	return b
}

func (b *testEngineBuilder) Build() (*Engine, func()) {
	b.t.Helper()

	engine, err := NewEngine(b.tmpDir, b.db, b.logger, b.cfg)
	if err != nil {
		for _, fn := range b.cleanupFns {
			fn()
		}
		b.t.Fatalf("failed to create engine: %v", err)
	}

	// Inject mocks if provided
	// Note: This requires the Engine to have settable fields or use interfaces
	// For now, we document this as an extension point

	cleanup := func() {
		for _, fn := range b.cleanupFns {
			fn()
		}
	}

	return engine, cleanup
}

// assertNoError is a helper to fail a test if an error occurred.
func assertNoError(t *testing.T, err error, msg string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", msg, err)
	}
}

// assertEqual is a helper to compare values.
func assertEqual[T comparable](t *testing.T, got, want T, msg string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %v, want %v", msg, got, want)
	}
}

// assertNotNil is a helper to check for nil values.
func assertNotNil(t *testing.T, val interface{}, msg string) {
	t.Helper()
	if val == nil {
		t.Errorf("%s: expected non-nil value", msg)
	}
}

// assertNil is a helper to check for nil values.
func assertNil(t *testing.T, val interface{}, msg string) {
	t.Helper()
	if val != nil {
		t.Errorf("%s: expected nil value, got %v", msg, val)
	}
}

// assertContains checks if a string contains a substring.
func assertContains(t *testing.T, s, substr, msg string) {
	t.Helper()
	if !strContains(s, substr) {
		t.Errorf("%s: %q does not contain %q", msg, s, substr)
	}
}

// strContains is a helper for string containment check.
func strContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && strContainsHelper(s, substr)))
}

func strContainsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
