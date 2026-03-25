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

// --- goto-usage: Structured programming — no goto ---

type gotoUsageCheck struct{}

func (c *gotoUsageCheck) ID() string       { return "goto-usage" }
func (c *gotoUsageCheck) Name() string     { return "Goto Statement Usage" }
func (c *gotoUsageCheck) Article() string   { return "Table B.1 IEC 61508-3" }
func (c *gotoUsageCheck) Severity() string  { return "warning" }

var gotoPattern = regexp.MustCompile(`(?m)^\s*goto\s+\w+`)

func (c *gotoUsageCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding
	silLevel := scope.Config.SILLevel
	if silLevel <= 0 {
		silLevel = 2
	}

	severity := "warning"
	if silLevel >= 3 {
		severity = "error"
	}

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		func() {
			f, err := os.Open(filepath.Join(scope.RepoRoot, file))
			if err != nil {
				return
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			lineNum := 0
			for scanner.Scan() {
				lineNum++
				line := scanner.Text()

				if gotoPattern.MatchString(line) {
					findings = append(findings, compliance.Finding{
						Severity:   severity,
						Article:    "Table B.1 IEC 61508-3",
						File:       file,
						StartLine:  lineNum,
						Message:    fmt.Sprintf("goto statement violates structured programming requirement (SIL %d)", silLevel),
						Suggestion: "Refactor to use loops, conditionals, or early returns instead of goto",
						Confidence: 0.95,
					})
				}
			}
		}()
	}

	return findings, nil
}

// --- recursion: No recursive function calls ---

type recursionCheck struct{}

func (c *recursionCheck) ID() string       { return "recursion" }
func (c *recursionCheck) Name() string     { return "Recursive Function Calls" }
func (c *recursionCheck) Article() string   { return "Table B.9 IEC 61508-3" }
func (c *recursionCheck) Severity() string  { return "warning" }

func (c *recursionCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	// Simple heuristic: find function definitions and check if the function name appears in its body
	funcDefPattern := regexp.MustCompile(`(?:func|def|function)\s+(\w+)`)

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		var currentFunc string
		var funcStartLine int
		braceDepth := 0

		for i, line := range lines {
			lineNum := i + 1

			if m := funcDefPattern.FindStringSubmatch(line); len(m) > 1 {
				currentFunc = m[1]
				funcStartLine = lineNum
				braceDepth = 0
			}

			braceDepth += strings.Count(line, "{") - strings.Count(line, "}")

			// Inside a function, check for self-call
			if currentFunc != "" && lineNum > funcStartLine {
				// Look for function calling itself
				callPattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(currentFunc) + `\s*\(`)
				if callPattern.MatchString(line) {
					trimmed := strings.TrimSpace(line)
					if !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "#") {
						findings = append(findings, compliance.Finding{
							Severity:   "warning",
							Article:    "Table B.9 IEC 61508-3",
							File:       file,
							StartLine:  lineNum,
							Message:    fmt.Sprintf("Recursive call detected in function '%s'", currentFunc),
							Suggestion: "Replace recursion with iterative approach for safety-critical code",
							Confidence: 0.80,
						})
					}
				}
			}

			// Reset on function end
			if currentFunc != "" && braceDepth <= 0 && lineNum > funcStartLine {
				currentFunc = ""
			}
		}
	}

	return findings, nil
}

// --- deep-nesting: Structured programming — max nesting depth ---

type deepNestingCheck struct{}

func (c *deepNestingCheck) ID() string       { return "deep-nesting" }
func (c *deepNestingCheck) Name() string     { return "Deep Nesting" }
func (c *deepNestingCheck) Article() string   { return "Table B.1 IEC 61508-3" }
func (c *deepNestingCheck) Severity() string  { return "warning" }

func (c *deepNestingCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding
	maxDepth := 4

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		func() {
			f, err := os.Open(filepath.Join(scope.RepoRoot, file))
			if err != nil {
				return
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			lineNum := 0
			depth := 0

			for scanner.Scan() {
				lineNum++
				line := scanner.Text()

				depth += strings.Count(line, "{") - strings.Count(line, "}")

				if depth > maxDepth {
					findings = append(findings, compliance.Finding{
						Severity:   "warning",
						Article:    "Table B.1 IEC 61508-3",
						File:       file,
						StartLine:  lineNum,
						Message:    fmt.Sprintf("Nesting depth %d exceeds limit of %d", depth, maxDepth),
						Suggestion: "Reduce nesting by extracting functions, using early returns, or guard clauses",
						Confidence: 0.85,
					})
				}
			}
		}()
	}

	return findings, nil
}

