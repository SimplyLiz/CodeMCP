package storage

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) (*DB, string) {
	// Create temporary directory for test database
	tmpDir, err := os.MkdirTemp("", "ckb-storage-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create logger
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Open database
	db, err := Open(tmpDir, logger)
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("Failed to open database: %v", err)
	}

	return db, tmpDir
}

func teardownTestDB(t *testing.T, db *DB, tmpDir string) {
	if err := db.Close(); err != nil {
		t.Errorf("Failed to close database: %v", err)
	}
	if err := os.RemoveAll(tmpDir); err != nil {
		t.Errorf("Failed to remove temp dir: %v", err)
	}
}

func TestDatabaseInitialization(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer teardownTestDB(t, db, tmpDir)

	// Verify database file was created
	dbPath := filepath.Join(tmpDir, ".ckb", "ckb.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("Database file was not created at %s", dbPath)
	}

	// Verify schema version
	version, err := db.getSchemaVersion()
	if err != nil {
		t.Fatalf("Failed to get schema version: %v", err)
	}

	if version != currentSchemaVersion {
		t.Errorf("Expected schema version %d, got %d", currentSchemaVersion, version)
	}
}

func TestSymbolRepository(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer teardownTestDB(t, db, tmpDir)

	repo := NewSymbolRepository(db)

	// Test Create
	mapping := &SymbolMapping{
		StableID:            "sym-123",
		State:               "active",
		FingerprintJSON:     `{"name":"testFunction","kind":"function"}`,
		LocationJSON:        `{"path":"test.go","line":10}`,
		LastVerifiedAt:      time.Now(),
		LastVerifiedStateID: "state-123",
	}

	if err := repo.Create(mapping); err != nil {
		t.Fatalf("Failed to create symbol mapping: %v", err)
	}

	// Test GetByStableID
	retrieved, err := repo.GetByStableID("sym-123")
	if err != nil {
		t.Fatalf("Failed to get symbol mapping: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected symbol mapping to be retrieved, got nil")
	}

	if retrieved.StableID != "sym-123" {
		t.Errorf("Expected stable_id 'sym-123', got '%s'", retrieved.StableID)
	}

	if retrieved.State != "active" {
		t.Errorf("Expected state 'active', got '%s'", retrieved.State)
	}

	// Test MarkAsDeleted
	if markErr := repo.MarkAsDeleted("sym-123", "state-456"); markErr != nil {
		t.Fatalf("Failed to mark symbol as deleted: %v", markErr)
	}

	deleted, err := repo.GetByStableID("sym-123")
	if err != nil {
		t.Fatalf("Failed to get deleted symbol: %v", err)
	}

	if deleted.State != "deleted" {
		t.Errorf("Expected state 'deleted', got '%s'", deleted.State)
	}

	if deleted.DeletedAt == nil {
		t.Error("Expected deleted_at to be set")
	}

	// Test ListByState
	active := &SymbolMapping{
		StableID:            "sym-456",
		State:               "active",
		FingerprintJSON:     `{"name":"anotherFunction"}`,
		LocationJSON:        `{"path":"other.go","line":20}`,
		LastVerifiedAt:      time.Now(),
		LastVerifiedStateID: "state-123",
	}
	if createErr := repo.Create(active); createErr != nil {
		t.Fatalf("Failed to create second symbol: %v", createErr)
	}

	activeList, err := repo.ListByState("active", 100)
	if err != nil {
		t.Fatalf("Failed to list active symbols: %v", err)
	}

	if len(activeList) != 1 {
		t.Errorf("Expected 1 active symbol, got %d", len(activeList))
	}
}

