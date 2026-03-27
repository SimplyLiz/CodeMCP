package do178c

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

// --- dead-code: §6.4.4.2 — dead code is prohibited ---

type deadCodeCheck struct{}

func (c *deadCodeCheck) ID() string       { return "dead-code" }
func (c *deadCodeCheck) Name() string     { return "Dead Code Detection" }
func (c *deadCodeCheck) Article() string  { return "§6.4.4.2 DO-178C" }
func (c *deadCodeCheck) Severity() string { return "error" }

var terminatorPattern = regexp.MustCompile(`^\s*(return\b|break\s*;|continue\s*;|goto\s+\w+)`)
var commentedCodePattern = regexp.MustCompile(`^\s*//\s*(if|for|while|switch|return|int|char|void|func|def|class)\b`)

func (c *deadCodeCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Skip test files
		if strings.Contains(file, "_test.go") || strings.Contains(file, "_test.") || strings.Contains(file, "test_") {
			continue
		}

		// Skip Go files — Go's unreachable-code detection is handled by the
		// bug-patterns check (tree-sitter AST-based, higher accuracy). The
		// commented-code heuristic produces excessive FPs in Go (commented
		// examples, build tag alternatives, documentation snippets).
		if strings.HasSuffix(file, ".go") {
			continue
		}

		fullPath := filepath.Join(scope.RepoRoot, file)

		// Check 1: Unreachable code after return/break/continue/goto
		unreachable := detectUnreachableCode(fullPath, file)
		findings = append(findings, unreachable...)

		// Check 2: Commented-out code blocks
		commented := detectCommentedCode(fullPath, file)
		findings = append(findings, commented...)
	}

	return findings, nil
}

func detectUnreachableCode(fullPath, relPath string) []compliance.Finding {
	var findings []compliance.Finding

	f, err := os.Open(fullPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	afterTerminator := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip empty lines, comments, braces
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") ||
			strings.HasPrefix(trimmed, "*") || trimmed == "}" || trimmed == "{" {
			if trimmed == "}" {
				afterTerminator = false
			}
			continue
		}

		if afterTerminator {
			// Don't flag labels or case/default
			if !strings.HasSuffix(trimmed, ":") || strings.HasPrefix(trimmed, "case ") || trimmed == "default:" {
				if !strings.HasPrefix(trimmed, "case ") && trimmed != "default:" {
					findings = append(findings, compliance.Finding{
						CheckID:    "dead-code",
						Framework:  compliance.FrameworkDO178C,
						Severity:   "error",
						Article:    "§6.4.4.2 DO-178C",
						File:       relPath,
						StartLine:  lineNum,
						Message:    fmt.Sprintf("Unreachable code after control flow terminator: %s", trimmed),
						Suggestion: "Remove dead code; DO-178C explicitly prohibits unreachable code in avionics software",
						Confidence: 0.70,
					})
				}
			}
		}

		if terminatorPattern.MatchString(line) {
			afterTerminator = true
		} else {
			afterTerminator = false
		}
	}

	return findings
}

func detectCommentedCode(fullPath, relPath string) []compliance.Finding {
	var findings []compliance.Finding

	f, err := os.Open(fullPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	consecutiveCommentedCode := 0
	commentBlockStart := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if commentedCodePattern.MatchString(line) {
			if consecutiveCommentedCode == 0 {
				commentBlockStart = lineNum
			}
			consecutiveCommentedCode++
		} else {
			if consecutiveCommentedCode >= 3 {
				findings = append(findings, compliance.Finding{
					CheckID:    "dead-code",
					Framework:  compliance.FrameworkDO178C,
					Severity:   "error",
					Article:    "§6.4.4.2 DO-178C",
					File:       relPath,
					StartLine:  commentBlockStart,
					EndLine:    lineNum - 1,
					Message:    fmt.Sprintf("Commented-out code block (%d lines)", consecutiveCommentedCode),
					Suggestion: "Remove commented-out code; use version control to track previous implementations",
					Confidence: 0.70,
				})
			}
			consecutiveCommentedCode = 0
		}
	}

	// Handle file ending with commented code
	if consecutiveCommentedCode >= 3 {
		findings = append(findings, compliance.Finding{
			CheckID:    "dead-code",
			Framework:  compliance.FrameworkDO178C,
			Severity:   "error",
			Article:    "§6.4.4.2 DO-178C",
			File:       relPath,
			StartLine:  commentBlockStart,
			EndLine:    lineNum,
			Message:    fmt.Sprintf("Commented-out code block (%d lines)", consecutiveCommentedCode),
			Suggestion: "Remove commented-out code; use version control to track previous implementations",
			Confidence: 0.70,
		})
	}

	return findings
}
