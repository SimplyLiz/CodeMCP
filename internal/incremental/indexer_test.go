package incremental

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ckb/internal/logging"
	"ckb/internal/storage"
)

func setupTestIndexer(t *testing.T) (*IncrementalIndexer, string, func()) {
	t.Helper()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "incremental-indexer-test")
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

	config := DefaultConfig()
	indexer := NewIncrementalIndexer(tmpDir, db, config, logger)

	cleanup := func() {
		db.Close() //nolint:errcheck // Test cleanup
		os.RemoveAll(tmpDir)
	}

	return indexer, tmpDir, cleanup
}

func TestNewIncrementalIndexer(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	if indexer == nil {
		t.Fatal("expected non-nil indexer")
	}
	if indexer.store == nil {
		t.Error("expected non-nil store")
	}
	if indexer.detector == nil {
		t.Error("expected non-nil detector")
	}
	if indexer.extractor == nil {
		t.Error("expected non-nil extractor")
	}
	if indexer.updater == nil {
		t.Error("expected non-nil updater")
	}
}

func TestNewIncrementalIndexer_NilConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "incremental-indexer-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ckbDir := filepath.Join(tmpDir, ".ckb")
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		t.Fatalf("failed to create .ckb dir: %v", err)
	}

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.ErrorLevel,
	})

	db, err := storage.Open(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close() //nolint:errcheck

	// Pass nil config - should use defaults
	indexer := NewIncrementalIndexer(tmpDir, db, nil, logger)

	if indexer.config == nil {
		t.Fatal("expected non-nil config after initialization")
	}
	if indexer.config.IndexPath != ".scip/index.scip" {
		t.Errorf("expected default index path '.scip/index.scip', got %q", indexer.config.IndexPath)
	}
}

func TestNeedsFullReindex_NoIndex(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	// Fresh database, no index exists
	needs, reason := indexer.NeedsFullReindex()
	if !needs {
		t.Error("expected NeedsFullReindex=true for fresh database")
	}
	if reason != "no previous index" {
		t.Errorf("expected reason 'no previous index', got %q", reason)
	}
}

func TestNeedsFullReindex_WithFullIndex(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	// HasIndex() checks if there are any files - need to add at least one
	if err := indexer.store.SaveFileState(&IndexedFile{Path: "main.go", Hash: "abc"}); err != nil {
		t.Fatalf("SaveFileState failed: %v", err)
	}

	// Simulate a full index having been run
	if err := indexer.store.SetIndexStateFull(); err != nil {
		t.Fatalf("SetIndexStateFull failed: %v", err)
	}
	if err := indexer.store.SetLastIndexedCommit("abc123"); err != nil {
		t.Fatalf("SetLastIndexedCommit failed: %v", err)
	}
	if err := indexer.store.SetMetaInt(MetaKeySchemaVersion, int64(CurrentSchemaVersion)); err != nil {
		t.Fatalf("SetMetaInt failed: %v", err)
	}

	// Non-git repo, so we should still need full reindex due to no tracked commit logic
	// Actually, looking at the code, isGitRepo() check affects the "no tracked commit" case
	needs, reason := indexer.NeedsFullReindex()
	// Since we set a commit and schema version, it should not need full reindex
	if needs {
		t.Errorf("expected NeedsFullReindex=false after setup, got true with reason: %q", reason)
	}
}

func TestNeedsFullReindex_SchemaMismatch(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	// Set up an index with wrong schema version
	if err := indexer.store.SetIndexStateFull(); err != nil {
		t.Fatalf("SetIndexStateFull failed: %v", err)
	}
	if err := indexer.store.SetLastIndexedCommit("abc123"); err != nil {
		t.Fatalf("SetLastIndexedCommit failed: %v", err)
	}
	// Set a different schema version
	wrongVersion := int64(CurrentSchemaVersion - 1)
	if wrongVersion == 0 {
		wrongVersion = CurrentSchemaVersion + 1
	}
	if err := indexer.store.SetMetaInt(MetaKeySchemaVersion, wrongVersion); err != nil {
		t.Fatalf("SetMetaInt failed: %v", err)
	}

	needs, reason := indexer.NeedsFullReindex()
	if !needs {
		t.Error("expected NeedsFullReindex=true for schema mismatch")
	}
	if reason == "" {
		t.Error("expected non-empty reason for schema mismatch")
	}
}

