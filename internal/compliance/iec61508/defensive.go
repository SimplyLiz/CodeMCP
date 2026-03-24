package iec61508

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- unchecked-error: Error detection and handling ---

type uncheckedErrorCheck struct{}

func (c *uncheckedErrorCheck) ID() string       { return "unchecked-error" }
func (c *uncheckedErrorCheck) Name() string     { return "Unchecked Error Returns" }
func (c *uncheckedErrorCheck) Article() string   { return "Table A.3 IEC 61508-3" }
func (c *uncheckedErrorCheck) Severity() string  { return "error" }

// Patterns for Go: common error-returning calls where error is discarded
var uncheckedErrorPatterns = []*regexp.Regexp{
	// Go: assigning to _ for error
	regexp.MustCompile(`\b\w+,\s*_\s*:?=\s*\w+\.\w+\(`),
	// Go: single return value ignored
	regexp.MustCompile(`^\s+\w+\.\w+\([^)]*\)\s*$`),
}

func (c *uncheckedErrorCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Only check Go files for this specific pattern (most reliable detection)
		if !strings.HasSuffix(file, ".go") {
			continue
		}

		// Skip test files
		if strings.Contains(file, "_test.go") {
			continue
		}

		f, err := os.Open(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		lineNum := 0

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			if strings.HasPrefix(trimmed, "//") {
				continue
			}

			// Detect error explicitly discarded with _
			if strings.Contains(line, ", _ =") || strings.Contains(line, ", _ :=") {
				// Check if it looks like an error being discarded
				if strings.Contains(strings.ToLower(line), "err") ||
					strings.Contains(line, "Close()") || strings.Contains(line, "Write(") ||
					strings.Contains(line, "Read(") || strings.Contains(line, "Flush(") {
					findings = append(findings, compliance.Finding{
						Severity:   "error",
						Article:    "Table A.3 IEC 61508-3",
						File:       file,
						StartLine:  lineNum,
						Message:    "Error return value explicitly discarded",
						Suggestion: "Handle all error returns; do not discard with _ in safety-critical code",
						Confidence: 0.85,
					})
				}
			}
		}
		f.Close()
	}

	return findings, nil
}

// --- complexity-exceeded: Complexity limits by SIL level ---

type complexityExceededCheck struct{}

func (c *complexityExceededCheck) ID() string       { return "complexity-exceeded" }
func (c *complexityExceededCheck) Name() string     { return "Complexity Limit Exceeded" }
func (c *complexityExceededCheck) Article() string   { return "Table B.9 IEC 61508-3" }
func (c *complexityExceededCheck) Severity() string  { return "error" }

// SIL level -> max cyclomatic complexity per function
var silComplexityLimits = map[int]int{
	1: 20,
	2: 15,
	3: 12,
	4: 10,
}

func (c *complexityExceededCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	silLevel := scope.Config.SILLevel
	if silLevel <= 0 {
		silLevel = 2
	}
	maxComplexity, ok := silComplexityLimits[silLevel]
	if !ok {
		maxComplexity = 15
	}

	// Use tree-sitter complexity analyzer if available
	if scope.ComplexityAnalyzer != nil {
		for _, file := range scope.Files {
			if ctx.Err() != nil {
				return findings, ctx.Err()
			}

			fullPath := filepath.Join(scope.RepoRoot, file)
			fc, err := scope.ComplexityAnalyzer.AnalyzeFile(ctx, fullPath)
			if err != nil || fc == nil || fc.Error != "" {
				continue
			}

			for _, fn := range fc.Functions {
				if fn.Cyclomatic > maxComplexity {
					findings = append(findings, compliance.Finding{
						Severity:   "error",
						Article:    "Table B.9 IEC 61508-3",
						File:       file,
						StartLine:  fn.StartLine,
						EndLine:    fn.EndLine,
						Message:    fmt.Sprintf("Function '%s' cyclomatic complexity %d exceeds SIL %d limit of %d", fn.Name, fn.Cyclomatic, silLevel, maxComplexity),
						Suggestion: fmt.Sprintf("Refactor to reduce complexity below %d for SIL %d compliance", maxComplexity, silLevel),
						Confidence: 0.95,
					})
				}
			}
		}
	}

	return findings, nil
}
