package git

import (
t"io"
t"log/slog"
	"os"
	"path/filepath"
	"testing"

	"ckb/internal/config"
)

// setupTestAdapter creates a test adapter using the current repository
func setupTestAdapter(t *testing.T) *GitAdapter {
	// Get the repo root (go up from internal/backends/git to project root)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Navigate up to repo root
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

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		Format: logging.HumanFormat,
		Level:  logging.DebugLevel,
	})

	adapter, err := NewGitAdapter(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	return adapter
}

func TestGitAdapter_ID(t *testing.T) {
	adapter := setupTestAdapter(t)

	if adapter.ID() != BackendID {
		t.Errorf("Expected ID %s, got %s", BackendID, adapter.ID())
	}
}

func TestGitAdapter_IsAvailable(t *testing.T) {
	adapter := setupTestAdapter(t)

	if !adapter.IsAvailable() {
		t.Error("Git adapter should be available in a git repository")
	}
}

func TestGitAdapter_Capabilities(t *testing.T) {
	adapter := setupTestAdapter(t)

	capabilities := adapter.Capabilities()
	if len(capabilities) == 0 {
		t.Error("Expected capabilities, got none")
	}

	// Check for expected capabilities
	expectedCaps := map[string]bool{
		"repo-state":    false,
		"file-history":  false,
		"churn-metrics": false,
		"blame-info":    false,
		"diff-stats":    false,
		"hotspots":      false,
	}

	for _, cap := range capabilities {
		expectedCaps[cap] = true
	}

	for cap, found := range expectedCaps {
		if !found {
			t.Errorf("Expected capability %s not found", cap)
		}
	}
}

func TestGitAdapter_GetHeadCommit(t *testing.T) {
	adapter := setupTestAdapter(t)

	commit, err := adapter.GetHeadCommit()
	if err != nil {
		t.Fatalf("Failed to get HEAD commit: %v", err)
	}

	if commit == "" {
		t.Error("Expected non-empty commit hash")
	}

	if len(commit) != 40 {
		t.Errorf("Expected 40 character commit hash, got %d characters", len(commit))
	}
}

func TestGitAdapter_GetCurrentBranch(t *testing.T) {
	adapter := setupTestAdapter(t)

	branch, err := adapter.GetCurrentBranch()
	if err != nil {
		t.Fatalf("Failed to get current branch: %v", err)
	}

	if branch == "" {
		t.Error("Expected non-empty branch name")
	}

	t.Logf("Current branch: %s", branch)
}

func TestGitAdapter_GetRepoState(t *testing.T) {
	adapter := setupTestAdapter(t)

	state, err := adapter.GetRepoState()
	if err != nil {
		t.Fatalf("Failed to get repo state: %v", err)
	}

	if state.RepoStateID == "" {
		t.Error("Expected non-empty repoStateId")
	}

	if state.HeadCommit == "" {
		t.Error("Expected non-empty headCommit")
	}

	if state.ComputedAt == "" {
		t.Error("Expected non-empty computedAt")
	}

	t.Logf("RepoStateID: %s", state.RepoStateID)
	t.Logf("HeadCommit: %s", state.HeadCommit)
	t.Logf("Dirty: %v", state.Dirty)
}

func TestGitAdapter_GetRepositoryInfo(t *testing.T) {
	adapter := setupTestAdapter(t)

	info, err := adapter.GetRepositoryInfo()
	if err != nil {
		t.Fatalf("Failed to get repository info: %v", err)
	}

	if info["repoRoot"] == "" {
		t.Error("Expected non-empty repoRoot")
	}

	if info["repoStateId"] == "" {
		t.Error("Expected non-empty repoStateId")
	}

	if info["headCommit"] == "" {
		t.Error("Expected non-empty headCommit")
	}

	t.Logf("Repository Info: %+v", info)
}

func TestGitAdapter_GetRecentCommits(t *testing.T) {
	adapter := setupTestAdapter(t)

	commits, err := adapter.GetRecentCommits(5)
	if err != nil {
		t.Fatalf("Failed to get recent commits: %v", err)
	}

	if len(commits) == 0 {
		t.Error("Expected at least one commit")
	}

	for i, commit := range commits {
		if commit.Hash == "" {
			t.Errorf("Commit %d has empty hash", i)
		}
		if commit.Author == "" {
			t.Errorf("Commit %d has empty author", i)
		}
		if commit.Timestamp == "" {
			t.Errorf("Commit %d has empty timestamp", i)
		}
		if commit.Message == "" {
			t.Errorf("Commit %d has empty message", i)
		}

		t.Logf("Commit %d: %s - %s by %s", i, commit.Hash[:8], commit.Message, commit.Author)
	}
}

func TestGitAdapter_GetUntrackedFiles(t *testing.T) {
	adapter := setupTestAdapter(t)

	files, err := adapter.GetUntrackedFiles()
	if err != nil {
		t.Fatalf("Failed to get untracked files: %v", err)
	}

	t.Logf("Found %d untracked files", len(files))
}

func TestGitAdapter_GetStagedDiff(t *testing.T) {
	adapter := setupTestAdapter(t)

	stats, err := adapter.GetStagedDiff()
	if err != nil {
		t.Fatalf("Failed to get staged diff: %v", err)
	}

	t.Logf("Found %d staged files", len(stats))
}

func TestGitAdapter_GetWorkingTreeDiff(t *testing.T) {
	adapter := setupTestAdapter(t)

	stats, err := adapter.GetWorkingTreeDiff()
	if err != nil {
		t.Fatalf("Failed to get working tree diff: %v", err)
	}

	t.Logf("Found %d modified files", len(stats))
}

func TestGitAdapter_GetDiffSummary(t *testing.T) {
	adapter := setupTestAdapter(t)

	summary, err := adapter.GetDiffSummary()
	if err != nil {
		t.Fatalf("Failed to get diff summary: %v", err)
	}

	t.Logf("Diff Summary: %+v", summary)
}
