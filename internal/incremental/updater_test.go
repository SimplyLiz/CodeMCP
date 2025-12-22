package incremental

import (
	"os"
	"path/filepath"
	"testing"

	"ckb/internal/logging"
	"ckb/internal/storage"
)

func setupTestUpdater(t *testing.T) (*IndexUpdater, *Store, *storage.DB, func()) {
	t.Helper()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "incremental-updater-test")
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
	updater := NewIndexUpdater(db, store, logger)

	cleanup := func() {
		db.Close() //nolint:errcheck // Test cleanup
		os.RemoveAll(tmpDir)
	}

	return updater, store, db, cleanup
}

func TestNewIndexUpdater(t *testing.T) {
	updater, _, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	if updater == nil {
		t.Fatal("expected non-nil updater")
	}
	if updater.db == nil {
		t.Error("expected non-nil db")
	}
	if updater.store == nil {
		t.Error("expected non-nil store")
	}
}

func TestApplyDelta_Empty(t *testing.T) {
	updater, _, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	delta := &SymbolDelta{
		FileDeltas: []FileDelta{},
	}

	err := updater.ApplyDelta(delta)
	if err != nil {
		t.Errorf("ApplyDelta with empty delta should succeed, got: %v", err)
	}
}

func TestApplyDelta_AddFile(t *testing.T) {
	updater, store, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	delta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:             "new_file.go",
				ChangeType:       ChangeAdded,
				Hash:             "abc123",
				SCIPDocumentHash: "def456",
				SymbolCount:      2,
				Symbols: []Symbol{
					{ID: "pkg.Foo", Name: "Foo", Kind: "function", FilePath: "new_file.go", StartLine: 10, EndLine: 15},
					{ID: "pkg.Bar", Name: "Bar", Kind: "type", FilePath: "new_file.go", StartLine: 20, EndLine: 25},
				},
				Refs: []Reference{
					{FromFile: "new_file.go", FromLine: 12, ToSymbolID: "fmt.Println", Kind: "reference"},
				},
			},
		},
	}

	err := updater.ApplyDelta(delta)
	if err != nil {
		t.Fatalf("ApplyDelta failed: %v", err)
	}

	// Verify file was added
	state, err := store.GetFileState("new_file.go")
	if err != nil {
		t.Fatalf("GetFileState failed: %v", err)
	}
	if state == nil {
		t.Fatal("expected file state to exist after add")
	}
	if state.Hash != "abc123" {
		t.Errorf("expected hash 'abc123', got %q", state.Hash)
	}
	if state.SymbolCount != 2 {
		t.Errorf("expected symbolCount 2, got %d", state.SymbolCount)
	}

	// Verify symbols were added
	symbols, err := store.GetSymbolsForFile("new_file.go")
	if err != nil {
		t.Fatalf("GetSymbolsForFile failed: %v", err)
	}
	if len(symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(symbols))
	}
}

func TestApplyDelta_ModifyFile(t *testing.T) {
	updater, store, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	// First add a file
	addDelta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:        "existing.go",
				ChangeType:  ChangeAdded,
				Hash:        "original",
				SymbolCount: 1,
				Symbols: []Symbol{
					{ID: "pkg.Old", Name: "Old", Kind: "function", FilePath: "existing.go"},
				},
			},
		},
	}
	if err := updater.ApplyDelta(addDelta); err != nil {
		t.Fatalf("initial ApplyDelta failed: %v", err)
	}

	// Now modify it
	modifyDelta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:        "existing.go",
				ChangeType:  ChangeModified,
				Hash:        "modified",
				SymbolCount: 2,
				Symbols: []Symbol{
					{ID: "pkg.New1", Name: "New1", Kind: "function", FilePath: "existing.go"},
					{ID: "pkg.New2", Name: "New2", Kind: "type", FilePath: "existing.go"},
				},
			},
		},
	}
	if err := updater.ApplyDelta(modifyDelta); err != nil {
		t.Fatalf("modify ApplyDelta failed: %v", err)
	}

	// Verify file was updated
	state, _ := store.GetFileState("existing.go")
	if state.Hash != "modified" {
		t.Errorf("expected hash 'modified', got %q", state.Hash)
	}
	if state.SymbolCount != 2 {
		t.Errorf("expected symbolCount 2, got %d", state.SymbolCount)
	}

	// Verify old symbols replaced with new
	symbols, _ := store.GetSymbolsForFile("existing.go")
	if len(symbols) != 2 {
		t.Errorf("expected 2 symbols after modify, got %d", len(symbols))
	}
}

