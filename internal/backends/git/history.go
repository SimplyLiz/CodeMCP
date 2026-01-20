package git

import (
	"fmt"
	"strconv"
	"strings"

	"ckb/internal/errors"
)

// CommitInfo represents information about a single commit
type CommitInfo struct {
	Hash      string `json:"hash"`
	Author    string `json:"author"`
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"` // First line only
}

// FileHistory represents the commit history for a file
type FileHistory struct {
	FilePath     string       `json:"filePath"`
	CommitCount  int          `json:"commitCount"`
	LastModified string       `json:"lastModified"`
	Commits      []CommitInfo `json:"commits"` // Most recent first
}

// GetFileHistory returns the commit history for a specific file
// limit: maximum number of commits to return (0 = no limit)
func (g *GitAdapter) GetFileHistory(filePath string, limit int) (*FileHistory, error) {
	if filePath == "" {
		return nil, errors.NewCkbError(
			errors.InternalError,
			"File path is required",
			nil,
			nil,
			nil,
		)
	}

	g.logger.Debug("Getting file history",
		"filePath", filePath,
		"limit", limit,
	)

	// Build git log command
	// Format: hash|author|timestamp|subject
	// %H = commit hash, %an = author name, %aI = author date ISO 8601, %s = subject
	args := []string{
		"log",
		"--format=%H|%an|%aI|%s",
		"--follow", // Follow file renames
	}

	if limit > 0 {
		args = append(args, fmt.Sprintf("-n%d", limit))
	}

	args = append(args, "--", filePath)

	// Execute git log
	lines, err := g.executeGitCommandLines(args...)
	if err != nil {
		return nil, err
	}

	if len(lines) == 0 {
		return nil, errors.NewCkbError(
			errors.SymbolNotFound,
			"No history found for file",
			nil,
			nil,
			nil,
		).WithDetails(map[string]interface{}{
			"filePath": filePath,
		})
	}

	// Parse commits
	commits := make([]CommitInfo, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			g.logger.Warn("Skipping malformed git log line",
				"line", line,
			)
			continue
		}

		commits = append(commits, CommitInfo{
			Hash:      parts[0],
			Author:    parts[1],
			Timestamp: parts[2],
			Message:   parts[3],
		})
	}

	// Get last modified timestamp (first commit in the list = most recent)
	lastModified := ""
	if len(commits) > 0 {
		lastModified = commits[0].Timestamp
	}

	return &FileHistory{
		FilePath:     filePath,
		CommitCount:  len(commits),
		LastModified: lastModified,
		Commits:      commits,
	}, nil
}

// GetRecentCommits returns the most recent commits in the repository
// limit: maximum number of commits to return
func (g *GitAdapter) GetRecentCommits(limit int) ([]CommitInfo, error) {
	if limit <= 0 {
		limit = 10 // Default to 10 commits
	}

	g.logger.Debug("Getting recent commits",
		"limit", limit,
	)

	// Build git log command
	args := []string{
		"log",
		"--format=%H|%an|%aI|%s",
		fmt.Sprintf("-n%d", limit),
	}

	// Execute git log
	lines, err := g.executeGitCommandLines(args...)
	if err != nil {
		return nil, err
	}

	if len(lines) == 0 {
		return []CommitInfo{}, nil
	}

	// Parse commits
	commits := make([]CommitInfo, 0, len(lines))
	for _, line := range lines {
		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			g.logger.Warn("Skipping malformed git log line",
				"line", line,
			)
			continue
		}

		commits = append(commits, CommitInfo{
			Hash:      parts[0],
			Author:    parts[1],
			Timestamp: parts[2],
			Message:   parts[3],
		})
	}

	return commits, nil
}

// GetFileCommitCount returns the number of commits that touched a file
func (g *GitAdapter) GetFileCommitCount(filePath string) (int, error) {
	if filePath == "" {
		return 0, errors.NewCkbError(
			errors.InternalError,
			"File path is required",
			nil,
			nil,
			nil,
		)
	}

	output, err := g.executeGitCommand("rev-list", "--count", "HEAD", "--", filePath)
	if err != nil {
		return 0, err
	}

	count, err := strconv.Atoi(output)
	if err != nil {
		return 0, errors.NewCkbError(
			errors.InternalError,
			"Failed to parse commit count",
			err,
			nil,
			nil,
		)
	}

	return count, nil
}

// GetFileLastModified returns the timestamp of the last commit that modified a file
func (g *GitAdapter) GetFileLastModified(filePath string) (string, error) {
	if filePath == "" {
		return "", errors.NewCkbError(
			errors.InternalError,
			"File path is required",
			nil,
			nil,
			nil,
		)
	}

	output, err := g.executeGitCommand("log", "-1", "--format=%aI", "--", filePath)
	if err != nil {
		return "", err
	}

	if output == "" {
		return "", errors.NewCkbError(
			errors.SymbolNotFound,
			"No commits found for file",
			nil,
			nil,
			nil,
		).WithDetails(map[string]interface{}{
			"filePath": filePath,
		})
	}

	return output, nil
}

// GetFileAuthors returns unique authors who modified a file
func (g *GitAdapter) GetFileAuthors(filePath string) ([]string, error) {
	if filePath == "" {
		return nil, errors.NewCkbError(
			errors.InternalError,
			"File path is required",
			nil,
			nil,
			nil,
		)
	}

	// Use shortlog to get unique authors
	lines, err := g.executeGitCommandLines("shortlog", "-sn", "HEAD", "--", filePath)
	if err != nil {
		return nil, err
	}

	if len(lines) == 0 {
		return []string{}, nil
	}

	// Parse author names from shortlog output
	// Format: "    42  John Doe"
	authors := make([]string, 0, len(lines))
	for _, line := range lines {
		// Split on whitespace and take everything after the count
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			// Join all parts except the first (count) to handle names with spaces
			author := strings.Join(parts[1:], " ")
			authors = append(authors, author)
		}
	}

	return authors, nil
}
