package query

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/complexity"
	"github.com/SimplyLiz/CodeMCP/internal/coupling"
	"github.com/SimplyLiz/CodeMCP/internal/ownership"
)

// CodeHealthDelta represents the health change for a single file.
type CodeHealthDelta struct {
	File         string `json:"file"`
	HealthBefore int    `json:"healthBefore"` // 0-100
	HealthAfter  int    `json:"healthAfter"`  // 0-100
	Delta        int    `json:"delta"`        // negative = degradation
	Grade        string `json:"grade"`        // A/B/C/D/F
	GradeBefore  string `json:"gradeBefore"`
	TopFactor    string `json:"topFactor"` // What drives the score most
}

// CodeHealthReport aggregates health deltas across the PR.
type CodeHealthReport struct {
	Deltas       []CodeHealthDelta `json:"deltas"`
	AverageDelta float64           `json:"averageDelta"`
	WorstFile    string            `json:"worstFile,omitempty"`
	WorstGrade   string            `json:"worstGrade,omitempty"`
	Degraded     int               `json:"degraded"` // Files that got worse
	Improved     int               `json:"improved"`  // Files that got better
}

// Health score weights
const (
	weightCyclomatic = 0.20
	weightCognitive  = 0.15
	weightFileSize   = 0.10
	weightChurn      = 0.15
	weightCoupling   = 0.10
	weightBusFactor  = 0.10
	weightAge        = 0.10
	weightCoverage   = 0.10
)

// checkCodeHealth calculates health score deltas for changed files.
func (e *Engine) checkCodeHealth(ctx context.Context, files []string, opts ReviewPROptions) (ReviewCheck, []ReviewFinding, *CodeHealthReport) {
	start := time.Now()

	var deltas []CodeHealthDelta
	var findings []ReviewFinding

	for _, file := range files {
		absPath := filepath.Join(e.repoRoot, file)
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			continue
		}

		after := e.calculateFileHealth(ctx, file)
		before := e.calculateBaseFileHealth(ctx, file, opts.BaseBranch)

		delta := after - before
		grade := healthGrade(after)
		gradeBefore := healthGrade(before)

		topFactor := "unchanged"
		if delta < -10 {
			topFactor = "significant health degradation"
		} else if delta < 0 {
			topFactor = "minor health decrease"
		} else if delta > 10 {
			topFactor = "health improvement"
		}

		d := CodeHealthDelta{
			File:         file,
			HealthBefore: before,
			HealthAfter:  after,
			Delta:        delta,
			Grade:        grade,
			GradeBefore:  gradeBefore,
			TopFactor:    topFactor,
		}
		deltas = append(deltas, d)

		// Generate findings for significant degradation
		if delta < -10 {
			sev := "warning"
			if after < 30 {
				sev = "error"
			}
			findings = append(findings, ReviewFinding{
				Check:    "health",
				Severity: sev,
				File:     file,
				Message:  fmt.Sprintf("Health %s→%s (%d→%d, %+d points)", gradeBefore, grade, before, after, delta),
				Category: "health",
				RuleID:   "ckb/health/degradation",
			})
		}
	}

	// Build report
	report := &CodeHealthReport{
		Deltas: deltas,
	}
	if len(deltas) > 0 {
		totalDelta := 0
		worstScore := 101
		for _, d := range deltas {
			totalDelta += d.Delta
			if d.Delta < 0 {
				report.Degraded++
			}
			if d.Delta > 0 {
				report.Improved++
			}
			if d.HealthAfter < worstScore {
				worstScore = d.HealthAfter
				report.WorstFile = d.File
				report.WorstGrade = d.Grade
			}
		}
		report.AverageDelta = float64(totalDelta) / float64(len(deltas))
	}

	status := "pass"
	summary := "No significant health changes"
	if report.Degraded > 0 {
		summary = fmt.Sprintf("%d file(s) degraded, %d improved (avg %+.1f)",
			report.Degraded, report.Improved, report.AverageDelta)
		if report.AverageDelta < -5 {
			status = "warn"
		}
	}

	return ReviewCheck{
		Name:     "health",
		Status:   status,
		Severity: "warning",
		Summary:  summary,
		Details:  report,
		Duration: time.Since(start).Milliseconds(),
	}, findings, report
}

