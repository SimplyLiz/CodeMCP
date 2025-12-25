package index

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadMeta_NoFile(t *testing.T) {
	tmpDir := t.TempDir()

	meta, err := LoadMeta(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta != nil {
		t.Fatal("expected nil meta when file doesn't exist")
	}
}

func TestSaveAndLoadMeta(t *testing.T) {
	tmpDir := t.TempDir()

	original := &IndexMeta{
		CreatedAt:   time.Now().Truncate(time.Second),
		CommitHash:  "abc123def456",
		RepoStateID: "state123",
		FileCount:   42,
		Duration:    "3.2s",
		Indexer:     "scip-go",
		IndexerArgs: []string{"scip-go"},
	}

	if err := original.Save(tmpDir); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	path := filepath.Join(tmpDir, metadataFile)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("metadata file was not created")
	}

	// Load it back
	loaded, err := LoadMeta(tmpDir)
	if err != nil {
		t.Fatalf("LoadMeta failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil metadata")
	}

	// Compare fields
	if loaded.Version != MetadataVersion {
		t.Errorf("Version: got %d, want %d", loaded.Version, MetadataVersion)
	}
	if !loaded.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("CreatedAt: got %v, want %v", loaded.CreatedAt, original.CreatedAt)
	}
	if loaded.CommitHash != original.CommitHash {
		t.Errorf("CommitHash: got %s, want %s", loaded.CommitHash, original.CommitHash)
	}
	if loaded.RepoStateID != original.RepoStateID {
		t.Errorf("RepoStateID: got %s, want %s", loaded.RepoStateID, original.RepoStateID)
	}
	if loaded.FileCount != original.FileCount {
		t.Errorf("FileCount: got %d, want %d", loaded.FileCount, original.FileCount)
	}
	if loaded.Duration != original.Duration {
		t.Errorf("Duration: got %s, want %s", loaded.Duration, original.Duration)
	}
	if loaded.Indexer != original.Indexer {
		t.Errorf("Indexer: got %s, want %s", loaded.Indexer, original.Indexer)
	}
}

func TestLoadMeta_VersionMismatch(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a file with wrong version
	content := `{"version": 999, "createdAt": "2024-01-01T00:00:00Z"}`
	path := filepath.Join(tmpDir, metadataFile)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	meta, err := LoadMeta(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta != nil {
		t.Fatal("expected nil meta for version mismatch")
	}
}

func TestCheckFreshness_NilMeta(t *testing.T) {
	var meta *IndexMeta
	result := meta.CheckFreshness("/tmp")

	if result.Fresh {
		t.Error("nil meta should not be fresh")
	}
	if result.Reason == "" {
		t.Error("should have a reason")
	}
}

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{30 * time.Second, "just now"},
		{5 * time.Minute, "5 minutes"},
		{1 * time.Minute, "1 minute"},
		{2 * time.Hour, "2 hours"},
		{1 * time.Hour, "1 hour"},
		{48 * time.Hour, "2 days"},
		{24 * time.Hour, "1 day"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result := humanDuration(tc.duration)
			if result != tc.expected {
				t.Errorf("humanDuration(%v) = %q, want %q", tc.duration, result, tc.expected)
			}
		})
	}
}

func TestCheckFreshness_TimeBased(t *testing.T) {
	// For non-git repos, freshness is time-based
	tmpDir := t.TempDir()

	// Recent index should be fresh
	recent := &IndexMeta{
		CreatedAt: time.Now().Add(-1 * time.Hour),
	}
	result := recent.CheckFreshness(tmpDir)
	if !result.Fresh {
		t.Error("recent index should be fresh in non-git repo")
	}

	// Old index should be stale
	old := &IndexMeta{
		CreatedAt: time.Now().Add(-48 * time.Hour),
	}
	result = old.CheckFreshness(tmpDir)
	if result.Fresh {
		t.Error("old index should be stale in non-git repo")
	}
}

