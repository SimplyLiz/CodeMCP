package repos

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveActiveRepo_Empty(t *testing.T) {
	// Create a temporary registry
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Clear any CKB_REPO env var
	t.Setenv("CKB_REPO", "")

	// Change to a non-git directory so auto-detection doesn't kick in
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	resolved, err := ResolveActiveRepo("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Entry != nil {
		t.Errorf("expected nil entry for empty registry, got %v", resolved.Entry)
	}

	if resolved.Source != ResolvedNone {
		t.Errorf("expected source %q, got %q", ResolvedNone, resolved.Source)
	}
}

func TestResolveActiveRepo_EnvVar(t *testing.T) {
	// Create a temporary registry with a repo
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create a test repo directory
	repoDir := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Register the repo
	registry, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Add("test", repoDir); err != nil {
		t.Fatal(err)
	}

	// Set env var
	t.Setenv("CKB_REPO", "test")

	resolved, err := ResolveActiveRepo("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Entry == nil {
		t.Fatal("expected entry, got nil")
	}

	if resolved.Entry.Name != "test" {
		t.Errorf("expected name %q, got %q", "test", resolved.Entry.Name)
	}

	if resolved.Source != ResolvedFromEnv {
		t.Errorf("expected source %q, got %q", ResolvedFromEnv, resolved.Source)
	}
}

func TestResolveActiveRepo_Flag(t *testing.T) {
	// Create a temporary registry with a repo
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Clear CKB_REPO env var
	t.Setenv("CKB_REPO", "")

	// Create a test repo directory
	repoDir := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Register the repo
	registry, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Add("test", repoDir); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveActiveRepo("test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Entry == nil {
		t.Fatal("expected entry, got nil")
	}

	if resolved.Entry.Name != "test" {
		t.Errorf("expected name %q, got %q", "test", resolved.Entry.Name)
	}

	if resolved.Source != ResolvedFromFlag {
		t.Errorf("expected source %q, got %q", ResolvedFromFlag, resolved.Source)
	}
}

