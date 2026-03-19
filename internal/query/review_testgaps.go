package query

import (
	"context"
	"fmt"
	"time"
)

// checkTestGaps finds untested functions in the changed files.
// Uses tree-sitter internally — acquires e.tsMu around AnalyzeTestGaps calls.
func (e *Engine) checkTestGaps(ctx context.Context, changedFiles []string, opts ReviewPROptions) (ReviewCheck, []ReviewFinding) {
	start := time.Now()

	minLines := opts.Policy.TestGapMinLines
	if minLines <= 0 {
		minLines = 5
	}

	// Filter to non-test source files, cap at 20
	var sourceFiles []string
	for _, f := range changedFiles {
		if isTestFilePathEnhanced(f) {
			continue
		}
		sourceFiles = append(sourceFiles, f)
		if len(sourceFiles) >= 20 {
			break
		}
	}

	var findings []ReviewFinding
	for _, file := range sourceFiles {
		if ctx.Err() != nil {
			break
		}
		e.tsMu.Lock()
		result, err := e.AnalyzeTestGaps(ctx, AnalyzeTestGapsOptions{
			Target:   file,
			MinLines: minLines,
			Limit:    10,
		})
		e.tsMu.Unlock()
		if err != nil {
			continue
		}

		for _, gap := range result.Gaps {
			hint := ""
			if gap.Function != "" {
				hint = fmt.Sprintf("→ ckb explain %s", gap.Function)
			}
			findings = append(findings, ReviewFinding{
				Check:     "test-gaps",
				Severity:  "info",
				File:      gap.File,
				StartLine: gap.StartLine,
				EndLine:   gap.EndLine,
				Message:   fmt.Sprintf("Untested function %s (complexity: %d)", gap.Function, gap.Complexity),
				Category:  "testing",
				RuleID:    fmt.Sprintf("ckb/test-gaps/%s", gap.Reason),
				Hint:      hint,
			})
		}
	}

	status := "pass"
	summary := "All changed functions have tests"
	if len(findings) > 0 {
		status = "info"
		summary = fmt.Sprintf("%d untested function(s) in changed files", len(findings))
	}

	return ReviewCheck{
		Name:     "test-gaps",
		Status:   status,
		Severity: "info",
		Summary:  summary,
		Duration: time.Since(start).Milliseconds(),
	}, findings
}
