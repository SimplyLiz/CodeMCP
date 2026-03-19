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
		absPath := filepath.Join(e.repoRoot, file)

		// Analyze current version
		afterResult, err := analyzer.AnalyzeFile(ctx, absPath)
		if err != nil || afterResult.Error != "" {
			continue
		}

		// Analyze base version by checking out the file temporarily
		beforeResult := getBaseComplexity(ctx, analyzer, e.repoRoot, file, opts.BaseBranch)
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

		// Only report if complexity increased
		if delta.CyclomaticDelta > 0 || delta.CognitiveDelta > 0 {
			deltas = append(deltas, delta)

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

// getBaseComplexity gets complexity of a file at a given git ref.
func getBaseComplexity(ctx context.Context, analyzer *complexity.Analyzer, repoRoot, file, ref string) *complexity.FileComplexity {
	// Use git show to get the base version content
	cmd := exec.CommandContext(ctx, "git", "show", ref+":"+file)
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		return nil // File doesn't exist in base (new file)
	}

	ext := strings.ToLower(filepath.Ext(file))
	lang, ok := complexity.LanguageFromExtension(ext)
	if !ok {
		return nil
	}

	result, err := analyzer.AnalyzeSource(ctx, file, output, lang)
	if err != nil || result.Error != "" {
		return nil
	}

	return result
}