func TestResolveActiveRepo_CWD(t *testing.T) {
	// Create a temporary registry with a repo
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Clear CKB_REPO env var
	t.Setenv("CKB_REPO", "")

	// Create a test repo directory
	repoDir := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Change to the repo directory FIRST
	origDir, _ := os.Getwd()
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Now register the repo (after chdir so paths resolve correctly)
	registry, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Add("test", repoDir); err != nil {
		t.Fatal(err)
	}

	// Get the absolute path of where we are
	cwd, _ := os.Getwd()
	t.Logf("CWD: %s", cwd)
	t.Logf("Registered repo path: %s", registry.Repos["test"].Path)

	// Use the registry we already have to test with the correct CWD
	resolved, err := ResolveActiveRepoWithRegistry(registry, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Entry == nil {
		t.Logf("findRepoContainingPath returned nil for CWD %s", cwd)
		t.Logf("Registry repos: %+v", registry.Repos)
		t.Fatal("expected entry, got nil")
	}

	if resolved.Entry.Name != "test" {
		t.Errorf("expected name %q, got %q", "test", resolved.Entry.Name)
	}

	if resolved.Source != ResolvedFromCWD {
		t.Errorf("expected source %q, got %q", ResolvedFromCWD, resolved.Source)
	}
}

func TestResolveActiveRepo_Default(t *testing.T) {
	// Create a temporary registry with a repo
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Clear CKB_REPO env var
	t.Setenv("CKB_REPO", "")

	// Create a test repo directory
	repoDir := filepath.Join(tmpDir, "test-repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Register the repo and set as default
	registry, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Add("test", repoDir); err != nil {
		t.Fatal(err)
	}
	if err := registry.SetDefault("test"); err != nil {
		t.Fatal(err)
	}

	// Change to a different directory (not in the repo)
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	resolved, err := ResolveActiveRepo("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Entry == nil {
		t.Fatal("expected entry, got nil")
	}

	if resolved.Entry.Name != "test" {
		t.Errorf("expected name %q, got %q", "test", resolved.Entry.Name)
	}

	if resolved.Source != ResolvedFromDefault {
		t.Errorf("expected source %q, got %q", ResolvedFromDefault, resolved.Source)
	}
}

func TestResolveActiveRepo_Priority(t *testing.T) {
	// Test that env var takes priority over flag and default
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create two test repo directories
	repo1 := filepath.Join(tmpDir, "repo1")
	repo2 := filepath.Join(tmpDir, "repo2")
	if err := os.MkdirAll(repo1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repo2, 0755); err != nil {
		t.Fatal(err)
	}

	// Register both repos
	registry, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Add("one", repo1); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add("two", repo2); err != nil {
		t.Fatal(err)
	}
	if err := registry.SetDefault("one"); err != nil {
		t.Fatal(err)
	}

	// Set env var to override
	t.Setenv("CKB_REPO", "two")

	// Resolve - env var should win
	resolved, err := ResolveActiveRepo("one") // Even with flag set to "one"
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Entry == nil {
		t.Fatal("expected entry, got nil")
	}

	if resolved.Entry.Name != "two" {
		t.Errorf("expected name %q (from env), got %q", "two", resolved.Entry.Name)
	}

	if resolved.Source != ResolvedFromEnv {
		t.Errorf("expected source %q, got %q", ResolvedFromEnv, resolved.Source)
	}
}

func TestFindRepoContainingPath_Subdirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a repo with a subdirectory
	repoDir := filepath.Join(tmpDir, "repo")
	subDir := filepath.Join(repoDir, "src", "pkg")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	registry := &Registry{
		Repos: map[string]RepoEntry{
			"test": {
				Name: "test",
				Path: repoDir,
			},
		},
	}

	// Find repo from subdirectory
	entry := findRepoContainingPath(registry, subDir)
	if entry == nil {
		t.Fatal("expected to find repo from subdirectory")
	}

	if entry.Name != "test" {
		t.Errorf("expected name %q, got %q", "test", entry.Name)
	}
}

func TestFindRepoContainingPath_NotInRepo(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a repo
	repoDir := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create another directory not in the repo
	otherDir := filepath.Join(tmpDir, "other")
	if err := os.MkdirAll(otherDir, 0755); err != nil {
		t.Fatal(err)
	}

	registry := &Registry{
		Repos: map[string]RepoEntry{
			"test": {
				Name: "test",
				Path: repoDir,
			},
		},
	}

	// Try to find repo from outside directory
	entry := findRepoContainingPath(registry, otherDir)
	if entry != nil {
		t.Errorf("expected nil for path outside repo, got %v", entry)
	}
}

func TestFindRepoContainingPath_LongestMatchWins(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested repos:
	// /work (registered as "parent")
	// /work/ckb (registered as "child")
	parentDir := filepath.Join(tmpDir, "work")
	childDir := filepath.Join(parentDir, "ckb")
	targetDir := filepath.Join(childDir, "internal")

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}

	registry := &Registry{
		Repos: map[string]RepoEntry{
			"parent": {
				Name: "parent",
				Path: parentDir,
			},
			"child": {
				Name: "child",
				Path: childDir,
			},
		},
	}

	// When in /work/ckb/internal, should match "child" not "parent"
	entry := findRepoContainingPath(registry, targetDir)
	if entry == nil {
		t.Fatal("expected to find repo")
	}

	if entry.Name != "child" {
		t.Errorf("expected most specific match 'child', got %q", entry.Name)
	}
}

func TestFindGitRoot(t *testing.T) {
	tmpDir := t.TempDir()

	// Resolve symlinks in tmpDir (macOS /var -> /private/var)
	tmpDir, _ = filepath.EvalSymlinks(tmpDir)

	// Create a git repo structure
	gitDir := filepath.Join(tmpDir, "myrepo", ".git")
	subDir := filepath.Join(tmpDir, "myrepo", "src", "pkg")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Test finding git root from subdirectory
	root := FindGitRoot(subDir)
	expected := filepath.Join(tmpDir, "myrepo")
	if root != expected {
		t.Errorf("expected git root %q, got %q", expected, root)
	}

	// Test finding git root from the root itself
	root = FindGitRoot(expected)
	if root != expected {
		t.Errorf("expected git root %q, got %q", expected, root)
	}

	// Test not in a git repo
	nonGitDir := filepath.Join(tmpDir, "notgit")
	if err := os.MkdirAll(nonGitDir, 0755); err != nil {
		t.Fatal(err)
	}
	root = FindGitRoot(nonGitDir)
	if root != "" {
		t.Errorf("expected empty string for non-git dir, got %q", root)
	}
}

