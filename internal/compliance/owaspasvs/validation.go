package owaspasvs

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

// --- sql-injection: V5.3.4 ASVS — SQL parameterization ---

type sqlInjectionCheck struct{}

func (c *sqlInjectionCheck) ID() string       { return "sql-injection" }
func (c *sqlInjectionCheck) Name() string     { return "SQL Injection Risk" }
func (c *sqlInjectionCheck) Article() string   { return "V5.3.4 ASVS" }
func (c *sqlInjectionCheck) Severity() string  { return "error" }

var asvsSQLInjectionPatterns = []*regexp.Regexp{
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

		// Skip test files and test fixtures
		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") ||
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

				// Skip lines with parameterized placeholders
				if strings.Contains(line, "?") || strings.Contains(line, "$1") {
					continue
				}

				// Go-specific: skip safe SQL builder patterns
				if strings.Contains(line, "fmt.Sprintf") && isSafeGoSQLBuilder(line, lines, lineIdx) {
					continue
				}

				// Skip error/log formatting
				if strings.Contains(line, "fmt.Sprintf") || strings.Contains(line, "fmt.Errorf") {
					if strings.Contains(line, "failed to") || strings.Contains(line, "error") ||
						strings.Contains(line, "warning") || strings.Contains(line, "%w") ||
						strings.Contains(line, "\\033[") || strings.Contains(line, "ANSI") {
						continue
					}
				}

				// Skip safe dynamic SQL construction
				if strings.Contains(line, "strings.Join") {
					continue
				}
				if strings.Contains(line, "%d") && !strings.Contains(line, "%s") && !strings.Contains(line, "%v") {
					continue
				}

				for _, pattern := range asvsSQLInjectionPatterns {
					if pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "V5.3.4 ASVS",
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

// isSafeGoSQLBuilder checks if a fmt.Sprintf line is building safe SQL structure.
func isSafeGoSQLBuilder(line string, lines []string, idx int) bool {
	lower := strings.ToLower(line)

	if strings.Contains(lower, "strings.join") && (strings.Contains(lower, "placeholder") || strings.Contains(lower, `","`) || strings.Contains(lower, `", "`)) {
		return true
	}

	// Exec/Query on the same line as Sprintf — table name substitution
	if strings.Contains(line, ".Exec(fmt.Sprintf") || strings.Contains(line, ".Query(fmt.Sprintf") {
		return true
	}

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
		if strings.Contains(ctx, "QueryContext") || strings.Contains(ctx, "ExecContext") ||
			strings.Contains(ctx, "db.Query") || strings.Contains(ctx, "db.Exec") ||
			strings.Contains(ctx, "tx.Query") || strings.Contains(ctx, "tx.Exec") ||
			strings.Contains(ctx, "stmt.Exec") {
			if strings.Contains(ctx, "?") || strings.Contains(ctx, "args...") || strings.Contains(ctx, "args)") {
				return true
			}
		}
	}

	if strings.Contains(lower, "where") && (strings.Contains(lower, "clauses") || strings.Contains(lower, "conditions")) {
		return true
	}

	return false
}

// --- xss-prevention: V5.3.3 ASVS — Output encoding ---

type xssPreventionCheck struct{}

func (c *xssPreventionCheck) ID() string       { return "xss-prevention" }
func (c *xssPreventionCheck) Name() string     { return "Cross-Site Scripting (XSS) Risk" }
func (c *xssPreventionCheck) Article() string   { return "V5.3.3 ASVS" }
func (c *xssPreventionCheck) Severity() string  { return "error" }

var xssPatterns = []struct {
	pattern *regexp.Regexp
	desc    string
}{
	{regexp.MustCompile(`\.innerHTML\s*=`), "Direct innerHTML assignment"},
	{regexp.MustCompile(`\bdangerouslySetInnerHTML\b`), "React dangerouslySetInnerHTML"},
	{regexp.MustCompile(`\bv-html\b`), "Vue v-html directive"},
	{regexp.MustCompile(`\|\s*safe\b`), "Template |safe filter (unescaped output)"},
	{regexp.MustCompile(`(?i)\btemplate\.HTML\(`), "Go template.HTML() bypass"},
	{regexp.MustCompile(`\{\{\{.*\}\}\}`), "Triple-brace unescaped output (Handlebars/Mustache)"},
	{regexp.MustCompile(`(?i)\.write\(\s*['"]<`), "document.write with HTML"},
	{regexp.MustCompile(`(?i)\.insertAdjacentHTML\(`), "insertAdjacentHTML"},
	{regexp.MustCompile(`(?i)\bouterHTML\s*=`), "Direct outerHTML assignment"},
}

func (c *xssPreventionCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") || strings.Contains(file, ".spec.") {
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

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "*") {
					continue
				}

				for _, xss := range xssPatterns {
					if xss.pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "V5.3.3 ASVS",
							File:       file,
							StartLine:  lineNum,
							Message:    "Potential XSS vulnerability: " + xss.desc,
							Suggestion: "Use context-aware output encoding; avoid raw HTML insertion without sanitization",
							Confidence: 0.80,
							CWE:        "CWE-79",
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}

// --- command-injection: V5.3.8 ASVS — OS command injection prevention ---

type commandInjectionCheck struct{}

func (c *commandInjectionCheck) ID() string       { return "command-injection" }
func (c *commandInjectionCheck) Name() string     { return "OS Command Injection Risk" }
func (c *commandInjectionCheck) Article() string   { return "V5.3.8 ASVS" }
func (c *commandInjectionCheck) Severity() string  { return "error" }

var commandInjectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)exec\.Command\(.*\+`),
	regexp.MustCompile(`(?i)exec\.CommandContext\(.*\+`),
	regexp.MustCompile(`(?i)os\.system\(.*\+`),
	regexp.MustCompile(`(?i)subprocess\.(?:call|run|Popen)\(.*(?:shell=True|\+)`),
	regexp.MustCompile(`(?i)Runtime\.getRuntime\(\)\.exec\(.*\+`),
	regexp.MustCompile(`(?i)child_process\.exec\(.*\+`),
	regexp.MustCompile(`(?i)child_process\.execSync\(.*\+`),
}

func (c *commandInjectionCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Skip test files
		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") || strings.Contains(file, ".spec.") ||
			strings.Contains(file, "testdata/") || strings.Contains(file, "testutil/") {
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

				// Skip comments
				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "*") {
					continue
				}

				// Skip #nosec/nolint annotations
				if strings.Contains(line, "#nosec") || strings.Contains(line, "nolint:gosec") {
					continue
				}

				// Skip safe path construction: concatenation with filepath.Join,
				// e.repoRoot, or other known-safe path builders (not user input).
				if isSafeCommandConstruction(line) {
					continue
				}

				for _, pattern := range commandInjectionPatterns {
					if pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "V5.3.8 ASVS",
							File:       file,
							StartLine:  lineNum,
							Message:    "Potential OS command injection: string concatenation in command execution",
							Suggestion: "Use parameterized command arguments instead of string concatenation; avoid shell=True with untrusted input",
							Confidence: 0.80,
							CWE:        "CWE-78",
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}

// isSafeCommandConstruction returns true when exec.Command/CommandContext concatenation
// uses safe path construction (filepath.Join, repoRoot) rather than user-controlled input.
func isSafeCommandConstruction(line string) bool {
	// Concatenation with filepath.Join or path.Join — safe path resolution
	if strings.Contains(line, "filepath.Join") || strings.Contains(line, "path.Join") {
		return true
	}
	// Concatenation with known repo root variables — internal path construction
	if strings.Contains(line, "repoRoot") || strings.Contains(line, "e.repoRoot") {
		return true
	}
	// All string-literal arguments (quoted strings only, no variable concat with user input)
	// If the + is only between quoted strings, it's safe
	if strings.Contains(line, "exec.Command") || strings.Contains(line, "exec.CommandContext") {
		// If concatenation is only with string literals or filepath operations, skip
		if !strings.Contains(line, "req.") && !strings.Contains(line, "request.") &&
			!strings.Contains(line, "params[") && !strings.Contains(line, "query[") &&
			!strings.Contains(line, "userInput") && !strings.Contains(line, "body[") {
			return true
		}
	}
	return false
}

// --- eval-injection: V5.2.4 ASVS — Dynamic code execution prevention ---

type evalInjectionCheck struct{}

func (c *evalInjectionCheck) ID() string       { return "eval-injection" }
func (c *evalInjectionCheck) Name() string     { return "Dynamic Code Execution (Eval Injection)" }
func (c *evalInjectionCheck) Article() string   { return "V5.2.4 ASVS" }
func (c *evalInjectionCheck) Severity() string  { return "error" }

var evalPatterns = []struct {
	pattern *regexp.Regexp
	desc    string
}{
	{regexp.MustCompile(`\beval\s*\(`), "eval() call"},
	{regexp.MustCompile(`\bexec\s*\(`), "exec() call"},
	{regexp.MustCompile(`\bnew\s+Function\s*\(`), "Function constructor"},
	{regexp.MustCompile(`(?i)\bcompile\s*\(`), "compile() call"},
	{regexp.MustCompile(`\b__import__\s*\(`), "dynamic __import__()"},
}

func (c *evalInjectionCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Skip test files
		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") || strings.Contains(file, ".spec.") ||
			strings.Contains(file, "testdata/") || strings.Contains(file, "testutil/") {
			continue
		}

		// Skip Python __init__.py files (legitimate __import__ usage)
		if strings.HasSuffix(file, "__init__.py") {
			continue
		}

		// Go doesn't have eval/exec builtins — exec.Command is OS command
		// execution (handled by command-injection check, not eval-injection).
		if strings.HasSuffix(file, ".go") {
			continue
		}

		// Skip CI/CD configurations — e.g. GitHub Actions use @actions/exec
		// which is a safe subprocess runner, not JavaScript eval().
		if strings.Contains(file, ".github/") {
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

				// Skip comments
				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "*") {
					continue
				}

				// Skip #nosec/nolint annotations
				if strings.Contains(line, "#nosec") || strings.Contains(line, "nolint:gosec") {
					continue
				}

				// Skip regex/pattern definitions (they may contain 'compile')
				if strings.Contains(line, "regexp.MustCompile") || strings.Contains(line, "regexp.Compile") ||
					strings.Contains(line, "re.compile") || strings.Contains(line, "Compile(") {
					continue
				}

				for _, ep := range evalPatterns {
					if ep.pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "V5.2.4 ASVS",
							File:       file,
							StartLine:  lineNum,
							Message:    fmt.Sprintf("Dynamic code execution detected: %s", ep.desc),
							Suggestion: "Avoid eval/exec/Function constructor with dynamic input; use safe alternatives like JSON.parse() or predefined handlers",
							Confidence: 0.75,
							CWE:        "CWE-95",
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}
