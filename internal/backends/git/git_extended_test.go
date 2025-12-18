package git

import (
	"os"
	"path/filepath"
	"testing"

	"ckb/internal/config"
	"ckb/internal/logging"
)

// TestGitAdapter_GetFileHistory tests file history retrieval
func TestGitAdapter_GetFileHistory(t *testing.T) {
	adapter := setupTestAdapter(t)

	// Get history for a file we know exists - README.md
	history, err := adapter.GetFileHistory("README.md", 5)
	if err != nil {
		t.Fatalf("Failed to get file history: %v", err)
	}

	if history.FilePath != "README.md" {
		t.Errorf("Expected file path 'README.md', got '%s'", history.FilePath)
	}

	if history.CommitCount == 0 {
		t.Error("Expected at least one commit in history")
	}

	if len(history.Commits) == 0 {
		t.Error("Expected at least one commit")
	}

	// Verify first commit has all fields
	if len(history.Commits) > 0 {
		commit := history.Commits[0]
		if commit.Hash == "" {
			t.Error("First commit should have a hash")
		}
		if commit.Author == "" {
			t.Error("First commit should have an author")
		}
		if commit.Timestamp == "" {
			t.Error("First commit should have a timestamp")
		}
		if commit.Message == "" {
			t.Error("First commit should have a message")
		}
	}
}

// TestGitAdapter_GetFileHistory_WithLimit tests limited history
func TestGitAdapter_GetFileHistory_WithLimit(t *testing.T) {
	adapter := setupTestAdapter(t)

	history, err := adapter.GetFileHistory("README.md", 2)
	if err != nil {
		t.Fatalf("Failed to get file history: %v", err)
	}

	if len(history.Commits) > 2 {
		t.Errorf("Expected at most 2 commits with limit=2, got %d", len(history.Commits))
	}
}

// TestGitAdapter_GetFileHistory_EmptyPath tests error for empty path
func TestGitAdapter_GetFileHistory_EmptyPath(t *testing.T) {
	adapter := setupTestAdapter(t)

	_, err := adapter.GetFileHistory("", 10)
	if err == nil {
		t.Error("Expected error for empty file path")
	}
}

