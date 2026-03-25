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

// --- default-credentials: CR 1.1 — no default/hardcoded credentials ---

type defaultCredentialsCheck struct{}

func (c *defaultCredentialsCheck) ID() string       { return "default-credentials" }
func (c *defaultCredentialsCheck) Name() string     { return "Default/Hardcoded Credentials" }
func (c *defaultCredentialsCheck) Article() string   { return "CR 1.1 IEC 62443-4-2" }
func (c *defaultCredentialsCheck) Severity() string  { return "error" }

var credentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*["'][\w!@#$%^&*]+["']`),
	regexp.MustCompile(`(?i)(api_key|apikey|api_secret|secret_key)\s*[:=]\s*["'][\w\-]+["']`),
	regexp.MustCompile(`(?i)(username|user)\s*[:=]\s*["'](admin|root|operator|default|test)["']`),
	regexp.MustCompile(`(?i)(token|auth_token|access_token)\s*[:=]\s*["'][\w\-\.]+["']`),
}

func (c *defaultCredentialsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Skip test files
		if strings.Contains(file, "_test.") || strings.Contains(file, "test_") || strings.Contains(file, "mock") {
			continue
		}
		// Skip example/sample/fixture files
		if strings.Contains(file, "example") || strings.Contains(file, "sample") || strings.Contains(file, "fixture") {
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

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") ||
					strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
					continue
				}

				for _, pattern := range credentialPatterns {
					if m := pattern.FindString(line); m != "" {
						findings = append(findings, compliance.Finding{
							CheckID:    "default-credentials",
							Framework:  compliance.FrameworkIEC62443,
							Severity:   "error",
							Article:    "CR 1.1 IEC 62443-4-2",
							File:       file,
							StartLine:  lineNum,
							Message:    fmt.Sprintf("Hardcoded credential detected: %s", m),
							Suggestion: "Use environment variables, a secrets manager, or secure configuration for credentials",
							Confidence: 0.85,
							CWE:        "CWE-798",
						})
						break // One finding per line
					}
				}
			}
		}()
	}

	return findings, nil
}

// --- missing-auth: CR 1.2 — control/command functions must have authentication ---

type missingAuthCheck struct{}

func (c *missingAuthCheck) ID() string       { return "missing-auth" }
func (c *missingAuthCheck) Name() string     { return "Missing Authentication on Control Functions" }
func (c *missingAuthCheck) Article() string   { return "CR 1.2 IEC 62443-4-2" }
func (c *missingAuthCheck) Severity() string  { return "error" }

// Control/command function name patterns
var controlFuncPattern = regexp.MustCompile(`(?i)func\s+.*\b(\w*_control|control_\w*|\w*_command|command_\w*|set_\w*|write_\w*|actuate_\w*)\s*\(`)
var authPattern = regexp.MustCompile(`(?i)(auth|authenticate|authorized|permission|credential|token|session|login|verify_user|check_auth|require_auth|is_authenticated)`)

func (c *missingAuthCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, "test_") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		var funcName string
		var funcStartLine int
		braceDepth := 0
		hasAuth := false

		for i, line := range lines {
			lineNum := i + 1

			if m := controlFuncPattern.FindStringSubmatch(line); len(m) > 1 {
				// Check previous function
				if funcName != "" && !hasAuth {
					findings = append(findings, compliance.Finding{
						CheckID:    "missing-auth",
						Framework:  compliance.FrameworkIEC62443,
						Severity:   "error",
						Article:    "CR 1.2 IEC 62443-4-2",
						File:       file,
						StartLine:  funcStartLine,
						Message:    fmt.Sprintf("Control function '%s' has no authentication check", funcName),
						Suggestion: "Add authentication/authorization check before executing control operations",
						Confidence: 0.70,
					})
				}
				funcName = m[1]
				funcStartLine = lineNum
				braceDepth = 0
				hasAuth = false
			}

			braceDepth += strings.Count(line, "{") - strings.Count(line, "}")

			if funcName != "" && authPattern.MatchString(line) {
				hasAuth = true
			}

			if funcName != "" && braceDepth <= 0 && lineNum > funcStartLine {
				if !hasAuth {
					findings = append(findings, compliance.Finding{
						CheckID:    "missing-auth",
						Framework:  compliance.FrameworkIEC62443,
						Severity:   "error",
						Article:    "CR 1.2 IEC 62443-4-2",
						File:       file,
						StartLine:  funcStartLine,
						Message:    fmt.Sprintf("Control function '%s' has no authentication check", funcName),
						Suggestion: "Add authentication/authorization check before executing control operations",
						Confidence: 0.70,
					})
				}
				funcName = ""
				hasAuth = false
			}
		}
	}

	return findings, nil
}