func TestGetStaleness_NilMeta(t *testing.T) {
	var meta *IndexMeta
	staleness := meta.GetStaleness("/tmp")

	if !staleness.IsStale {
		t.Error("nil meta should be stale")
	}
	if staleness.Reason == "" {
		t.Error("should have a reason")
	}
}

func TestGetStaleness_Fresh(t *testing.T) {
	tmpDir := t.TempDir()

	meta := &IndexMeta{
		CreatedAt: time.Now().Add(-1 * time.Hour),
	}

	staleness := meta.GetStaleness(tmpDir)

	if staleness.IsStale {
		t.Error("recent index should not be stale")
	}
	if staleness.IndexAge == "" {
		t.Error("IndexAge should be set")
	}
}

func TestGetStaleness_Stale(t *testing.T) {
	tmpDir := t.TempDir()

	meta := &IndexMeta{
		CreatedAt: time.Now().Add(-48 * time.Hour),
	}

	staleness := meta.GetStaleness(tmpDir)

	if !staleness.IsStale {
		t.Error("old index should be stale")
	}
	if staleness.IndexAge == "" {
		t.Error("IndexAge should be set")
	}
	if staleness.Reason == "" {
		t.Error("Reason should be set for stale index")
	}
}

func TestStaleness_Fields(t *testing.T) {
	staleness := Staleness{
		IsStale:       true,
		CommitsBehind: 5,
		IndexAge:      "2 hours",
		Reason:        "5 commit(s) behind HEAD",
	}

	if !staleness.IsStale {
		t.Error("expected IsStale=true")
	}
	if staleness.CommitsBehind != 5 {
		t.Errorf("expected CommitsBehind=5, got %d", staleness.CommitsBehind)
	}
	if staleness.IndexAge != "2 hours" {
		t.Errorf("expected IndexAge='2 hours', got %q", staleness.IndexAge)
	}
	if staleness.Reason != "5 commit(s) behind HEAD" {
		t.Errorf("expected Reason='5 commit(s) behind HEAD', got %q", staleness.Reason)
	}
}

func TestCountCommitsBehind_EmptyRefs(t *testing.T) {
	// Empty refs should return 0
	count := countCommitsBehind("/tmp", "", "abc123")
	if count != 0 {
		t.Errorf("expected 0 for empty fromCommit, got %d", count)
	}

	count = countCommitsBehind("/tmp", "abc123", "")
	if count != 0 {
		t.Errorf("expected 0 for empty toCommit, got %d", count)
	}

	count = countCommitsBehind("/tmp", "", "")
	if count != 0 {
		t.Errorf("expected 0 for both empty, got %d", count)
	}
}

func TestCountCommitsBehind_InvalidRepo(t *testing.T) {
	// Invalid repo should return 0 (no error, just 0)
	count := countCommitsBehind("/nonexistent/repo", "abc123", "def456")
	if count != 0 {
		t.Errorf("expected 0 for invalid repo, got %d", count)
	}
}

func TestFreshnessResult_Fields(t *testing.T) {
	result := FreshnessResult{
		Fresh:            false,
		Reason:           "5 commit(s) behind HEAD",
		CommitsBehind:    5,
		HasUncommitted:   true,
		IndexedCommit:    "abc123",
		CurrentCommit:    "def456",
		CurrentRepoState: "state789",
	}

	if result.Fresh {
		t.Error("expected Fresh=false")
	}
	if result.CommitsBehind != 5 {
		t.Errorf("expected CommitsBehind=5, got %d", result.CommitsBehind)
	}
	if !result.HasUncommitted {
		t.Error("expected HasUncommitted=true")
	}
	if result.IndexedCommit != "abc123" {
		t.Errorf("expected IndexedCommit='abc123', got %q", result.IndexedCommit)
	}
	if result.CurrentCommit != "def456" {
		t.Errorf("expected CurrentCommit='def456', got %q", result.CurrentCommit)
	}
	if result.CurrentRepoState != "state789" {
		t.Errorf("expected CurrentRepoState='state789', got %q", result.CurrentRepoState)
	}
}