func TestApplyDelta_DeleteFile(t *testing.T) {
	updater, store, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	// First add a file
	addDelta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:        "todelete.go",
				ChangeType:  ChangeAdded,
				Hash:        "exists",
				SymbolCount: 1,
				Symbols: []Symbol{
					{ID: "pkg.ToDelete", Name: "ToDelete", Kind: "function", FilePath: "todelete.go"},
				},
			},
		},
	}
	if err := updater.ApplyDelta(addDelta); err != nil {
		t.Fatalf("initial ApplyDelta failed: %v", err)
	}

	// Verify it exists
	state, _ := store.GetFileState("todelete.go")
	if state == nil {
		t.Fatal("expected file to exist before delete")
	}

	// Now delete it
	deleteDelta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:       "todelete.go",
				ChangeType: ChangeDeleted,
			},
		},
	}
	if err := updater.ApplyDelta(deleteDelta); err != nil {
		t.Fatalf("delete ApplyDelta failed: %v", err)
	}

	// Verify file was deleted
	state, _ = store.GetFileState("todelete.go")
	if state != nil {
		t.Error("expected file to be deleted")
	}

	// Verify symbols were deleted
	symbols, _ := store.GetSymbolsForFile("todelete.go")
	if len(symbols) != 0 {
		t.Errorf("expected 0 symbols after delete, got %d", len(symbols))
	}
}

func TestApplyDelta_RenameFile(t *testing.T) {
	updater, store, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	// First add a file
	addDelta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:        "oldname.go",
				ChangeType:  ChangeAdded,
				Hash:        "content",
				SymbolCount: 1,
				Symbols: []Symbol{
					{ID: "pkg.Sym", Name: "Sym", Kind: "function", FilePath: "oldname.go"},
				},
			},
		},
	}
	if err := updater.ApplyDelta(addDelta); err != nil {
		t.Fatalf("initial ApplyDelta failed: %v", err)
	}

	// Now rename it
	renameDelta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:        "newname.go",
				OldPath:     "oldname.go",
				ChangeType:  ChangeRenamed,
				Hash:        "content",
				SymbolCount: 1,
				Symbols: []Symbol{
					{ID: "pkg.Sym", Name: "Sym", Kind: "function", FilePath: "newname.go"},
				},
			},
		},
	}
	if err := updater.ApplyDelta(renameDelta); err != nil {
		t.Fatalf("rename ApplyDelta failed: %v", err)
	}

	// Verify old path is gone
	oldState, _ := store.GetFileState("oldname.go")
	if oldState != nil {
		t.Error("expected old path to be deleted after rename")
	}

	// Verify new path exists
	newState, _ := store.GetFileState("newname.go")
	if newState == nil {
		t.Fatal("expected new path to exist after rename")
	}
	if newState.Hash != "content" {
		t.Errorf("expected hash 'content', got %q", newState.Hash)
	}
}

func TestApplyDelta_RenameWithoutOldPath(t *testing.T) {
	updater, _, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	// Rename without OldPath should fail
	delta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:       "newname.go",
				OldPath:    "", // Missing!
				ChangeType: ChangeRenamed,
			},
		},
	}

	err := updater.ApplyDelta(delta)
	if err == nil {
		t.Error("expected error for rename without OldPath")
	}
}

