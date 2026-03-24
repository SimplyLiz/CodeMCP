package iec62443

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

// --- unsafe-functions: SD-4 — banned functions ---

type unsafeFunctionsCheck struct{}

func (c *unsafeFunctionsCheck) ID() string       { return "unsafe-functions" }
func (c *unsafeFunctionsCheck) Name() string     { return "Unsafe/Banned Functions" }
func (c *unsafeFunctionsCheck) Article() string   { return "SD-4 IEC 62443-4-1" }
func (c *unsafeFunctionsCheck) Severity() string  { return "error" }

var bannedFuncPattern = regexp.MustCompile(`\b(gets|sprintf|strcpy|strcat|scanf|system|popen|exec)\s*\(`)

func (c *unsafeFunctionsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	cExts := map[string]bool{".c": true, ".cpp": true, ".h": true, ".hpp": true}

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Skip test files
		if strings.Contains(file, "_test.") || strings.Contains(file, "test_") || strings.Contains(file, "test/") {
			continue
		}

		ext := strings.ToLower(filepath.Ext(file))
		if !cExts[ext] {
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

			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
				continue
			}

			if m := bannedFuncPattern.FindStringSubmatch(line); len(m) > 1 {
				funcName := m[1]
				findings = append(findings, compliance.Finding{
					CheckID:    "unsafe-functions",
					Framework:  compliance.FrameworkIEC62443,
					Severity:   "error",
					Article:    "SD-4 IEC 62443-4-1",
					File:       file,
					StartLine:  lineNum,
					Message:    fmt.Sprintf("Banned unsafe function '%s' used in industrial control system code", funcName),
					Suggestion: fmt.Sprintf("Replace '%s' with a safe alternative per IEC 62443 secure development requirements", funcName),
					Confidence: 0.95,
					CWE:        "CWE-676",
				})
			}
		}
		f.Close()
	}

	return findings, nil
}

// --- missing-error-handling: SD-4 — error returns must be handled ---

type missingErrorHandlingCheck struct{}

func (c *missingErrorHandlingCheck) ID() string       { return "missing-error-handling" }
func (c *missingErrorHandlingCheck) Name() string     { return "Missing Error Handling" }
func (c *missingErrorHandlingCheck) Article() string   { return "SD-4 IEC 62443-4-1" }
func (c *missingErrorHandlingCheck) Severity() string  { return "warning" }

func (c *missingErrorHandlingCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Check Go files for discarded errors
		if strings.HasSuffix(file, ".go") {
			if strings.Contains(file, "_test.go") {
				continue
			}
			goFindings := checkGoErrorHandling(scope.RepoRoot, file)
			findings = append(findings, goFindings...)
			continue
		}

		// Check C/C++ files for unchecked return values
		ext := strings.ToLower(filepath.Ext(file))
		cExts := map[string]bool{".c": true, ".cpp": true}
		if !cExts[ext] {
			continue
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, "test_") {
			continue
		}

		cFindings := checkCErrorHandling(scope.RepoRoot, file)
		findings = append(findings, cFindings...)
	}

	return findings, nil
}

func checkGoErrorHandling(repoRoot, file string) []compliance.Finding {
	var findings []compliance.Finding

	f, err := os.Open(filepath.Join(repoRoot, file))
	if err != nil {
		return nil
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

		if strings.Contains(line, ", _ =") || strings.Contains(line, ", _ :=") {
			if strings.Contains(strings.ToLower(line), "err") ||
				strings.Contains(line, "Close()") || strings.Contains(line, "Write(") ||
				strings.Contains(line, "Read(") || strings.Contains(line, "Flush(") {
				findings = append(findings, compliance.Finding{
					CheckID:    "missing-error-handling",
					Framework:  compliance.FrameworkIEC62443,
					Severity:   "warning",
					Article:    "SD-4 IEC 62443-4-1",
					File:       file,
					StartLine:  lineNum,
					Message:    "Error return value explicitly discarded",
					Suggestion: "Handle all error returns in industrial control system code; do not discard with _",
					Confidence: 0.85,
				})
			}
		}
	}

	return findings
}

// checkCErrorHandling detects common patterns of ignored return values in C code
var cReturnIgnorePattern = regexp.MustCompile(`^\s+(fopen|fclose|fread|fwrite|fseek|fprintf|fgets|read|write|close|send|recv|connect|bind|listen|accept)\s*\(`)

func checkCErrorHandling(repoRoot, file string) []compliance.Finding {
	var findings []compliance.Finding

	f, err := os.Open(filepath.Join(repoRoot, file))
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		// Detect function calls at statement level (not assigned to variable)
		if cReturnIgnorePattern.MatchString(line) {
			// Check if the return value is being captured
			if !strings.Contains(line, "=") && !strings.Contains(line, "if") {
				findings = append(findings, compliance.Finding{
					CheckID:    "missing-error-handling",
					Framework:  compliance.FrameworkIEC62443,
					Severity:   "warning",
					Article:    "SD-4 IEC 62443-4-1",
					File:       file,
					StartLine:  lineNum,
					Message:    fmt.Sprintf("Return value of system call ignored at line %d", lineNum),
					Suggestion: "Check return values of all system and I/O calls in industrial control system code",
					Confidence: 0.70,
				})
			}
		}
	}

	return findings
}
