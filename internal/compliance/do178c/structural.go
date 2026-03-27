package do178c

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// DAL level labels
var dalLabels = map[int]string{
	4: "DAL A", 3: "DAL B", 2: "DAL C", 1: "DAL D",
}

func dalLabel(silLevel int) string {
	if l, ok := dalLabels[silLevel]; ok {
		return l
	}
	return fmt.Sprintf("DAL (SIL %d)", silLevel)
}

// --- complexity-exceeded: §6.3.4 — cyclomatic complexity limits ---

type complexityExceededCheck struct{}

func (c *complexityExceededCheck) ID() string       { return "complexity-exceeded" }
func (c *complexityExceededCheck) Name() string     { return "Complexity Limit Exceeded" }
func (c *complexityExceededCheck) Article() string  { return "§6.3.4 DO-178C" }
func (c *complexityExceededCheck) Severity() string { return "error" }

// SILLevel mapping: 4=DAL A, 3=DAL B, 2=DAL C, 1=DAL D
var dalComplexityLimits = map[int]int{
	4: 10, // DAL A (catastrophic)
	3: 15, // DAL B
	2: 20, // DAL C
	1: 30, // DAL D
}

func (c *complexityExceededCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	silLevel := scope.Config.SILLevel
	if silLevel <= 0 {
		silLevel = 2
	}
	maxComplexity, ok := dalComplexityLimits[silLevel]
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
					Framework:  compliance.FrameworkDO178C,
					Severity:   "error",
					Article:    "§6.3.4 DO-178C",
					File:       file,
					StartLine:  fn.StartLine,
					EndLine:    fn.EndLine,
					Message:    fmt.Sprintf("Function '%s' cyclomatic complexity %d exceeds %s limit of %d", fn.Name, fn.Cyclomatic, dalLabel(silLevel), maxComplexity),
					Suggestion: fmt.Sprintf("Refactor to reduce complexity below %d for %s compliance", maxComplexity, dalLabel(silLevel)),
					Confidence: 0.95,
				})
			}
		}
	}

	return findings, nil
}

// --- goto-usage: §6.3.4 — goto prohibited at all DAL levels ---

type gotoUsageCheck struct{}

func (c *gotoUsageCheck) ID() string       { return "goto-usage" }
func (c *gotoUsageCheck) Name() string     { return "Goto Statement Usage" }
func (c *gotoUsageCheck) Article() string  { return "§6.3.4 DO-178C" }
func (c *gotoUsageCheck) Severity() string { return "error" }

var gotoPattern = regexp.MustCompile(`(?m)^\s*goto\s+\w+`)

func (c *gotoUsageCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if gotoPattern.MatchString(line) {
				findings = append(findings, compliance.Finding{
					CheckID:    "goto-usage",
					Framework:  compliance.FrameworkDO178C,
					Severity:   "error",
					Article:    "§6.3.4 DO-178C",
					File:       file,
					StartLine:  i + 1,
					Message:    "goto statement prohibited in avionics code at all DAL levels",
					Suggestion: "Refactor to use structured control flow (loops, conditionals, early returns)",
					Confidence: 0.95,
				})
			}
		}
	}

	return findings, nil
}

// --- recursion: §6.3.4 — recursion detection ---

type recursionCheck struct{}

func (c *recursionCheck) ID() string       { return "recursion" }
func (c *recursionCheck) Name() string     { return "Recursive Function Calls" }
func (c *recursionCheck) Article() string  { return "§6.3.4 DO-178C" }
func (c *recursionCheck) Severity() string { return "error" }

func (c *recursionCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	silLevel := scope.Config.SILLevel
	if silLevel <= 0 {
		silLevel = 2
	}

	severity := "warning"
	if silLevel >= 3 {
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
							Framework:  compliance.FrameworkDO178C,
							Severity:   severity,
							Article:    "§6.3.4 DO-178C",
							File:       file,
							StartLine:  lineNum,
							Message:    fmt.Sprintf("Recursive call detected in function '%s' (%s)", currentFunc, dalLabel(silLevel)),
							Suggestion: "Replace recursion with iterative approach for avionics safety-critical code",
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
