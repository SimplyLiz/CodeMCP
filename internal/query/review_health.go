package query

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/complexity"
	"github.com/SimplyLiz/CodeMCP/internal/ownership"
)

// CodeHealthDelta represents the health change for a single file.
type CodeHealthDelta struct {
	File         string  `json:"file"`
	HealthBefore int     `json:"healthBefore"` // 0-100
	HealthAfter  int     `json:"healthAfter"`  // 0-100
	Delta        int     `json:"delta"`        // negative = degradation
	Grade        string  `json:"grade"`        // A/B/C/D/F
	GradeBefore  string  `json:"gradeBefore"`
	TopFactor    string  `json:"topFactor"` // What drives the score most
	NewFile      bool    `json:"newFile,omitempty"`
	Confidence   float64 `json:"confidence"` // 0.0-1.0
	Parseable    bool    `json:"parseable"`  // false = tree-sitter can't analyze
}

// healthResult holds the output of calculateFileHealth including metadata.
type healthResult struct {
	score      int
	confidence float64
	parseable  bool
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

// Health score weights — must sum to 1.0.
// Coverage was removed because no coverage data source is available yet.
// When coverage is added, reduce churn and cyclomatic by 0.05 each.
const (
	weightCyclomatic = 0.15
	weightCognitive  = 0.25
	weightFileSize   = 0.10
	weightChurn      = 0.15
	weightCoupling   = 0.10
	weightBusFactor  = 0.10
	weightAge        = 0.15

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

	// Filter to existing files
	var existingFiles []string
	for _, file := range capped {
		absPath := filepath.Join(e.repoRoot, file)
		if _, err := os.Stat(absPath); !os.IsNotExist(err) {
			existingFiles = append(existingFiles, file)
		}
	}

	// Batch compute repo-level metrics (churn, coupling, bus factor, age)
	// in 3 git calls + parallel blame instead of 4 × N sequential calls.
	metricsMap := e.batchRepoMetrics(ctx, existingFiles)

	for _, file := range existingFiles {
		if ctx.Err() != nil {
			break
		}

		rm := metricsMap[file]

		e.tsMu.Lock()
		afterResult := e.calculateFileHealth(ctx, file, rm, analyzer)
		e.tsMu.Unlock()

		beforeScore, isNew := e.calculateBaseFileHealthLocked(ctx, file, opts.BaseBranch, rm, analyzer)

		after := afterResult.score
		before := beforeScore
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
			Confidence:   afterResult.confidence,
			Parseable:    afterResult.parseable,
		}
		deltas = append(deltas, d)

		// Generate findings for significant degradation (skip new files —
		// they don't have a prior state to degrade from)
		if !isNew && delta < -10 {
			sev := "warning"
			if after < 30 {
				sev = "error"
			}
			msg := fmt.Sprintf("Health %s→%s (%d→%d, %+d points)", gradeBefore, grade, before, after, delta)
			if d.Confidence < 0.6 {
				msg += " (low confidence)"
			}
			if !d.Parseable {
				msg += " [unparseable]"
			}
			findings = append(findings, ReviewFinding{
				Check:    "health",
				Severity: sev,
				File:     file,
				Message:  msg,
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

// batchRepoMetrics computes repo-level metrics for all files using batched
// git operations instead of 4 × N individual subprocess calls.
//
// Before: 30 files × (git log + git blame + coupling analyze + git log) = ~120+ calls
// After:  1 git log --name-only + parallel git blame = ~12 calls
func (e *Engine) batchRepoMetrics(ctx context.Context, files []string) map[string]repoMetrics {
	result := make(map[string]repoMetrics, len(files))
	defaultMetrics := repoMetrics{churn: 75, coupling: 75, bus: 75, age: 75}
	for _, f := range files {
		result[f] = defaultMetrics
	}

	if e.gitAdapter == nil || !e.gitAdapter.IsAvailable() {
		if e.logger != nil {
			e.logger.Warn("git unavailable, health scores use default metrics (75) and may not reflect actual quality")
		}
		return result
	}

	// --- Batch 1: Single git log for churn + age + coupling ---
	// One command replaces per-file GetFileHistory + coupling.Analyze calls.
	sinceDate := time.Now().AddDate(0, 0, -365).Format("2006-01-02")
	cmd := exec.CommandContext(ctx, "git", "log",
		"--format=COMMIT:%aI", "--name-only",
		"--since="+sinceDate)
	cmd.Dir = e.repoRoot
	logOutput, err := cmd.Output()
	if err == nil {
		churnAge, cochangeMatrix := parseGitLogBatch(string(logOutput))

		// Build file set for fast lookup
		fileSet := make(map[string]bool, len(files))
		for _, f := range files {
			fileSet[f] = true
		}

		for _, f := range files {
			rm := result[f]

			// Churn score — commit count in last 30 days
			if ca, ok := churnAge[f]; ok {
				rm.churn = churnCountToScore(ca.commitCount30d)
				rm.age = ageDaysToScore(ca.daysSinceLastCommit)
			}

			// Coupling score — count of highly correlated files
			if commits, ok := cochangeMatrix[f]; ok && len(commits) > 0 {
				coupled := countCoupledFiles(f, commits, cochangeMatrix, fileSet)
				rm.coupling = coupledCountToScore(coupled)
			}

			result[f] = rm
		}
	}

	// --- Batch 2: Parallel git blame for bus factor ---
	// Run up to 5 concurrent blame calls instead of 30 sequential.
	const maxBlameWorkers = 5
	blameCh := make(chan string, len(files))
	for _, f := range files {
		blameCh <- f
	}
	close(blameCh)

	var blameMu sync.Mutex
	var blameWg sync.WaitGroup
	workers := maxBlameWorkers
	if len(files) < workers {
		workers = len(files)
	}
	for i := 0; i < workers; i++ {
		blameWg.Add(1)
		go func() {
			defer blameWg.Done()
			for file := range blameCh {
				if ctx.Err() != nil {
					return
				}
				busScore := e.busFactorToScore(file)
				blameMu.Lock()
				rm := result[file]
				rm.bus = busScore
				result[file] = rm
				blameMu.Unlock()
			}
		}()
	}
	blameWg.Wait()

	return result
}

// churnAgeInfo holds per-file data extracted from a single git log scan.
type churnAgeInfo struct {
	commitCount30d      int
	daysSinceLastCommit float64
}

// parseGitLogBatch parses output of `git log --format=COMMIT:%aI --name-only`
// and returns per-file churn/age info plus a co-change matrix (file → list of commit indices).
func parseGitLogBatch(output string) (map[string]churnAgeInfo, map[string][]int) {
	churnAge := make(map[string]churnAgeInfo)
	cochange := make(map[string][]int) // file → commit indices

	now := time.Now()
	thirtyDaysAgo := now.AddDate(0, 0, -30)

	lines := strings.Split(output, "\n")
	commitIdx := -1
	var commitTime time.Time

	for _, line := range lines {
		if strings.HasPrefix(line, "COMMIT:") {
			commitIdx++
			ts := strings.TrimPrefix(line, "COMMIT:")
			parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(ts))
			if err == nil {
				commitTime = parsed
			}
			continue
		}

		file := strings.TrimSpace(line)
		if file == "" {
			continue
		}

		// Track co-change matrix
		cochange[file] = append(cochange[file], commitIdx)

		// Track churn + age
		ca := churnAge[file]
		if !commitTime.IsZero() {
			if commitTime.After(thirtyDaysAgo) {
				ca.commitCount30d++
			}
			daysSince := now.Sub(commitTime).Hours() / 24
			if ca.daysSinceLastCommit == 0 || daysSince < ca.daysSinceLastCommit {
				ca.daysSinceLastCommit = daysSince
			}
		}
		churnAge[file] = ca
	}

	return churnAge, cochange
}

// countCoupledFiles counts how many files are correlated (>= 30% co-change rate)
// with the target file, considering only files in the review set.
func countCoupledFiles(target string, targetCommits []int, cochange map[string][]int, fileSet map[string]bool) int {
	if len(targetCommits) == 0 {
		return 0
	}

	// Build set of target's commit indices
	commitSet := make(map[int]bool, len(targetCommits))
	for _, c := range targetCommits {
		commitSet[c] = true
	}

	coupled := 0
	for file, commits := range cochange {
		if file == target {
			continue
		}
		// Count overlapping commits
		overlap := 0
		for _, c := range commits {
			if commitSet[c] {
				overlap++
			}
		}
		rate := float64(overlap) / float64(len(targetCommits))
		if rate >= 0.3 {
			coupled++
		}
	}
	return coupled
}

func churnCountToScore(commits int) float64 {
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

func ageDaysToScore(days float64) float64 {
	switch {
	case days <= 30:
		return 100
	case days <= 90:
		return 85
	case days <= 180:
		return 70
	case days <= 365:
		return 50
	default:
		return 30
	}
}

func coupledCountToScore(coupled int) float64 {
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

// calculateFileHealth computes a 0-100 health score for a file in its current state.
// analyzer may be nil if tree-sitter is not available.
func (e *Engine) calculateFileHealth(ctx context.Context, file string, rm repoMetrics, analyzer *complexity.Analyzer) healthResult {
	absPath := filepath.Join(e.repoRoot, file)
	score := 100.0
	confidence := 1.0
	parseable := true

	// Cyclomatic complexity (15%) + Cognitive complexity (25%)
	complexityApplied := false
	if analyzer != nil {
		result, err := analyzer.AnalyzeFile(ctx, absPath)
		if err == nil && result.Error == "" {
			complexityApplied = true
			cycScore := complexityToScore(result.MaxCyclomatic)
			score -= (100 - cycScore) * weightCyclomatic

			cogScore := complexityToScore(result.MaxCognitive)
			score -= (100 - cogScore) * weightCognitive
		}
	}
	if !complexityApplied {
		// Tree-sitter couldn't parse this file (binary, unsupported language, etc.).
		// Apply a neutral-pessimistic penalty so unparseable files don't get
		// artificially high scores. 50 = middle of the scale.
		score -= (100 - 50) * weightCyclomatic
		score -= (100 - 50) * weightCognitive
		confidence -= 0.4
		parseable = false
	}

	// Check if all repo metrics are at default (75) — indicates no git data available
	defaultRM := repoMetrics{churn: 75, coupling: 75, bus: 75, age: 75}
	if rm == defaultRM {
		confidence -= 0.3
	}

	// Check if bus factor is at default
	if rm.bus == 75 && rm != defaultRM {
		confidence -= 0.2
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
	if confidence < 0 {
		confidence = 0
	}
	return healthResult{
		score:      int(math.Round(score)),
		confidence: confidence,
		parseable:  parseable,
	}
}

// calculateBaseFileHealthLocked gets the health of a file at a base branch ref.
// Acquires tsMu only for tree-sitter calls; git show runs unlocked.
func (e *Engine) calculateBaseFileHealthLocked(ctx context.Context, file string, baseBranch string, rm repoMetrics, analyzer *complexity.Analyzer) (int, bool) {
	if baseBranch == "" {
		e.tsMu.Lock()
		result := e.calculateFileHealth(ctx, file, rm, analyzer)
		e.tsMu.Unlock()
		return result.score, false
	}

	// git show runs without the tree-sitter lock
	cmd := exec.CommandContext(ctx, "git", "-C", e.repoRoot, "show", baseBranch+":"+file)
	content, err := cmd.Output()
	if err != nil {
		return 0, true // New file
	}

	tmpFile, err := os.CreateTemp("", "ckb-base-*"+filepath.Ext(file))
	if err != nil {
		e.tsMu.Lock()
		result := e.calculateFileHealth(ctx, file, rm, analyzer)
		e.tsMu.Unlock()
		return result.score, false
	}
	defer func() {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
	}()

	if _, err := tmpFile.Write(content); err != nil {
		e.tsMu.Lock()
		result := e.calculateFileHealth(ctx, file, rm, analyzer)
		e.tsMu.Unlock()
		return result.score, false
	}
	tmpFile.Close()

	score := 100.0

	// Tree-sitter: lock only for AnalyzeFile
	complexityApplied := false
	if analyzer != nil {
		e.tsMu.Lock()
		result, err := analyzer.AnalyzeFile(ctx, tmpFile.Name())
		e.tsMu.Unlock()
		if err == nil && result.Error == "" {
			complexityApplied = true
			cycScore := complexityToScore(result.MaxCyclomatic)
			score -= (100 - cycScore) * weightCyclomatic

			cogScore := complexityToScore(result.MaxCognitive)
			score -= (100 - cogScore) * weightCognitive
		}
	}
	if !complexityApplied {
		score -= (100 - 50) * weightCyclomatic
		score -= (100 - 50) * weightCognitive
	}

	loc := countLines(tmpFile.Name())
	locScore := fileSizeToScore(loc)
	score -= (100 - locScore) * weightFileSize

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