// calculateFileHealth computes a 0-100 health score for a file in its current state.
func (e *Engine) calculateFileHealth(ctx context.Context, file string) int {
	absPath := filepath.Join(e.repoRoot, file)
	score := 100.0

	// Cyclomatic complexity (20%)
	if complexity.IsAvailable() {
		analyzer := complexity.NewAnalyzer()
		result, err := analyzer.AnalyzeFile(ctx, absPath)
		if err == nil && result.Error == "" {
			cycScore := complexityToScore(result.MaxCyclomatic)
			score -= (100 - cycScore) * weightCyclomatic

			// Cognitive complexity (15%)
			cogScore := complexityToScore(result.MaxCognitive)
			score -= (100 - cogScore) * weightCognitive
		}
	}

	// File size (10%)
	loc := countLines(absPath)
	locScore := fileSizeToScore(loc)
	score -= (100 - locScore) * weightFileSize

	// Churn (15%) — number of recent changes
	churnScore := e.churnToScore(ctx, file)
	score -= (100 - churnScore) * weightChurn

	// Coupling degree (10%)
	couplingScore := e.couplingToScore(ctx, file)
	score -= (100 - couplingScore) * weightCoupling

	// Bus factor (10%)
	busScore := e.busFactorToScore(file)
	score -= (100 - busScore) * weightBusFactor

	// Age since last change (10%) — older unchanged = higher risk of rot
	ageScore := e.ageToScore(ctx, file)
	score -= (100 - ageScore) * weightAge

	// Coverage placeholder (10%) — not yet implemented, assume neutral
	// When coverage data is available, this will be filled in

	if score < 0 {
		score = 0
	}
	return int(math.Round(score))
}

// calculateBaseFileHealth gets the health of a file at a base branch ref.
// Uses current health as approximation — full implementation would analyze
// the file content at the base ref independently.
func (e *Engine) calculateBaseFileHealth(ctx context.Context, file string, _ string) int {
	// For files that exist, approximate base health as current health.
	// This is conservative — it won't detect improvements or degradations
	// from the base. Full implementation would use git show + analyze.
	return e.calculateFileHealth(ctx, file)
}

// --- Scoring helper functions ---

func complexityToScore(maxComplexity int) float64 {
	switch {
	case maxComplexity <= 5:
		return 100
	case maxComplexity <= 10:
		return 85
	case maxComplexity <= 20:
		return 65
	case maxComplexity <= 30:
		return 40
	default:
		return 20
	}
}

func fileSizeToScore(loc int) float64 {
	switch {
	case loc <= 100:
		return 100
	case loc <= 300:
		return 85
	case loc <= 500:
		return 70
	case loc <= 1000:
		return 50
	default:
		return 30
	}
}

func (e *Engine) churnToScore(ctx context.Context, file string) float64 {
	if e.gitAdapter == nil {
		return 75
	}
	history, err := e.gitAdapter.GetFileHistory(file, 30)
	if err != nil || history == nil {
		return 75
	}
	commits := history.CommitCount
	switch {
	case commits <= 2:
		return 100
	case commits <= 5:
		return 80
	case commits <= 10:
		return 60
	case commits <= 20:
		return 40
	default:
		return 20
	}
}

func (e *Engine) couplingToScore(ctx context.Context, file string) float64 {
	analyzer := coupling.NewAnalyzer(e.repoRoot, e.logger)
	result, err := analyzer.Analyze(ctx, coupling.AnalyzeOptions{
		RepoRoot:       e.repoRoot,
		Target:         file,
		MinCorrelation: 0.3,
		Limit:          20,
	})
	if err != nil {
		return 75
	}
	coupled := len(result.Correlations)
	switch {
	case coupled <= 2:
		return 100
	case coupled <= 5:
		return 80
	case coupled <= 10:
		return 60
	default:
		return 40
	}
}

func (e *Engine) busFactorToScore(file string) float64 {
	result, err := ownership.RunGitBlame(e.repoRoot, file)
	if err != nil {
		return 75
	}
	config := ownership.BlameConfig{
		TimeDecayHalfLife: 365,
	}
	own := ownership.ComputeBlameOwnership(result, config)
	if own == nil {
		return 75
	}
	contributors := len(own.Contributors)
	switch {
	case contributors >= 5:
		return 100 // Shared knowledge
	case contributors >= 3:
		return 85
	case contributors >= 2:
		return 60
	default:
		return 30 // Single author = bus factor 1
	}
}

func (e *Engine) ageToScore(_ context.Context, file string) float64 {
	if e.gitAdapter == nil {
		return 75
	}
	history, err := e.gitAdapter.GetFileHistory(file, 1)
	if err != nil || history == nil || len(history.Commits) == 0 {
		return 75
	}
	ts, err := time.Parse(time.RFC3339, history.Commits[0].Timestamp)
	if err != nil {
		return 75
	}
	daysSince := time.Since(ts).Hours() / 24
	switch {
	case daysSince <= 30:
		return 100 // Recently maintained
	case daysSince <= 90:
		return 85
	case daysSince <= 180:
		return 70
	case daysSince <= 365:
		return 50
	default:
		return 30 // Stale
	}
}

func healthGrade(score int) string {
	switch {
	case score >= 90:
		return "A"
	case score >= 70:
		return "B"
	case score >= 50:
		return "C"
	case score >= 30:
		return "D"
	default:
		return "F"
	}
}

func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		count++
	}
	return count
}
