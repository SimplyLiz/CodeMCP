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
	// Require SQL DML keywords in plausible query context:
	// SELECT ... FROM, INSERT INTO, UPDATE ... SET, DELETE FROM
	regexp.MustCompile(`(?i)["'].*SELECT\s+.+FROM\s.*["'].*\+\s*\w`),
	regexp.MustCompile(`(?i)["'].*SELECT\s+.+FROM\s.*%[sv]`),
	regexp.MustCompile(`(?i)["'].*(?:INSERT\s+INTO|UPDATE\s+\w+\s+SET|DELETE\s+FROM)\s.*%[sv]`),
	regexp.MustCompile(`(?i)fmt\.Sprintf\(\s*["'].*SELECT\s+.+FROM\s.*%[sv]`),
	regexp.MustCompile(`(?i)fmt\.Sprintf\(\s*["'].*(?:INSERT\s+INTO|UPDATE\s+\w+\s+SET|DELETE\s+FROM)\s.*%[sv]`),
	regexp.MustCompile(`(?i)f["'].*(?:SELECT\s+.+FROM|INSERT\s+INTO|UPDATE\s+\w+\s+SET|DELETE\s+FROM)\s.*\{`),
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

		// Skip test files — test fixtures naturally contain SQL-like strings
		if strings.HasSuffix(file, "_test.go") || strings.HasSuffix(file, "_test.py") ||
			strings.HasSuffix(file, ".test.ts") || strings.HasSuffix(file, ".test.js") ||
			strings.Contains(file, "testdata/") || strings.Contains(file, "fixtures") ||
			strings.Contains(file, "testutil/") {
			continue
		}

		func() {
			f, err := os.Open(filepath.Join(scope.RepoRoot, file))
			if err != nil {
				return
			}
			defer f.Close()

			// Read all lines so we can check context around flagged lines
			var lines []string
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				lines = append(lines, scanner.Text())
			}

			for lineIdx, line := range lines {
				lineNum := lineIdx + 1
				trimmed := strings.TrimSpace(line)

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
					continue
				}

				// Skip regex/pattern definitions
				if strings.Contains(line, "regexp.MustCompile") || strings.Contains(line, "Compile(") {
					continue
				}

				// Skip lines marked safe by other linters (check current + previous line)
				if strings.Contains(line, "#nosec") || strings.Contains(line, "nolint:gosec") {
					continue
				}
				if lineIdx > 0 {
					prev := lines[lineIdx-1]
					if strings.Contains(prev, "#nosec") || strings.Contains(prev, "nolint:gosec") {
						continue
					}
				}

				// Skip lines with parameterized placeholders on the same line
				if strings.Contains(line, "?") || strings.Contains(line, "$1") {
					continue
				}

				// Go-specific: skip fmt.Sprintf that builds placeholder lists.
				// Pattern: fmt.Sprintf("...IN (%s)", strings.Join(placeholders...))
				// These are safe because %s inserts "?,?,?" not user data.
				if strings.Contains(line, "fmt.Sprintf") && isSafeGoSQLBuilder(line, lines, lineIdx) {
					continue
				}

				// Skip error/log formatting that mentions SQL keywords
				if strings.Contains(line, "fmt.Sprintf") || strings.Contains(line, "fmt.Errorf") {
					if strings.Contains(line, "failed to") || strings.Contains(line, "error") ||
						strings.Contains(line, "warning") || strings.Contains(line, "%w") ||
						strings.Contains(line, "\\033[") || strings.Contains(line, "ANSI") {
						continue
					}
				}

				// Skip dynamic WHERE/LIMIT construction with safe types:
				// query += " WHERE " + strings.Join(...) — builds from hardcoded clauses
				// fmt.Sprintf(" LIMIT %d", n) — integer interpolation is safe
				if strings.Contains(line, "strings.Join") {
					continue
				}
				if strings.Contains(line, "%d") && !strings.Contains(line, "%s") && !strings.Contains(line, "%v") {
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

// isSafeGoSQLBuilder checks if a fmt.Sprintf line is building safe SQL structure
// (placeholder lists, table/column names) rather than injecting user input.
func isSafeGoSQLBuilder(line string, lines []string, idx int) bool {
	lower := strings.ToLower(line)

	// Building placeholder lists: strings.Join(placeholders, ",")
	if strings.Contains(lower, "strings.join") && (strings.Contains(lower, "placeholder") || strings.Contains(lower, `","`) || strings.Contains(lower, `", "`)) {
		return true
	}

	// Exec/Query on the same line as Sprintf — table name substitution
	// e.g., tx.Exec(fmt.Sprintf("DELETE FROM %s", table))
	if strings.Contains(line, ".Exec(fmt.Sprintf") || strings.Contains(line, ".Query(fmt.Sprintf") {
		return true
	}

	// Check surrounding lines (±5) for parameterized query execution
	// If nearby code calls db.Query/Exec with ?, the Sprintf is building structure
	start := idx - 5
	if start < 0 {
		start = 0
	}
	end := idx + 5
	if end > len(lines) {
		end = len(lines)
	}
	for i := start; i < end; i++ {
		ctx := lines[i]
		// Parameterized execution nearby
		if strings.Contains(ctx, "QueryContext") || strings.Contains(ctx, "ExecContext") ||
			strings.Contains(ctx, "db.Query") || strings.Contains(ctx, "db.Exec") ||
			strings.Contains(ctx, "tx.Query") || strings.Contains(ctx, "tx.Exec") ||
			strings.Contains(ctx, "stmt.Exec") {
			if strings.Contains(ctx, "?") || strings.Contains(ctx, "args...") || strings.Contains(ctx, "args)") {
				return true
			}
		}
	}

	// Building WHERE clause structure with pre-validated column names
	if strings.Contains(lower, "where") && (strings.Contains(lower, "clauses") || strings.Contains(lower, "conditions")) {
		return true
	}

	return false
}

// --- path-traversal: A.8.28 — User input in file paths ---

type pathTraversalCheck struct{}

func (c *pathTraversalCheck) ID() string       { return "path-traversal" }
func (c *pathTraversalCheck) Name() string     { return "Path Traversal Risk" }
func (c *pathTraversalCheck) Article() string   { return "A.8.28 ISO 27001:2022" }
func (c *pathTraversalCheck) Severity() string  { return "error" }

var pathTraversalPatterns = []*regexp.Regexp{
	// Require word boundaries around variable names to avoid matching "requirements" as "req"
	regexp.MustCompile(`(?i)filepath\.Join\(.*(?:r\.URL|request\b|req\b|param\b|query\b|body\b)`),
	regexp.MustCompile(`(?i)os\.Open\(.*(?:r\.URL|request\b|req\b|param\b|query\b|body\b|userInput)`),
	regexp.MustCompile(`(?i)os\.ReadFile\(.*(?:r\.URL|request\b|req\b|param\b|query\b|body\b|userInput)`),
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
	regexp.MustCompile(`(?i)\byaml\.load\(`),               // Python yaml.load without Loader=SafeLoader
	// Note: yaml.Unmarshal (Go) is typed deserialization and generally safe — not flagged.
	regexp.MustCompile(`(?i)\beval\(\s*(?:request|req\b|params|user|input)`),
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