func TestGetIndexState_Initial(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	state := indexer.GetIndexState()

	// Initial state should be "unknown" or empty
	if state.State != "unknown" && state.State != "" {
		t.Errorf("expected initial state 'unknown' or empty, got %q", state.State)
	}
}

func TestGetIndexState_AfterFull(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	// Set full index state
	if err := indexer.store.SetIndexStateFull(); err != nil {
		t.Fatalf("SetIndexStateFull failed: %v", err)
	}
	if err := indexer.store.SetLastIndexedCommit("abc123def456"); err != nil {
		t.Fatalf("SetLastIndexedCommit failed: %v", err)
	}

	state := indexer.GetIndexState()
	if state.State != "full" {
		t.Errorf("expected state 'full', got %q", state.State)
	}
	if state.Commit != "abc123def456" {
		t.Errorf("expected commit 'abc123def456', got %q", state.Commit)
	}
}

func TestGetIndexState_AfterPartial(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	// Set partial index state
	if err := indexer.store.SetIndexStatePartial(5); err != nil {
		t.Fatalf("SetIndexStatePartial failed: %v", err)
	}

	state := indexer.GetIndexState()
	if state.State != "partial" {
		t.Errorf("expected state 'partial', got %q", state.State)
	}
	if state.FilesSinceFull != 5 {
		t.Errorf("expected FilesSinceFull=5, got %d", state.FilesSinceFull)
	}
}

func TestGetStore(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	store := indexer.GetStore()
	if store == nil {
		t.Error("expected non-nil store from GetStore()")
	}
	if store != indexer.store {
		t.Error("GetStore() should return the same store instance")
	}
}

func TestGetDetector(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	detector := indexer.GetDetector()
	if detector == nil {
		t.Error("expected non-nil detector from GetDetector()")
	}
	if detector != indexer.detector {
		t.Error("GetDetector() should return the same detector instance")
	}
}

