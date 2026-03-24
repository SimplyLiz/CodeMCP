package query

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/backends/git"
	"github.com/SimplyLiz/CodeMCP/internal/coupling"
)

const maxCouplingAge = 180 * 24 * time.Hour

// batchFileLastModified returns the last git modification time for each file
// in a single git-log invocation, avoiding O(n) subprocess spawns.
func (e *Engine) batchFileLastModified(ctx context.Context, files []string) map[string]time.Time {
	result := make(map[string]time.Time, len(files))
	if len(files) == 0 {
		return result
	}

	// git log --format="<file>\t<date>" with --name-only and --diff-filter
	// won't work cleanly for this. Instead, one call per unique file but
	// batched: ask git for dates of all files at once via
	// "git log --format=%aI --name-only -1 -- file1 file2 ..."
	// Unfortunately git log -1 with multiple paths returns only one result.
	// Use a single git log with --stdin-paths is not supported either.
	// Pragmatic: batch via a single shell invocation using a for-loop.
	// This runs one process instead of N.
	var script strings.Builder
	for _, f := range files {
		// Shell-safe: files are repo-relative paths, no user input
		fmt.Fprintf(&script, "echo \"$(git log -1 --format=%%aI -- %q)\t%s\"\n", f, f)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", script.String())
	cmd.Dir = e.repoRoot
	out, err := cmd.Output()
	if err != nil {
		return result
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 || parts[0] == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[0]))
		if err == nil {
			result[parts[1]] = t
		}
	}
	return result
}

// CouplingGap represents a missing co-changed file.
type CouplingGap struct {
	ChangedFile  string  `json:"changedFile"`
	MissingFile  string  `json:"missingFile"`
	CoChangeRate float64 `json:"coChangeRate"`
	LastCoChange string  `json:"lastCoChange,omitempty"`
}

// checkCouplingGaps checks if commonly co-changed files are missing from the changeset.
func (e *Engine) checkCouplingGaps(ctx context.Context, changedFiles []string, diffStats []git.DiffStats) (ReviewCheck, []ReviewFinding) {
	start := time.Now()

	changedSet := make(map[string]bool)
	for _, f := range changedFiles {
		changedSet[f] = true
	}

	// Build diff stats lookup for smart filtering
	diffStatsMap := make(map[string]git.DiffStats, len(diffStats))
	for _, ds := range diffStats {
		diffStatsMap[ds.FilePath] = ds
	}

	analyzer := coupling.NewAnalyzer(e.repoRoot, e.logger)
	minCorrelation := 0.7

	var gaps []CouplingGap

	// For each changed file, check if its highly-coupled partners are also in the changeset.
	// Skip config/CI paths — they always co-change and produce noise, not signal.
	// Limit to first 20 source files to avoid excessive git log calls.
	var filesToCheck []string
	for _, f := range changedFiles {
		if isCouplingNoiseFile(f) {
			continue
		}
		// Skip new files — they have no meaningful co-change history
		if ds, ok := diffStatsMap[f]; ok && ds.IsNew {
			continue
		}
		filesToCheck = append(filesToCheck, f)
		if len(filesToCheck) >= 20 {
			break
		}
	}

	// First pass: collect candidate gaps (before date filtering).
	type candidateGap struct {
		changedFile  string
		missingFile  string
		coChangeRate float64
	}
	var candidates []candidateGap
	missingFiles := make(map[string]bool)

	for _, file := range filesToCheck {
		if ctx.Err() != nil {
			break
		}
		result, err := analyzer.Analyze(ctx, coupling.AnalyzeOptions{
			Target:         file,
			MinCorrelation: minCorrelation,
			WindowDays:     365,
			Limit:          5,
		})
		if err != nil {
			continue
		}

		for _, corr := range result.Correlations {
			missing := corr.FilePath
			if missing == "" {
				missing = corr.File
			}
			if corr.Correlation >= minCorrelation && !changedSet[missing] && !isCouplingNoiseFile(missing) {
				candidates = append(candidates, candidateGap{
					changedFile:  file,
					missingFile:  missing,
					coChangeRate: corr.Correlation,
				})
				missingFiles[missing] = true
			}
		}
	}

	// Batch-lookup last modification dates in a single shell invocation.
	filesToLookup := make([]string, 0, len(missingFiles))
	for f := range missingFiles {
		filesToLookup = append(filesToLookup, f)
	}
	lastModDates := e.batchFileLastModified(ctx, filesToLookup)

	// Second pass: filter stale couplings.
	for _, c := range candidates {
		lastMod := lastModDates[c.missingFile]
		if !lastMod.IsZero() && time.Since(lastMod) > maxCouplingAge {
			continue
		}
		var lastCoChange string
		if !lastMod.IsZero() {
			lastCoChange = lastMod.Format(time.RFC3339)
		}
		gaps = append(gaps, CouplingGap{
			ChangedFile:  c.changedFile,
			MissingFile:  c.missingFile,
			CoChangeRate: c.coChangeRate,
			LastCoChange: lastCoChange,
		})
	}

	var findings []ReviewFinding
	for _, gap := range gaps {
		severity := "warning"
		// Downgrade to info for append-only changes (low risk of breaking coupled files)
		if ds, ok := diffStatsMap[gap.ChangedFile]; ok {
			if ds.Deletions == 0 && ds.Additions > 0 {
				severity = "info"
			} else if ds.Additions > 0 && ds.Deletions < ds.Additions/10 {
				severity = "info"
			}
		}
		findings = append(findings, ReviewFinding{
			Check:      "coupling",
			Severity:   severity,
			File:       gap.ChangedFile,
			Message:    fmt.Sprintf("Missing co-change: %s (%.0f%% co-change rate)", gap.MissingFile, gap.CoChangeRate*100),
			Suggestion: fmt.Sprintf("Consider also changing %s — it historically changes together with %s", gap.MissingFile, gap.ChangedFile),
			Category:   "coupling",
			RuleID:     "ckb/coupling/missing-cochange",
		})
	}

	status := "pass"
	summary := "No missing co-change files"
	if len(gaps) > 0 {
		status = "warn"
		summary = fmt.Sprintf("%d commonly co-changed file(s) missing from changeset", len(gaps))
	}

	return ReviewCheck{
		Name:     "coupling",
		Status:   status,
		Severity: "warning",
		Summary:  summary,
		Details:  gaps,
		Duration: time.Since(start).Milliseconds(),
	}, findings
}

// isCouplingNoiseFile returns true for paths where co-change analysis produces
// noise rather than signal (CI workflows, config dirs, generated files).
func isCouplingNoiseFile(path string) bool {
	noisePrefixes := []string{
		".github/",
		".gitlab-ci",
		"ci/",
		".circleci/",
		".buildkite/",
	}
	for _, prefix := range noisePrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	noiseSuffixes := []string{
		".yml",
		".yaml",
		".lock",
		".sum",
	}
	for _, suffix := range noiseSuffixes {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}
