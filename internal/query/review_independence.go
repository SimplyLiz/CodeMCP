package query

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// IndependenceResult holds the outcome of reviewer independence analysis.
type IndependenceResult struct {
	Authors          []string `json:"authors"`          // PR authors
	CriticalFiles    []string `json:"criticalFiles"`    // Critical-path files in the PR
	RequiresSignoff  bool     `json:"requiresSignoff"`  // Whether independent review is required
	MinReviewers     int      `json:"minReviewers"`     // Minimum required reviewers
}

// checkReviewerIndependence verifies that the PR will receive independent review.
// This is a compliance check — it flags the requirement, it doesn't enforce it.
func (e *Engine) checkReviewerIndependence(ctx context.Context, opts ReviewPROptions) (ReviewCheck, []ReviewFinding) {
	start := time.Now()

	if e.gitAdapter == nil {
		return ReviewCheck{
			Name:     "independence",
			Status:   "skip",
			Severity: "warning",
			Summary:  "Git adapter not available",
			Duration: time.Since(start).Milliseconds(),
		}, nil
	}

	// Get PR authors from commit range
	commits, err := e.gitAdapter.GetCommitRange(opts.BaseBranch, opts.HeadBranch)
	if err != nil {
		return ReviewCheck{
			Name:     "independence",
			Status:   "skip",
			Severity: "warning",
			Summary:  fmt.Sprintf("Could not analyze: %v", err),
			Duration: time.Since(start).Milliseconds(),
		}, nil
	}

	authorSet := make(map[string]bool)
	for _, c := range commits {
		authorSet[c.Author] = true
	}

	authors := make([]string, 0, len(authorSet))
	for a := range authorSet {
		authors = append(authors, a)
	}

	minReviewers := opts.Policy.MinReviewers
	if minReviewers <= 0 {
		minReviewers = 1
	}

	var findings []ReviewFinding

	// Check if critical paths are touched (makes independence more important)
	hasCriticalFiles := false
	if len(opts.Policy.CriticalPaths) > 0 {
		diffStats, err := e.gitAdapter.GetCommitRangeDiff(opts.BaseBranch, opts.HeadBranch)
		if err == nil {
			for _, df := range diffStats {
				for _, pattern := range opts.Policy.CriticalPaths {
					matched, _ := matchGlob(pattern, df.FilePath)
					if matched {
						hasCriticalFiles = true
						break
					}
				}
				if hasCriticalFiles {
					break
				}
			}
		}
	}

	severity := "warning"
	if hasCriticalFiles {
		severity = "error"
	}

	authorList := strings.Join(authors, ", ")

	findings = append(findings, ReviewFinding{
		Check:      "independence",
		Severity:   severity,
		Message:    fmt.Sprintf("Requires independent review (not by: %s); min %d reviewer(s)", authorList, minReviewers),
		Suggestion: "Ensure the reviewer is not the author of the changes",
		Category:   "compliance",
		RuleID:     "ckb/independence/require-independent-reviewer",
	})

	if hasCriticalFiles {
		findings = append(findings, ReviewFinding{
			Check:    "independence",
			Severity: "error",
			Message:  "Safety-critical files changed — independent verification required per IEC 61508 / ISO 26262",
			Category: "compliance",
			RuleID:   "ckb/independence/critical-path-review",
		})
	}

	status := "warn"
	summary := fmt.Sprintf("Independent review required (authors: %s)", authorList)
	if hasCriticalFiles {
		status = "fail"
		summary = fmt.Sprintf("Critical files — independent review required (authors: %s)", authorList)
	}

	return ReviewCheck{
		Name:     "independence",
		Status:   status,
		Severity: severity,
		Summary:  summary,
		Details: IndependenceResult{
			Authors:         authors,
			RequiresSignoff: true,
			MinReviewers:    minReviewers,
		},
		Duration: time.Since(start).Milliseconds(),
	}, findings
}
