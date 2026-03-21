package query

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/complexity"
)

// ComplexityDelta represents complexity change for a single file.
type ComplexityDelta struct {
	File             string `json:"file"`
	CyclomaticBefore int    `json:"cyclomaticBefore"`
	CyclomaticAfter  int    `json:"cyclomaticAfter"`
	CyclomaticDelta  int    `json:"cyclomaticDelta"`
	CognitiveBefore  int    `json:"cognitiveBefore"`
	CognitiveAfter   int    `json:"cognitiveAfter"`
	CognitiveDelta   int    `json:"cognitiveDelta"`
	HottestFunction  string `json:"hottestFunction,omitempty"`
}

// checkComplexityDelta compares complexity before and after for changed files.
func (e *Engine) checkComplexityDelta(ctx context.Context, files []string, opts ReviewPROptions) (ReviewCheck, []ReviewFinding) {
	start := time.Now()

	if !complexity.IsAvailable() {
		return ReviewCheck{
			Name:     "complexity",
			Status:   "skip",
			Severity: "warning",
			Summary:  "Complexity analysis not available (tree-sitter not built)",
			Duration: time.Since(start).Milliseconds(),
		}, nil
	}

	analyzer := complexity.NewAnalyzer()
	var deltas []ComplexityDelta
	var findings []ReviewFinding

	maxDelta := opts.Policy.MaxComplexityDelta

	for _, file := range files {
		if ctx.Err() != nil {
			break
		}
		absPath := filepath.Join(e.repoRoot, file)

		// Analyze current version (tree-sitter — requires lock)
		e.tsMu.Lock()
		afterResult, err := analyzer.AnalyzeFile(ctx, absPath)
		e.tsMu.Unlock()
		if err != nil || afterResult.Error != "" {
			continue
		}

		// Analyze base version — git show runs without lock, tree-sitter with lock
		beforeResult := e.getBaseComplexityLocked(ctx, analyzer, file, opts.BaseBranch)
		if beforeResult == nil {
			continue // New file, no before
		}

		delta := ComplexityDelta{
			File:             file,
			CyclomaticBefore: beforeResult.TotalCyclomatic,
			CyclomaticAfter:  afterResult.TotalCyclomatic,
			CyclomaticDelta:  afterResult.TotalCyclomatic - beforeResult.TotalCyclomatic,
			CognitiveBefore:  beforeResult.TotalCognitive,
			CognitiveAfter:   afterResult.TotalCognitive,
			CognitiveDelta:   afterResult.TotalCognitive - beforeResult.TotalCognitive,
		}

		// Find the function with highest complexity increase
		if afterResult.MaxCyclomatic > 0 {
			for _, fn := range afterResult.Functions {
				if fn.Cyclomatic == afterResult.MaxCyclomatic {
					delta.HottestFunction = fn.Name
					break
				}
			}
		}

		// Track all increases for the summary, but only emit per-file
		// findings for significant deltas (>=5 cyclomatic). Small increases
		// (+1, +2) are normal growth and create noise without actionability.
		if delta.CyclomaticDelta > 0 || delta.CognitiveDelta > 0 {
			deltas = append(deltas, delta)

			const minFindingDelta = 5
			if delta.CyclomaticDelta >= minFindingDelta {
				sev := "info"
				if maxDelta > 0 && delta.CyclomaticDelta > maxDelta {
					sev = "warning"
				}

				msg := fmt.Sprintf("Complexity %d→%d (+%d cyclomatic)",
					delta.CyclomaticBefore, delta.CyclomaticAfter, delta.CyclomaticDelta)
				if delta.HottestFunction != "" {
					msg += fmt.Sprintf(" in %s()", delta.HottestFunction)
				}

				findings = append(findings, ReviewFinding{
					Check:    "complexity",
					Severity: sev,
					File:     file,
					Message:  msg,
					Category: "complexity",
					RuleID:   "ckb/complexity/increase",
				})
			}
		}
	}

	status := "pass"
	summary := "No significant complexity increase"
	totalDelta := 0
	for _, d := range deltas {
		totalDelta += d.CyclomaticDelta
	}
	if totalDelta > 0 {
		summary = fmt.Sprintf("+%d cyclomatic complexity across %d file(s)", totalDelta, len(deltas))
		if maxDelta > 0 && totalDelta > maxDelta {
			status = "warn"
		}
	}

	return ReviewCheck{
		Name:     "complexity",
		Status:   status,
		Severity: "warning",
		Summary:  summary,
		Details:  deltas,
		Duration: time.Since(start).Milliseconds(),
	}, findings
}

// getBaseComplexityLocked gets complexity of a file at a given git ref,
// acquiring tsMu only for the tree-sitter AnalyzeSource call.
func (e *Engine) getBaseComplexityLocked(ctx context.Context, analyzer *complexity.Analyzer, file, ref string) *complexity.FileComplexity {
	// git show runs without the tree-sitter lock
	cmd := exec.CommandContext(ctx, "git", "show", ref+":"+file)
	cmd.Dir = e.repoRoot
	output, err := cmd.Output()
	if err != nil {
		return nil // File doesn't exist in base (new file)
	}

	ext := strings.ToLower(filepath.Ext(file))
	lang, ok := complexity.LanguageFromExtension(ext)
	if !ok {
		return nil
	}

	e.tsMu.Lock()
	result, err := analyzer.AnalyzeSource(ctx, file, output, lang)
	e.tsMu.Unlock()
	if err != nil || result.Error != "" {
		return nil
	}

	return result
}
