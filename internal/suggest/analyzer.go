package suggest

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/SimplyLiz/CodeMCP/internal/audit"
	"github.com/SimplyLiz/CodeMCP/internal/complexity"
	"github.com/SimplyLiz/CodeMCP/internal/coupling"
	"github.com/SimplyLiz/CodeMCP/internal/deadcode"

	scip "github.com/SimplyLiz/CodeMCP/internal/backends/scip"
)

// Analyzer detects refactoring opportunities by combining existing analyzers.
type Analyzer struct {
	repoRoot    string
	logger      *slog.Logger
	scipAdapter *scip.SCIPAdapter
}

// NewAnalyzer creates a new suggestion analyzer.
func NewAnalyzer(repoRoot string, logger *slog.Logger, scipAdapter *scip.SCIPAdapter) *Analyzer {
	return &Analyzer{
		repoRoot:    repoRoot,
		logger:      logger,
		scipAdapter: scipAdapter,
	}
}

// Analyze runs all detection heuristics in parallel and returns prioritized suggestions.
func (a *Analyzer) Analyze(ctx context.Context, opts AnalyzeOptions) (*SuggestResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	if opts.MinSeverity == "" {
		opts.MinSeverity = "low"
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var suggestions []Suggestion

	// 1. Complexity analysis → extract_function, simplify_function
	wg.Add(1)
	go func() {
		defer wg.Done()
		results := a.analyzeComplexity(ctx, opts.Scope)
		mu.Lock()
		suggestions = append(suggestions, results...)
		mu.Unlock()
	}()

	// 2. Coupling analysis → reduce_coupling, split_file
	wg.Add(1)
	go func() {
		defer wg.Done()
		results := a.analyzeCoupling(ctx, opts.Scope)
		mu.Lock()
		suggestions = append(suggestions, results...)
		mu.Unlock()
	}()

	// 3. Dead code detection → remove_dead_code
	wg.Add(1)
	go func() {
		defer wg.Done()
		results := a.analyzeDeadCode(ctx, opts.Scope)
		mu.Lock()
		suggestions = append(suggestions, results...)
		mu.Unlock()
	}()

	// 4. Risk + test gaps → add_tests
	wg.Add(1)
	go func() {
		defer wg.Done()
		results := a.analyzeTestGaps(ctx, opts.Scope)
		mu.Lock()
		suggestions = append(suggestions, results...)
		mu.Unlock()
	}()

	wg.Wait()

	// Filter by type if specified
	if len(opts.Types) > 0 {
		typeSet := make(map[string]bool)
		for _, t := range opts.Types {
			typeSet[t] = true
		}
		var filtered []Suggestion
		for _, s := range suggestions {
			if typeSet[string(s.Type)] {
				filtered = append(filtered, s)
			}
		}
		suggestions = filtered
	}

	// Filter by minimum severity
	suggestions = filterBySeverity(suggestions, opts.MinSeverity)

	// Deduplicate (same target + type)
	suggestions = dedup(suggestions)

	// Sort by severity (critical > high > medium > low), then by priority (higher first)
	sort.Slice(suggestions, func(i, j int) bool {
		si := severityRank(suggestions[i].Severity)
		sj := severityRank(suggestions[j].Severity)
		if si != sj {
			return si > sj
		}
		return suggestions[i].Priority > suggestions[j].Priority
	})

	totalFound := len(suggestions)

	// Truncate
	if len(suggestions) > opts.Limit {
		suggestions = suggestions[:opts.Limit]
	}

	// Build summary
	summary := buildSummary(suggestions)

	return &SuggestResult{
		Suggestions: suggestions,
		Summary:     summary,
		TotalFound:  totalFound,
	}, nil
}

// analyzeComplexity finds functions with high complexity.
func (a *Analyzer) analyzeComplexity(ctx context.Context, scope string) []Suggestion {
	if !complexity.IsAvailable() {
		return nil
	}

	analyzer := complexity.NewAnalyzer()
	if analyzer == nil {
		return nil
	}

	var suggestions []Suggestion

	files := a.listSourceFiles(scope)
	for _, file := range files {
		absPath := filepath.Join(a.repoRoot, file)
		fc, err := analyzer.AnalyzeFile(ctx, absPath)
		if err != nil || fc == nil {
			continue
		}

		for _, fn := range fc.Functions {
			// High cyclomatic + long → extract_function
			if fn.Cyclomatic > 10 && fn.Lines > 50 {
				suggestions = append(suggestions, Suggestion{
					Type:     SuggestExtractFunction,
					Severity: complexitySeverity(fn.Cyclomatic),
					Target:   fmt.Sprintf("%s:%s", file, fn.Name),
					Title:    fmt.Sprintf("Extract parts of %s (cyclomatic: %d, %d lines)", fn.Name, fn.Cyclomatic, fn.Lines),
					Description: fmt.Sprintf(
						"Function %s has cyclomatic complexity %d and spans %d lines. Consider extracting logical sub-sections into smaller functions.",
						fn.Name, fn.Cyclomatic, fn.Lines,
					),
					Rationale: []string{
						fmt.Sprintf("Cyclomatic complexity %d exceeds threshold of 10", fn.Cyclomatic),
						fmt.Sprintf("Function spans %d lines (threshold: 50)", fn.Lines),
					},
					Effort:   complexityEffort(fn.Lines),
					Priority: fn.Cyclomatic + fn.Lines/10,
				})
			}

			// High cognitive → simplify_function
			if fn.Cognitive > 15 {
				suggestions = append(suggestions, Suggestion{
					Type:     SuggestSimplifyFunction,
					Severity: complexitySeverity(fn.Cognitive),
					Target:   fmt.Sprintf("%s:%s", file, fn.Name),
					Title:    fmt.Sprintf("Simplify %s (cognitive complexity: %d)", fn.Name, fn.Cognitive),
					Description: fmt.Sprintf(
						"Function %s has cognitive complexity %d, indicating deeply nested or hard-to-follow logic. Consider flattening conditionals or extracting helpers.",
						fn.Name, fn.Cognitive,
					),
					Rationale: []string{
						fmt.Sprintf("Cognitive complexity %d exceeds threshold of 15", fn.Cognitive),
					},
					Effort:   "medium",
					Priority: fn.Cognitive,
				})
			}
		}
	}

	return suggestions
}

// analyzeCoupling finds highly coupled files.
func (a *Analyzer) analyzeCoupling(ctx context.Context, scope string) []Suggestion {
	var suggestions []Suggestion

	couplingAnalyzer := coupling.NewAnalyzer(a.repoRoot, a.logger)
	files := a.listSourceFiles(scope)

	for _, file := range files {
		result, err := couplingAnalyzer.Analyze(ctx, coupling.AnalyzeOptions{
			Target:         file,
			MinCorrelation: 0.7,
			WindowDays:     90,
			Limit:          5,
		})
		if err != nil || result == nil {
			continue
		}

		highCouplings := 0
		for _, corr := range result.Correlations {
			if corr.Correlation >= 0.7 {
				highCouplings++
			}
		}

		if highCouplings > 0 {
			suggestions = append(suggestions, Suggestion{
				Type:     SuggestReduceCoupling,
				Severity: couplingSeverity(highCouplings),
				Target:   file,
				Title:    fmt.Sprintf("Reduce coupling for %s (%d highly coupled files)", file, highCouplings),
				Description: fmt.Sprintf(
					"%s has %d files with co-change correlation > 0.7. Consider extracting shared logic into a common module.",
					file, highCouplings,
				),
				Rationale: []string{
					fmt.Sprintf("%d files change together > 70%% of the time", highCouplings),
				},
				Effort:   "medium",
				Priority: highCouplings * 10,
			})
		}

		// Files with > 10 incoming deps → split_file
		if len(result.Correlations) > 10 {
			suggestions = append(suggestions, Suggestion{
				Type:     SuggestSplitFile,
				Severity: "medium",
				Target:   file,
				Title:    fmt.Sprintf("Consider splitting %s (%d dependents)", file, len(result.Correlations)),
				Description: fmt.Sprintf(
					"%s has %d correlated files, suggesting it may have too many responsibilities.",
					file, len(result.Correlations),
				),
				Rationale: []string{
					fmt.Sprintf("File has %d co-change correlations (threshold: 10)", len(result.Correlations)),
				},
				Effort:   "large",
				Priority: len(result.Correlations),
			})
		}
	}

	return suggestions
}

// analyzeDeadCode finds unused code.
func (a *Analyzer) analyzeDeadCode(ctx context.Context, scope string) []Suggestion {
	if a.scipAdapter == nil || !a.scipAdapter.IsAvailable() {
		return nil
	}

	dcAnalyzer := deadcode.NewAnalyzer(a.scipAdapter, a.repoRoot, a.logger, nil)
	opts := deadcode.DefaultOptions()
	if scope != "" {
		opts.Scope = []string{scope}
	}
	opts.Limit = 20

	result, err := dcAnalyzer.Analyze(ctx, opts)
	if err != nil || result == nil {
		return nil
	}

	var suggestions []Suggestion
	for _, item := range result.DeadCode {
		suggestions = append(suggestions, Suggestion{
			Type:     SuggestRemoveDeadCode,
			Severity: deadCodeSeverity(item.Confidence),
			Target:   fmt.Sprintf("%s:%s", item.FilePath, item.SymbolName),
			Title:    fmt.Sprintf("Remove unused %s %s", item.Kind, item.SymbolName),
			Description: fmt.Sprintf(
				"%s %s in %s appears to be dead code (%s, confidence: %.0f%%).",
				item.Kind, item.SymbolName, item.FilePath, item.Reason, item.Confidence*100,
			),
			Rationale: []string{item.Reason},
			Effort:    "small",
			Priority:  int(item.Confidence * 100),
		})
	}

	return suggestions
}

// analyzeTestGaps finds high-risk items without tests.
func (a *Analyzer) analyzeTestGaps(ctx context.Context, scope string) []Suggestion {
	auditAnalyzer := audit.NewAnalyzer(a.repoRoot, a.logger)
	auditOpts := audit.AuditOptions{
		RepoRoot: a.repoRoot,
		MinScore: 40,
		Limit:    20,
	}

	result, err := auditAnalyzer.Analyze(ctx, auditOpts)
	if err != nil || result == nil {
		return nil
	}

	var suggestions []Suggestion
	for _, item := range result.Items {
		// Only suggest tests for high/critical risk items
		if item.RiskLevel != "high" && item.RiskLevel != "critical" {
			continue
		}

		// Check if this file already has tests nearby
		if hasTestFile(a.repoRoot, item.File) {
			continue
		}

		if scope != "" && !strings.HasPrefix(item.File, scope) {
			continue
		}

		suggestions = append(suggestions, Suggestion{
			Type:     SuggestAddTests,
			Severity: item.RiskLevel,
			Target:   item.File,
			Title:    fmt.Sprintf("Add tests for %s (risk: %s, score: %.0f)", filepath.Base(item.File), item.RiskLevel, item.RiskScore),
			Description: fmt.Sprintf(
				"%s has risk score %.0f (%s) but no test file was found. Adding tests would reduce risk.",
				item.File, item.RiskScore, item.RiskLevel,
			),
			Rationale: []string{
				fmt.Sprintf("Risk score: %.0f (%s)", item.RiskScore, item.RiskLevel),
				"No corresponding test file found",
			},
			Effort:   "medium",
			Priority: int(item.RiskScore),
		})
	}

	return suggestions
}

// listSourceFiles returns source files under the given scope.
func (a *Analyzer) listSourceFiles(scope string) []string {
	root := a.repoRoot
	if scope != "" {
		root = filepath.Join(a.repoRoot, scope)
	}

	var files []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext == ".go" || ext == ".ts" || ext == ".js" || ext == ".py" || ext == ".rs" || ext == ".java" {
			rel, err := filepath.Rel(a.repoRoot, path)
			if err == nil {
				files = append(files, rel)
			}
		}
		return nil
	})

	// Limit to avoid analyzing too many files
	if len(files) > 100 {
		files = files[:100]
	}

	return files
}

