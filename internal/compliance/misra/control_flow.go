package misra

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

var misraExts = map[string]bool{
	".c": true, ".cpp": true, ".h": true, ".hpp": true,
}

func isMISRAFile(file string) bool {
	ext := strings.ToLower(filepath.Ext(file))
	return misraExts[ext]
}

// --- goto-usage: Rule 15.1 — goto shall not be used ---

type gotoUsageCheck struct{}

func (c *gotoUsageCheck) ID() string       { return "goto-usage" }
func (c *gotoUsageCheck) Name() string     { return "Goto Statement Usage" }
func (c *gotoUsageCheck) Article() string   { return "Rule 15.1 MISRA C" }
func (c *gotoUsageCheck) Severity() string  { return "error" }

var misraGotoPattern = regexp.MustCompile(`(?m)^\s*goto\s+\w+`)

func (c *gotoUsageCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}
		if !isMISRAFile(file) {
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

			if misraGotoPattern.MatchString(line) {
				findings = append(findings, compliance.Finding{
					CheckID:    "goto-usage",
					Framework:  compliance.FrameworkMISRA,
					Severity:   "error",
					Article:    "Rule 15.1 MISRA C",
					File:       file,
					StartLine:  lineNum,
					Message:    "goto statement violates MISRA C Rule 15.1",
					Suggestion: "Refactor to use structured control flow (loops, conditionals, early returns)",
					Confidence: 0.95,
				})
			}
		}
		f.Close()
	}

	return findings, nil
}

// --- unreachable-code: Rule 2.1 — code shall not be unreachable ---

type unreachableCodeCheck struct{}

func (c *unreachableCodeCheck) ID() string       { return "unreachable-code" }
func (c *unreachableCodeCheck) Name() string     { return "Unreachable Code" }
func (c *unreachableCodeCheck) Article() string   { return "Rule 2.1 MISRA C" }
func (c *unreachableCodeCheck) Severity() string  { return "warning" }

var terminatorPattern = regexp.MustCompile(`^\s*(return\b|break\s*;|continue\s*;|goto\s+\w+)`)

func (c *unreachableCodeCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}
		if !isMISRAFile(file) {
			continue
		}

		f, err := os.Open(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		lineNum := 0
		afterTerminator := false

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			// Skip empty lines, comments, and closing braces
			if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") ||
				strings.HasPrefix(trimmed, "*") || trimmed == "}" || trimmed == "{" {
				if trimmed == "}" {
					afterTerminator = false
				}
				continue
			}

			// If previous non-blank line was a terminator, this code is unreachable
			if afterTerminator {
				// Don't flag labels (used by goto/switch)
				if !strings.HasSuffix(trimmed, ":") || strings.HasPrefix(trimmed, "case ") || trimmed == "default:" {
					if !strings.HasPrefix(trimmed, "case ") && trimmed != "default:" {
						findings = append(findings, compliance.Finding{
							CheckID:    "unreachable-code",
							Framework:  compliance.FrameworkMISRA,
							Severity:   "warning",
							Article:    "Rule 2.1 MISRA C",
							File:       file,
							StartLine:  lineNum,
							Message:    fmt.Sprintf("Code after control flow terminator is unreachable: %s", trimmed),
							Suggestion: "Remove unreachable code or restructure control flow",
							Confidence: 0.75,
						})
					}
				}
				afterTerminator = false
			}

			if terminatorPattern.MatchString(line) {
				afterTerminator = true
			} else {
				afterTerminator = false
			}
		}
		f.Close()
	}

	return findings, nil
}

// --- missing-switch-default: Rule 16.4 — every switch shall have a default ---

type missingSwitchDefaultCheck struct{}

func (c *missingSwitchDefaultCheck) ID() string       { return "missing-switch-default" }
func (c *missingSwitchDefaultCheck) Name() string     { return "Missing Switch Default Case" }
func (c *missingSwitchDefaultCheck) Article() string   { return "Rule 16.4 MISRA C" }
func (c *missingSwitchDefaultCheck) Severity() string  { return "warning" }

var switchPattern = regexp.MustCompile(`\bswitch\s*\(`)

func (c *missingSwitchDefaultCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}
		if !isMISRAFile(file) {
			continue
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")

		for i, line := range lines {
			if !switchPattern.MatchString(line) {
				continue
			}

			switchLine := i + 1
			braceDepth := 0
			foundOpen := false
			hasDefault := false

			// Scan from switch to its closing brace
			for j := i; j < len(lines); j++ {
				braceDepth += strings.Count(lines[j], "{") - strings.Count(lines[j], "}")
				if strings.Contains(lines[j], "{") {
					foundOpen = true
				}
				if strings.Contains(strings.TrimSpace(lines[j]), "default:") || strings.Contains(strings.TrimSpace(lines[j]), "default :") {
					hasDefault = true
				}
				if foundOpen && braceDepth <= 0 {
					break
				}
			}

			if foundOpen && !hasDefault {
				findings = append(findings, compliance.Finding{
					CheckID:    "missing-switch-default",
					Framework:  compliance.FrameworkMISRA,
					Severity:   "warning",
					Article:    "Rule 16.4 MISRA C",
					File:       file,
					StartLine:  switchLine,
					Message:    "switch statement without default case",
					Suggestion: "Add a default case to handle unexpected values",
					Confidence: 0.80,
				})
			}
		}
	}

	return findings, nil
}