// TestGitAdapter_GetFileHistory_NonExistentFile tests handling of non-existent file
func TestGitAdapter_GetFileHistory_NonExistentFile(t *testing.T) {
	adapter := setupTestAdapter(t)

	_, err := adapter.GetFileHistory("nonexistent-file-xyz-123.go", 10)
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

// TestGitAdapter_GetFileCommitCount tests commit count retrieval
func TestGitAdapter_GetFileCommitCount(t *testing.T) {
	adapter := setupTestAdapter(t)

	count, err := adapter.GetFileCommitCount("README.md")
	if err != nil {
		t.Fatalf("Failed to get commit count: %v", err)
	}

	if count == 0 {
		t.Error("Expected at least one commit for README.md")
	}

	t.Logf("README.md has %d commits", count)
}

// TestGitAdapter_GetFileCommitCount_EmptyPath tests error for empty path
func TestGitAdapter_GetFileCommitCount_EmptyPath(t *testing.T) {
	adapter := setupTestAdapter(t)

	_, err := adapter.GetFileCommitCount("")
	if err == nil {
		t.Error("Expected error for empty file path")
	}
}

// TestGitAdapter_GetFileLastModified tests last modified timestamp
func TestGitAdapter_GetFileLastModified(t *testing.T) {
	adapter := setupTestAdapter(t)

	timestamp, err := adapter.GetFileLastModified("README.md")
	if err != nil {
		t.Fatalf("Failed to get last modified: %v", err)
	}

	if timestamp == "" {
		t.Error("Expected non-empty timestamp")
	}

	// Should be ISO 8601 format
	if len(timestamp) < 10 {
		t.Errorf("Timestamp appears malformed: %s", timestamp)
	}

	t.Logf("README.md last modified: %s", timestamp)
}

// TestGitAdapter_GetFileLastModified_EmptyPath tests error for empty path
func TestGitAdapter_GetFileLastModified_EmptyPath(t *testing.T) {
	adapter := setupTestAdapter(t)

	_, err := adapter.GetFileLastModified("")
	if err == nil {
		t.Error("Expected error for empty file path")
	}
}

// TestGitAdapter_GetFileAuthors tests author retrieval
func TestGitAdapter_GetFileAuthors(t *testing.T) {
	adapter := setupTestAdapter(t)

	authors, err := adapter.GetFileAuthors("README.md")
	if err != nil {
		t.Fatalf("Failed to get file authors: %v", err)
	}

	if len(authors) == 0 {
		t.Error("Expected at least one author")
	}

	for _, author := range authors {
		if author == "" {
			t.Error("Found empty author name")
		}
	}

	t.Logf("README.md has %d authors: %v", len(authors), authors)
}

// TestGitAdapter_GetFileAuthors_EmptyPath tests error for empty path
func TestGitAdapter_GetFileAuthors_EmptyPath(t *testing.T) {
	adapter := setupTestAdapter(t)

	_, err := adapter.GetFileAuthors("")
	if err == nil {
		t.Error("Expected error for empty file path")
	}
}

// TestGitAdapter_GetFileChurn tests churn metrics
func TestGitAdapter_GetFileChurn(t *testing.T) {
	adapter := setupTestAdapter(t)

	churn, err := adapter.GetFileChurn("README.md", "")
	if err != nil {
		t.Fatalf("Failed to get file churn: %v", err)
	}

	if churn.FilePath != "README.md" {
		t.Errorf("Expected file path 'README.md', got '%s'", churn.FilePath)
	}

	if churn.ChangeCount == 0 {
		t.Error("Expected at least one change")
	}

	if churn.AuthorCount == 0 {
		t.Error("Expected at least one author")
	}

	t.Logf("Churn: changes=%d, authors=%d, hotspotScore=%.2f",
		churn.ChangeCount, churn.AuthorCount, churn.HotspotScore)
}

// TestGitAdapter_GetFileChurn_WithSince tests churn with time filter
func TestGitAdapter_GetFileChurn_WithSince(t *testing.T) {
	adapter := setupTestAdapter(t)

	// Get churn since 1 year ago
	churn, err := adapter.GetFileChurn("README.md", "1 year ago")
	if err != nil {
		t.Fatalf("Failed to get file churn: %v", err)
	}

	t.Logf("Churn since 1 year ago: changes=%d", churn.ChangeCount)
}

// TestGitAdapter_GetFileChurn_EmptyPath tests error for empty path
func TestGitAdapter_GetFileChurn_EmptyPath(t *testing.T) {
	adapter := setupTestAdapter(t)

	_, err := adapter.GetFileChurn("", "")
	if err == nil {
		t.Error("Expected error for empty file path")
	}
}

// TestGitAdapter_GetHotspots tests hotspot detection
func TestGitAdapter_GetHotspots(t *testing.T) {
	adapter := setupTestAdapter(t)

	hotspots, err := adapter.GetHotspots(5, "")
	if err != nil {
		t.Fatalf("Failed to get hotspots: %v", err)
	}

	if len(hotspots) == 0 {
		t.Log("No hotspots found (may be expected for new repos)")
	}

	// Verify hotspots are sorted by score (descending)
	for i := 1; i < len(hotspots); i++ {
		if hotspots[i].HotspotScore > hotspots[i-1].HotspotScore {
			t.Error("Hotspots should be sorted by score descending")
		}
	}

	for i, h := range hotspots {
		t.Logf("Hotspot %d: %s (score=%.2f, changes=%d)", i, h.FilePath, h.HotspotScore, h.ChangeCount)
	}
}

// TestGitAdapter_GetHotspots_DefaultLimit tests default limit
func TestGitAdapter_GetHotspots_DefaultLimit(t *testing.T) {
	adapter := setupTestAdapter(t)

	// Pass 0 to use default limit
	hotspots, err := adapter.GetHotspots(0, "")
	if err != nil {
		t.Fatalf("Failed to get hotspots: %v", err)
	}

	if len(hotspots) > 10 {
		t.Errorf("Default limit should be 10, got %d hotspots", len(hotspots))
	}
}

// TestGitAdapter_GetTotalChurnMetrics tests total churn metrics
func TestGitAdapter_GetTotalChurnMetrics(t *testing.T) {
	adapter := setupTestAdapter(t)

	metrics, err := adapter.GetTotalChurnMetrics("")
	if err != nil {
		t.Fatalf("Failed to get total churn metrics: %v", err)
	}

	if metrics["totalCommits"] == nil {
		t.Error("Expected totalCommits in metrics")
	}

	if metrics["totalAuthors"] == nil {
		t.Error("Expected totalAuthors in metrics")
	}

	if metrics["changedFiles"] == nil {
		t.Error("Expected changedFiles in metrics")
	}

	t.Logf("Total metrics: %+v", metrics)
}

// TestGitAdapter_GetTotalChurnMetrics_WithSince tests total churn with time filter
func TestGitAdapter_GetTotalChurnMetrics_WithSince(t *testing.T) {
	adapter := setupTestAdapter(t)

	metrics, err := adapter.GetTotalChurnMetrics("30 days ago")
	if err != nil {
		t.Fatalf("Failed to get total churn metrics: %v", err)
	}

	t.Logf("Metrics since 30 days ago: %+v", metrics)
}

// TestGitAdapterCreation_NilLogger tests error with nil logger
func TestGitAdapterCreation_NilLogger(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.RepoRoot = "."

	_, err := NewGitAdapter(cfg, nil)
	if err == nil {
		t.Error("Expected error for nil logger")
	}
}

// TestGitAdapterCreation_NonGitDirectory tests error for non-git directory
func TestGitAdapterCreation_NonGitDirectory(t *testing.T) {
	// Create a temp directory that's not a git repo
	tempDir, err := os.MkdirTemp("", "ckb-git-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cfg := config.DefaultConfig()
	cfg.RepoRoot = tempDir

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	_, err = NewGitAdapter(cfg, logger)
	if err == nil {
		t.Error("Expected error for non-git directory")
	}
}

// TestBackendID tests the backend ID constant
func TestBackendID(t *testing.T) {
	if BackendID != "git" {
		t.Errorf("Expected BackendID 'git', got '%s'", BackendID)
	}
}

// TestCommitInfoStructure tests CommitInfo structure
func TestCommitInfoStructure(t *testing.T) {
	commit := CommitInfo{
		Hash:      "abc123",
		Author:    "Test Author",
		Timestamp: "2023-01-01T00:00:00Z",
		Message:   "Test commit",
	}

	if commit.Hash != "abc123" {
		t.Errorf("Expected hash 'abc123', got '%s'", commit.Hash)
	}
	if commit.Author != "Test Author" {
		t.Errorf("Expected author 'Test Author', got '%s'", commit.Author)
	}
	if commit.Timestamp != "2023-01-01T00:00:00Z" {
		t.Errorf("Expected timestamp, got '%s'", commit.Timestamp)
	}
	if commit.Message != "Test commit" {
		t.Errorf("Expected message 'Test commit', got '%s'", commit.Message)
	}
}

// TestChurnMetricsStructure tests ChurnMetrics structure
func TestChurnMetricsStructure(t *testing.T) {
	metrics := ChurnMetrics{
		FilePath:       "test.go",
		ChangeCount:    10,
		AuthorCount:    3,
		LastModified:   "2023-01-01T00:00:00Z",
		AverageChanges: 25.5,
		HotspotScore:   15.3,
	}

	if metrics.FilePath != "test.go" {
		t.Errorf("Expected file path 'test.go', got '%s'", metrics.FilePath)
	}
	if metrics.ChangeCount != 10 {
		t.Errorf("Expected change count 10, got %d", metrics.ChangeCount)
	}
	if metrics.AuthorCount != 3 {
		t.Errorf("Expected author count 3, got %d", metrics.AuthorCount)
	}
	if metrics.AverageChanges != 25.5 {
		t.Errorf("Expected average changes 25.5, got %f", metrics.AverageChanges)
	}
	if metrics.HotspotScore != 15.3 {
		t.Errorf("Expected hotspot score 15.3, got %f", metrics.HotspotScore)
	}
}

// TestFileHistoryStructure tests FileHistory structure
func TestFileHistoryStructure(t *testing.T) {
	history := FileHistory{
		FilePath:     "test.go",
		CommitCount:  5,
		LastModified: "2023-01-01T00:00:00Z",
		Commits: []CommitInfo{
			{Hash: "abc123", Author: "Test", Timestamp: "2023-01-01T00:00:00Z", Message: "Test"},
		},
	}

	if history.FilePath != "test.go" {
		t.Errorf("Expected file path 'test.go', got '%s'", history.FilePath)
	}
	if history.CommitCount != 5 {
		t.Errorf("Expected commit count 5, got %d", history.CommitCount)
	}
	if len(history.Commits) != 1 {
		t.Errorf("Expected 1 commit, got %d", len(history.Commits))
	}
}

// TestGitAdapter_GetRecentCommitsWithSmallLimit tests with very small limit
func TestGitAdapter_GetRecentCommitsWithSmallLimit(t *testing.T) {
	adapter := setupTestAdapter(t)

	commits, err := adapter.GetRecentCommits(1)
	if err != nil {
		t.Fatalf("Failed to get recent commits: %v", err)
	}

	if len(commits) > 1 {
		t.Errorf("Expected at most 1 commit with limit=1, got %d", len(commits))
	}
}

// TestGitAdapter_GetRecentCommitsWithDefaultLimit tests default limit behavior
func TestGitAdapter_GetRecentCommitsWithDefaultLimit(t *testing.T) {
	adapter := setupTestAdapter(t)

	// Pass 0 to use default
	commits, err := adapter.GetRecentCommits(0)
	if err != nil {
		t.Fatalf("Failed to get recent commits: %v", err)
	}

	if len(commits) > 10 {
		t.Errorf("Default limit should be 10, got %d commits", len(commits))
	}
}

// TestGitAdapter_CustomTimeout tests custom timeout configuration
func TestGitAdapter_CustomTimeout(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	repoRoot := filepath.Join(cwd, "..", "..", "..")

	cfg := &config.Config{
		RepoRoot: repoRoot,
		Backends: config.BackendsConfig{
			Git: config.GitConfig{
				Enabled: true,
			},
		},
		QueryPolicy: config.QueryPolicyConfig{
			TimeoutMs: map[string]int{
				"git": 10000, // 10 seconds
			},
		},
	}

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	adapter, err := NewGitAdapter(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	// Adapter should be created successfully with custom timeout
	if adapter == nil {
		t.Fatal("Adapter should not be nil")
	}
}

// BenchmarkGetRecentCommits benchmarks recent commit retrieval
func BenchmarkGetRecentCommits(b *testing.B) {
	cwd, _ := os.Getwd()
	repoRoot := filepath.Join(cwd, "..", "..", "..")

	cfg := &config.Config{
		RepoRoot: repoRoot,
		Backends: config.BackendsConfig{
			Git: config.GitConfig{
				Enabled: true,
			},
		},
		QueryPolicy: config.QueryPolicyConfig{
			TimeoutMs: map[string]int{
				"git": 5000,
			},
		},
	}

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.ErrorLevel,
	})

	adapter, _ := NewGitAdapter(cfg, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = adapter.GetRecentCommits(10)
	}
}

// BenchmarkGetHeadCommit benchmarks HEAD commit retrieval
func BenchmarkGetHeadCommit(b *testing.B) {
	cwd, _ := os.Getwd()
	repoRoot := filepath.Join(cwd, "..", "..", "..")

	cfg := &config.Config{
		RepoRoot: repoRoot,
		Backends: config.BackendsConfig{
			Git: config.GitConfig{
				Enabled: true,
			},
		},
		QueryPolicy: config.QueryPolicyConfig{
			TimeoutMs: map[string]int{
				"git": 5000,
			},
		},
	}

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.ErrorLevel,
	})

	adapter, _ := NewGitAdapter(cfg, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = adapter.GetHeadCommit()
	}
}
