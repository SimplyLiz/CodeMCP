package incremental

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ckb/internal/logging"
	"ckb/internal/storage"
)

func setupTestStore(t *testing.T) (*Store, func()) {
	t.Helper()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "incremental-store-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Create .ckb directory
	ckbDir := filepath.Join(tmpDir, ".ckb")
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create .ckb dir: %v", err)
	}

	// Create logger
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.ErrorLevel,
	})

	// Open database
	db, err := storage.Open(tmpDir, logger)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to open database: %v", err)
	}

	store := NewStore(db, logger)

	cleanup := func() {
		db.Close() //nolint:errcheck // Test cleanup
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

func TestStoreFileState(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Initially no file state - returns (nil, nil) for non-existent
	state, err := store.GetFileState("main.go")
	if err != nil {
		t.Errorf("unexpected error for non-existent file: %v", err)
	}
	if state != nil {
		t.Error("expected nil state for non-existent file")
	}

	// Save file state
	now := time.Now()
	file := &IndexedFile{
		Path:             "main.go",
		Hash:             "abc123",
		Mtime:            now.Unix(),
		IndexedAt:        now,
		SCIPDocumentHash: "def456",
		SymbolCount:      5,
	}

	if err := store.SaveFileState(file); err != nil {
		t.Fatalf("SaveFileState failed: %v", err)
	}

	// Retrieve file state
	state, err = store.GetFileState("main.go")
	if err != nil {
		t.Fatalf("GetFileState failed: %v", err)
	}

	if state.Path != "main.go" {
		t.Errorf("expected path 'main.go', got %q", state.Path)
	}
	if state.Hash != "abc123" {
		t.Errorf("expected hash 'abc123', got %q", state.Hash)
	}
	if state.SymbolCount != 5 {
		t.Errorf("expected symbolCount 5, got %d", state.SymbolCount)
	}

	// Delete file state
	if err := store.DeleteFileState("main.go"); err != nil {
		t.Fatalf("DeleteFileState failed: %v", err)
	}

	// Verify deletion - returns (nil, nil) for deleted
	state, err = store.GetFileState("main.go")
	if err != nil {
		t.Errorf("unexpected error after deletion: %v", err)
	}
	if state != nil {
		t.Error("expected nil state after deletion")
	}
}

func TestStoreIndexState(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Initially no index
	if store.HasIndex() {
		t.Error("expected no index initially")
	}

	// Set partial index state
	if err := store.SetIndexStatePartial(10); err != nil {
		t.Fatalf("SetIndexStatePartial failed: %v", err)
	}

	state := store.GetIndexState()
	if state.State != "partial" {
		t.Errorf("expected state 'partial', got %q", state.State)
	}

	// Set full index state
	if err := store.SetIndexStateFull(); err != nil {
		t.Fatalf("SetIndexStateFull failed: %v", err)
	}

	state = store.GetIndexState()
	if state.State != "full" {
		t.Errorf("expected state 'full', got %q", state.State)
	}
}

func TestStoreCommitTracking(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Initially no commit
	commit := store.GetLastIndexedCommit()
	if commit != "" {
		t.Errorf("expected empty commit, got %q", commit)
	}

	// Set commit
	if err := store.SetLastIndexedCommit("abc123def"); err != nil {
		t.Fatalf("SetLastIndexedCommit failed: %v", err)
	}

	commit = store.GetLastIndexedCommit()
	if commit != "abc123def" {
		t.Errorf("expected 'abc123def', got %q", commit)
	}
}

func TestStoreFileSymbols(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Save file state first
	file := &IndexedFile{
		Path:      "main.go",
		Hash:      "abc123",
		IndexedAt: time.Now(),
	}
	if err := store.SaveFileState(file); err != nil {
		t.Fatalf("SaveFileState failed: %v", err)
	}

	// Save symbols
	symbols := []string{"pkg.Foo", "pkg.Bar", "pkg.Baz"}
	if err := store.SaveFileSymbols("main.go", symbols); err != nil {
		t.Fatalf("SaveFileSymbols failed: %v", err)
	}

	// Retrieve symbols
	retrieved, err := store.GetSymbolsForFile("main.go")
	if err != nil {
		t.Fatalf("GetSymbolsForFile failed: %v", err)
	}

	if len(retrieved) != len(symbols) {
		t.Errorf("expected %d symbols, got %d", len(symbols), len(retrieved))
	}

	// Delete file state (also removes symbols via cascade or updater)
	if err := store.DeleteFileState("main.go"); err != nil {
		t.Fatalf("DeleteFileState failed: %v", err)
	}
}

func TestStoreSchemaVersion(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Initially no schema version set in index_meta (returns 0)
	version := store.GetSchemaVersion()
	if version != 0 {
		t.Errorf("expected initial schema version 0, got %d", version)
	}

	// Set schema version via meta
	if err := store.SetMetaInt(MetaKeySchemaVersion, int64(CurrentSchemaVersion)); err != nil {
		t.Fatalf("SetMetaInt failed: %v", err)
	}

	version = store.GetSchemaVersion()
	if version != CurrentSchemaVersion {
		t.Errorf("expected schema version %d, got %d", CurrentSchemaVersion, version)
	}
}

func TestStoreTotalFileCount(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Initially zero files
	count := store.GetTotalFileCount()
	if count != 0 {
		t.Errorf("expected 0 files, got %d", count)
	}

	// Add some files
	now := time.Now()
	for i, path := range []string{"a.go", "b.go", "c.go"} {
		file := &IndexedFile{
			Path:        path,
			Hash:        "hash" + string(rune('0'+i)),
			IndexedAt:   now,
			SymbolCount: i,
		}
		if err := store.SaveFileState(file); err != nil {
			t.Fatalf("SaveFileState for %s failed: %v", path, err)
		}
	}

	count = store.GetTotalFileCount()
	if count != 3 {
		t.Errorf("expected 3 files, got %d", count)
	}
}

// ============================================================================
// v2 Transitive Invalidation Store Tests
// ============================================================================

func TestStoreGetPendingRescanCount(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Initially zero rescans
	count := store.GetPendingRescanCount()
	if count != 0 {
		t.Errorf("expected 0 pending rescans initially, got %d", count)
	}
}

func TestStoreGetIndexState_WithPendingRescans(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Set initial state as "partial"
	if err := store.SetIndexStatePartial(5); err != nil {
		t.Fatalf("SetIndexStatePartial failed: %v", err)
	}

	state := store.GetIndexState()
	if state.State != "partial" {
		t.Errorf("expected state 'partial', got %q", state.State)
	}
	if state.PendingRescans != 0 {
		t.Errorf("expected 0 pending rescans, got %d", state.PendingRescans)
	}
}

func TestStoreGetIndexState_PendingAdjustsState(t *testing.T) {
	// Create a fresh database for this test
	tmpDir := t.TempDir()

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.ErrorLevel,
	})

	db, err := storage.Open(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	testStore := NewStore(db, logger)

	// Set state as "full"
	if err := testStore.SetIndexStateFull(); err != nil {
		t.Fatalf("SetIndexStateFull failed: %v", err)
	}

	// Verify full state with no pending
	state := testStore.GetIndexState()
	if state.State != "full" {
		t.Errorf("expected state 'full', got %q", state.State)
	}

	// Add pending rescans
	_, err = db.Exec(`INSERT INTO rescan_queue (file_path, reason, depth, enqueued_at, attempts) VALUES ('test.go', 'dep_change', 1, 12345, 0)`)
	if err != nil {
		t.Fatalf("Insert rescan_queue failed: %v", err)
	}

	// Now state should be "pending" because rescans are queued
	state = testStore.GetIndexState()
	if state.State != "pending" {
		t.Errorf("expected state 'pending' when rescans queued, got %q", state.State)
	}
	if state.PendingRescans != 1 {
		t.Errorf("expected 1 pending rescan, got %d", state.PendingRescans)
	}
}