// hasTestFile checks if a test file exists for the given source file.
func hasTestFile(repoRoot, filePath string) bool {
	ext := filepath.Ext(filePath)
	base := strings.TrimSuffix(filePath, ext)

	// Go: _test.go
	if ext == ".go" {
		testPath := filepath.Join(repoRoot, base+"_test.go")
		if _, err := os.Stat(testPath); err == nil {
			return true
		}
	}

	// JS/TS: .test.ts, .spec.ts
	if ext == ".ts" || ext == ".js" || ext == ".tsx" || ext == ".jsx" {
		for _, suffix := range []string{".test" + ext, ".spec" + ext} {
			testPath := filepath.Join(repoRoot, base+suffix)
			if _, err := os.Stat(testPath); err == nil {
				return true
			}
		}
	}

	// Python: test_*.py
	if ext == ".py" {
		dir := filepath.Dir(filePath)
		name := filepath.Base(base)
		testPath := filepath.Join(repoRoot, dir, "test_"+name+ext)
		if _, err := os.Stat(testPath); err == nil {
			return true
		}
	}

	return false
}

// Helper functions

func complexitySeverity(value int) string {
	if value > 30 {
		return "critical"
	}
	if value > 20 {
		return "high"
	}
	if value > 10 {
		return "medium"
	}
	return "low"
}

