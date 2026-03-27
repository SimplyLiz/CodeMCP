package nist80053

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- insufficient-audit-content: AU-3 — Audit records missing required fields ---

type insufficientAuditContentCheck struct{}

func (c *insufficientAuditContentCheck) ID() string       { return "insufficient-audit-content" }
func (c *insufficientAuditContentCheck) Name() string     { return "Insufficient Audit Record Content" }
func (c *insufficientAuditContentCheck) Article() string  { return "AU-3 NIST 800-53" }
func (c *insufficientAuditContentCheck) Severity() string { return "warning" }

// Required audit fields per NIST AU-3.
var auditRequiredFields = map[string][]string{
	"who":     {"user_id", "userid", "subject", "actor", "principal", "username", "user_name"},
	"what":    {"action", "event_type", "event_name", "operation", "activity"},
	"when":    {"timestamp", "time", "created_at", "logged_at", "event_time"},
	"outcome": {"success", "failure", "result", "status", "outcome", "error"},
}

var auditLogPattern = regexp.MustCompile(`(?i)(audit|security|event).*log`)

func (c *insufficientAuditContentCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		text := string(content)

		// Only check files that have audit/security logging patterns
		if !auditLogPattern.MatchString(text) {
			continue
		}

		// Only check audit-relevant files: auth/, api/ directories,
		// or files with authentication/authorization patterns.
		// Skip purely internal utility files (slogutil, suggest, config, tool_impls, etc.)
		isAuditRelevantPath := strings.Contains(file, "auth/") || strings.Contains(file, "api/") ||
			strings.Contains(file, "middleware/") || strings.Contains(file, "handler/") ||
			strings.Contains(file, "security/")
		if !isAuditRelevantPath {
			textCheck := strings.ToLower(text)
			hasAuthPatterns := strings.Contains(textCheck, "authenticate") || strings.Contains(textCheck, "authorization") ||
				strings.Contains(textCheck, "login") || strings.Contains(textCheck, "access control") ||
				strings.Contains(textCheck, "permission") || strings.Contains(textCheck, "credential")
			if !hasAuthPatterns {
				continue
			}
		}

		textLower := strings.ToLower(text)

		// Structured loggers (slog, logger) capture context automatically
		// via attributes — treat them as having adequate audit content.
		if strings.Contains(text, "slog.") || strings.Contains(text, "logger.") {
			continue
		}

		// Skip pure type/const definition files and files with no I/O operations
		// (no logging, no network, no file system — nothing to audit)
		if !strings.Contains(text, "func ") && !strings.Contains(text, "func(") {
			continue
		}
		hasIO := strings.Contains(text, "os.") || strings.Contains(text, "http.") ||
			strings.Contains(text, "io.") || strings.Contains(text, "net.") ||
			strings.Contains(text, "log.") || strings.Contains(text, "slog.") ||
			strings.Contains(text, "fmt.Print") || strings.Contains(text, "sql.")
		if !hasIO {
			continue
		}

		// Check which required fields are present
		var missingCategories []string
		for category, fields := range auditRequiredFields {
			found := false
			for _, field := range fields {
				if strings.Contains(textLower, field) {
					found = true
					break
				}
			}
			if !found {
				missingCategories = append(missingCategories, category)
			}
		}

		if len(missingCategories) > 0 {
			findings = append(findings, compliance.Finding{
				Severity:   "warning",
				Article:    "AU-3 NIST 800-53",
				File:       file,
				Message:    "Audit logging missing required content fields: " + strings.Join(missingCategories, ", "),
				Suggestion: "NIST AU-3 requires audit records to include: who (user/subject), what (action/event), when (timestamp), and outcome (success/failure)",
				Confidence: 0.65,
			})
		}
	}

	return findings, nil
}

// --- missing-audit-events: AU-2 — Auth operations without audit logging ---

type missingAuditEventsCheck struct{}

func (c *missingAuditEventsCheck) ID() string       { return "missing-audit-events" }
func (c *missingAuditEventsCheck) Name() string     { return "Missing Auditable Events" }
func (c *missingAuditEventsCheck) Article() string  { return "AU-2 NIST 800-53" }
func (c *missingAuditEventsCheck) Severity() string { return "warning" }

var authOperationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(login|log_in|sign_in|signin|authenticate)\s*\(`),
	regexp.MustCompile(`(?i)(logout|log_out|sign_out|signout)\s*\(`),
	regexp.MustCompile(`(?i)(failed_auth|auth_fail|invalid_password|wrong_password)`),
	regexp.MustCompile(`(?i)(privilege_change|role_change|permission_update|grant_role|revoke_role)`),
	regexp.MustCompile(`(?i)(change_password|reset_password|update_password)\s*\(`),
}

func (c *missingAuditEventsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
			continue
		}

		// Skip documentation directories — not auth components
		if strings.Contains(file, "docs/") || strings.Contains(file, "/docs/") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		text := string(content)

		// Check if file has auth operations
		hasAuthOps := false
		for _, pattern := range authOperationPatterns {
			if pattern.MatchString(text) {
				hasAuthOps = true
				break
			}
		}

		if !hasAuthOps {
			continue
		}

		// Check for audit/security logging
		textLower := strings.ToLower(text)
		hasAuditLogging := false
		auditIndicators := []string{
			"audit", "security_log", "securitylog", "event_log", "eventlog",
		}
		for _, indicator := range auditIndicators {
			if strings.Contains(textLower, indicator) {
				hasAuditLogging = true
				break
			}
		}

		// Also check for general logging as a weaker signal
		if !hasAuditLogging {
			for _, lp := range compliance.LogFunctionPatterns {
				if strings.Contains(textLower, lp) {
					// Has logging, but not audit-specific — less severe
					hasAuditLogging = true
					break
				}
			}

			if !hasAuditLogging {
				findings = append(findings, compliance.Finding{
					Severity:   "warning",
					Article:    "AU-2 NIST 800-53",
					File:       file,
					Message:    "Authentication/authorization operations without audit event logging",
					Suggestion: "Log all security-relevant events: login, logout, failed authentication, and privilege changes per NIST AU-2",
					Confidence: 0.70,
				})
			}
		}
	}

	return findings, nil
}
