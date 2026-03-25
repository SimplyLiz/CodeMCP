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

// --- dynamic-allocation: Rule 21.3 — no dynamic memory allocation ---

type dynamicAllocationCheck struct{}

func (c *dynamicAllocationCheck) ID() string       { return "dynamic-allocation" }
func (c *dynamicAllocationCheck) Name() string     { return "Dynamic Memory Allocation" }
func (c *dynamicAllocationCheck) Article() string   { return "Rule 21.3 MISRA C" }
func (c *dynamicAllocationCheck) Severity() string  { return "warning" }

var dynamicAllocPattern = regexp.MustCompile(`\b(malloc|calloc|realloc|free|new\s+\w|delete\s+|delete\[)\b`)

func (c *dynamicAllocationCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}
		if !isMISRAFile(file) {
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

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
					continue
				}

				if m := dynamicAllocPattern.FindString(line); m != "" {
					findings = append(findings, compliance.Finding{
						CheckID:    "dynamic-allocation",
						Framework:  compliance.FrameworkMISRA,
						Severity:   "warning",
						Article:    "Rule 21.3 MISRA C",
						File:       file,
						StartLine:  lineNum,
						Message:    fmt.Sprintf("Dynamic memory allocation '%s' used in safety-critical code", strings.TrimSpace(m)),
						Suggestion: "Use statically allocated buffers or memory pools instead of dynamic allocation",
						Confidence: 0.90,
					})
				}
			}
		}()
	}

	return findings, nil
}

// --- unsafe-string-functions: Rule 21.14 — banned unsafe functions ---

type unsafeStringFunctionsCheck struct{}

func (c *unsafeStringFunctionsCheck) ID() string       { return "unsafe-string-functions" }
func (c *unsafeStringFunctionsCheck) Name() string     { return "Unsafe String Functions" }
func (c *unsafeStringFunctionsCheck) Article() string   { return "Rule 21.14 MISRA C" }
func (c *unsafeStringFunctionsCheck) Severity() string  { return "error" }

var unsafeFuncReplacements = map[string]string{
	"gets":    "fgets",
	"sprintf": "snprintf",
	"strcpy":  "strncpy",
	"strcat":  "strncat",
	"scanf":   "fscanf",
}

var unsafeFuncPattern = regexp.MustCompile(`\b(gets|sprintf|strcpy|strcat|scanf)\s*\(`)

func (c *unsafeStringFunctionsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}
		if !isMISRAFile(file) {
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

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
					continue
				}

				if m := unsafeFuncPattern.FindStringSubmatch(line); len(m) > 1 {
					funcName := m[1]
					replacement := unsafeFuncReplacements[funcName]
					findings = append(findings, compliance.Finding{
						CheckID:    "unsafe-string-functions",
						Framework:  compliance.FrameworkMISRA,
						Severity:   "error",
						Article:    "Rule 21.14 MISRA C",
						File:       file,
						StartLine:  lineNum,
						Message:    fmt.Sprintf("Banned unsafe function '%s' used", funcName),
						Suggestion: fmt.Sprintf("Replace '%s' with bounds-checked '%s'", funcName, replacement),
						Confidence: 0.95,
						CWE:        "CWE-676",
					})
				}
			}
		}()
	}

	return findings, nil
}
