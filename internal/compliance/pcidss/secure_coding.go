package pcidss

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- sql-injection: Req 6.2.4 — SQL injection prevention ---

type sqlInjectionCheck struct{}

func (c *sqlInjectionCheck) ID() string       { return "sql-injection" }
func (c *sqlInjectionCheck) Name() string     { return "SQL Injection Risk" }
func (c *sqlInjectionCheck) Article() string   { return "Req 6.2.4 PCI DSS 4.0" }
func (c *sqlInjectionCheck) Severity() string  { return "error" }

var pciSQLInjectionPatterns = []*regexp.Regexp{
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

			for _, pattern := range pciSQLInjectionPatterns {
				if pattern.MatchString(line) {
					findings = append(findings, compliance.Finding{
						Severity:   "error",
						Article:    "Req 6.2.4 PCI DSS 4.0",
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

// --- xss-prevention: Req 6.2.4 — Cross-site scripting prevention ---

type xssPreventionCheck struct{}

func (c *xssPreventionCheck) ID() string       { return "xss-prevention" }
func (c *xssPreventionCheck) Name() string     { return "Cross-Site Scripting (XSS) Risk" }
func (c *xssPreventionCheck) Article() string   { return "Req 6.2.4 PCI DSS 4.0" }
func (c *xssPreventionCheck) Severity() string  { return "error" }

var xssPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\.innerHTML\s*=`),
	regexp.MustCompile(`(?i)v-html\s*=`),
	regexp.MustCompile(`(?i)dangerouslySetInnerHTML`),
	regexp.MustCompile(`\{!!\s*.*\s*!!\}`),
	regexp.MustCompile(`(?i)\|\s*safe\b`),
	regexp.MustCompile(`(?i)autoescape\s+(off|false)`),
	regexp.MustCompile(`(?i)document\.write\(`),
	regexp.MustCompile(`(?i)\.outerHTML\s*=`),
	regexp.MustCompile(`(?i)\$\(\s*['"].*['"]\s*\)\.html\(`),
}

func (c *xssPreventionCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

			for _, pattern := range xssPatterns {
				if pattern.MatchString(line) {
					findings = append(findings, compliance.Finding{
						Severity:   "error",
						Article:    "Req 6.2.4 PCI DSS 4.0",
						File:       file,
						StartLine:  lineNum,
						Message:    "Potential XSS: unescaped user input rendered in HTML",
						Suggestion: "Use context-aware output encoding; avoid innerHTML, dangerouslySetInnerHTML, and unescaped template directives",
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
