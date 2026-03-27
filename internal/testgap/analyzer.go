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
		// Skip barrel/re-export files — they contain no logic worth testing
		absFile := filepath.Join(a.repoRoot, file)
		ext := filepath.Ext(file)
		if (ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx") && isBarrelFile(absFile) {
			continue
		}

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
		// Check for module-level mock coverage — test files that mock this source
		// module via vi.mock/jest.mock cover all its exports indirectly.
		if a.isModuleMocked(file) {
			return true, ""
		}
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

	// Even without a direct name match, a module-level mock covers all exports
	if a.isModuleMocked(file) {
		return true, ""
	}

	return false, "no-name-match"
}

// findTestFiles locates test files for a given source file.
//
// Checks suffix patterns ({base}_test.ext, {base}.test.ext, {base}.spec.ext)
// and the Python/pytest prefix pattern (test_{base}.ext) in the same directory
// and in a sibling tests/ directory.
func (a *Analyzer) findTestFiles(file string) []string {
	ext := filepath.Ext(file)
	base := strings.TrimSuffix(file, ext)
	dir := filepath.Dir(file)
	name := filepath.Base(base) // filename without dir or extension

	// Suffix patterns (Go, JS/TS convention)
	candidates := []string{
		base + "_test" + ext,
		base + ".test" + ext,
		base + ".spec" + ext,
	}

	// Prefix pattern (Python/pytest convention): test_{name}.ext
	// Check same directory
	candidates = append(candidates, filepath.Join(dir, "test_"+name+ext))

	// Also check a sibling tests/ directory (common in Python projects)
	// e.g., src/pkg/foo.py → tests/test_foo.py
	testsDir := filepath.Join(filepath.Dir(dir), "tests")
	candidates = append(candidates, filepath.Join(testsDir, "test_"+name+ext))

	// Also check a top-level tests/ directory
	candidates = append(candidates, filepath.Join("tests", "test_"+name+ext))

	var found []string
	seen := map[string]bool{}
	for _, c := range candidates {
		if seen[c] {
			continue
		}
		seen[c] = true
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

// isModuleMocked checks whether any test file mocks this source module via
// vi.mock(...) or jest.mock(...). Module-level mocks provide factory replacements
// for all exports, so every exported function is covered indirectly.
func (a *Analyzer) isModuleMocked(file string) bool {
	ext := filepath.Ext(file)
	// Only relevant for JS/TS ecosystems
	if ext != ".ts" && ext != ".tsx" && ext != ".js" && ext != ".jsx" {
		return false
	}

	// Build relative import paths that test files would use to reference this module
	dir := filepath.Dir(file)
	base := strings.TrimSuffix(filepath.Base(file), ext)
	// An index file can be imported as the directory itself
	isIndex := base == "index"

	// Walk test files in the same directory and parent directories looking for mocks
	testExts := []string{".test.ts", ".test.tsx", ".test.js", ".test.jsx", ".spec.ts", ".spec.tsx", ".spec.js", ".spec.jsx"}
	var testPaths []string

	// Check same directory and parent
	for _, d := range []string{dir, filepath.Dir(dir)} {
		absDir := filepath.Join(a.repoRoot, d)
		entries, err := os.ReadDir(absDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			for _, te := range testExts {
				if strings.HasSuffix(name, te) {
					testPaths = append(testPaths, filepath.Join(d, name))
				}
			}
		}
	}

	// Also check __tests__ subdirectory
	testsDir := filepath.Join(dir, "__tests__")
	absTestsDir := filepath.Join(a.repoRoot, testsDir)
	if entries, err := os.ReadDir(absTestsDir); err == nil {
		for _, e := range entries {
			name := e.Name()
			for _, te := range testExts {
				if strings.HasSuffix(name, te) {
					testPaths = append(testPaths, filepath.Join(testsDir, name))
				}
			}
		}
	}

	for _, tp := range testPaths {
		absPath := filepath.Join(a.repoRoot, tp)
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		text := string(content)

		// Look for vi.mock('.../<module>') or jest.mock('.../<module>')
		// The mock path can reference the file by name or the directory (for index files)
		if strings.Contains(text, "vi.mock(") || strings.Contains(text, "jest.mock(") {
			// Check if the mock path references this file
			if strings.Contains(text, "/"+base+"'") || strings.Contains(text, "/"+base+"\"") || strings.Contains(text, "/"+base+"`") {
				return true
			}
			// For index files, mock can reference the directory
			if isIndex {
				dirName := filepath.Base(dir)
				if strings.Contains(text, "/"+dirName+"'") || strings.Contains(text, "/"+dirName+"\"") || strings.Contains(text, "/"+dirName+"`") {
					return true
				}
			}
		}
	}
	return false
}

// isBarrelFile returns true if the file consists only of re-exports with no
// real logic. Barrel files (e.g., index.ts that just re-exports from siblings)
// should not be flagged for missing tests.
func isBarrelFile(absPath string) bool {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return false
	}

	lines := strings.Split(string(content), "\n")
	hasExport := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}
		// Valid barrel lines: export { ... } from '...', export * from '...', export type { ... } from '...'
		if strings.HasPrefix(trimmed, "export ") && strings.Contains(trimmed, " from ") {
			hasExport = true
			continue
		}
		// Any other non-trivial line means this is not a pure barrel file
		return false
	}
	return hasExport
}