func TestFindGitRoot_WorktreeFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Resolve symlinks in tmpDir (macOS /var -> /private/var)
	tmpDir, _ = filepath.EvalSymlinks(tmpDir)

	// Create a git worktree structure (.git is a file, not directory)
	repoDir := filepath.Join(tmpDir, "worktree")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	gitFile := filepath.Join(repoDir, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: /some/path/.git/worktrees/test"), 0644); err != nil {
		t.Fatal(err)
	}

	root := FindGitRoot(repoDir)
	if root != repoDir {
		t.Errorf("expected git root %q for worktree, got %q", repoDir, root)
	}
}

func TestResolveActiveRepo_CWDGit(t *testing.T) {
	// Test auto-detection of unregistered git repos
	tmpDir := t.TempDir()

	// Resolve symlinks in tmpDir (macOS /var -> /private/var)
	tmpDir, _ = filepath.EvalSymlinks(tmpDir)

	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Clear CKB_REPO env var
	t.Setenv("CKB_REPO", "")

	// Create a git repo that's NOT registered
	gitRepoDir := filepath.Join(tmpDir, "unregistered-project")
	gitDir := filepath.Join(gitRepoDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Change to the git repo directory
	origDir, _ := os.Getwd()
	if err := os.Chdir(gitRepoDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Resolve - should auto-detect the git repo
	resolved, err := ResolveActiveRepo("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Entry == nil {
		t.Fatal("expected entry, got nil")
	}

	if resolved.Entry.Name != "unregistered-project" {
		t.Errorf("expected name %q (from dir name), got %q", "unregistered-project", resolved.Entry.Name)
	}

	if resolved.Source != ResolvedFromCWDGit {
		t.Errorf("expected source %q, got %q", ResolvedFromCWDGit, resolved.Source)
	}

	if resolved.State != RepoStateUninitialized {
		t.Errorf("expected state %q, got %q", RepoStateUninitialized, resolved.State)
	}

	if resolved.DetectedGitRoot != gitRepoDir {
		t.Errorf("expected DetectedGitRoot %q, got %q", gitRepoDir, resolved.DetectedGitRoot)
	}
}

func TestResolveActiveRepo_CWDGitWithDefault(t *testing.T) {
	// Test that auto-detected git repo takes precedence over default
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Clear CKB_REPO env var
	t.Setenv("CKB_REPO", "")

	// Create and register a default repo
	defaultRepo := filepath.Join(tmpDir, "default-repo")
	if err := os.MkdirAll(defaultRepo, 0755); err != nil {
		t.Fatal(err)
	}
	registry, err := LoadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Add("default", defaultRepo); err != nil {
		t.Fatal(err)
	}
	if err := registry.SetDefault("default"); err != nil {
		t.Fatal(err)
	}

	// Create an unregistered git repo
	gitRepoDir := filepath.Join(tmpDir, "other-project")
	gitDir := filepath.Join(gitRepoDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Change to the git repo directory
	origDir, _ := os.Getwd()
	if err := os.Chdir(gitRepoDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	// Resolve - should use git repo, not default
	resolved, err := ResolveActiveRepo("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Entry == nil {
		t.Fatal("expected entry, got nil")
	}

	if resolved.Entry.Name != "other-project" {
		t.Errorf("expected git repo name, got %q", resolved.Entry.Name)
	}

	if resolved.Source != ResolvedFromCWDGit {
		t.Errorf("expected source %q, got %q", ResolvedFromCWDGit, resolved.Source)
	}

	if resolved.SkippedDefault != "default" {
		t.Errorf("expected SkippedDefault %q, got %q", "default", resolved.SkippedDefault)
	}
}

func TestCheckCkbInitialized(t *testing.T) {
	tmpDir := t.TempDir()

	// Not initialized
	if checkCkbInitialized(tmpDir) {
		t.Error("expected false for non-initialized dir")
	}

	// Initialize
	ckbDir := filepath.Join(tmpDir, ".ckb")
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		t.Fatal(err)
	}

	if !checkCkbInitialized(tmpDir) {
		t.Error("expected true for initialized dir")
	}
}
