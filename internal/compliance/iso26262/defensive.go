package iso26262

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

// --- missing-null-check: Part 6, 8.4.4 — defensive programming ---

type missingNullCheckCheck struct{}

func (c *missingNullCheckCheck) ID() string       { return "missing-null-check" }
func (c *missingNullCheckCheck) Name() string     { return "Missing Null Check Before Dereference" }
func (c *missingNullCheckCheck) Article() string  { return "Part 6, 8.4.4 ISO 26262" }
func (c *missingNullCheckCheck) Severity() string { return "warning" }

// Detect pointer dereferences: *ptr or ptr->member
var derefPattern = regexp.MustCompile(`(\*\w+[\.\[]|(\w+)->)`)

// Detect null checks: if (ptr != NULL), if (ptr), if (ptr != nullptr)
var nullCheckPattern = regexp.MustCompile(`if\s*\(\s*\w+\s*(!=\s*(NULL|nullptr|0)|==\s*(NULL|nullptr|0))\s*\)|if\s*\(\s*!?\w+\s*\)`)

func (c *missingNullCheckCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	cExts := map[string]bool{".c": true, ".cpp": true, ".h": true, ".hpp": true}

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		ext := strings.ToLower(filepath.Ext(file))
		if !cExts[ext] {
			continue
		}

		func() {
			f, err := os.Open(filepath.Join(scope.RepoRoot, file))
			if err != nil {
				return
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			lineNum := 0
			recentNullCheck := false

			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				trimmed := strings.TrimSpace(line)

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
					continue
				}

				// Track null checks — if we see one, subsequent dereferences are guarded
				if nullCheckPattern.MatchString(line) {
					recentNullCheck = true
					continue
				}

				// Reset null check tracking at closing braces (conservative)
				if trimmed == "}" {
					recentNullCheck = false
					continue
				}

				// Detect dereferences without recent null check
				if !recentNullCheck && derefPattern.MatchString(line) {
					// Skip declarations (type *name = ...)
					if strings.Contains(line, "=") && strings.Contains(line, "*") && !strings.Contains(line, "==") {
						// Likely a pointer declaration, not a dereference
						continue
					}

					findings = append(findings, compliance.Finding{
						CheckID:    "missing-null-check",
						Framework:  compliance.FrameworkISO26262,
						Severity:   "warning",
						Article:    "Part 6, 8.4.4 ISO 26262",
						File:       file,
						StartLine:  lineNum,
						Message:    "Pointer dereference without preceding null check",
						Suggestion: "Add null/nullptr check before dereferencing pointer for defensive programming",
						Confidence: 0.60,
					})
				}
			}
		}()
	}

	return findings, nil
}

// --- unchecked-return: Part 6, 8.4.4 — all return values must be checked ---

type uncheckedReturnCheck struct{}

func (c *uncheckedReturnCheck) ID() string       { return "unchecked-return" }
func (c *uncheckedReturnCheck) Name() string     { return "Unchecked Return Value" }
func (c *uncheckedReturnCheck) Article() string  { return "Part 6, 8.4.4 ISO 26262" }
func (c *uncheckedReturnCheck) Severity() string { return "error" }

func (c *uncheckedReturnCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if !strings.HasSuffix(file, ".go") {
			continue
		}
		if strings.Contains(file, "_test.go") {
			continue
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
				trimmed := strings.TrimSpace(line)

				if strings.HasPrefix(trimmed, "//") {
					continue
				}

				// Detect error explicitly discarded with _
				if strings.Contains(line, ", _ =") || strings.Contains(line, ", _ :=") {
					if strings.Contains(strings.ToLower(line), "err") ||
						strings.Contains(line, "Close()") || strings.Contains(line, "Write(") ||
						strings.Contains(line, "Read(") || strings.Contains(line, "Flush(") {
						findings = append(findings, compliance.Finding{
							CheckID:    "unchecked-return",
							Framework:  compliance.FrameworkISO26262,
							Severity:   "error",
							Article:    "Part 6, 8.4.4 ISO 26262",
							File:       file,
							StartLine:  lineNum,
							Message:    fmt.Sprintf("Error return value explicitly discarded at line %d", lineNum),
							Suggestion: "Handle all error returns; do not discard with _ in automotive safety-critical code",
							Confidence: 0.85,
						})
					}
				}
			}
		}()
	}

	return findings, nil
}