func TestStoreIndexStatePartial_Accumulates(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// First partial update
	if err := store.SetIndexStatePartial(5); err != nil {
		t.Fatalf("SetIndexStatePartial failed: %v", err)
	}

	state := store.GetIndexState()
	if state.FilesSinceFull != 5 {
		t.Errorf("expected FilesSinceFull=5, got %d", state.FilesSinceFull)
	}

	// Second partial update should accumulate
	if err := store.SetIndexStatePartial(3); err != nil {
		t.Fatalf("SetIndexStatePartial (2nd) failed: %v", err)
	}

	state = store.GetIndexState()
	if state.FilesSinceFull != 8 {
		t.Errorf("expected FilesSinceFull=8 (5+3), got %d", state.FilesSinceFull)
	}

	// Full reindex should reset
	if err := store.SetIndexStateFull(); err != nil {
		t.Fatalf("SetIndexStateFull failed: %v", err)
	}

	state = store.GetIndexState()
	if state.FilesSinceFull != 0 {
		t.Errorf("expected FilesSinceFull=0 after full, got %d", state.FilesSinceFull)
	}
}

func TestStoreGetMeta_NonExistent(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Non-existent key should return empty string
	value := store.GetMeta("nonexistent_key")
	if value != "" {
		t.Errorf("expected empty string for non-existent key, got %q", value)
	}

	// Non-existent int key should return 0
	intValue := store.GetMetaInt("nonexistent_int_key")
	if intValue != 0 {
		t.Errorf("expected 0 for non-existent int key, got %d", intValue)
	}
}