func TestUpdateIndexState(t *testing.T) {
	updater, store, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	// Update with file count and commit
	err := updater.UpdateIndexState(10, "abc123")
	if err != nil {
		t.Fatalf("UpdateIndexState failed: %v", err)
	}

	state := store.GetIndexState()
	if state.State != "partial" {
		t.Errorf("expected state 'partial', got %q", state.State)
	}
	if state.FilesSinceFull != 10 {
		t.Errorf("expected FilesSinceFull=10, got %d", state.FilesSinceFull)
	}

	commit := store.GetLastIndexedCommit()
	if commit != "abc123" {
		t.Errorf("expected commit 'abc123', got %q", commit)
	}
}

func TestUpdateIndexState_EmptyCommit(t *testing.T) {
	updater, store, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	// Set initial commit
	if err := store.SetLastIndexedCommit("initial"); err != nil {
		t.Fatalf("SetLastIndexedCommit failed: %v", err)
	}

	// Update with empty commit (should preserve existing)
	err := updater.UpdateIndexState(5, "")
	if err != nil {
		t.Fatalf("UpdateIndexState failed: %v", err)
	}

	commit := store.GetLastIndexedCommit()
	if commit != "initial" {
		t.Errorf("expected commit to be preserved as 'initial', got %q", commit)
	}
}

func TestSetFullIndexComplete(t *testing.T) {
	updater, store, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	err := updater.SetFullIndexComplete("fullcommit123")
	if err != nil {
		t.Fatalf("SetFullIndexComplete failed: %v", err)
	}

	state := store.GetIndexState()
	if state.State != "full" {
		t.Errorf("expected state 'full', got %q", state.State)
	}

	commit := store.GetLastIndexedCommit()
	if commit != "fullcommit123" {
		t.Errorf("expected commit 'fullcommit123', got %q", commit)
	}
}

func TestSetFullIndexComplete_EmptyCommit(t *testing.T) {
	updater, store, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	// Set initial commit
	if err := store.SetLastIndexedCommit("previous"); err != nil {
		t.Fatalf("SetLastIndexedCommit failed: %v", err)
	}

	// Complete without new commit
	err := updater.SetFullIndexComplete("")
	if err != nil {
		t.Fatalf("SetFullIndexComplete failed: %v", err)
	}

	state := store.GetIndexState()
	if state.State != "full" {
		t.Errorf("expected state 'full', got %q", state.State)
	}

	// Commit should be preserved
	commit := store.GetLastIndexedCommit()
	if commit != "previous" {
		t.Errorf("expected commit to be preserved as 'previous', got %q", commit)
	}
}

func TestGetUpdateStats(t *testing.T) {
	updater, _, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	delta := &SymbolDelta{
		Stats: DeltaStats{
			FilesChanged:   5,
			FilesAdded:     2,
			FilesDeleted:   1,
			SymbolsAdded:   20,
			SymbolsRemoved: 5,
		},
	}

	stats := updater.GetUpdateStats(delta)

	if stats.FilesChanged != 5 {
		t.Errorf("expected FilesChanged=5, got %d", stats.FilesChanged)
	}
	if stats.FilesAdded != 2 {
		t.Errorf("expected FilesAdded=2, got %d", stats.FilesAdded)
	}
	if stats.FilesDeleted != 1 {
		t.Errorf("expected FilesDeleted=1, got %d", stats.FilesDeleted)
	}
	if stats.SymbolsAdded != 20 {
		t.Errorf("expected SymbolsAdded=20, got %d", stats.SymbolsAdded)
	}
	if stats.SymbolsRemoved != 5 {
		t.Errorf("expected SymbolsRemoved=5, got %d", stats.SymbolsRemoved)
	}
}

