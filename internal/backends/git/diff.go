package git

import (
	"strconv"
	"strings"

	"ckb/internal/errors"
)

// DiffStats represents statistics for a file in a diff
type DiffStats struct {
	FilePath  string `json:"filePath"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	IsNew     bool   `json:"isNew"`
	IsDeleted bool   `json:"isDeleted"`
	IsRenamed bool   `json:"isRenamed"`
	OldPath   string `json:"oldPath,omitempty"` // If renamed
}

// GetStagedDiff returns statistics for staged changes
func (g *GitAdapter) GetStagedDiff() ([]DiffStats, error) {
	g.logger.Debug("Getting staged diff", nil)

	// Use git diff --cached --numstat to get staged changes
	lines, err := g.executeGitCommandLines("diff", "--cached", "--numstat")
	if err != nil {
		return nil, err
	}

	if len(lines) == 0 {
		return []DiffStats{}, nil
	}

	stats, err := g.parseDiffStats(lines)
	if err != nil {
		return nil, err
	}

	// Check for renames and new/deleted files in staged changes
	err = g.enrichDiffStatsStaged(stats)
	if err != nil {
		// Non-fatal, log and continue
		g.logger.Warn("Failed to enrich staged diff stats", map[string]interface{}{
			"error": err.Error(),
		})
	}

	return stats, nil
}

// GetWorkingTreeDiff returns statistics for working tree changes
func (g *GitAdapter) GetWorkingTreeDiff() ([]DiffStats, error) {
	g.logger.Debug("Getting working tree diff", nil)

	// Use git diff --numstat to get working tree changes
	lines, err := g.executeGitCommandLines("diff", "--numstat")
	if err != nil {
		return nil, err
	}

	if len(lines) == 0 {
		return []DiffStats{}, nil
	}

	stats, err := g.parseDiffStats(lines)
	if err != nil {
		return nil, err
	}

	// Check for renames and new/deleted files in working tree
	err = g.enrichDiffStatsWorking(stats)
	if err != nil {
		// Non-fatal, log and continue
		g.logger.Warn("Failed to enrich working tree diff stats", map[string]interface{}{
			"error": err.Error(),
		})
	}

	return stats, nil
}

// GetUntrackedFiles returns list of untracked files
func (g *GitAdapter) GetUntrackedFiles() ([]string, error) {
	g.logger.Debug("Getting untracked files", nil)

	lines, err := g.executeGitCommandLines("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}

	return lines, nil
}

// parseDiffStats parses numstat output into DiffStats
// Format: "additions deletions filename"
func (g *GitAdapter) parseDiffStats(lines []string) ([]DiffStats, error) {
	stats := make([]DiffStats, 0, len(lines))

	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) < 3 {
			g.logger.Warn("Skipping malformed numstat line", map[string]interface{}{
				"line": line,
			})
			continue
		}

		additions := 0
		deletions := 0
		isBinary := false

		// Handle binary files (marked with "-")
		if parts[0] == "-" || parts[1] == "-" {
			isBinary = true
		} else {
			var err error
			additions, err = strconv.Atoi(parts[0])
			if err != nil {
				g.logger.Warn("Failed to parse additions", map[string]interface{}{
					"line":  line,
					"error": err.Error(),
				})
				continue
			}

			deletions, err = strconv.Atoi(parts[1])
			if err != nil {
				g.logger.Warn("Failed to parse deletions", map[string]interface{}{
					"line":  line,
					"error": err.Error(),
				})
				continue
			}
		}

		// Handle rename syntax: "oldpath => newpath" or "path{old => new}"
		filePath := strings.Join(parts[2:], " ")
		oldPath := ""
		isRenamed := false

		if strings.Contains(filePath, " => ") {
			isRenamed = true
			renameParts := strings.SplitN(filePath, " => ", 2)
			if len(renameParts) == 2 {
				oldPath = strings.TrimSpace(renameParts[0])
				filePath = strings.TrimSpace(renameParts[1])
			}
		}

		stat := DiffStats{
			FilePath:  filePath,
			Additions: additions,
			Deletions: deletions,
			IsRenamed: isRenamed,
			OldPath:   oldPath,
		}

		// For binary files, we can't determine additions/deletions
		if isBinary {
			g.logger.Debug("Binary file in diff", map[string]interface{}{
				"filePath": filePath,
			})
		}

		stats = append(stats, stat)
	}

	return stats, nil
}

// enrichDiffStatsStaged adds IsNew and IsDeleted flags for staged changes
func (g *GitAdapter) enrichDiffStatsStaged(stats []DiffStats) error {
	// Get status for staged files
	lines, err := g.executeGitCommandLines("diff", "--cached", "--name-status")
	if err != nil {
		return err
	}

	// Build status map: filepath -> status
	statusMap := make(map[string]string)
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		status := parts[0]
		filePath := strings.Join(parts[1:], " ")

		// Handle rename syntax
		if strings.Contains(filePath, " => ") {
			renameParts := strings.SplitN(filePath, " => ", 2)
			if len(renameParts) == 2 {
				filePath = strings.TrimSpace(renameParts[1])
			}
		}

		statusMap[filePath] = status
	}

	// Enrich stats
	for i := range stats {
		status, ok := statusMap[stats[i].FilePath]
		if !ok {
			continue
		}

		switch status[0] {
		case 'A':
			stats[i].IsNew = true
		case 'D':
			stats[i].IsDeleted = true
		case 'R':
			stats[i].IsRenamed = true
		}
	}

	return nil
}

// enrichDiffStatsWorking adds IsNew and IsDeleted flags for working tree changes
func (g *GitAdapter) enrichDiffStatsWorking(stats []DiffStats) error {
	// Get status for working tree files
	lines, err := g.executeGitCommandLines("diff", "--name-status")
	if err != nil {
		return err
	}

	// Build status map
	statusMap := make(map[string]string)
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		status := parts[0]
		filePath := strings.Join(parts[1:], " ")

		// Handle rename syntax
		if strings.Contains(filePath, " => ") {
			renameParts := strings.SplitN(filePath, " => ", 2)
			if len(renameParts) == 2 {
				filePath = strings.TrimSpace(renameParts[1])
			}
		}

		statusMap[filePath] = status
	}

	// Enrich stats
	for i := range stats {
		status, ok := statusMap[stats[i].FilePath]
		if !ok {
			continue
		}

		switch status[0] {
		case 'A':
			stats[i].IsNew = true
		case 'D':
			stats[i].IsDeleted = true
		case 'R':
			stats[i].IsRenamed = true
		}
	}

	return nil
}

// GetDiffSummary returns a summary of all changes (staged + working tree + untracked)
func (g *GitAdapter) GetDiffSummary() (map[string]interface{}, error) {
	g.logger.Debug("Getting diff summary", nil)

	staged, err := g.GetStagedDiff()
	if err != nil {
		return nil, err
	}

	working, err := g.GetWorkingTreeDiff()
	if err != nil {
		return nil, err
	}

	untracked, err := g.GetUntrackedFiles()
	if err != nil {
		return nil, err
	}

	// Calculate totals
	totalAdditions := 0
	totalDeletions := 0
	for _, stat := range staged {
		totalAdditions += stat.Additions
		totalDeletions += stat.Deletions
	}
	for _, stat := range working {
		totalAdditions += stat.Additions
		totalDeletions += stat.Deletions
	}

	return map[string]interface{}{
		"stagedFiles":    len(staged),
		"modifiedFiles":  len(working),
		"untrackedFiles": len(untracked),
		"totalAdditions": totalAdditions,
		"totalDeletions": totalDeletions,
		"staged":         staged,
		"working":        working,
		"untracked":      untracked,
	}, nil
}

// GetCommitRangeDiff returns the diff between two commits (base..head)
func (g *GitAdapter) GetCommitRangeDiff(base, head string) ([]DiffStats, error) {
	if base == "" || head == "" {
		return nil, errors.NewCkbError(
			errors.InternalError,
			"Both base and head commits are required",
			nil,
			nil,
			nil,
		)
	}

	g.logger.Debug("Getting commit range diff", map[string]interface{}{
		"base": base,
		"head": head,
	})

	// Get numstat for the commit range
	lines, err := g.executeGitCommandLines("diff", "--numstat", base, head)
	if err != nil {
		return nil, err
	}

	if len(lines) == 0 {
		return []DiffStats{}, nil
	}

	stats, err := g.parseDiffStats(lines)
	if err != nil {
		return nil, err
	}

	// Get status info for the commit range
	statusLines, err := g.executeGitCommandLines("diff", "--name-status", base, head)
	if err != nil {
		return stats, nil //nolint:nilerr // status info is supplementary
	}

	// Build status map
	statusMap := make(map[string]string)
	for _, line := range statusLines {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		status := parts[0]
		filePath := strings.Join(parts[1:], " ")
		if strings.Contains(filePath, " => ") {
			renameParts := strings.SplitN(filePath, " => ", 2)
			if len(renameParts) == 2 {
				filePath = strings.TrimSpace(renameParts[1])
			}
		}
		statusMap[filePath] = status
	}

	// Enrich stats
	for i := range stats {
		status, ok := statusMap[stats[i].FilePath]
		if !ok {
			continue
		}
		switch status[0] {
		case 'A':
			stats[i].IsNew = true
		case 'D':
			stats[i].IsDeleted = true
		case 'R':
			stats[i].IsRenamed = true
		}
	}

	return stats, nil
}

// GetCommitsSinceDate returns commits since a specific date
func (g *GitAdapter) GetCommitsSinceDate(since string, limit int) ([]CommitInfo, error) {
	if since == "" {
		return nil, errors.NewCkbError(
			errors.InternalError,
			"Since date is required",
			nil,
			nil,
			nil,
		)
	}

	if limit <= 0 {
		limit = 100 // Default cap
	}

	g.logger.Debug("Getting commits since date", map[string]interface{}{
		"since": since,
		"limit": limit,
	})

	args := []string{
		"log",
		"--format=%H|%an|%aI|%s",
		"--since=" + since,
		"-n", strconv.Itoa(limit),
	}

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

// GetFileDiffContent returns the actual diff content for a commit range
func (g *GitAdapter) GetFileDiffContent(base, head, filePath string) (string, error) {
	args := []string{"diff", base, head, "--", filePath}
	output, err := g.executeGitCommand(args...)
	if err != nil {
		return "", err
	}
	return output, nil
}

// GetCommitDiff returns the diff for a specific commit
func (g *GitAdapter) GetCommitDiff(commitHash string) ([]DiffStats, error) {
	if commitHash == "" {
		return nil, errors.NewCkbError(
			errors.InternalError,
			"Commit hash is required",
			nil,
			nil,
			nil,
		)
	}

	g.logger.Debug("Getting commit diff", map[string]interface{}{
		"commit": commitHash,
	})

	// Get numstat for the commit
	lines, err := g.executeGitCommandLines("diff", "--numstat", commitHash+"^", commitHash)
	if err != nil {
		return nil, err
	}

	if len(lines) == 0 {
		return []DiffStats{}, nil
	}

	stats, err := g.parseDiffStats(lines)
	if err != nil {
		return nil, err
	}

	// Get status info for the commit
	statusLines, err := g.executeGitCommandLines("diff", "--name-status", commitHash+"^", commitHash)
	if err != nil {
		// Non-fatal - status info is supplementary
		return stats, nil //nolint:nilerr // intentionally ignore status lookup errors
	}

	// Build status map
	statusMap := make(map[string]string)
	for _, line := range statusLines {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		status := parts[0]
		filePath := strings.Join(parts[1:], " ")
		if strings.Contains(filePath, " => ") {
			renameParts := strings.SplitN(filePath, " => ", 2)
			if len(renameParts) == 2 {
				filePath = strings.TrimSpace(renameParts[1])
			}
		}
		statusMap[filePath] = status
	}

	// Enrich stats
	for i := range stats {
		status, ok := statusMap[stats[i].FilePath]
		if !ok {
			continue
		}
		switch status[0] {
		case 'A':
			stats[i].IsNew = true
		case 'D':
			stats[i].IsDeleted = true
		case 'R':
			stats[i].IsRenamed = true
		}
	}

	return stats, nil
}