func complexityEffort(lines int) string {
	if lines > 200 {
		return "large"
	}
	if lines > 100 {
		return "medium"
	}
	return "small"
}

func couplingSeverity(count int) string {
	if count >= 5 {
		return "high"
	}
	if count >= 3 {
		return "medium"
	}
	return "low"
}

func deadCodeSeverity(confidence float64) string {
	if confidence >= 0.9 {
		return "high"
	}
	if confidence >= 0.7 {
		return "medium"
	}
	return "low"
}

func severityRank(severity string) int {
	switch severity {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func filterBySeverity(suggestions []Suggestion, minSeverity string) []Suggestion {
	minRank := severityRank(minSeverity)
	var filtered []Suggestion
	for _, s := range suggestions {
		if severityRank(s.Severity) >= minRank {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func dedup(suggestions []Suggestion) []Suggestion {
	seen := make(map[string]bool)
	var result []Suggestion
	for _, s := range suggestions {
		key := string(s.Type) + ":" + s.Target
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, s)
	}
	return result
}

func buildSummary(suggestions []Suggestion) *SuggestSummary {
	summary := &SuggestSummary{
		BySeverity: make(map[string]int),
		ByType:     make(map[string]int),
	}
	for _, s := range suggestions {
		summary.BySeverity[s.Severity]++
		summary.ByType[string(s.Type)]++
	}
	return summary
}
