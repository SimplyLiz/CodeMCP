// Package testgap identifies functions that lack test coverage by cross-referencing
// complexity analysis with test file detection via SCIP references or heuristics.
package testgap

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/backends"
	"github.com/SimplyLiz/CodeMCP/internal/backends/scip"
	"github.com/SimplyLiz/CodeMCP/internal/complexity"
)

// TestGap describes a function that appears untested.
type TestGap struct {
	File       string `json:"file"`
	Function   string `json:"function"`
	StartLine  int    `json:"startLine"`
	EndLine    int    `json:"endLine"`
	Complexity int    `json:"complexity"`
	Reason     string `json:"reason"` // "no-test-reference", "no-test-file", "no-name-match"
}

// GapSummary provides aggregate test gap statistics.
type GapSummary struct {
	TotalFunctions  int     `json:"totalFunctions"`
	TestedFunctions int     `json:"testedFunctions"`
	CoveragePercent float64 `json:"coveragePercent"`
}

// TestGapResult holds the analysis output.
type TestGapResult struct {
	Gaps    []TestGap  `json:"gaps"`
	Summary GapSummary `json:"summary"`
	Method  string     `json:"method"` // "scip" or "heuristic"
}

// AnalyzeOptions configures test gap analysis.
type AnalyzeOptions struct {
	Target   string // file or directory path (relative to repo root)
	MinLines int    // minimum function lines to include (default 3)
	Limit    int    // max gaps to return (default 50)
}

// Analyzer detects untested functions.
type Analyzer struct {
	repoRoot           string
	logger             *slog.Logger
	scipAdapter        *scip.SCIPAdapter
	complexityAnalyzer *complexity.Analyzer
}

// NewAnalyzer creates a test gap analyzer.
func NewAnalyzer(repoRoot string, logger *slog.Logger, scipAdapter *scip.SCIPAdapter) *Analyzer {
	var ca *complexity.Analyzer
	if complexity.IsAvailable() {
		ca = complexity.NewAnalyzer()
	}
	return &Analyzer{
		repoRoot:           repoRoot,
		logger:             logger,
		scipAdapter:        scipAdapter,
		complexityAnalyzer: ca,
	}
}

// Analyze runs test gap analysis on the target file(s).
func (a *Analyzer) Analyze(ctx context.Context, opts AnalyzeOptions) (*TestGapResult, error) {
	if opts.MinLines <= 0 {
		opts.MinLines = 3
	}
	if opts.Limit <= 0 {
		opts.Limit = 50
	}

	// Collect files to analyze
	files, err := a.collectFiles(opts.Target)
	if err != nil {
		return nil, err
	}

	useSCIP := a.scipAdapter != nil && a.scipAdapter.IsAvailable()
	method := "heuristic"
	if useSCIP {
		method = "scip"
	}

	var allGaps []TestGap
	totalFunctions := 0
	testedFunctions := 0

	for _, file := range files {
		functions, err := a.extractFunctions(ctx, file)
		if err != nil {
			a.logger.Debug("Failed to extract functions", "file", file, "error", err.Error())
			continue
		}

		for _, fn := range functions {
			if fn.Lines < opts.MinLines {
				continue
			}
			totalFunctions++

			tested := false
			reason := ""

			if useSCIP {
				tested, reason = a.checkTestedViaSCIP(ctx, file, fn)
			} else {
				tested, reason = a.checkTestedViaHeuristic(file, fn)
			}

			if tested {
				testedFunctions++
			} else {
				allGaps = append(allGaps, TestGap{
					File:       file,
					Function:   fn.Name,
					StartLine:  fn.StartLine,
					EndLine:    fn.EndLine,
					Complexity: fn.Cyclomatic,
					Reason:     reason,
				})
			}
		}
	}

	// Sort by complexity descending
	sort.Slice(allGaps, func(i, j int) bool {
		return allGaps[i].Complexity > allGaps[j].Complexity
	})

	// Apply limit
	if len(allGaps) > opts.Limit {
		allGaps = allGaps[:opts.Limit]
	}

	coveragePct := 0.0
	if totalFunctions > 0 {
		coveragePct = float64(testedFunctions) / float64(totalFunctions) * 100
	}

	return &TestGapResult{
		Gaps: allGaps,
		Summary: GapSummary{
			TotalFunctions:  totalFunctions,
			TestedFunctions: testedFunctions,
			CoveragePercent: coveragePct,
		},
		Method: method,
	}, nil
}

