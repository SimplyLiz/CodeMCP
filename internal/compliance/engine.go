package compliance

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/complexity"
	"github.com/SimplyLiz/CodeMCP/internal/query"
)

// RunAudit executes a compliance audit against the selected frameworks.
func RunAudit(ctx context.Context, opts AuditOptions, logger *slog.Logger) (*ComplianceReport, error) {
	start := time.Now()

	// Defaults
	if opts.MinConfidence <= 0 {
		opts.MinConfidence = 0.5
	}
	if opts.FailOn == "" {
		opts.FailOn = "error"
	}
	if opts.SILLevel <= 0 || opts.SILLevel > 4 {
		opts.SILLevel = 2
	}

	// Resolve frameworks
	frameworks, err := resolveFrameworks(opts.Frameworks)
	if err != nil {
		return nil, err
	}

	// Find source files
	files, err := findSourceFiles(opts.RepoRoot, opts.Scope)
	if err != nil {
		return nil, fmt.Errorf("finding source files: %w", err)
	}

	// Exclude the compliance package itself from scanning — its check
	// definitions contain pattern strings (regex for "http://", "md5.New()",
	// "dangerouslySetInnerHTML", etc.) that trigger sibling checks,
	// producing systematic false positives.
	filtered := files[:0]
	for _, f := range files {
		if !strings.HasPrefix(f, "internal/compliance/") {
			filtered = append(filtered, f)
		}
	}
	files = filtered

	logger.Debug("Compliance audit starting",
		"frameworks", len(frameworks),
		"files", len(files),
		"repoRoot", opts.RepoRoot,
	)

	// Build scan scope
	var ca *complexity.Analyzer
	if complexity.IsAvailable() {
		ca = complexity.NewAnalyzer()
	}

	config := &ComplianceConfig{
		SILLevel: opts.SILLevel,
	}

	scope := &ScanScope{
		RepoRoot:           opts.RepoRoot,
		Files:              files,
		Config:             config,
		Logger:             logger,
		ComplexityAnalyzer: ca,
	}

	// Collect all checks from selected frameworks
	type checkEntry struct {
		framework Framework
		check     Check
	}
	var allChecks []checkEntry

	for _, fw := range frameworks {
		for _, c := range fw.Checks() {
			// Apply check filter if specified
			if len(opts.Checks) > 0 && !matchesCheckFilter(c.ID(), string(fw.ID()), opts.Checks) {
				continue
			}
			allChecks = append(allChecks, checkEntry{framework: fw, check: c})
		}
	}

	// Run checks in parallel
	type checkResult struct {
		framework  FrameworkID
		checkID    string
		checkName  string
		article    string
		severity   string
		findings   []Finding
		err        error
		durationMs int64
	}

	results := make([]checkResult, len(allChecks))
	var wg sync.WaitGroup

	// Limit concurrency to avoid exhausting file descriptors.
	// Each check opens files; 126 checks × N files can exceed ulimit.
	maxWorkers := runtime.GOMAXPROCS(0) * 4
	if maxWorkers > 32 {
		maxWorkers = 32
	}
	sem := make(chan struct{}, maxWorkers)

	for i, entry := range allChecks {
		wg.Add(1)
		go func(idx int, fw Framework, c Check) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release
			checkStart := time.Now()

			findings, err := c.Run(ctx, scope)

			// Tag findings with framework/check metadata
			for j := range findings {
				findings[j].Framework = fw.ID()
				findings[j].CheckID = c.ID()
				if findings[j].Article == "" {
					findings[j].Article = c.Article()
				}
				if findings[j].Severity == "" {
					findings[j].Severity = c.Severity()
				}
			}

			results[idx] = checkResult{
				framework:  fw.ID(),
				checkID:    c.ID(),
				checkName:  c.Name(),
				article:    c.Article(),
				severity:   c.Severity(),
				findings:   findings,
				err:        err,
				durationMs: time.Since(checkStart).Milliseconds(),
			}
		}(i, entry.framework, entry.check)
	}

	wg.Wait()

	// Aggregate results
	var allFindings []query.ReviewFinding
	var checks []query.ReviewCheck
	filesWithIssues := make(map[string]bool)

	// Per-framework tracking
	fwStats := make(map[FrameworkID]*FrameworkCoverage)
	for _, fw := range frameworks {
		fwStats[fw.ID()] = &FrameworkCoverage{
			Framework: fw.ID(),
			Name:      fw.Name(),
		}
	}

	for _, r := range results {
		stat := fwStats[r.framework]
		stat.TotalChecks++

		// Build ReviewCheck
		status := "pass"
		summary := "No issues found"

		if r.err != nil {
			status = "skip"
			summary = "Error: " + r.err.Error()
			stat.Skipped++
		} else {
			// Filter by confidence
			var filtered []Finding
			for _, f := range r.findings {
				if f.Confidence >= opts.MinConfidence {
					filtered = append(filtered, f)
				}
			}

			if len(filtered) > 0 {
				hasError := false
				for _, f := range filtered {
					if f.Severity == "error" {
						hasError = true
						break
					}
				}
				if hasError {
					status = "fail"
					stat.Failed++
				} else {
					status = "warn"
					stat.Warned++
				}
				summary = fmt.Sprintf("%d finding(s) — %s", len(filtered), r.article)
			} else {
				stat.Passed++
			}

			// Convert findings to ReviewFinding.
			// Cap findings per check to avoid a single noisy check dominating output.
			const maxFindingsPerCheck = 50
			for fi, f := range filtered {
				if fi >= maxFindingsPerCheck {
					summary = fmt.Sprintf("%d finding(s) — %s (showing %d)", len(filtered), r.article, maxFindingsPerCheck)
					break
				}
				rf := f.ToReviewFinding()
				allFindings = append(allFindings, rf)
				if f.File != "" {
					filesWithIssues[f.File] = true
				}
			}
		}

		checks = append(checks, query.ReviewCheck{
			Name:     string(r.framework) + "/" + r.checkID,
			Status:   status,
			Severity: r.severity,
			Summary:  summary,
			Duration: r.durationMs,
		})
	}

	// Enrich findings with cross-framework references
	allFindings = EnrichWithCrossReferences(allFindings)

	// Sort findings by severity then file
	sort.Slice(allFindings, func(i, j int) bool {
		si := severityOrder(allFindings[i].Severity)
		sj := severityOrder(allFindings[j].Severity)
		if si != sj {
			return si < sj
		}
		return allFindings[i].File < allFindings[j].File
	})

	// Calculate per-framework scores
	var coverage []FrameworkCoverage
	for _, fw := range frameworks {
		stat := fwStats[fw.ID()]
		if stat.TotalChecks > 0 {
			stat.Score = int(float64(stat.Passed) / float64(stat.TotalChecks) * 100)
		}
		coverage = append(coverage, *stat)
	}

	// Overall verdict and score
	verdict := "pass"
	totalChecks := 0
	totalPassed := 0
	bySeverity := make(map[string]int)

	for _, c := range coverage {
		totalChecks += c.TotalChecks
		totalPassed += c.Passed
		if c.Failed > 0 {
			verdict = "fail"
		} else if c.Warned > 0 && verdict != "fail" {
			verdict = "warn"
		}
	}

	for _, f := range allFindings {
		bySeverity[f.Severity]++
	}

	score := 100
	if totalChecks > 0 {
		score = int(float64(totalPassed) / float64(totalChecks) * 100)
	}

	report := &ComplianceReport{
		Repo:       filepath.Base(opts.RepoRoot),
		AnalyzedAt: time.Now(),
		Frameworks: opts.Frameworks,
		Verdict:    verdict,
		Score:      score,
		Checks:     checks,
		Findings:   allFindings,
		Coverage:   coverage,
		Summary: ComplianceSummary{
			TotalFindings:   len(allFindings),
			BySeverity:      bySeverity,
			FilesScanned:    len(files),
			FilesWithIssues: len(filesWithIssues),
		},
	}

	logger.Debug("Compliance audit complete",
		"frameworks", len(frameworks),
		"checks", len(checks),
		"findings", len(allFindings),
		"verdict", verdict,
		"score", score,
		"duration", time.Since(start).Milliseconds(),
	)

	return report, nil
}