func TestStoreSetMetaInt(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Set and get int value
	if err := store.SetMetaInt("test_int", 12345); err != nil {
		t.Fatalf("SetMetaInt failed: %v", err)
	}

	value := store.GetMetaInt("test_int")
	if value != 12345 {
		t.Errorf("expected 12345, got %d", value)
	}

	// Negative value
	if err := store.SetMetaInt("negative", -42); err != nil {
		t.Fatalf("SetMetaInt (negative) failed: %v", err)
	}

	value = store.GetMetaInt("negative")
	if value != -42 {
		t.Errorf("expected -42, got %d", value)
	}
}

func TestStoreGetIndexState_UnknownDefault(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Without setting any state, should return "unknown"
	state := store.GetIndexState()
	if state.State != "unknown" {
		t.Errorf("expected state 'unknown' initially, got %q", state.State)
	}
}

func TestStoreHasIndex(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Initially no index
	if store.HasIndex() {
		t.Error("expected HasIndex=false initially")
	}

	// Add a file
	file := &IndexedFile{
		Path:      "test.go",
		Hash:      "hash",
		IndexedAt: time.Now(),
	}
	if err := store.SaveFileState(file); err != nil {
		t.Fatalf("SaveFileState failed: %v", err)
	}

	// Now has index
	if !store.HasIndex() {
		t.Error("expected HasIndex=true after adding file")
	}
}

func TestStoreClearAllFileData(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	// Add some data
	now := time.Now()
	for _, path := range []string{"a.go", "b.go"} {
		file := &IndexedFile{Path: path, Hash: "hash", IndexedAt: now}
		if err := store.SaveFileState(file); err != nil {
			t.Fatalf("SaveFileState failed: %v", err)
		}
		if err := store.SaveFileSymbols(path, []string{"sym1", "sym2"}); err != nil {
			t.Fatalf("SaveFileSymbols failed: %v", err)
		}
	}

	// Verify data exists
	if count := store.GetTotalFileCount(); count != 2 {
		t.Fatalf("expected 2 files, got %d", count)
	}

	// Clear all data
	if err := store.ClearAllFileData(); err != nil {
		t.Fatalf("ClearAllFileData failed: %v", err)
	}

	// Verify data is gone
	if count := store.GetTotalFileCount(); count != 0 {
		t.Errorf("expected 0 files after clear, got %d", count)
	}
}
