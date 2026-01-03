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