func TestApplyDelta_MultipleFiles(t *testing.T) {
	updater, store, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	delta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:        "file1.go",
				ChangeType:  ChangeAdded,
				Hash:        "hash1",
				SymbolCount: 1,
				Symbols: []Symbol{
					{ID: "pkg.A", Name: "A", FilePath: "file1.go"},
				},
			},
			{
				Path:        "file2.go",
				ChangeType:  ChangeAdded,
				Hash:        "hash2",
				SymbolCount: 2,
				Symbols: []Symbol{
					{ID: "pkg.B", Name: "B", FilePath: "file2.go"},
					{ID: "pkg.C", Name: "C", FilePath: "file2.go"},
				},
			},
			{
				Path:        "file3.go",
				ChangeType:  ChangeAdded,
				Hash:        "hash3",
				SymbolCount: 0,
				Symbols:     []Symbol{},
			},
		},
	}

	err := updater.ApplyDelta(delta)
	if err != nil {
		t.Fatalf("ApplyDelta failed: %v", err)
	}

	// Verify all files were added
	count := store.GetTotalFileCount()
	if count != 3 {
		t.Errorf("expected 3 files, got %d", count)
	}

	// Verify each file
	for _, path := range []string{"file1.go", "file2.go", "file3.go"} {
		state, _ := store.GetFileState(path)
		if state == nil {
			t.Errorf("expected file %s to exist", path)
		}
	}
}

// ============================================================================
// v2 Transitive Invalidation Tests
// ============================================================================

func TestApplyDeltaWithInvalidation(t *testing.T) {
	updater, store, db, cleanup := setupTestUpdater(t)
	defer cleanup()

	// Setup initial files with dependencies
	// a.go references symbols in b.go
	initialDelta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:        "b.go",
				ChangeType:  ChangeAdded,
				Hash:        "hash_b",
				SymbolCount: 1,
				Symbols: []Symbol{
					{ID: "pkg.B", Name: "B", Kind: "function", FilePath: "b.go"},
				},
			},
			{
				Path:        "a.go",
				ChangeType:  ChangeAdded,
				Hash:        "hash_a",
				SymbolCount: 1,
				Symbols: []Symbol{
					{ID: "pkg.A", Name: "A", Kind: "function", FilePath: "a.go"},
				},
				Refs: []Reference{
					{FromFile: "a.go", FromLine: 10, ToSymbolID: "pkg.B", Kind: "reference"},
				},
			},
		},
	}

	if err := updater.ApplyDelta(initialDelta); err != nil {
		t.Fatalf("Initial ApplyDelta failed: %v", err)
	}

	// Now modify b.go - should trigger invalidation of a.go
	modifyDelta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:        "b.go",
				ChangeType:  ChangeModified,
				Hash:        "hash_b_modified",
				SymbolCount: 2,
				Symbols: []Symbol{
					{ID: "pkg.B", Name: "B", Kind: "function", FilePath: "b.go"},
					{ID: "pkg.B2", Name: "B2", Kind: "function", FilePath: "b.go"},
				},
			},
		},
	}

	enqueued, err := updater.ApplyDeltaWithInvalidation(modifyDelta)
	if err != nil {
		t.Fatalf("ApplyDeltaWithInvalidation failed: %v", err)
	}

	// a.go depends on b.go, so it should be enqueued
	if enqueued != 1 {
		t.Errorf("Expected 1 file enqueued for invalidation, got %d", enqueued)
	}

	// Verify file was modified
	state, _ := store.GetFileState("b.go")
	if state.Hash != "hash_b_modified" {
		t.Errorf("Expected hash 'hash_b_modified', got %q", state.Hash)
	}

	// Verify rescan queue has a.go
	var count int
	row := db.QueryRow(`SELECT COUNT(*) FROM rescan_queue WHERE file_path = 'a.go'`)
	row.Scan(&count)
	if count != 1 {
		t.Errorf("Expected a.go in rescan queue, count=%d", count)
	}
}

