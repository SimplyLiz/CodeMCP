package repostate

import (
	"crypto/sha256"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"ckb/internal/errors"
)

const (
	// EmptyHash represents an empty diff/list hash
	EmptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// RepoState represents the current state of the repository
type RepoState struct {
	RepoStateID         string `json:"repoStateId"`
	HeadCommit          string `json:"headCommit"`
	StagedDiffHash      string `json:"stagedDiffHash"`
	WorkingTreeDiffHash string `json:"workingTreeDiffHash"`
	UntrackedListHash   string `json:"untrackedListHash"`
	Dirty               bool   `json:"dirty"`
	ComputedAt          string `json:"computedAt"`
}

// ComputeRepoState computes the current repository state using git commands
func ComputeRepoState(repoRoot string) (*RepoState, error) {
	// Get HEAD commit
	headCommit, err := gitRevParse(repoRoot, "HEAD")
	if err != nil {
		return nil, errors.NewCkbError(
			errors.InternalError,
			"Failed to get HEAD commit",
			err,
			[]errors.FixAction{
				{
					Type:        errors.RunCommand,
					Command:     "git status",
					Safe:        true,
					Description: "Check if you're in a valid git repository",
				},
			},
			nil,
		)
	}

	// Get staged diff
	stagedDiff, err := gitDiff(repoRoot, "--cached")
	if err != nil {
		return nil, errors.NewCkbError(
			errors.InternalError,
			"Failed to get staged diff",
			err,
			nil,
			nil,
		)
	}
	stagedDiffHash := hashString(stagedDiff)

	// Get working tree diff
	workingDiff, err := gitDiff(repoRoot, "HEAD")
	if err != nil {
		return nil, errors.NewCkbError(
			errors.InternalError,
			"Failed to get working tree diff",
			err,
			nil,
			nil,
		)
	}
	workingTreeDiffHash := hashString(workingDiff)

	// Get untracked files
	untrackedFiles, err := gitLsFilesOthers(repoRoot)
	if err != nil {
		return nil, errors.NewCkbError(
			errors.InternalError,
			"Failed to get untracked files",
			err,
			nil,
			nil,
		)
	}
	untrackedListHash := hashString(untrackedFiles)

	// Determine if repo is dirty
	dirty := stagedDiffHash != EmptyHash ||
	         workingTreeDiffHash != EmptyHash ||
	         untrackedListHash != EmptyHash

	// Compute composite repoStateId
	repoStateId := computeRepoStateID(headCommit, stagedDiffHash, workingTreeDiffHash, untrackedListHash)

	return &RepoState{
		RepoStateID:         repoStateId,
		HeadCommit:          headCommit,
		StagedDiffHash:      stagedDiffHash,
		WorkingTreeDiffHash: workingTreeDiffHash,
		UntrackedListHash:   untrackedListHash,
		Dirty:               dirty,
		ComputedAt:          time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// gitRevParse executes git rev-parse
func gitRevParse(repoRoot string, args ...string) (string, error) {
	fullArgs := append([]string{"rev-parse"}, args...)
	cmd := exec.Command("git", fullArgs...)
	cmd.Dir = repoRoot

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// gitDiff executes git diff and returns the output
func gitDiff(repoRoot string, args ...string) (string, error) {
	fullArgs := append([]string{"diff"}, args...)
	cmd := exec.Command("git", fullArgs...)
	cmd.Dir = repoRoot

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

// gitLsFilesOthers executes git ls-files --others --exclude-standard
func gitLsFilesOthers(repoRoot string) (string, error) {
	cmd := exec.Command("git", "ls-files", "--others", "--exclude-standard")
	cmd.Dir = repoRoot

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

// hashString computes SHA256 hash of a string
func hashString(s string) string {
	if s == "" {
		return EmptyHash
	}
	h := sha256.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// computeRepoStateID computes the composite repoStateId from all components
func computeRepoStateID(headCommit, stagedHash, workingHash, untrackedHash string) string {
	composite := fmt.Sprintf("%s:%s:%s:%s", headCommit, stagedHash, workingHash, untrackedHash)
	return hashString(composite)
}

// IsGitRepository checks if the given path is a git repository
func IsGitRepository(repoRoot string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = repoRoot
	err := cmd.Run()
	return err == nil
}

// GetRepoRoot finds the git repository root from the given directory
func GetRepoRoot(startPath string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = startPath

	output, err := cmd.Output()
	if err != nil {
		return "", errors.NewCkbError(
			errors.InternalError,
			"Not a git repository",
			err,
			[]errors.FixAction{
				{
					Type:        errors.RunCommand,
					Command:     "git init",
					Safe:        false,
					Description: "Initialize a git repository",
				},
			},
			nil,
		)
	}

	return strings.TrimSpace(string(output)), nil
}
