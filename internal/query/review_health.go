package query

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
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
	NewFile      bool   `json:"newFile,omitempty"`
}

// CodeHealthReport aggregates health deltas across the PR.
type CodeHealthReport struct {
	Deltas       []CodeHealthDelta `json:"deltas"`
	AverageDelta float64           `json:"averageDelta"`
	WorstFile    string            `json:"worstFile,omitempty"`
	WorstGrade   string            `json:"worstGrade,omitempty"`
	Degraded     int               `json:"degraded"` // Files that got worse
	Improved     int               `json:"improved"` // Files that got better
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

	// Maximum files to compute health for. Beyond this, the check
	// reports results for the first N files only.
	maxHealthFiles = 30
)

// repoMetrics caches branch-independent per-file metrics (churn, coupling,
// bus factor, age) so they're computed once, not twice (before + after).
type repoMetrics struct {
	churn    float64
	coupling float64
	bus      float64
	age      float64
}

// checkCodeHealth calculates health score deltas for changed files.
func (e *Engine) checkCodeHealth(ctx context.Context, files []string, opts ReviewPROptions) (ReviewCheck, []ReviewFinding, *CodeHealthReport) {
	start := time.Now()

	var deltas []CodeHealthDelta
	var findings []ReviewFinding

	// Create a single complexity analyzer to reuse across all files.
	// Each call to NewAnalyzer allocates a cgo tree-sitter Parser;
	// reusing one avoids 60+ unnecessary alloc/free cycles.
	var analyzer *complexity.Analyzer
	if complexity.IsAvailable() {
		analyzer = complexity.NewAnalyzer()
	}

	// Cap file count to avoid excessive subprocess calls
	capped := files
	if len(capped) > maxHealthFiles {
		capped = capped[:maxHealthFiles]
	}

	for _, file := range capped {
		// Check for context cancellation between files
		if ctx.Err() != nil {
			break
		}

		absPath := filepath.Join(e.repoRoot, file)
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			continue
		}

		// Compute repo-level metrics once — they are branch-independent
		// so before/after values are identical and contribute zero to the delta.
		rm := e.computeRepoMetrics(ctx, file)

		after := e.calculateFileHealth(ctx, file, rm, analyzer)
		before, isNew := e.calculateBaseFileHealth(ctx, file, opts.BaseBranch, rm, analyzer)

		delta := after - before
		grade := healthGrade(after)
		gradeBefore := healthGrade(before)

		topFactor := "unchanged"
		if isNew {
			topFactor = "new file"
		} else if delta < -10 {
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
			NewFile:      isNew,
		}
		deltas = append(deltas, d)

		// Generate findings for significant degradation (skip new files —
		// they don't have a prior state to degrade from)
		if !isNew && delta < -10 {
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
		existingCount := 0
		worstScore := 101
		for _, d := range deltas {
			if !d.NewFile {
				totalDelta += d.Delta
				existingCount++
				if d.Delta < 0 {
					report.Degraded++
				}
				if d.Delta > 0 {
					report.Improved++
				}
			}
			if d.HealthAfter < worstScore {
				worstScore = d.HealthAfter
				report.WorstFile = d.File
				report.WorstGrade = d.Grade
			}
		}
		if existingCount > 0 {
			report.AverageDelta = float64(totalDelta) / float64(existingCount)
		}
	}

	status := "pass"
	summary := "No significant health changes"
	if report.Degraded > 0 {
		summary = fmt.Sprintf("%d file(s) degraded, %d improved (avg %+.1f)",
			report.Degraded, report.Improved, report.AverageDelta)
		if report.AverageDelta < -5 {
			status = "warn"
		}
	} else if report.Degraded == 0 && len(deltas) > 0 {
		// All changes are new files or unchanged — not a health concern
		newCount := 0
		for _, d := range deltas {
			if d.NewFile {
				newCount++
			}
		}
		if newCount > 0 {
			summary = fmt.Sprintf("%d new file(s), %d unchanged", newCount, len(deltas)-newCount)
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

// computeRepoMetrics computes branch-independent metrics for a file once.
func (e *Engine) computeRepoMetrics(ctx context.Context, file string) repoMetrics {
	return repoMetrics{
		churn:    e.churnToScore(ctx, file),
		coupling: e.couplingToScore(ctx, file),
		bus:      e.busFactorToScore(file),
		age:      e.ageToScore(ctx, file),
	}
}

// calculateFileHealth computes a 0-100 health score for a file in its current state.
// analyzer may be nil if tree-sitter is not available.
func (e *Engine) calculateFileHealth(ctx context.Context, file string, rm repoMetrics, analyzer *complexity.Analyzer) int {
	absPath := filepath.Join(e.repoRoot, file)
	score := 100.0

	// Cyclomatic complexity (20%)
	if analyzer != nil {
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

	// Repo-level metrics (pre-computed, branch-independent)
	score -= (100 - rm.churn) * weightChurn
	score -= (100 - rm.coupling) * weightCoupling
	score -= (100 - rm.bus) * weightBusFactor
	score -= (100 - rm.age) * weightAge

	if score < 0 {
		score = 0
	}
	return int(math.Round(score))
}

// calculateBaseFileHealth gets the health of a file at a base branch ref.
// Only computes file-specific metrics (complexity, size) from the base version.
// Repo-level metrics (churn, coupling, bus factor, age) are branch-independent
// and already included via the shared repoMetrics.
// analyzer may be nil if tree-sitter is not available.
// calculateBaseFileHealth returns (health score, isNewFile).
func (e *Engine) calculateBaseFileHealth(ctx context.Context, file string, baseBranch string, rm repoMetrics, analyzer *complexity.Analyzer) (int, bool) {
	if baseBranch == "" {
		return e.calculateFileHealth(ctx, file, rm, analyzer), false
	}

	// Get the file content at the base branch
	cmd := exec.CommandContext(ctx, "git", "-C", e.repoRoot, "show", baseBranch+":"+file)
	content, err := cmd.Output()
	if err != nil {
		// File doesn't exist at base — it's a new file.
		// Use 0 as baseline so the delta is purely the file's health score.
		return 0, true
	}

	// Write to temp file for analysis
	tmpFile, err := os.CreateTemp("", "ckb-base-*"+filepath.Ext(file))
	if err != nil {
		return e.calculateFileHealth(ctx, file, rm, analyzer), false
	}
	defer func() {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
	}()

	if _, err := tmpFile.Write(content); err != nil {
		return e.calculateFileHealth(ctx, file, rm, analyzer), false
	}
	tmpFile.Close()

	score := 100.0

	// Cyclomatic complexity (20%) — from base file content
	if analyzer != nil {
		result, err := analyzer.AnalyzeFile(ctx, tmpFile.Name())
		if err == nil && result.Error == "" {
			cycScore := complexityToScore(result.MaxCyclomatic)
			score -= (100 - cycScore) * weightCyclomatic

			cogScore := complexityToScore(result.MaxCognitive)
			score -= (100 - cogScore) * weightCognitive
		}
	}

	// File size (10%) — from base file content
	loc := countLines(tmpFile.Name())
	locScore := fileSizeToScore(loc)
	score -= (100 - locScore) * weightFileSize

	// Repo-level metrics — same as current (branch-independent)
	score -= (100 - rm.churn) * weightChurn
	score -= (100 - rm.coupling) * weightCoupling
	score -= (100 - rm.bus) * weightBusFactor
	score -= (100 - rm.age) * weightAge

	if score < 0 {
		score = 0
	}
	return int(math.Round(score)), false
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