// collectFiles returns relative file paths for the target.
func (a *Analyzer) collectFiles(target string) ([]string, error) {
	absPath := target
	if !filepath.IsAbs(target) {
		absPath = filepath.Join(a.repoRoot, target)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		rel, _ := filepath.Rel(a.repoRoot, absPath)
		return []string{rel}, nil
	}

	var files []string
	err = filepath.Walk(absPath, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil //nolint:nilerr
		}
		if fi.IsDir() {
			name := fi.Name()
			if name == "node_modules" || name == "vendor" || name == ".git" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if isAnalyzableSource(filepath.Ext(path)) && !isTestFile(path) {
			rel, _ := filepath.Rel(a.repoRoot, path)
			files = append(files, rel)
		}
		return nil
	})
	return files, err
}

// extractFunctions uses the complexity analyzer to get per-function info.
func (a *Analyzer) extractFunctions(ctx context.Context, relPath string) ([]complexity.ComplexityResult, error) {
	if a.complexityAnalyzer == nil {
		return nil, nil
	}
	absPath := filepath.Join(a.repoRoot, relPath)
	fc, err := a.complexityAnalyzer.AnalyzeFile(ctx, absPath)
	if err != nil {
		return nil, err
	}
	if fc == nil || fc.Error != "" {
		return nil, nil
	}
	return fc.Functions, nil
}

// checkTestedViaSCIP checks if a function has references from test files using SCIP.
func (a *Analyzer) checkTestedViaSCIP(ctx context.Context, file string, fn complexity.ComplexityResult) (bool, string) {
	if fn.Name == "<anonymous>" || fn.Name == "<unknown>" {
		return false, "no-test-reference"
	}

	// Search for the function symbol within the file scope
	searchResult, err := a.scipAdapter.SearchSymbols(ctx, fn.Name, backends.SearchOptions{
		Scope:      []string{file},
		MaxResults: 5,
	})
	if err != nil || searchResult == nil || len(searchResult.Symbols) == 0 {
		// Fall back to heuristic if SCIP can't find the symbol
		return a.checkTestedViaHeuristic(file, fn)
	}

	// Check references for each matching symbol
	for _, sym := range searchResult.Symbols {
		refsResult, err := a.scipAdapter.FindReferences(ctx, sym.StableID, backends.RefOptions{
			IncludeTests: true,
			MaxResults:   100,
		})
		if err != nil || refsResult == nil {
			continue
		}
		for _, ref := range refsResult.References {
			if isTestFile(ref.Location.Path) {
				return true, ""
			}
		}
	}

	return false, "no-test-reference"
}

// checkTestedViaHeuristic checks if a function name appears in test files.
func (a *Analyzer) checkTestedViaHeuristic(file string, fn complexity.ComplexityResult) (bool, string) {
	if fn.Name == "<anonymous>" || fn.Name == "<unknown>" {
		return false, "no-name-match"
	}

	// Find corresponding test files
	testFiles := a.findTestFiles(file)
	if len(testFiles) == 0 {
		return false, "no-test-file"
	}

	// Check if function name appears in any test file
	for _, tf := range testFiles {
		absPath := filepath.Join(a.repoRoot, tf)
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		text := string(content)

		// Check for function name reference in test code
		if strings.Contains(text, fn.Name) {
			return true, ""
		}
	}

	return false, "no-name-match"
}

// findTestFiles locates test files for a given source file.
func (a *Analyzer) findTestFiles(file string) []string {
	ext := filepath.Ext(file)
	base := strings.TrimSuffix(file, ext)

	candidates := []string{
		base + "_test" + ext,
		base + ".test" + ext,
		base + ".spec" + ext,
	}

	var found []string
	for _, c := range candidates {
		absPath := filepath.Join(a.repoRoot, c)
		if _, err := os.Stat(absPath); err == nil {
			found = append(found, c)
		}
	}
	return found
}

func isTestFile(path string) bool {
	return strings.Contains(path, "_test.go") ||
		strings.Contains(path, ".test.ts") ||
		strings.Contains(path, ".test.js") ||
		strings.Contains(path, ".spec.ts") ||
		strings.Contains(path, ".spec.js") ||
		strings.Contains(path, "test_") ||
		strings.HasSuffix(path, "_test.py")
}

func isAnalyzableSource(ext string) bool {
	switch ext {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".java", ".kt", ".rs":
		return true
	}
	return false
}