func TestAliasRepository(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer teardownTestDB(t, db, tmpDir)

	// First create symbol mappings (required by foreign key constraint)
	symbolRepo := NewSymbolRepository(db)
	if err := symbolRepo.Create(&SymbolMapping{
		StableID:            "sym-old",
		State:               "deleted",
		FingerprintJSON:     `{}`,
		LocationJSON:        `{}`,
		LastVerifiedAt:      time.Now(),
		LastVerifiedStateID: "state-1",
		DeletedAt:           timePtr(time.Now()),
		DeletedInStateID:    strPtr("state-1"),
	}); err != nil {
		t.Fatalf("Failed to create old symbol: %v", err)
	}

	if err := symbolRepo.Create(&SymbolMapping{
		StableID:            "sym-new",
		State:               "active",
		FingerprintJSON:     `{}`,
		LocationJSON:        `{}`,
		LastVerifiedAt:      time.Now(),
		LastVerifiedStateID: "state-1",
	}); err != nil {
		t.Fatalf("Failed to create new symbol: %v", err)
	}

	// Test Create alias
	aliasRepo := NewAliasRepository(db)
	alias := &SymbolAlias{
		OldStableID:    "sym-old",
		NewStableID:    "sym-new",
		Reason:         "refactored",
		Confidence:     0.95,
		CreatedAt:      time.Now(),
		CreatedStateID: "state-2",
	}

	if err := aliasRepo.Create(alias); err != nil {
		t.Fatalf("Failed to create alias: %v", err)
	}

	// Test GetByOldStableID
	aliases, err := aliasRepo.GetByOldStableID("sym-old")
	if err != nil {
		t.Fatalf("Failed to get aliases: %v", err)
	}

	if len(aliases) != 1 {
		t.Fatalf("Expected 1 alias, got %d", len(aliases))
	}

	if aliases[0].NewStableID != "sym-new" {
		t.Errorf("Expected new_stable_id 'sym-new', got '%s'", aliases[0].NewStableID)
	}
}

func TestCacheOperations(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer teardownTestDB(t, db, tmpDir)

	cache := NewCache(db)

	// Test Query Cache
	err := cache.SetQueryCache("query-key-1", `{"result":"data"}`, "commit-123", "state-1", 300)
	if err != nil {
		t.Fatalf("Failed to set query cache: %v", err)
	}

	value, found, err := cache.GetQueryCache("query-key-1", "commit-123")
	if err != nil {
		t.Fatalf("Failed to get query cache: %v", err)
	}

	if !found {
		t.Fatal("Expected cache entry to be found")
	}

	if value != `{"result":"data"}` {
		t.Errorf("Expected cached value '{\"result\":\"data\"}', got '%s'", value)
	}

	// Test View Cache
	err = cache.SetViewCache("view-key-1", `{"view":"data"}`, "state-1", 3600)
	if err != nil {
		t.Fatalf("Failed to set view cache: %v", err)
	}

	viewValue, found, err := cache.GetViewCache("view-key-1", "state-1")
	if err != nil {
		t.Fatalf("Failed to get view cache: %v", err)
	}

	if !found {
		t.Fatal("Expected view cache entry to be found")
	}

	if viewValue != `{"view":"data"}` {
		t.Errorf("Expected cached value '{\"view\":\"data\"}', got '%s'", viewValue)
	}

	// Test Negative Cache
	err = cache.SetNegativeCache("error-key-1", "symbol-not-found", "Symbol not found", "state-1", 60)
	if err != nil {
		t.Fatalf("Failed to set negative cache: %v", err)
	}

	negEntry, err := cache.GetNegativeCache("error-key-1", "state-1")
	if err != nil {
		t.Fatalf("Failed to get negative cache: %v", err)
	}

	if negEntry == nil {
		t.Fatal("Expected negative cache entry to be found")
	}

	if negEntry.ErrorType != "symbol-not-found" {
		t.Errorf("Expected error type 'symbol-not-found', got '%s'", negEntry.ErrorType)
	}

	// Test Cache Invalidation by State
	err = cache.InvalidateByStateID("state-1")
	if err != nil {
		t.Fatalf("Failed to invalidate cache by state: %v", err)
	}

	// Verify entries are gone
	_, found, err = cache.GetQueryCache("query-key-1", "commit-123")
	if err != nil {
		t.Fatalf("Failed to check query cache after invalidation: %v", err)
	}
	if found {
		t.Error("Expected query cache entry to be invalidated")
	}

	_, found, err = cache.GetViewCache("view-key-1", "state-1")
	if err != nil {
		t.Fatalf("Failed to check view cache after invalidation: %v", err)
	}
	if found {
		t.Error("Expected view cache entry to be invalidated")
	}

	negEntry, err = cache.GetNegativeCache("error-key-1", "state-1")
	if err != nil {
		t.Fatalf("Failed to check negative cache after invalidation: %v", err)
	}
	if negEntry != nil {
		t.Error("Expected negative cache entry to be invalidated")
	}
}

