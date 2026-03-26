package soc2

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- swallowed-errors: CC7.2 — Empty catch/except blocks ---

type swallowedErrorsCheck struct{}

func (c *swallowedErrorsCheck) ID() string       { return "swallowed-errors" }
func (c *swallowedErrorsCheck) Name() string     { return "Swallowed Errors" }
func (c *swallowedErrorsCheck) Article() string   { return "CC7.2 SOC 2" }
func (c *swallowedErrorsCheck) Severity() string  { return "warning" }

var swallowedErrorPatterns = []*regexp.Regexp{
	// JavaScript/TypeScript: empty catch
	regexp.MustCompile(`catch\s*\([^)]*\)\s*\{\s*\}`),
	// Python: bare except pass
	regexp.MustCompile(`except\s*:\s*pass`),
	regexp.MustCompile(`except\s+\w+\s*:\s*pass`),
	// Java/C#: empty catch
	regexp.MustCompile(`catch\s*\([^)]+\)\s*\{\s*\}`),
	// Note: Go `_ = obj.Method()` pattern was removed — too broad.
	// Go-specific `_ = err` is handled by goErrSuppressPattern below.
}

// More specific Go pattern for suppressed errors.
var goErrSuppressPattern = regexp.MustCompile(`_\s*=\s*err\b`)

func (c *swallowedErrorsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

				// Check Go-specific error suppression (skip annotated suppressions)
				if strings.Contains(line, "non-critical") || strings.Contains(line, "best-effort") ||
					strings.Contains(line, "#nosec") || strings.Contains(line, "nolint") {
					continue
				}
				if goErrSuppressPattern.MatchString(line) {
					findings = append(findings, compliance.Finding{
						Severity:   "warning",
						Article:    "CC7.2 SOC 2",
						File:       file,
						StartLine:  lineNum,
						Message:    "Error explicitly suppressed — may hide operational issues",
						Suggestion: "Handle or log errors instead of suppressing them; unhandled errors impair incident detection",
						Confidence: 0.70,
					})
					continue
				}

				// Check language-agnostic patterns
				for _, pattern := range swallowedErrorPatterns {
					if pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "warning",
							Article:    "CC7.2 SOC 2",
							File:       file,
							StartLine:  lineNum,
							Message:    "Empty error handler detected — errors are silently swallowed",
							Suggestion: "Log errors at minimum; empty catch/except blocks hide failures and impair monitoring",
							Confidence: 0.80,
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}

// --- missing-security-logging: CC7.2 — Auth code without logging ---

type missingSecurityLoggingCheck struct{}

func (c *missingSecurityLoggingCheck) ID() string       { return "missing-security-logging" }
func (c *missingSecurityLoggingCheck) Name() string     { return "Missing Security Event Logging" }
func (c *missingSecurityLoggingCheck) Article() string   { return "CC7.2 SOC 2" }
func (c *missingSecurityLoggingCheck) Severity() string  { return "warning" }

var securityEventPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(login|log_in|sign_in|signin|authenticate)\s*\(`),
	regexp.MustCompile(`(?i)(logout|log_out|sign_out|signout)\s*\(`),
	regexp.MustCompile(`(?i)(change_password|reset_password|update_password)\s*\(`),
	regexp.MustCompile(`(?i)(grant|revoke|change).*permission`),
	regexp.MustCompile(`(?i)(add|remove).*role`),
}

func (c *missingSecurityLoggingCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
			continue
		}

		// Skip documentation directories — not security components
		if strings.Contains(file, "docs/") || strings.Contains(file, "/docs/") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		text := string(content)

		// Check if file contains security events
		hasSecurityEvents := false
		for _, pattern := range securityEventPatterns {
			if pattern.MatchString(text) {
				hasSecurityEvents = true
				break
			}
		}

		if !hasSecurityEvents {
			continue
		}

		// Check if file has logging
		textLower := strings.ToLower(text)
		hasLogging := false
		for _, lp := range compliance.LogFunctionPatterns {
			if strings.Contains(textLower, lp) {
				hasLogging = true
				break
			}
		}

		if !hasLogging {
			findings = append(findings, compliance.Finding{
				Severity:   "warning",
				Article:    "CC7.2 SOC 2",
				File:       file,
				Message:    "Authentication/authorization code without logging statements",
				Suggestion: "Add security event logging for login, logout, password changes, and permission modifications",
				Confidence: 0.65,
			})
		}
	}

	return findings, nil
}
