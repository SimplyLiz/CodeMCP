package incremental

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ckb/internal/logging"
	"ckb/internal/project"
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

// Multi-language support tests (v7.6)

func TestCanUseIncremental_SupportedLanguages(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	// Test unsupported languages - these should always return false with specific reason
	unsupportedTests := []struct {
		name     string
		lang     string
		wantDesc string // Partial match on reason
	}{
		{"Java", "java", "not enabled"},
		{"Kotlin", "kotlin", "not enabled"},
		{"Cpp", "cpp", "not enabled"},
		{"Ruby", "ruby", "not enabled"},
		{"CSharp", "csharp", "not enabled"},
		{"PHP", "php", "not enabled"},
		{"Unknown", "unknown", "no indexer configured"},
	}

	for _, tt := range unsupportedTests {
		t.Run(tt.name, func(t *testing.T) {
			lang := parseTestLanguage(tt.lang)
			canUse, reason := indexer.CanUseIncremental(lang)

			if canUse {
				t.Errorf("CanUseIncremental(%s) = true, want false", tt.lang)
			}

			if !contains(reason, tt.wantDesc) {
				t.Errorf("CanUseIncremental(%s) reason = %q, want to contain %q", tt.lang, reason, tt.wantDesc)
			}
		})
	}

	// Test supported languages - result depends on whether indexer is installed
	supportedTests := []string{"go", "typescript", "javascript", "python", "dart", "rust"}

	for _, lang := range supportedTests {
		t.Run("Supported_"+lang, func(t *testing.T) {
			parsedLang := parseTestLanguage(lang)
			canUse, reason := indexer.CanUseIncremental(parsedLang)

			// Either it works (indexer installed) or it should mention "not installed"
			if !canUse && !contains(reason, "not installed") {
				t.Errorf("CanUseIncremental(%s) = false but reason %q doesn't mention 'not installed'", lang, reason)
			}
			// If canUse is true, reason should be empty
			if canUse && reason != "" {
				t.Errorf("CanUseIncremental(%s) = true but reason is not empty: %q", lang, reason)
			}
		})
	}
}

func TestIndexIncrementalWithLang_UnsupportedLanguage(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	ctx := t.Context()
	lang := parseTestLanguage("java") // Java doesn't support incremental

	_, err := indexer.IndexIncrementalWithLang(ctx, "", lang)

	if err == nil {
		t.Fatal("expected error for unsupported language")
	}

	// Should return ErrIncrementalNotSupported
	if !contains(err.Error(), "not supported") && !contains(err.Error(), "not enabled") {
		t.Errorf("expected 'not supported' or 'not enabled' error, got: %v", err)
	}
}

func TestIndexIncrementalWithLang_UnknownLanguage(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	ctx := t.Context()
	lang := parseTestLanguage("unknown")

	_, err := indexer.IndexIncrementalWithLang(ctx, "", lang)

	if err == nil {
		t.Fatal("expected error for unknown language")
	}

	if !contains(err.Error(), "not supported") {
		t.Errorf("expected 'not supported' error, got: %v", err)
	}
}

func TestIndexIncrementalWithLang_IndexerNotInstalled(t *testing.T) {
	indexer, _, cleanup := setupTestIndexer(t)
	defer cleanup()

	ctx := t.Context()
	lang := parseTestLanguage("go") // Go supports incremental but scip-go may not be installed

	_, err := indexer.IndexIncrementalWithLang(ctx, "", lang)

	if err == nil {
		// If scip-go is installed, the test still passes (we're just testing the flow)
		t.Skip("scip-go is installed, skipping indexer-not-installed test")
	}

	// Should return ErrIndexerNotInstalled or similar
	if !contains(err.Error(), "not installed") && !contains(err.Error(), "install") {
		t.Errorf("expected 'not installed' error, got: %v", err)
	}
}

func TestErrorTypes(t *testing.T) {
	// Test that error types are properly defined
	if ErrIncrementalNotSupported == nil {
		t.Error("ErrIncrementalNotSupported should not be nil")
	}
	if ErrIndexerNotInstalled == nil {
		t.Error("ErrIndexerNotInstalled should not be nil")
	}

	// Test error messages
	if !contains(ErrIncrementalNotSupported.Error(), "not supported") {
		t.Errorf("ErrIncrementalNotSupported message should contain 'not supported', got: %s",
			ErrIncrementalNotSupported.Error())
	}
	if !contains(ErrIndexerNotInstalled.Error(), "not installed") {
		t.Errorf("ErrIndexerNotInstalled message should contain 'not installed', got: %s",
			ErrIndexerNotInstalled.Error())
	}
}

// parseTestLanguage converts a string to project.Language for testing
func parseTestLanguage(s string) project.Language {
	switch s {
	case "go":
		return project.LangGo
	case "typescript":
		return project.LangTypeScript
	case "javascript":
		return project.LangJavaScript
	case "python":
		return project.LangPython
	case "rust":
		return project.LangRust
	case "java":
		return project.LangJava
	case "kotlin":
		return project.LangKotlin
	case "cpp":
		return project.LangCpp
	case "dart":
		return project.LangDart
	case "ruby":
		return project.LangRuby
	case "csharp":
		return project.LangCSharp
	case "php":
		return project.LangPHP
	default:
		return project.LangUnknown
	}
}
