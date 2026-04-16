package git

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SimplyLiz/CodeMCP/internal/config"
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

// setupRepoPair builds a bare "origin" repo with one commit on main, and a
// clone of it, returning (origin bare dir, clone dir). Isolates EnsureRef
// tests from the host repo's network/credentials.
func setupRepoPair(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	originBare := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")
	clone := filepath.Join(root, "clone")

	runGit := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	runGit(root, "init", "--bare", "-b", "main", originBare)
	runGit(root, "init", "-b", "main", work)
	if err := os.WriteFile(filepath.Join(work, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(work, "add", ".")
	runGit(work, "commit", "-m", "init")
	runGit(work, "remote", "add", "origin", originBare)
	runGit(work, "push", "origin", "main")
	runGit(root, "clone", originBare, clone)
	return originBare, clone
}

func adapterFor(t *testing.T, repoRoot string) *GitAdapter {
	t.Helper()
	cfg := &config.Config{
		RepoRoot: repoRoot,
		Backends: config.BackendsConfig{Git: config.GitConfig{Enabled: true}},
		QueryPolicy: config.QueryPolicyConfig{
			TimeoutMs: map[string]int{"git": 10000},
		},
	}
	a, err := NewGitAdapter(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewGitAdapter: %v", err)
	}
	return a
}

func TestEnsureRef_EmptyInput(t *testing.T) {
	a := setupTestAdapter(t)
	if _, err := a.EnsureRef(""); err == nil {
		t.Fatal("expected error for empty ref")
	}
}

func TestEnsureRef_AlreadyLocal(t *testing.T) {
	_, clone := setupRepoPair(t)
	a := adapterFor(t, clone)
	// main is a local branch in a fresh clone; no fetch should be needed.
	got, err := a.EnsureRef("main")
	if err != nil {
		t.Fatalf("EnsureRef(main): %v", err)
	}
	if got != "main" {
		t.Errorf("expected input returned unchanged; got %q", got)
	}
}

func TestEnsureRef_FetchesMissingFromOrigin(t *testing.T) {
	_, clone := setupRepoPair(t)
	a := adapterFor(t, clone)

	// Simulate shallow-clone state: remove remote-tracking ref for main so
	// only the local 'main' branch remains. Then delete local main too —
	// now neither 'main' nor 'origin/main' resolves locally.
	if _, err := a.executeGitCommand("update-ref", "-d", "refs/remotes/origin/main"); err != nil {
		t.Fatalf("delete origin/main: %v", err)
	}
	// Can't delete the currently checked-out branch — detach first.
	if _, err := a.executeGitCommand("checkout", "--detach", "HEAD"); err != nil {
		t.Fatalf("detach: %v", err)
	}
	if _, err := a.executeGitCommand("branch", "-D", "main"); err != nil {
		t.Fatalf("delete local main: %v", err)
	}

	got, err := a.EnsureRef("refs/heads/main")
	if err != nil {
		t.Fatalf("EnsureRef: %v", err)
	}
	if got != "origin/main" {
		t.Errorf("expected origin/main after fetch, got %q", got)
	}
	// Verify the fetch actually populated the remote-tracking ref.
	if _, err := a.executeGitCommand("rev-parse", "--verify", "origin/main^{commit}"); err != nil {
		t.Errorf("origin/main still missing after EnsureRef: %v", err)
	}
}

func TestIsAuthError(t *testing.T) {
	cases := []struct {
		msg  string
		want bool
	}{
		{"fatal: Authentication failed for 'https://github.com/foo/bar'", true},
		{"fatal: could not read Username for 'https://github.com': terminal prompts disabled", true},
		{"remote: HTTP 401 Unauthorized", true},
		{"fatal: repository 'https://github.com/foo/bar' not found", true},
		{"Permission denied (publickey).", true},
		{"fatal: couldn't find remote ref nonexistent", false},
		{"fatal: bad revision 'xyz'", false},
		{"", false},
	}
	for _, c := range cases {
		var err error
		if c.msg != "" {
			err = fmt.Errorf("%s", c.msg)
		}
		if got := isAuthError(err); got != c.want {
			t.Errorf("isAuthError(%q) = %v, want %v", c.msg, got, c.want)
		}
	}
}

func TestEnsureRef_UnreachableOriginSurfacesError(t *testing.T) {
	_, clone := setupRepoPair(t)
	a := adapterFor(t, clone)

	// Point origin at a nonexistent path.
	if _, err := a.executeGitCommand("remote", "set-url", "origin", filepath.Join(t.TempDir(), "does-not-exist.git")); err != nil {
		t.Fatalf("set-url: %v", err)
	}
	_, err := a.EnsureRef("refs/heads/totally-made-up")
	if err == nil {
		t.Fatal("expected error when origin unreachable")
	}
	if !strings.Contains(err.Error(), "totally-made-up") {
		t.Errorf("error should name the ref; got: %v", err)
	}
}
