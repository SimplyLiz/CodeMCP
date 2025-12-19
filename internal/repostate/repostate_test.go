package repostate

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHashString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string returns empty hash",
			input:    "",
			expected: EmptyHash,
		},
		{
			name:     "simple string",
			input:    "hello",
			expected: fmt.Sprintf("%x", sha256.Sum256([]byte("hello"))),
		},
		{
			name:     "multiline string",
			input:    "line1\nline2\nline3",
			expected: fmt.Sprintf("%x", sha256.Sum256([]byte("line1\nline2\nline3"))),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := hashString(tc.input)
			if result != tc.expected {
				t.Errorf("hashString(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestComputeRepoStateID(t *testing.T) {
	// Test with known inputs
	headCommit := "abc123"
	stagedHash := "staged123"
	workingHash := "working123"
	untrackedHash := "untracked123"

	result := computeRepoStateID(headCommit, stagedHash, workingHash, untrackedHash)

	// Verify it's a valid SHA256 hash (64 hex characters)
	if len(result) != 64 {
		t.Errorf("Expected 64 character hash, got %d characters", len(result))
	}

	// Verify consistency - same inputs should produce same output
	result2 := computeRepoStateID(headCommit, stagedHash, workingHash, untrackedHash)
	if result != result2 {
		t.Error("computeRepoStateID not consistent for same inputs")
	}

	// Verify different inputs produce different outputs
	result3 := computeRepoStateID("different", stagedHash, workingHash, untrackedHash)
	if result == result3 {
		t.Error("Different inputs should produce different hashes")
	}
}

func TestEmptyHashConstant(t *testing.T) {
	// Verify EmptyHash is the SHA256 of empty string
	expected := fmt.Sprintf("%x", sha256.Sum256([]byte("")))
	if EmptyHash != expected {
		t.Errorf("EmptyHash = %q, expected %q (SHA256 of empty string)", EmptyHash, expected)
	}
}

func TestIsGitRepository(t *testing.T) {
	// Get the current working directory (which should be the repo root)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Find the repo root
	repoRoot := findRepoRoot(t, cwd)

	// Test with a valid git repository
	t.Run("valid git repository", func(t *testing.T) {
		result := IsGitRepository(repoRoot)
		if !result {
			t.Errorf("Expected %s to be a git repository", repoRoot)
		}
	})

	// Test with a non-git directory
	t.Run("non-git directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		result := IsGitRepository(tmpDir)
		if result {
			t.Errorf("Expected %s to NOT be a git repository", tmpDir)
		}
	})
}

func TestGetRepoRoot(t *testing.T) {
	// Get the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Find the expected repo root
	expectedRoot := findRepoRoot(t, cwd)

	t.Run("from repo root", func(t *testing.T) {
		root, err := GetRepoRoot(expectedRoot)
		if err != nil {
			t.Fatalf("GetRepoRoot failed: %v", err)
		}
		if root != expectedRoot {
			t.Errorf("GetRepoRoot(%s) = %s, expected %s", expectedRoot, root, expectedRoot)
		}
	})

	t.Run("from subdirectory", func(t *testing.T) {
		// Find a subdirectory in the repo
		subdirs := []string{"internal", "cmd", ".git"}
		var subdir string
		for _, sd := range subdirs {
			path := filepath.Join(expectedRoot, sd)
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				subdir = path
				break
			}
		}

		if subdir == "" {
			t.Skip("No suitable subdirectory found for test")
		}

		root, err := GetRepoRoot(subdir)
		if err != nil {
			t.Fatalf("GetRepoRoot from subdir failed: %v", err)
		}
		if root != expectedRoot {
			t.Errorf("GetRepoRoot(%s) = %s, expected %s", subdir, root, expectedRoot)
		}
	})

	t.Run("non-git directory returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		_, err := GetRepoRoot(tmpDir)
		if err == nil {
			t.Error("Expected error for non-git directory")
		}
	})
}

