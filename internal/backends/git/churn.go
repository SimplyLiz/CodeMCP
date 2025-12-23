package git

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"ckb/internal/errors"
)

// ChurnMetrics represents churn statistics for a file
type ChurnMetrics struct {
	FilePath       string  `json:"filePath"`
	ChangeCount    int     `json:"changeCount"`    // Number of commits touching this file
	AuthorCount    int     `json:"authorCount"`    // Unique authors
	LastModified   string  `json:"lastModified"`   // ISO 8601 timestamp
	AverageChanges float64 `json:"averageChanges"` // Lines changed per commit
	HotspotScore   float64 `json:"hotspotScore"`   // Composite churn score
}

// GetFileChurn returns churn metrics for a specific file
// since: optional date/commit to start analysis from (e.g., "2023-01-01", "HEAD~10")
func (g *GitAdapter) GetFileChurn(filePath string, since string) (*ChurnMetrics, error) {
	if filePath == "" {
		return nil, errors.NewCkbError(
			errors.InternalError,
			"File path is required",
			nil,
			nil,
			nil,
		)
	}

	g.logger.Debug("Getting file churn", map[string]interface{}{
		"filePath": filePath,
		"since":    since,
	})

	// Get commit count
	changeCount, err := g.getFileCommitCountSince(filePath, since)
	if err != nil {
		return nil, err
	}

	if changeCount == 0 {
		return &ChurnMetrics{
			FilePath:       filePath,
			ChangeCount:    0,
			AuthorCount:    0,
			LastModified:   "",
			AverageChanges: 0,
			HotspotScore:   0,
		}, nil
	}

	// Get unique authors
	authors, err := g.getFileAuthorsSince(filePath, since)
	if err != nil {
		return nil, err
	}

	// Get last modified timestamp
	lastModified, err := g.GetFileLastModified(filePath)
	if err != nil {
		// Non-fatal, continue with empty timestamp
		g.logger.Warn("Could not get last modified time", map[string]interface{}{
			"filePath": filePath,
			"error":    err.Error(),
		})
		lastModified = ""
	}

	// Calculate average lines changed per commit
	avgChanges, err := g.getAverageChanges(filePath, since)
	if err != nil {
		// Non-fatal, default to 0
		g.logger.Warn("Could not calculate average changes", map[string]interface{}{
			"filePath": filePath,
			"error":    err.Error(),
		})
		avgChanges = 0
	}

	// Calculate hotspot score
	// Score = sqrt(changeCount) * log(authorCount + 1) * log(avgChanges + 1)
	// This gives higher weight to files with many changes by multiple authors
	hotspotScore := math.Sqrt(float64(changeCount)) *
		math.Log(float64(len(authors))+1) *
		math.Log(avgChanges+1)

	return &ChurnMetrics{
		FilePath:       filePath,
		ChangeCount:    changeCount,
		AuthorCount:    len(authors),
		LastModified:   lastModified,
		AverageChanges: avgChanges,
		HotspotScore:   hotspotScore,
	}, nil
}

// GetHotspots returns the top files with highest churn
// limit: maximum number of files to return
// since: optional date/commit to start analysis from
// Optimized: uses single git log command instead of O(n) commands per file
func (g *GitAdapter) GetHotspots(limit int, since string) ([]ChurnMetrics, error) {
	if limit <= 0 {
		limit = 10 // Default to top 10
	}

	g.logger.Debug("Getting hotspots", map[string]interface{}{
		"limit": limit,
		"since": since,
	})

	// Use single git log command to get all data at once
	// Format: commit hash, author, timestamp, then numstat for files
	args := []string{"log", "--format=%H|%an|%aI", "--numstat"}
	if since != "" {
		args = append(args, fmt.Sprintf("--since=%s", since))
	}
	args = append(args, "HEAD")

	lines, err := g.executeGitCommandLines(args...)
	if err != nil {
		return nil, err
	}

	if len(lines) == 0 {
		return []ChurnMetrics{}, nil
	}

	// Parse git log output and aggregate per-file metrics
	type fileStats struct {
		changeCount  int
		authors      map[string]bool
		lastModified string
		totalAdded   int
		totalDeleted int
	}
	fileMetrics := make(map[string]*fileStats)

	var currentCommitTime string
	var currentAuthor string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if this is a commit line (format: hash|author|timestamp)
		if strings.Contains(line, "|") {
			parts := strings.Split(line, "|")
			if len(parts) >= 3 {
				currentAuthor = parts[1]
				currentCommitTime = parts[2]
				continue
			}
		}

		// This is a numstat line: "added<tab>deleted<tab>filename"
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		// Skip binary files (marked with "-")
		added, err1 := strconv.Atoi(parts[0])
		deleted, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			continue
		}

		filePath := strings.Join(parts[2:], " ") // Handle filenames with spaces

		// Initialize or update file stats
		stats, exists := fileMetrics[filePath]
		if !exists {
			stats = &fileStats{
				authors:      make(map[string]bool),
				lastModified: currentCommitTime,
			}
			fileMetrics[filePath] = stats
		}

		stats.changeCount++
		stats.authors[currentAuthor] = true
		stats.totalAdded += added
		stats.totalDeleted += deleted
		// First commit seen is the most recent (git log is newest first)
		if stats.lastModified == "" {
			stats.lastModified = currentCommitTime
		}
	}

	// Convert to ChurnMetrics slice
	metrics := make([]ChurnMetrics, 0, len(fileMetrics))
	for filePath, stats := range fileMetrics {
		if stats.changeCount == 0 {
			continue
		}

		avgChanges := float64(stats.totalAdded+stats.totalDeleted) / float64(stats.changeCount)
		authorCount := len(stats.authors)

		// Calculate hotspot score
		hotspotScore := math.Sqrt(float64(stats.changeCount)) *
			math.Log(float64(authorCount)+1) *
			math.Log(avgChanges+1)

		metrics = append(metrics, ChurnMetrics{
			FilePath:       filePath,
			ChangeCount:    stats.changeCount,
			AuthorCount:    authorCount,
			LastModified:   stats.lastModified,
			AverageChanges: avgChanges,
			HotspotScore:   hotspotScore,
		})
	}

	// Sort by hotspot score (descending)
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].HotspotScore > metrics[j].HotspotScore
	})

	// Return top N
	if len(metrics) > limit {
		metrics = metrics[:limit]
	}

	return metrics, nil
}

