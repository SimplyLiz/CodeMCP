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
