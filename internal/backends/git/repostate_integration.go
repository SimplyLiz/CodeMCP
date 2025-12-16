package git

import (
	"ckb/internal/repostate"
)

// GetRepoState returns the current repository state
// This integrates with the existing internal/repostate package
func (g *GitAdapter) GetRepoState() (*repostate.RepoState, error) {
	g.logger.Debug("Computing repository state", map[string]interface{}{
		"repoRoot": g.repoRoot,
	})

	state, err := repostate.ComputeRepoState(g.repoRoot)
	if err != nil {
		return nil, err
	}

	g.logger.Debug("Repository state computed", map[string]interface{}{
		"repoStateId": state.RepoStateID,
		"headCommit":  state.HeadCommit,
		"dirty":       state.Dirty,
	})

	return state, nil
}

// GetRepoStateID returns just the composite repoStateId for cache keys
// This is a convenience method for cache key generation
func (g *GitAdapter) GetRepoStateID() (string, error) {
	state, err := g.GetRepoState()
	if err != nil {
		return "", err
	}
	return state.RepoStateID, nil
}

// GetHeadCommit returns the current HEAD commit hash
func (g *GitAdapter) GetHeadCommit() (string, error) {
	g.logger.Debug("Getting HEAD commit", nil)

	output, err := g.executeGitCommand("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}

	return output, nil
}

// IsDirty checks if the repository has uncommitted changes
func (g *GitAdapter) IsDirty() (bool, error) {
	state, err := g.GetRepoState()
	if err != nil {
		return false, err
	}
	return state.Dirty, nil
}

// GetCurrentBranch returns the name of the current branch
func (g *GitAdapter) GetCurrentBranch() (string, error) {
	g.logger.Debug("Getting current branch", nil)

	output, err := g.executeGitCommand("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}

	return output, nil
}

// GetRemoteURL returns the URL of the remote repository
// Uses 'origin' as the default remote name
func (g *GitAdapter) GetRemoteURL() (string, error) {
	g.logger.Debug("Getting remote URL", nil)

	output, err := g.executeGitCommand("remote", "get-url", "origin")
	if err != nil {
		// Non-fatal - repository might not have a remote
		g.logger.Debug("No remote URL found", map[string]interface{}{
			"error": err.Error(),
		})
		return "", nil //nolint:nilerr // no remote is valid
	}

	return output, nil
}

// GetRepositoryInfo returns comprehensive repository information
func (g *GitAdapter) GetRepositoryInfo() (map[string]interface{}, error) {
	g.logger.Debug("Getting repository info", nil)

	// Get repo state
	state, err := g.GetRepoState()
	if err != nil {
		return nil, err
	}

	// Get current branch
	branch, err := g.GetCurrentBranch()
	if err != nil {
		// Non-fatal
		branch = ""
	}

	// Get remote URL
	remoteURL, err := g.GetRemoteURL()
	if err != nil {
		// Non-fatal
		remoteURL = ""
	}

	// Get untracked files count
	untracked, err := g.GetUntrackedFiles()
	if err != nil {
		// Non-fatal
		untracked = []string{}
	}

	// Get staged files count
	staged, err := g.GetStagedDiff()
	if err != nil {
		// Non-fatal
		staged = []DiffStats{}
	}

	// Get working tree changes count
	working, err := g.GetWorkingTreeDiff()
	if err != nil {
		// Non-fatal
		working = []DiffStats{}
	}

	return map[string]interface{}{
		"repoRoot":        g.repoRoot,
		"repoStateId":     state.RepoStateID,
		"headCommit":      state.HeadCommit,
		"dirty":           state.Dirty,
		"branch":          branch,
		"remoteURL":       remoteURL,
		"stagedFiles":     len(staged),
		"modifiedFiles":   len(working),
		"untrackedFiles":  len(untracked),
		"stagedDiffHash":  state.StagedDiffHash,
		"workingDiffHash": state.WorkingTreeDiffHash,
		"untrackedHash":   state.UntrackedListHash,
		"computedAt":      state.ComputedAt,
	}, nil
}

// ValidateRepoStateID checks if the current repo state matches the given ID
// This is useful for cache validation
func (g *GitAdapter) ValidateRepoStateID(expectedID string) (bool, error) {
	currentID, err := g.GetRepoStateID()
	if err != nil {
		return false, err
	}

	return currentID == expectedID, nil
}

// GetFileStatus returns the git status for a specific file
func (g *GitAdapter) GetFileStatus(filePath string) (string, error) {
	g.logger.Debug("Getting file status", map[string]interface{}{
		"filePath": filePath,
	})

	// Use git status --porcelain to get machine-readable status
	output, err := g.executeGitCommand("status", "--porcelain", "--", filePath)
	if err != nil {
		return "", err
	}

	if output == "" {
		return "unmodified", nil
	}

	// Parse porcelain output
	// Format: "XY filename" where X is staged status, Y is unstaged status
	if len(output) < 2 {
		return "unknown", nil
	}

	stagedStatus := output[0:1]
	unstagedStatus := output[1:2]

	// Interpret status codes
	if stagedStatus == "?" {
		return "untracked", nil
	} else if stagedStatus == "A" {
		return "added", nil
	} else if stagedStatus == "M" {
		return "staged-modified", nil
	} else if stagedStatus == "D" {
		return "deleted", nil
	} else if stagedStatus == "R" {
		return "renamed", nil
	} else if unstagedStatus == "M" {
		return "modified", nil
	} else if unstagedStatus == "D" {
		return "deleted", nil
	}

	return "unknown", nil
}

// IsFileTracked checks if a file is tracked by git
func (g *GitAdapter) IsFileTracked(filePath string) (bool, error) {
	g.logger.Debug("Checking if file is tracked", map[string]interface{}{
		"filePath": filePath,
	})

	// Use git ls-files to check if file is tracked
	output, err := g.executeGitCommand("ls-files", "--error-unmatch", "--", filePath)
	if err != nil {
		// If error, file is not tracked
		return false, nil //nolint:nilerr // error means untracked
	}

	return output != "", nil
}
