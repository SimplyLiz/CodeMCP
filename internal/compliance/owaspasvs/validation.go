package owaspasvs

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- sql-injection: V5.3.3 ASVS — SQL parameterization ---

type sqlInjectionCheck struct{}

func (c *sqlInjectionCheck) ID() string       { return "sql-injection" }
func (c *sqlInjectionCheck) Name() string     { return "SQL Injection Risk" }
func (c *sqlInjectionCheck) Article() string   { return "V5.3.3 ASVS" }
func (c *sqlInjectionCheck) Severity() string  { return "error" }

var asvsSQLInjectionPatterns = []*regexp.Regexp{
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

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
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

			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
				continue
			}

			for _, pattern := range asvsSQLInjectionPatterns {
				if pattern.MatchString(line) {
					findings = append(findings, compliance.Finding{
						Severity:   "error",
						Article:    "V5.3.3 ASVS",
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
		f.Close()
	}

	return findings, nil
}

// --- xss-prevention: V5.3.4 ASVS — Output encoding ---

type xssPreventionCheck struct{}

func (c *xssPreventionCheck) ID() string       { return "xss-prevention" }
func (c *xssPreventionCheck) Name() string     { return "Cross-Site Scripting (XSS) Risk" }
func (c *xssPreventionCheck) Article() string   { return "V5.3.4 ASVS" }
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

			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "*") {
				continue
			}

			for _, xss := range xssPatterns {
				if xss.pattern.MatchString(line) {
					findings = append(findings, compliance.Finding{
						Severity:   "error",
						Article:    "V5.3.4 ASVS",
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
		f.Close()
	}

	return findings, nil
}
