package iso26262

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// ASIL level labels for messages
var asilLabels = map[int]string{
	1: "ASIL A", 2: "ASIL B", 3: "ASIL C", 4: "ASIL D",
}

func asilLabel(level int) string {
	if l, ok := asilLabels[level]; ok {
		return l
	}
	return fmt.Sprintf("ASIL %d", level)
}

// --- complexity-exceeded: Part 6, Table 3 — cyclomatic complexity limits ---

type complexityExceededCheck struct{}

func (c *complexityExceededCheck) ID() string       { return "complexity-exceeded" }
func (c *complexityExceededCheck) Name() string     { return "Complexity Limit Exceeded" }
func (c *complexityExceededCheck) Article() string   { return "Part 6, Table 3 ISO 26262" }
func (c *complexityExceededCheck) Severity() string  { return "error" }

// ASIL level -> max cyclomatic complexity per function
var asilComplexityLimits = map[int]int{
	1: 25, // ASIL A
	2: 20, // ASIL B
	3: 15, // ASIL C
	4: 10, // ASIL D
}

func (c *complexityExceededCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	asilLevel := scope.Config.SILLevel
	if asilLevel <= 0 {
		asilLevel = 2
	}
	maxComplexity, ok := asilComplexityLimits[asilLevel]
	if !ok {
		maxComplexity = 20
	}

	if scope.ComplexityAnalyzer == nil {
		return findings, nil
	}

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		fullPath := filepath.Join(scope.RepoRoot, file)
		fc, err := scope.AnalyzeFileComplexity(ctx, fullPath)
		if err != nil || fc == nil || fc.Error != "" {
			continue
		}

		for _, fn := range fc.Functions {
			if fn.Cyclomatic > maxComplexity {
				findings = append(findings, compliance.Finding{
					CheckID:    "complexity-exceeded",
					Framework:  compliance.FrameworkISO26262,
					Severity:   "error",
					Article:    "Part 6, Table 3 ISO 26262",
					File:       file,
					StartLine:  fn.StartLine,
					EndLine:    fn.EndLine,
					Message:    fmt.Sprintf("Function '%s' cyclomatic complexity %d exceeds %s limit of %d", fn.Name, fn.Cyclomatic, asilLabel(asilLevel), maxComplexity),
					Suggestion: fmt.Sprintf("Refactor to reduce complexity below %d for %s compliance", maxComplexity, asilLabel(asilLevel)),
					Confidence: 0.95,
				})
			}
		}
	}

	return findings, nil
}

// --- recursion: Part 6, Table 3 — no recursive function calls ---

type recursionCheck struct{}

func (c *recursionCheck) ID() string       { return "recursion" }
func (c *recursionCheck) Name() string     { return "Recursive Function Calls" }
func (c *recursionCheck) Article() string   { return "Part 6, Table 3 ISO 26262" }
func (c *recursionCheck) Severity() string  { return "warning" }

func (c *recursionCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	asilLevel := scope.Config.SILLevel
	if asilLevel <= 0 {
		asilLevel = 2
	}

	severity := "warning"
	if asilLevel >= 3 {
		severity = "error"
	}

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

			if currentFunc != "" && lineNum > funcStartLine {
				callPattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(currentFunc) + `\s*\(`)
				if callPattern.MatchString(line) {
					trimmed := strings.TrimSpace(line)
					if !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "#") {
						findings = append(findings, compliance.Finding{
							CheckID:    "recursion",
							Framework:  compliance.FrameworkISO26262,
							Severity:   severity,
							Article:    "Part 6, Table 3 ISO 26262",
							File:       file,
							StartLine:  lineNum,
							Message:    fmt.Sprintf("Recursive call detected in function '%s' (%s)", currentFunc, asilLabel(asilLevel)),
							Suggestion: "Replace recursion with iterative approach for automotive safety-critical code",
							Confidence: 0.80,
						})
					}
				}
			}

			if currentFunc != "" && braceDepth <= 0 && lineNum > funcStartLine {
				currentFunc = ""
			}
		}
	}

	return findings, nil
}

// --- dynamic-memory: Part 6, Table 3 — no dynamic memory allocation ---

type dynamicMemoryCheck struct{}

func (c *dynamicMemoryCheck) ID() string       { return "dynamic-memory" }
func (c *dynamicMemoryCheck) Name() string     { return "Dynamic Memory Allocation" }
func (c *dynamicMemoryCheck) Article() string   { return "Part 6, Table 3 ISO 26262" }
func (c *dynamicMemoryCheck) Severity() string  { return "warning" }

var dynamicMemPattern = regexp.MustCompile(`\b(malloc|calloc|realloc|new\s+\w|make\s*\()\b`)

func (c *dynamicMemoryCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	asilLevel := scope.Config.SILLevel
	if asilLevel <= 0 {
		asilLevel = 2
	}

	severity := "warning"
	if asilLevel >= 3 {
		severity = "error"
	}

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Skip test files
		if strings.Contains(file, "_test.go") || strings.Contains(file, "_test.") || strings.Contains(file, "test_") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
				continue
			}

			if m := dynamicMemPattern.FindString(line); m != "" {
				findings = append(findings, compliance.Finding{
					CheckID:    "dynamic-memory",
					Framework:  compliance.FrameworkISO26262,
					Severity:   severity,
					Article:    "Part 6, Table 3 ISO 26262",
					File:       file,
					StartLine:  i + 1,
					Message:    fmt.Sprintf("Dynamic memory allocation '%s' prohibited at %s", strings.TrimSpace(m), asilLabel(asilLevel)),
					Suggestion: "Use statically allocated memory or pre-allocated pools for automotive safety-critical code",
					Confidence: 0.90,
				})
			}
		}
	}

	return findings, nil
}