func TestApplyDeltaWithInvalidation_DeletedFile(t *testing.T) {
	updater, _, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	// Add a file first
	addDelta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:        "todelete.go",
				ChangeType:  ChangeAdded,
				Hash:        "hash",
				SymbolCount: 1,
				Symbols: []Symbol{
					{ID: "pkg.ToDelete", Name: "ToDelete", FilePath: "todelete.go"},
				},
			},
		},
	}
	if err := updater.ApplyDelta(addDelta); err != nil {
		t.Fatalf("Initial ApplyDelta failed: %v", err)
	}

	// Delete it - deleted files should not trigger invalidation
	deleteDelta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:       "todelete.go",
				ChangeType: ChangeDeleted,
			},
		},
	}

	enqueued, err := updater.ApplyDeltaWithInvalidation(deleteDelta)
	if err != nil {
		t.Fatalf("ApplyDeltaWithInvalidation failed: %v", err)
	}

	// Deleted files should not be included in invalidation
	if enqueued != 0 {
		t.Errorf("Expected 0 enqueued for deleted file, got %d", enqueued)
	}
}

func TestSetConfig(t *testing.T) {
	updater, _, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	// Get original dep tracker
	origTracker := updater.GetDependencyTracker()
	if origTracker == nil {
		t.Fatal("Expected non-nil dep tracker")
	}

	// Create new config with different settings
	newConfig := &Config{
		IndexPath:            ".custom/index.scip",
		IncrementalThreshold: 75,
		Transitive: TransitiveConfig{
			Enabled:        true,
			Mode:           InvalidationEager,
			Depth:          5,
			MaxRescanFiles: 500,
			MaxRescanMs:    3000,
		},
	}

	updater.SetConfig(newConfig)

	// Verify config was updated
	newTracker := updater.GetDependencyTracker()
	if newTracker == origTracker {
		t.Error("Expected new dep tracker instance after SetConfig")
	}
}

func TestApplyDelta_FileDeps(t *testing.T) {
	updater, _, db, cleanup := setupTestUpdater(t)
	defer cleanup()

	// Add files with references
	delta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:        "utils.go",
				ChangeType:  ChangeAdded,
				Hash:        "hash_utils",
				SymbolCount: 2,
				Symbols: []Symbol{
					{ID: "pkg.Helper", Name: "Helper", Kind: "function", FilePath: "utils.go"},
					{ID: "pkg.Config", Name: "Config", Kind: "type", FilePath: "utils.go"},
				},
			},
			{
				Path:        "main.go",
				ChangeType:  ChangeAdded,
				Hash:        "hash_main",
				SymbolCount: 1,
				Symbols: []Symbol{
					{ID: "pkg.Main", Name: "Main", Kind: "function", FilePath: "main.go"},
				},
				Refs: []Reference{
					{FromFile: "main.go", FromLine: 5, ToSymbolID: "pkg.Helper", Kind: "reference"},
					{FromFile: "main.go", FromLine: 7, ToSymbolID: "pkg.Config", Kind: "reference"},
				},
			},
		},
	}

	if err := updater.ApplyDelta(delta); err != nil {
		t.Fatalf("ApplyDelta failed: %v", err)
	}

	// Verify file_deps was populated
	var count int
	row := db.QueryRow(`SELECT COUNT(*) FROM file_deps WHERE dependent_file = 'main.go' AND defining_file = 'utils.go'`)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 file_dep entry, got %d", count)
	}
}

func TestApplyDelta_DeleteClearsFileDeps(t *testing.T) {
	updater, _, db, cleanup := setupTestUpdater(t)
	defer cleanup()

	// Add files with dependencies
	delta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:        "lib.go",
				ChangeType:  ChangeAdded,
				Hash:        "hash_lib",
				SymbolCount: 1,
				Symbols: []Symbol{
					{ID: "pkg.Lib", Name: "Lib", Kind: "function", FilePath: "lib.go"},
				},
			},
			{
				Path:        "consumer.go",
				ChangeType:  ChangeAdded,
				Hash:        "hash_consumer",
				SymbolCount: 1,
				Symbols: []Symbol{
					{ID: "pkg.Consumer", Name: "Consumer", Kind: "function", FilePath: "consumer.go"},
				},
				Refs: []Reference{
					{FromFile: "consumer.go", FromLine: 10, ToSymbolID: "pkg.Lib", Kind: "reference"},
				},
			},
		},
	}
	if err := updater.ApplyDelta(delta); err != nil {
		t.Fatalf("Initial ApplyDelta failed: %v", err)
	}

	// Verify dep exists
	var count int
	row := db.QueryRow(`SELECT COUNT(*) FROM file_deps WHERE dependent_file = 'consumer.go'`)
	row.Scan(&count)
	if count != 1 {
		t.Fatalf("Expected 1 dep before delete, got %d", count)
	}

	// Delete consumer.go
	deleteDelta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:       "consumer.go",
				ChangeType: ChangeDeleted,
			},
		},
	}
	if err := updater.ApplyDelta(deleteDelta); err != nil {
		t.Fatalf("Delete ApplyDelta failed: %v", err)
	}

	// Verify dep was removed
	row = db.QueryRow(`SELECT COUNT(*) FROM file_deps WHERE dependent_file = 'consumer.go'`)
	row.Scan(&count)
	if count != 0 {
		t.Errorf("Expected 0 deps after delete, got %d", count)
	}
}

