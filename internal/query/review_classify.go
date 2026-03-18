package query

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/backends/git"
)

// ChangeCategory classifies the type of change for a file.
const (
	CategoryNew        = "new"
	CategoryRefactor   = "refactoring"
	CategoryMoved      = "moved"
	CategoryChurn      = "churn"
	CategoryConfig     = "config"
	CategoryTest       = "test"
	CategoryGenerated  = "generated"
	CategoryModified   = "modified"
)

// ChangeClassification categorizes a file change for review prioritization.
type ChangeClassification struct {
	File           string  `json:"file"`
	Category       string  `json:"category"`       // One of the Category* constants
	Confidence     float64 `json:"confidence"`      // 0-1
	Detail         string  `json:"detail"`          // Human-readable explanation
	ReviewPriority string  `json:"reviewPriority"`  // "high", "medium", "low", "skip"
}

// ChangeBreakdown summarizes classifications across the entire PR.
type ChangeBreakdown struct {
	Classifications []ChangeClassification `json:"classifications"`
	Summary         map[string]int         `json:"summary"` // category → file count
}

// classifyChanges categorizes each changed file by the type of change.
func (e *Engine) classifyChanges(ctx context.Context, diffStats []git.DiffStats, generatedSet map[string]bool, opts ReviewPROptions) *ChangeBreakdown {
	classifications := make([]ChangeClassification, 0, len(diffStats))
	summary := make(map[string]int)

	for _, ds := range diffStats {
		c := e.classifyFile(ctx, ds, generatedSet, opts)
		classifications = append(classifications, c)
		summary[c.Category]++
	}

	return &ChangeBreakdown{
		Classifications: classifications,
		Summary:         summary,
	}
}

func (e *Engine) classifyFile(ctx context.Context, ds git.DiffStats, generatedSet map[string]bool, opts ReviewPROptions) ChangeClassification {
	file := ds.FilePath

	// Generated files
	if generatedSet[file] {
		return ChangeClassification{
			File:           file,
			Category:       CategoryGenerated,
			Confidence:     1.0,
			Detail:         "Generated file — review source instead",
			ReviewPriority: "skip",
		}
	}

	// Moved/renamed files
	if ds.IsRenamed {
		similarity := estimateRenameSimilarity(ds)
		if similarity > 0.8 {
			return ChangeClassification{
				File:           file,
				Category:       CategoryMoved,
				Confidence:     similarity,
				Detail:         fmt.Sprintf("Renamed from %s (%.0f%% similar)", ds.OldPath, similarity*100),
				ReviewPriority: "low",
			}
		}
		return ChangeClassification{
			File:           file,
			Category:       CategoryRefactor,
			Confidence:     0.7,
			Detail:         fmt.Sprintf("Renamed from %s with significant changes", ds.OldPath),
			ReviewPriority: "medium",
		}
	}

	// New files
	if ds.IsNew {
		return ChangeClassification{
			File:           file,
			Category:       CategoryNew,
			Confidence:     1.0,
			Detail:         fmt.Sprintf("New file (+%d lines)", ds.Additions),
			ReviewPriority: "high",
		}
	}

	// Test files
	if isTestFilePath(file) {
		return ChangeClassification{
			File:           file,
			Category:       CategoryTest,
			Confidence:     1.0,
			Detail:         "Test file update",
			ReviewPriority: "medium",
		}
	}

	// Config/build files
	if isConfigFile(file) {
		return ChangeClassification{
			File:           file,
			Category:       CategoryConfig,
			Confidence:     1.0,
			Detail:         "Configuration/build file",
			ReviewPriority: "low",
		}
	}

	// Churn detection: file changed frequently in recent history
	if e.isChurning(ctx, file) {
		return ChangeClassification{
			File:           file,
			Category:       CategoryChurn,
			Confidence:     0.8,
			Detail:         "File changed frequently in the last 30 days — stability concern",
			ReviewPriority: "high",
		}
	}

	// Default: modified
	return ChangeClassification{
		File:           file,
		Category:       CategoryModified,
		Confidence:     1.0,
		Detail:         fmt.Sprintf("+%d −%d", ds.Additions, ds.Deletions),
		ReviewPriority: "medium",
	}
}

// estimateRenameSimilarity estimates how similar a renamed file is to its original.
// Uses the ratio of unchanged lines to total lines.
func estimateRenameSimilarity(ds git.DiffStats) float64 {
	total := ds.Additions + ds.Deletions
	if total == 0 {
		return 1.0 // Pure rename, no content change
	}
	// Rough heuristic: if additions ≈ deletions and both are small relative
	// to what a full rewrite would be, it's mostly unchanged
	if ds.Additions == 0 && ds.Deletions == 0 {
		return 1.0
	}
	// Smaller diffs → more similar
	maxChange := ds.Additions
	if ds.Deletions > maxChange {
		maxChange = ds.Deletions
	}
	if maxChange < 5 {
		return 0.95
	}
	if maxChange < 20 {
		return 0.85
	}
	return 0.5
}

// isConfigFile returns true for common config/build file patterns.
func isConfigFile(path string) bool {
	base := filepath.Base(path)

	configFiles := map[string]bool{
		"Makefile": true, "CMakeLists.txt": true, "Dockerfile": true,
		"docker-compose.yml": true, "docker-compose.yaml": true,
		".gitignore": true, ".eslintrc": true, ".prettierrc": true,
		"tsconfig.json": true, "package.json": true, "package-lock.json": true,
		"go.mod": true, "go.sum": true, "Cargo.toml": true, "Cargo.lock": true,
		"pyproject.toml": true, "setup.py": true, "setup.cfg": true,
		"pom.xml": true, "build.gradle": true,
		".github": true, "Jenkinsfile": true,
	}
	if configFiles[base] {
		return true
	}

	ext := filepath.Ext(base)
	if ext == ".yml" || ext == ".yaml" {
		dir := filepath.Dir(path)
		if strings.Contains(dir, ".github") || strings.Contains(dir, "ci/") ||
			strings.Contains(dir, ".ci/") || strings.Contains(dir, ".circleci") {
			return true
		}
	}

	return false
}

// isChurning checks if a file was changed frequently in the last 30 days.
func (e *Engine) isChurning(_ context.Context, file string) bool {
	if e.gitAdapter == nil {
		return false
	}

	history, err := e.gitAdapter.GetFileHistory(file, 10)
	if err != nil || history.CommitCount < 3 {
		return false
	}

	since := time.Now().AddDate(0, 0, -30)
	recentCount := 0
	for _, c := range history.Commits {
		ts, err := time.Parse(time.RFC3339, c.Timestamp)
		if err != nil {
			continue
		}
		if ts.After(since) {
			recentCount++
		}
	}

	return recentCount >= 3
}
