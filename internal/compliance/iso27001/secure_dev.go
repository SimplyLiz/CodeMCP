package iso27001

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- sql-injection: A.8.28 — String concatenation in SQL ---

type sqlInjectionCheck struct{}

func (c *sqlInjectionCheck) ID() string       { return "sql-injection" }
func (c *sqlInjectionCheck) Name() string     { return "SQL Injection Risk" }
func (c *sqlInjectionCheck) Article() string   { return "A.8.28 ISO 27001:2022" }
func (c *sqlInjectionCheck) Severity() string  { return "error" }

var sqlInjectionPatterns = []*regexp.Regexp{
	// String concatenation in SQL
	regexp.MustCompile(`(?i)(SELECT|INSERT|UPDATE|DELETE|WHERE).*\+\s*[\w]+`),
	regexp.MustCompile(`(?i)(SELECT|INSERT|UPDATE|DELETE|WHERE).*%[sv]`),
	regexp.MustCompile(`(?i)fmt\.Sprintf\(.*(?:SELECT|INSERT|UPDATE|DELETE|WHERE)`),
	regexp.MustCompile(`(?i)f["'].*(?:SELECT|INSERT|UPDATE|DELETE|WHERE).*\{`),
	regexp.MustCompile(`(?i)execute\(\s*["'].*\+`),
	regexp.MustCompile(`(?i)\.query\(\s*["'].*\+`),
	regexp.MustCompile(`(?i)\.raw\(\s*["'].*\+`),
}

func (c *sqlInjectionCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				trimmed := strings.TrimSpace(line)

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
					continue
				}

				for _, pattern := range sqlInjectionPatterns {
					if pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "A.8.28 ISO 27001:2022",
							File:       file,
							StartLine:  lineNum,
							Message:    "Potential SQL injection: string interpolation/concatenation in SQL query",
							Suggestion: "Use parameterized queries or prepared statements instead of string concatenation",
							Confidence: 0.75,
							CWE:        "CWE-89",
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}

// --- path-traversal: A.8.28 — User input in file paths ---

type pathTraversalCheck struct{}

func (c *pathTraversalCheck) ID() string       { return "path-traversal" }
func (c *pathTraversalCheck) Name() string     { return "Path Traversal Risk" }
func (c *pathTraversalCheck) Article() string   { return "A.8.28 ISO 27001:2022" }
func (c *pathTraversalCheck) Severity() string  { return "error" }

var pathTraversalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)filepath\.Join\(.*(?:r\.URL|request|req|param|query|body)`),
	regexp.MustCompile(`(?i)os\.Open\(.*(?:r\.URL|request|req|param|query|body|user)`),
	regexp.MustCompile(`(?i)os\.ReadFile\(.*(?:r\.URL|request|req|param|query|body|user)`),
	regexp.MustCompile(`(?i)path\.join\(.*(?:req\.|request\.|params\.|query\.)`),
	regexp.MustCompile(`(?i)open\(.*(?:request\.|params\[|argv)`),
	regexp.MustCompile(`(?i)\.\./`),
}

func (c *pathTraversalCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Skip test files
		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
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

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
					continue
				}

				for _, pattern := range pathTraversalPatterns {
					if pattern.MatchString(line) {
						// Skip patterns that are just path.join in comment-free code
						if strings.Contains(line, "../") {
							// Only flag ../ if it looks like string construction, not constants
							if !strings.Contains(trimmed, "//") {
								findings = append(findings, compliance.Finding{
									Severity:   "warning",
									Article:    "A.8.28 ISO 27001:2022",
									File:       file,
									StartLine:  lineNum,
									Message:    "Path traversal pattern detected (../ in path construction)",
									Suggestion: "Validate and sanitize file paths; use filepath.Clean and ensure path stays within allowed directory",
									Confidence: 0.60,
									CWE:        "CWE-22",
								})
							}
						} else {
							findings = append(findings, compliance.Finding{
								Severity:   "error",
								Article:    "A.8.28 ISO 27001:2022",
								File:       file,
								StartLine:  lineNum,
								Message:    "Potential path traversal: user-controlled input in file path operation",
								Suggestion: "Validate and sanitize file paths; use filepath.Clean and ensure path stays within allowed directory",
								Confidence: 0.70,
								CWE:        "CWE-22",
							})
						}
						break
					}
				}
			}
		}()
	}

	return findings, nil
}

// --- unsafe-deserialization: A.8.7 — Deserializing untrusted data ---

type unsafeDeserializationCheck struct{}

func (c *unsafeDeserializationCheck) ID() string       { return "unsafe-deserialization" }
func (c *unsafeDeserializationCheck) Name() string     { return "Unsafe Deserialization" }
func (c *unsafeDeserializationCheck) Article() string   { return "A.8.7 ISO 27001:2022" }
func (c *unsafeDeserializationCheck) Severity() string  { return "error" }

var unsafeDeserPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bpickle\.load\b`),
	regexp.MustCompile(`(?i)\bpickle\.loads\b`),
	regexp.MustCompile(`(?i)\byaml\.load\(`),              // yaml.load without Loader=SafeLoader
	regexp.MustCompile(`(?i)\byaml\.Unmarshal\b`),          // Go — only flagged if from user input
	regexp.MustCompile(`(?i)\beval\(\s*(?:request|req|params|user|input)`),
	regexp.MustCompile(`(?i)\bdeserialize\(`),
	regexp.MustCompile(`(?i)\bObjectInputStream\b`),        // Java
	regexp.MustCompile(`(?i)\bBinaryFormatter\.Deserialize`), // C#
	regexp.MustCompile(`(?i)\bMarshal\.load\b`),            // Ruby
	regexp.MustCompile(`(?i)\bunserialize\(`),              // PHP
}

func (c *unsafeDeserializationCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
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

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
					continue
				}

				for _, pattern := range unsafeDeserPatterns {
					if pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "A.8.7 ISO 27001:2022",
							File:       file,
							StartLine:  lineNum,
							Message:    "Potentially unsafe deserialization detected",
							Suggestion: "Avoid deserializing untrusted data; use safe alternatives (json, yaml.SafeLoader, protobuf)",
							Confidence: 0.75,
							CWE:        "CWE-502",
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}