func TestApplyDelta_CallEdges(t *testing.T) {
	updater, _, db, cleanup := setupTestUpdater(t)
	defer cleanup()

	// Add file with call edges
	delta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:        "caller.go",
				ChangeType:  ChangeAdded,
				Hash:        "hash",
				SymbolCount: 1,
				Symbols: []Symbol{
					{ID: "pkg.Caller", Name: "Caller", Kind: "function", FilePath: "caller.go"},
				},
				CallEdges: []CallEdge{
					{CallerID: "pkg.Caller", CallerFile: "caller.go", CalleeID: "fmt.Println", Line: 5, Column: 2},
					{CallerID: "pkg.Caller", CallerFile: "caller.go", CalleeID: "os.Exit", Line: 6, Column: 2},
				},
			},
		},
	}

	if err := updater.ApplyDelta(delta); err != nil {
		t.Fatalf("ApplyDelta failed: %v", err)
	}

	// Verify call edges were inserted
	var count int
	row := db.QueryRow(`SELECT COUNT(*) FROM callgraph WHERE caller_file = 'caller.go'`)
	row.Scan(&count)
	if count != 2 {
		t.Errorf("Expected 2 call edges, got %d", count)
	}
}

func TestApplyDelta_CallEdgesWithNullCaller(t *testing.T) {
	updater, _, db, cleanup := setupTestUpdater(t)
	defer cleanup()

	// Add file with top-level call (no caller ID)
	delta := &SymbolDelta{
		FileDeltas: []FileDelta{
			{
				Path:        "init.go",
				ChangeType:  ChangeAdded,
				Hash:        "hash",
				SymbolCount: 0,
				Symbols:     []Symbol{},
				CallEdges: []CallEdge{
					{CallerID: "", CallerFile: "init.go", CalleeID: "pkg.Init", Line: 3, Column: 1},
				},
			},
		},
	}

	if err := updater.ApplyDelta(delta); err != nil {
		t.Fatalf("ApplyDelta failed: %v", err)
	}

	// Verify call edge with NULL caller_id was inserted
	var count int
	row := db.QueryRow(`SELECT COUNT(*) FROM callgraph WHERE caller_file = 'init.go' AND caller_id IS NULL`)
	row.Scan(&count)
	if count != 1 {
		t.Errorf("Expected 1 call edge with NULL caller_id, got %d", count)
	}
}

func TestGetDependencyTracker(t *testing.T) {
	updater, _, _, cleanup := setupTestUpdater(t)
	defer cleanup()

	tracker := updater.GetDependencyTracker()
	if tracker == nil {
		t.Fatal("Expected non-nil dependency tracker")
	}

	// Verify tracker is functional
	if err := tracker.EnqueueRescan("test.go", RescanDepChange, 1); err != nil {
		t.Fatalf("EnqueueRescan via tracker failed: %v", err)
	}

	count := tracker.GetPendingRescanCount()
	if count != 1 {
		t.Errorf("Expected 1 pending rescan, got %d", count)
	}
}