func TestNegativeCacheManager(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer teardownTestDB(t, db, tmpDir)

	cache := NewCache(db)
	manager := NewNegativeCacheManager(cache)

	// Test CacheError
	err := manager.CacheError("test-key", SymbolNotFound, "Symbol 'foo' not found", "state-1")
	if err != nil {
		t.Fatalf("Failed to cache error: %v", err)
	}

	// Test CheckError
	entry, err := manager.CheckError("test-key", "state-1")
	if err != nil {
		t.Fatalf("Failed to check error: %v", err)
	}

	if entry == nil {
		t.Fatal("Expected error entry to be found")
	}

	if entry.ErrorType != string(SymbolNotFound) {
		t.Errorf("Expected error type 'symbol-not-found', got '%s'", entry.ErrorType)
	}

	// Test GetErrorStats
	err = manager.CacheError("test-key-2", BackendUnavailable, "LSP server unavailable", "state-1")
	if err != nil {
		t.Fatalf("Failed to cache second error: %v", err)
	}

	stats, err := manager.GetErrorStats()
	if err != nil {
		t.Fatalf("Failed to get error stats: %v", err)
	}

	if stats[string(SymbolNotFound)] != 1 {
		t.Errorf("Expected 1 symbol-not-found error, got %d", stats[string(SymbolNotFound)])
	}

	if stats[string(BackendUnavailable)] != 1 {
		t.Errorf("Expected 1 backend-unavailable error, got %d", stats[string(BackendUnavailable)])
	}
}

func TestModuleAndDependencyRepositories(t *testing.T) {
	db, tmpDir := setupTestDB(t)
	defer teardownTestDB(t, db, tmpDir)

	moduleRepo := NewModuleRepository(db)
	depRepo := NewDependencyRepository(db)

	// Test Create modules
	module1 := &Module{
		ModuleID:   "mod-1",
		Name:       "core",
		RootPath:   "/src/core",
		DetectedAt: time.Now(),
		StateID:    "state-1",
	}

	module2 := &Module{
		ModuleID:   "mod-2",
		Name:       "utils",
		RootPath:   "/src/utils",
		DetectedAt: time.Now(),
		StateID:    "state-1",
	}

	if err := moduleRepo.Create(module1); err != nil {
		t.Fatalf("Failed to create module 1: %v", err)
	}

	if err := moduleRepo.Create(module2); err != nil {
		t.Fatalf("Failed to create module 2: %v", err)
	}

	// Test GetByID
	retrieved, err := moduleRepo.GetByID("mod-1")
	if err != nil {
		t.Fatalf("Failed to get module: %v", err)
	}

	if retrieved.Name != "core" {
		t.Errorf("Expected module name 'core', got '%s'", retrieved.Name)
	}

	// Test Create dependency
	edge := &DependencyEdge{
		FromModule: "mod-1",
		ToModule:   "mod-2",
		Kind:       "import",
		Strength:   10,
	}

	if depErr := depRepo.Create(edge); depErr != nil {
		t.Fatalf("Failed to create dependency: %v", depErr)
	}

	// Test GetByFromModule
	deps, err := depRepo.GetByFromModule("mod-1")
	if err != nil {
		t.Fatalf("Failed to get dependencies: %v", err)
	}

	if len(deps) != 1 {
		t.Fatalf("Expected 1 dependency, got %d", len(deps))
	}

	if deps[0].ToModule != "mod-2" {
		t.Errorf("Expected to_module 'mod-2', got '%s'", deps[0].ToModule)
	}

	// Test GetByToModule
	reverseDeps, err := depRepo.GetByToModule("mod-2")
	if err != nil {
		t.Fatalf("Failed to get reverse dependencies: %v", err)
	}

	if len(reverseDeps) != 1 {
		t.Fatalf("Expected 1 reverse dependency, got %d", len(reverseDeps))
	}
}

// Helper functions
func timePtr(t time.Time) *time.Time {
	return &t
}

func strPtr(s string) *string {
	return &s
}
