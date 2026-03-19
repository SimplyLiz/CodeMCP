package query

import (
	"context"
	"fmt"
	"path/filepath"
	"time"
)

// checkDeadCode finds dead code within the changed files using the SCIP index.
func (e *Engine) checkDeadCode(ctx context.Context, changedFiles []string, opts ReviewPROptions) (ReviewCheck, []ReviewFinding) {
	start := time.Now()

	// Build scope from changed file directories
	dirSet := make(map[string]bool)
	for _, f := range changedFiles {
		dirSet[filepath.Dir(f)] = true
	}
	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}

	minConf := opts.Policy.DeadCodeMinConfidence
	if minConf <= 0 {
		minConf = 0.8
	}

	resp, err := e.FindDeadCode(ctx, FindDeadCodeOptions{
		Scope:           dirs,
		MinConfidence:   minConf,
		IncludeExported: true,
		Limit:           50,
	})
	if err != nil {
		return ReviewCheck{
			Name:     "dead-code",
			Status:   "skip",
			Severity: "warning",
			Summary:  fmt.Sprintf("Could not analyze: %v", err),
			Duration: time.Since(start).Milliseconds(),
		}, nil
	}

	// Filter to only items in the changed files
	changedSet := make(map[string]bool)
	for _, f := range changedFiles {
		changedSet[f] = true
	}

	var findings []ReviewFinding
	for _, item := range resp.DeadCode {
		if !changedSet[item.FilePath] {
			continue
		}
		hint := ""
		if item.SymbolName != "" {
			hint = fmt.Sprintf("→ ckb explain %s", item.SymbolName)
		}
		findings = append(findings, ReviewFinding{
			Check:     "dead-code",
			Severity:  "warning",
			File:      item.FilePath,
			StartLine: item.LineNumber,
			Message:   fmt.Sprintf("Dead code: %s (%s) — %s", item.SymbolName, item.Kind, item.Reason),
			Category:  "dead-code",
			RuleID:    fmt.Sprintf("ckb/dead-code/%s", item.Category),
			Hint:      hint,
		})
	}

	status := "pass"
	summary := "No dead code in changed files"
	if len(findings) > 0 {
		status = "warn"
		summary = fmt.Sprintf("%d dead code item(s) found in changed files", len(findings))
	}

	return ReviewCheck{
		Name:     "dead-code",
		Status:   status,
		Severity: "warning",
		Summary:  summary,
		Duration: time.Since(start).Milliseconds(),
	}, findings
}