func resolveFrameworks(ids []FrameworkID) ([]Framework, error) {
	var frameworks []Framework

	for _, id := range ids {
		if id == "all" {
			return All(), nil
		}
		fw, ok := Get(id)
		if !ok {
			return nil, fmt.Errorf("unknown framework: %q (available: %s)", id, strings.Join(frameworkNames(), ", "))
		}
		frameworks = append(frameworks, fw)
	}

	if len(frameworks) == 0 {
		return nil, fmt.Errorf("no frameworks specified (available: %s)", strings.Join(frameworkNames(), ", "))
	}

	return frameworks, nil
}

func frameworkNames() []string {
	names := make([]string, len(AllFrameworkIDs))
	for i, id := range AllFrameworkIDs {
		names[i] = string(id)
	}
	return names
}

func matchesCheckFilter(checkID, frameworkID string, filters []string) bool {
	for _, f := range filters {
		if f == checkID || f == frameworkID+"/"+checkID {
			return true
		}
	}
	return false
}

func severityOrder(s string) int {
	switch s {
	case "error":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

// findSourceFiles finds all source files, optionally filtered by scope prefix.
func findSourceFiles(repoRoot, scope string) ([]string, error) {
	var files []string

	err := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr
		}

		if info.IsDir() {
			name := info.Name()
			if name == "node_modules" || name == "vendor" || name == ".git" ||
				name == "__pycache__" || name == ".ckb" || name == "dist" ||
				name == "build" || name == ".next" || name == "target" {
				return filepath.SkipDir
			}
			return nil
		}

		ext := filepath.Ext(path)
		if !isSourceExt(ext) {
			return nil
		}

		relPath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}

		// Apply scope filter
		if scope != "" && !strings.HasPrefix(relPath, scope) {
			return nil
		}

		files = append(files, relPath)
		return nil
	})

	return files, err
}

func isSourceExt(ext string) bool {
	switch ext {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py",
		".java", ".kt", ".rs", ".rb",
		".c", ".cpp", ".h", ".hpp",
		".cs", ".swift", ".dart", ".scala":
		return true
	}
	return false
}