// getFileCommitCountSince returns the number of commits touching a file since a given time
func (g *GitAdapter) getFileCommitCountSince(filePath string, since string) (int, error) {
	args := []string{"rev-list", "--count"}

	if since != "" {
		args = append(args, fmt.Sprintf("--since=%s", since))
	}

	args = append(args, "HEAD", "--", filePath)

	output, err := g.executeGitCommand(args...)
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

// getFileAuthorsSince returns unique authors who modified a file since a given time
func (g *GitAdapter) getFileAuthorsSince(filePath string, since string) ([]string, error) {
	args := []string{"shortlog", "-sn"}

	if since != "" {
		args = append(args, fmt.Sprintf("--since=%s", since))
	}

	args = append(args, "HEAD", "--", filePath)

	lines, err := g.executeGitCommandLines(args...)
	if err != nil {
		return nil, err
	}

	if len(lines) == 0 {
		return []string{}, nil
	}

	// Parse author names
	authors := make([]string, 0, len(lines))
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			author := strings.Join(parts[1:], " ")
			authors = append(authors, author)
		}
	}

	return authors, nil
}

// getAverageChanges calculates the average number of lines changed per commit
func (g *GitAdapter) getAverageChanges(filePath string, since string) (float64, error) {
	args := []string{"log", "--numstat", "--format="}

	if since != "" {
		args = append(args, fmt.Sprintf("--since=%s", since))
	}

	args = append(args, "HEAD", "--", filePath)

	lines, err := g.executeGitCommandLines(args...)
	if err != nil {
		return 0, err
	}

	if len(lines) == 0 {
		return 0, nil
	}

	// Parse numstat output: "additions deletions filename"
	totalChanges := 0
	commitCount := 0

	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		// Parse additions and deletions
		additions, err1 := strconv.Atoi(parts[0])
		deletions, err2 := strconv.Atoi(parts[1])

		// Skip binary files (marked with "-")
		if err1 != nil || err2 != nil {
			continue
		}

		totalChanges += additions + deletions
		commitCount++
	}

	if commitCount == 0 {
		return 0, nil
	}

	return float64(totalChanges) / float64(commitCount), nil
}

// getChangedFiles returns all files that changed since a given time
func (g *GitAdapter) getChangedFiles(since string) ([]string, error) {
	args := []string{"log", "--name-only", "--format=", "--pretty="}

	if since != "" {
		args = append(args, fmt.Sprintf("--since=%s", since))
	}

	args = append(args, "HEAD")

	lines, err := g.executeGitCommandLines(args...)
	if err != nil {
		return nil, err
	}

	if len(lines) == 0 {
		return []string{}, nil
	}

	// Deduplicate files
	fileSet := make(map[string]bool)
	for _, file := range lines {
		if file != "" {
			fileSet[file] = true
		}
	}

	// Convert to slice
	files := make([]string, 0, len(fileSet))
	for file := range fileSet {
		files = append(files, file)
	}

	return files, nil
}

// GetTotalChurnMetrics returns aggregate churn metrics for the entire repository
func (g *GitAdapter) GetTotalChurnMetrics(since string) (map[string]interface{}, error) {
	g.logger.Debug("Getting total churn metrics", map[string]interface{}{
		"since": since,
	})

	// Get total commit count
	args := []string{"rev-list", "--count"}
	if since != "" {
		args = append(args, fmt.Sprintf("--since=%s", since))
	}
	args = append(args, "HEAD")

	output, err := g.executeGitCommand(args...)
	if err != nil {
		return nil, err
	}

	totalCommits, err := strconv.Atoi(output)
	if err != nil {
		return nil, errors.NewCkbError(
			errors.InternalError,
			"Failed to parse commit count",
			err,
			nil,
			nil,
		)
	}

	// Get total authors
	authorArgs := []string{"shortlog", "-sn"}
	if since != "" {
		authorArgs = append(authorArgs, fmt.Sprintf("--since=%s", since))
	}
	authorArgs = append(authorArgs, "HEAD")

	authorLines, err := g.executeGitCommandLines(authorArgs...)
	if err != nil {
		return nil, err
	}

	// Get changed files
	files, err := g.getChangedFiles(since)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"totalCommits": totalCommits,
		"totalAuthors": len(authorLines),
		"changedFiles": len(files),
		"since":        since,
	}, nil
}