func TestFormatStats_Unchanged(t *testing.T) {
	stats := &DeltaStats{
		IndexState: "unchanged",
	}
	state := IndexState{}

	result := FormatStats(stats, state)

	expected := "Index is up to date. Nothing to do."
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatStats_WithChanges(t *testing.T) {
	stats := &DeltaStats{
		FilesChanged:   3,
		FilesAdded:     1,
		FilesDeleted:   0,
		SymbolsAdded:   10,
		SymbolsRemoved: 2,
		RefsAdded:      25,
		Duration:       150 * time.Millisecond,
		IndexState:     "partial",
	}
	state := IndexState{
		Commit:         "abc123def456789",
		FilesSinceFull: 15,
		IsDirty:        false,
	}

	result := FormatStats(stats, state)

	// Verify key elements are present
	if result == "" {
		t.Error("expected non-empty result")
	}
	if !contains(result, "3 modified") {
		t.Error("expected '3 modified' in output")
	}
	if !contains(result, "1 added") {
		t.Error("expected '1 added' in output")
	}
	if !contains(result, "abc123d") {
		t.Error("expected truncated commit hash in output")
	}
	if !contains(result, "15 files since last full") {
		t.Error("expected files since full count in output")
	}
}

func TestFormatStats_DirtyState(t *testing.T) {
	stats := &DeltaStats{
		FilesChanged: 1,
		Duration:     100 * time.Millisecond,
		IndexState:   "partial",
	}
	state := IndexState{
		Commit:         "abc123",
		FilesSinceFull: 1,
		IsDirty:        true,
	}

	result := FormatStats(stats, state)

	if !contains(result, "(+dirty)") {
		t.Error("expected '(+dirty)' indicator for dirty state")
	}
}

// contains checks if substr is in s
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFormatStats_PendingRescans(t *testing.T) {
	stats := &DeltaStats{
		FilesChanged: 2,
		Duration:     200 * time.Millisecond,
		IndexState:   "partial",
	}
	state := IndexState{
		Commit:         "def789",
		FilesSinceFull: 5,
		PendingRescans: 3,
	}

	result := FormatStats(stats, state)

	if !contains(result, "3 files queued for rescan") {
		t.Error("expected pending rescans count in output")
	}
}

func TestFormatStats_FullAccuracy(t *testing.T) {
	stats := &DeltaStats{
		FilesChanged: 1,
		Duration:     100 * time.Millisecond,
		IndexState:   "partial",
	}
	state := IndexState{
		State:          "full",
		Commit:         "abc123",
		PendingRescans: 0, // No pending rescans
	}

	result := FormatStats(stats, state)

	// When state is full and no pending rescans, accuracy should be "accurate"
	if !contains(result, "accurate") {
		t.Error("expected 'accurate' in output for full state with no pending rescans")
	}
}

func TestFormatAccuracyMarker(t *testing.T) {
	tests := []struct {
		accuracy string
		expected string
	}{
		{"accurate", "OK"},
		{"may be stale", "!!"},
		{"unknown", "!!"},
	}

	for _, tt := range tests {
		t.Run(tt.accuracy, func(t *testing.T) {
			result := formatAccuracyMarker(tt.accuracy)
			if result != tt.expected {
				t.Errorf("formatAccuracyMarker(%q) = %q, want %q", tt.accuracy, result, tt.expected)
			}
		})
	}
}

func TestIndexerConfig(t *testing.T) {
	indexer, tmpDir, cleanup := setupTestIndexer(t)
	defer cleanup()

	// Test that config is properly set
	if indexer.config == nil {
		t.Fatal("expected non-nil config")
	}

	// Test repoRoot is set correctly
	if indexer.repoRoot != tmpDir {
		t.Errorf("expected repoRoot %q, got %q", tmpDir, indexer.repoRoot)
	}
}

func TestIndexerGetters(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	// Test GetStore
	store := indexer.GetStore()
	if store == nil {
		t.Error("GetStore() returned nil")
	}

	// Test GetDetector
	detector := indexer.GetDetector()
	if detector == nil {
		t.Error("GetDetector() returned nil")
	}
}

// TestIndexState_DirtyModifiers tests the dirty state modifiers in GetIndexState
func TestIndexState_DirtyModifiers(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	// Set partial state
	if err := indexer.store.SetIndexStatePartial(3); err != nil {
		t.Fatalf("SetIndexStatePartial failed: %v", err)
	}

	state := indexer.GetIndexState()

	// In a non-git repo, IsDirty should be false
	if state.IsDirty {
		t.Error("expected IsDirty=false in non-git repo")
	}

	// State should be "partial" (not "partial_dirty" since not dirty)
	if state.State != "partial" {
		t.Errorf("expected state 'partial', got %q", state.State)
	}
}

func TestNeedsFullReindex_NoCommit_NonGitRepo(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	// Set up an index without commit (but not a git repo)
	if err := indexer.store.SaveFileState(&IndexedFile{Path: "main.go", Hash: "abc"}); err != nil {
		t.Fatalf("SaveFileState failed: %v", err)
	}
	if err := indexer.store.SetIndexStateFull(); err != nil {
		t.Fatalf("SetIndexStateFull failed: %v", err)
	}
	if err := indexer.store.SetMetaInt(MetaKeySchemaVersion, int64(CurrentSchemaVersion)); err != nil {
		t.Fatalf("SetMetaInt failed: %v", err)
	}
	// Don't set commit - simulate non-git repo

	// In a non-git repo, missing commit should NOT trigger full reindex
	// because the isGitRepo() check should prevent the "no tracked commit" error
	needs, reason := indexer.NeedsFullReindex()

	// The detector.isGitRepo() should return false, so "no tracked commit" check is skipped
	if needs {
		t.Errorf("expected NeedsFullReindex=false in non-git repo, got true with reason: %q", reason)
	}
}
