//go:build !cgo

package query

import (
	"context"
	"time"
)

// checkBugPatterns is a stub for non-CGO builds.
func (e *Engine) checkBugPatterns(ctx context.Context, files []string, opts ReviewPROptions) (ReviewCheck, []ReviewFinding) {
	return ReviewCheck{
		Name:     "bug-patterns",
		Status:   "skip",
		Severity: "warning",
		Summary:  "Bug pattern analysis requires CGO (tree-sitter)",
		Duration: 0,
	}, nil
}

// checkBugPatternsWithDiff is a stub for non-CGO builds.
func (e *Engine) checkBugPatternsWithDiff(ctx context.Context, files []string, opts ReviewPROptions) (ReviewCheck, []ReviewFinding) {
	return e.checkBugPatterns(ctx, files, opts)
}