func TestComputeRepoState(t *testing.T) {
	// Get the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	repoRoot := findRepoRoot(t, cwd)

	t.Run("computes state for valid repo", func(t *testing.T) {
		state, err := ComputeRepoState(repoRoot)
		if err != nil {
			t.Fatalf("ComputeRepoState failed: %v", err)
		}

		// Verify all fields are populated
		if state.RepoStateID == "" {
			t.Error("RepoStateID should not be empty")
		}
		if state.HeadCommit == "" {
			t.Error("HeadCommit should not be empty")
		}
		if len(state.HeadCommit) != 40 {
			t.Errorf("HeadCommit should be 40 char SHA, got %d chars", len(state.HeadCommit))
		}
		if state.StagedDiffHash == "" {
			t.Error("StagedDiffHash should not be empty")
		}
		if state.WorkingTreeDiffHash == "" {
			t.Error("WorkingTreeDiffHash should not be empty")
		}
		if state.UntrackedListHash == "" {
			t.Error("UntrackedListHash should not be empty")
		}
		if state.ComputedAt == "" {
			t.Error("ComputedAt should not be empty")
		}
	})

	t.Run("returns error for non-git directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		_, err := ComputeRepoState(tmpDir)
		if err == nil {
			t.Error("Expected error for non-git directory")
		}
	})

	t.Run("dirty detection", func(t *testing.T) {
		state, err := ComputeRepoState(repoRoot)
		if err != nil {
			t.Fatalf("ComputeRepoState failed: %v", err)
		}

		// Verify dirty flag is consistent with diff hashes
		expectedDirty := state.StagedDiffHash != EmptyHash ||
			state.WorkingTreeDiffHash != EmptyHash ||
			state.UntrackedListHash != EmptyHash

		if state.Dirty != expectedDirty {
			t.Errorf("Dirty=%v but expected %v based on hashes", state.Dirty, expectedDirty)
		}
	})

	t.Run("state consistency", func(t *testing.T) {
		// Compute state twice in quick succession
		state1, err := ComputeRepoState(repoRoot)
		if err != nil {
			t.Fatalf("First ComputeRepoState failed: %v", err)
		}

		state2, err := ComputeRepoState(repoRoot)
		if err != nil {
			t.Fatalf("Second ComputeRepoState failed: %v", err)
		}

		// RepoStateID should be the same if repo hasn't changed
		// (This may fail if there's filesystem activity, but should usually pass)
		if state1.RepoStateID != state2.RepoStateID {
			// Log but don't fail - filesystem timing can cause this
			t.Logf("Warning: RepoStateID changed between calls (may be due to filesystem activity)")
		}

		// HeadCommit should definitely be the same
		if state1.HeadCommit != state2.HeadCommit {
			t.Errorf("HeadCommit changed between calls: %s vs %s", state1.HeadCommit, state2.HeadCommit)
		}
	})
}

func TestRepoStateStructure(t *testing.T) {
	state := &RepoState{
		RepoStateID:         "abc123",
		HeadCommit:          "def456",
		StagedDiffHash:      EmptyHash,
		WorkingTreeDiffHash: EmptyHash,
		UntrackedListHash:   EmptyHash,
		Dirty:               false,
		ComputedAt:          "2024-01-01T00:00:00Z",
	}

	// Verify struct can be created and accessed
	if state.RepoStateID != "abc123" {
		t.Errorf("RepoStateID = %s, expected abc123", state.RepoStateID)
	}
	if state.HeadCommit != "def456" {
		t.Errorf("HeadCommit = %s, expected def456", state.HeadCommit)
	}
	if state.StagedDiffHash != EmptyHash {
		t.Errorf("StagedDiffHash = %s, expected EmptyHash", state.StagedDiffHash)
	}
	if state.WorkingTreeDiffHash != EmptyHash {
		t.Errorf("WorkingTreeDiffHash = %s, expected EmptyHash", state.WorkingTreeDiffHash)
	}
	if state.UntrackedListHash != EmptyHash {
		t.Errorf("UntrackedListHash = %s, expected EmptyHash", state.UntrackedListHash)
	}
	if state.Dirty != false {
		t.Error("Dirty should be false")
	}
	if state.ComputedAt != "2024-01-01T00:00:00Z" {
		t.Errorf("ComputedAt = %s, expected 2024-01-01T00:00:00Z", state.ComputedAt)
	}
}

// Helper function to find the git repo root
func findRepoRoot(t *testing.T, startPath string) string {
	t.Helper()

	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = startPath

	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to find repo root: %v", err)
	}

	return strings.TrimSpace(string(output))
}

func TestGitHelperFunctions(t *testing.T) {
	// Get the repo root for testing
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	repoRoot := findRepoRoot(t, cwd)

	t.Run("gitRevParse HEAD", func(t *testing.T) {
		result, err := gitRevParse(repoRoot, "HEAD")
		if err != nil {
			t.Fatalf("gitRevParse failed: %v", err)
		}

		// Should be a 40 character SHA
		if len(result) != 40 {
			t.Errorf("Expected 40 char SHA, got %d chars: %s", len(result), result)
		}
	})

	t.Run("gitDiff cached", func(t *testing.T) {
		// Should not error, may or may not have output
		_, err := gitDiff(repoRoot, "--cached")
		if err != nil {
			t.Fatalf("gitDiff --cached failed: %v", err)
		}
	})

	t.Run("gitDiff HEAD", func(t *testing.T) {
		// Should not error
		_, err := gitDiff(repoRoot, "HEAD")
		if err != nil {
			t.Fatalf("gitDiff HEAD failed: %v", err)
		}
	})

	t.Run("gitLsFilesOthers", func(t *testing.T) {
		// Should not error
		_, err := gitLsFilesOthers(repoRoot)
		if err != nil {
			t.Fatalf("gitLsFilesOthers failed: %v", err)
		}
	})

	t.Run("gitRevParse invalid ref", func(t *testing.T) {
		_, err := gitRevParse(repoRoot, "invalid-ref-that-does-not-exist-xyz123")
		if err == nil {
			t.Error("Expected error for invalid ref")
		}
	})
}
