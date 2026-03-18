package query

import (
	"context"
	"fmt"
	"time"

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
func (e *Engine) checkCouplingGaps(ctx context.Context, changedFiles []string) (ReviewCheck, []ReviewFinding) {
	start := time.Now()

	changedSet := make(map[string]bool)
	for _, f := range changedFiles {
		changedSet[f] = true
	}

	analyzer := coupling.NewAnalyzer(e.repoRoot, e.logger)
	minCorrelation := 0.7

	var gaps []CouplingGap

	// For each changed file, check if its highly-coupled partners are also in the changeset
	// Limit to first 30 files to avoid excessive git log calls
	filesToCheck := changedFiles
	if len(filesToCheck) > 30 {
		filesToCheck = filesToCheck[:30]
	}

	for _, file := range filesToCheck {
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
			if corr.Correlation >= minCorrelation && !changedSet[corr.File] {
				gaps = append(gaps, CouplingGap{
					ChangedFile:  file,
					MissingFile:  corr.File,
					CoChangeRate: corr.Correlation,
				})
			}
		}
	}

	var findings []ReviewFinding
	for _, gap := range gaps {
		findings = append(findings, ReviewFinding{
			Check:      "coupling",
			Severity:   "warning",
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