// --- large-function: Modular approach — function size limit ---

type largeFunctionCheck struct{}

func (c *largeFunctionCheck) ID() string       { return "large-function" }
func (c *largeFunctionCheck) Name() string     { return "Large Function" }
func (c *largeFunctionCheck) Article() string   { return "Table B.9 IEC 61508-3" }
func (c *largeFunctionCheck) Severity() string  { return "warning" }

func (c *largeFunctionCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding
	maxLines := 75

	funcDefPattern := regexp.MustCompile(`(?:func|def|function)\s+(\w+)`)

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		var currentFunc string
		var funcStart int
		braceDepth := 0

		for i, line := range lines {
			if m := funcDefPattern.FindStringSubmatch(line); len(m) > 1 {
				// Check if previous function was too large
				if currentFunc != "" && (i-funcStart) > maxLines {
					findings = append(findings, compliance.Finding{
						Severity:   "warning",
						Article:    "Table B.9 IEC 61508-3",
						File:       file,
						StartLine:  funcStart + 1,
						Message:    fmt.Sprintf("Function '%s' has %d lines (limit: %d)", currentFunc, i-funcStart, maxLines),
						Suggestion: "Break large functions into smaller, focused sub-functions",
						Confidence: 0.90,
					})
				}
				currentFunc = m[1]
				funcStart = i
				braceDepth = 0
			}

			braceDepth += strings.Count(line, "{") - strings.Count(line, "}")

			if currentFunc != "" && braceDepth <= 0 && i > funcStart {
				if (i - funcStart) > maxLines {
					findings = append(findings, compliance.Finding{
						Severity:   "warning",
						Article:    "Table B.9 IEC 61508-3",
						File:       file,
						StartLine:  funcStart + 1,
						Message:    fmt.Sprintf("Function '%s' has %d lines (limit: %d)", currentFunc, i-funcStart, maxLines),
						Suggestion: "Break large functions into smaller, focused sub-functions",
						Confidence: 0.90,
					})
				}
				currentFunc = ""
			}
		}
	}

	return findings, nil
}

// --- global-state: Modular approach — global mutable state ---

type globalStateCheck struct{}

func (c *globalStateCheck) ID() string       { return "global-state" }
func (c *globalStateCheck) Name() string     { return "Global Mutable State" }
func (c *globalStateCheck) Article() string   { return "Table B.9 IEC 61508-3" }
func (c *globalStateCheck) Severity() string  { return "warning" }

var globalMutablePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^var\s+\w+\s+(?:=|[^(])`),       // Go: var x = ... (not var block)
	regexp.MustCompile(`^let\s+\w+\s*=`),                  // JS: let x = (global scope)
	regexp.MustCompile(`^(?:static\s+)?(?:mut\s+)?static\s`), // Rust: static mut
}

func (c *globalStateCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		func() {
			f, err := os.Open(filepath.Join(scope.RepoRoot, file))
			if err != nil {
				return
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			lineNum := 0
			braceDepth := 0

			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				trimmed := strings.TrimSpace(line)

				braceDepth += strings.Count(line, "{") - strings.Count(line, "}")

				// Only check top-level declarations (braceDepth 0 or 1 for Go package level)
				if braceDepth > 1 {
					continue
				}

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
					continue
				}

				for _, pattern := range globalMutablePatterns {
					if pattern.MatchString(trimmed) {
						// Skip constants and immutable declarations
						if strings.Contains(trimmed, "const") || strings.Contains(trimmed, "sync.") ||
							strings.Contains(trimmed, "Mutex") {
							continue
						}
						findings = append(findings, compliance.Finding{
							Severity:   "warning",
							Article:    "Table B.9 IEC 61508-3",
							File:       file,
							StartLine:  lineNum,
							Message:    "Global mutable state detected",
							Suggestion: "Avoid global mutable state in safety-critical code; use dependency injection or pass state explicitly",
							Confidence: 0.65,
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}
