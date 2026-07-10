package query

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/cartographer"
)

// checkLayerViolations uses Cartographer to detect architectural boundary crossings
// in the files touched by the PR. Returns a skip check when Cartographer is not
// compiled in (graceful degradation — never blocks the build).
func (e *Engine) checkLayerViolations(_ context.Context, files []string, _ ReviewPROptions) (ReviewCheck, []ReviewFinding) {
	start := time.Now()

	if !cartographer.Available() {
		return ReviewCheck{
			Name:     "layers",
			Status:   "skip",
			Severity: "info",
			Summary:  "Cartographer not compiled in this build",
			Duration: time.Since(start).Milliseconds(),
		}, nil
	}

	layersPath := ""
	if candidate := filepath.Join(e.repoRoot, ".cartographer", "layers.toml"); fileExists(candidate) {
		layersPath = candidate
	}
	violations, err := cartographer.CheckLayers(e.repoRoot, layersPath)
	if err != nil {
		return ReviewCheck{
			Name:     "layers",
			Status:   "skip",
			Severity: "info",
			Summary:  fmt.Sprintf("layer check skipped: %v", err),
			Duration: time.Since(start).Milliseconds(),
		}, nil
	}

	// Index changed files for O(1) lookup.
	changedSet := make(map[string]struct{}, len(files))
	for _, f := range files {
		changedSet[f] = struct{}{}
	}

	var findings []ReviewFinding
	for _, v := range violations {
		_, srcChanged := changedSet[v.SourcePath]
		_, tgtChanged := changedSet[v.TargetPath]
		if !srcChanged && !tgtChanged {
			continue // violation not in this PR's scope
		}
		findings = append(findings, ReviewFinding{
			Check:    "layers",
			Severity: "warning",
			File:     v.SourcePath,
			Message:  fmt.Sprintf("layer violation: %s → %s (%s → %s)", v.SourcePath, v.TargetPath, v.SourceLayer, v.TargetLayer),
			Category: "architecture",
			RuleID:   "ckb/layers/boundary",
			Tier:     2,
		})
	}

	status := "pass"
	summary := "No layer violations in changed files"
	if len(findings) > 0 {
		status = "warn"
		summary = fmt.Sprintf("%d layer violation(s) in changed files", len(findings))
	}

	return ReviewCheck{
		Name:     "layers",
		Status:   status,
		Severity: "warning",
		Summary:  summary,
		Duration: time.Since(start).Milliseconds(),
	}, findings
}
