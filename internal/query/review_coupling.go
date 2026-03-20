package query

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/backends/git"
	"github.com/SimplyLiz/CodeMCP/internal/coupling"
)

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
				gaps = append(gaps, CouplingGap{
					ChangedFile:  file,
					MissingFile:  missing,
					CoChangeRate: corr.Correlation,
				})
			}
		}
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
