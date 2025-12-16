package git

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"ckb/internal/config"
	"ckb/internal/errors"
	"ckb/internal/logging"
	"ckb/internal/repostate"
)

const (
	// BackendID is the unique identifier for the Git backend
	BackendID = "git"

	// DefaultQueryTimeout is the default timeout for git operations (5000ms)
	DefaultQueryTimeout = 5000 * time.Millisecond
)

// GitAdapter implements the GitBackend interface
type GitAdapter struct {
	repoRoot     string
	queryTimeout time.Duration
	logger       *logging.Logger
	enabled      bool
}

// NewGitAdapter creates a new Git backend adapter
// Git backend is always enabled per design ("git is always")
func NewGitAdapter(cfg *config.Config, logger *logging.Logger) (*GitAdapter, error) {
	if logger == nil {
		return nil, errors.NewCkbError(
			errors.InternalError,
			"Logger is required for GitAdapter",
			nil,
			nil,
			nil,
		)
	}

	// Get query timeout from config, default to 5000ms
	timeout := DefaultQueryTimeout
	if timeoutMs, ok := cfg.QueryPolicy.TimeoutMs[BackendID]; ok && timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}

	// Git is always enabled per design
	enabled := true
	if !cfg.Backends.Git.Enabled {
		logger.Warn("Git backend is disabled in config, but Git is always available", map[string]interface{}{
			"backend": BackendID,
		})
	}

	adapter := &GitAdapter{
		repoRoot:     cfg.RepoRoot,
		queryTimeout: timeout,
		logger:       logger,
		enabled:      enabled,
	}

	// Verify git is available
	if !adapter.IsAvailable() {
		return nil, errors.NewCkbError(
			errors.BackendUnavailable,
			"Git is not available in this repository",
			nil,
			[]errors.FixAction{
				{
					Type:        errors.RunCommand,
					Command:     "git status",
					Safe:        true,
					Description: "Verify you're in a git repository",
				},
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

	logger.Info("Git adapter initialized", map[string]interface{}{
		"backend":      BackendID,
		"repoRoot":     cfg.RepoRoot,
		"timeout":      timeout.String(),
		"enabled":      enabled,
	})

	return adapter, nil
}

// ID returns the backend identifier
func (g *GitAdapter) ID() string {
	return BackendID
}

// IsAvailable checks if git is available and this is a git repository
func (g *GitAdapter) IsAvailable() bool {
	if !g.enabled {
		return false
	}
	return repostate.IsGitRepository(g.repoRoot)
}

// Capabilities returns the list of capabilities this backend supports
func (g *GitAdapter) Capabilities() []string {
	return []string{
		"repo-state",
		"file-history",
		"churn-metrics",
		"blame-info",
		"diff-stats",
		"hotspots",
	}
}

// executeGitCommand runs a git command with timeout and returns the output
func (g *GitAdapter) executeGitCommand(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), g.queryTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.repoRoot

	g.logger.Debug("Executing git command", map[string]interface{}{
		"args":    args,
		"timeout": g.queryTimeout.String(),
	})

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", errors.NewCkbError(
				errors.Timeout,
				"Git command timed out",
				err,
				nil,
				nil,
			)
		}

		// Check if it's an exit error with stderr
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			return "", errors.NewCkbError(
				errors.InternalError,
				"Git command failed",
				err,
				nil,
				nil,
			).WithDetails(map[string]interface{}{
				"args":   args,
				"stderr": stderr,
			})
		}

		return "", errors.NewCkbError(
			errors.InternalError,
			"Failed to execute git command",
			err,
			nil,
			nil,
		)
	}

	return strings.TrimSpace(string(output)), nil
}

// executeGitCommandLines runs a git command and returns output as lines
func (g *GitAdapter) executeGitCommandLines(args ...string) ([]string, error) {
	output, err := g.executeGitCommand(args...)
	if err != nil {
		return nil, err
	}

	if output == "" {
		return []string{}, nil
	}

	lines := strings.Split(output, "\n")
	// Filter out empty lines
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result, nil
}
