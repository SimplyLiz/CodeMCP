package gdpr

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- no-retention-policy: Art. 5(1)(e) — PII without TTL/expiry ---

type noRetentionPolicyCheck struct{}

func (c *noRetentionPolicyCheck) ID() string       { return "no-retention-policy" }
func (c *noRetentionPolicyCheck) Name() string     { return "Missing Data Retention Policy" }
func (c *noRetentionPolicyCheck) Article() string   { return "Art. 5(1)(e) GDPR" }
func (c *noRetentionPolicyCheck) Severity() string  { return "warning" }

func (c *noRetentionPolicyCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	piiScanner := compliance.NewPIIScanner(scope.Config.PIIFieldPatterns)
	piiFields, err := piiScanner.ScanFiles(ctx, scope)
	if err != nil {
		return nil, err
	}

	// Get unique files with PII
	piiFiles := make(map[string]bool)
	for _, f := range piiFields {
		piiFiles[f.File] = true
	}

	var findings []compliance.Finding

	retentionIndicators := []string{
		"ttl", "expir", "retention", "purge", "cleanup", "archive",
		"delete_after", "max_age", "lifetime", "aufbewahrung",
	}

	// Check if the overall codebase has retention patterns
	hasRetention := false
	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		lower := strings.ToLower(string(content))
		for _, indicator := range retentionIndicators {
			if strings.Contains(lower, indicator) {
				hasRetention = true
				break
			}
		}
		if hasRetention {
			break
		}
	}

	if !hasRetention && len(piiFiles) > 0 {
		findings = append(findings, compliance.Finding{
			Severity:   "warning",
			Article:    "Art. 5(1)(e) GDPR",
			File:       "",
			Message:    "No data retention/expiry mechanisms detected in codebase with PII processing",
			Suggestion: "Implement TTL, expiry, or scheduled purge mechanisms for personal data",
			Confidence: 0.65,
		})
	}

	return findings, nil
}

// --- no-deletion-endpoint: Art. 17 — No erasure capability ---

type noDeletionEndpointCheck struct{}

func (c *noDeletionEndpointCheck) ID() string       { return "no-deletion-endpoint" }
func (c *noDeletionEndpointCheck) Name() string     { return "Missing Right to Erasure" }
func (c *noDeletionEndpointCheck) Article() string   { return "Art. 17 GDPR" }
func (c *noDeletionEndpointCheck) Severity() string  { return "warning" }

func (c *noDeletionEndpointCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	// Check if the codebase has deletion/erasure patterns
	deletionPatterns := []string{
		"delete_user", "deleteuser", "remove_user", "removeuser",
		"erase_data", "erasedata", "purge_user", "purgeuser",
		"anonymize", "pseudonymize", "gdpr_delete", "gdprdelete",
		"right_to_erasure", "data_deletion", "forget_user",
		"loeschen", "datenloesch",
	}

	// Also check for HTTP DELETE endpoints handling user data
	httpDeletePatterns := []string{
		"delete", "destroy", "remove",
	}

	hasDeleteCapability := false
	hasHTTPDelete := false

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			break
		}

		func() {
			f, err := os.Open(filepath.Join(scope.RepoRoot, file))
			if err != nil {
				return
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				lower := strings.ToLower(scanner.Text())

				for _, p := range deletionPatterns {
					if strings.Contains(lower, p) {
						hasDeleteCapability = true
						break
					}
				}

				// Check for DELETE HTTP method handlers
				if strings.Contains(lower, "\"delete\"") || strings.Contains(lower, "'delete'") ||
					strings.Contains(lower, "methods.delete") || strings.Contains(lower, "handledelete") ||
					strings.Contains(lower, ".delete(") {
					for _, hp := range httpDeletePatterns {
						if strings.Contains(lower, hp) && (strings.Contains(lower, "user") || strings.Contains(lower, "account") || strings.Contains(lower, "profile")) {
							hasHTTPDelete = true
							break
						}
					}
				}

				if hasDeleteCapability {
					break
				}
			}
		}()

		if hasDeleteCapability {
			break
		}
	}

	var findings []compliance.Finding
	if !hasDeleteCapability && !hasHTTPDelete {
		// Only flag this if there's PII in the codebase
		piiScanner := compliance.NewPIIScanner(scope.Config.PIIFieldPatterns)
		piiFields, _ := piiScanner.ScanFiles(ctx, scope)
		if len(piiFields) > 0 {
			findings = append(findings, compliance.Finding{
				Severity:   "warning",
				Article:    "Art. 17 GDPR",
				Message:    "No data deletion/erasure capability detected for personal data",
				Suggestion: "Implement a user data deletion endpoint or function to support the right to erasure",
				Confidence: 0.60,
			})
		}
	}

	return findings, nil
}

// --- missing-consent: Art. 6, 7 — No consent verification ---

type missingConsentCheck struct{}

func (c *missingConsentCheck) ID() string       { return "missing-consent" }
func (c *missingConsentCheck) Name() string     { return "Missing Consent Verification" }
func (c *missingConsentCheck) Article() string   { return "Art. 6, 7 GDPR" }
func (c *missingConsentCheck) Severity() string  { return "warning" }

func (c *missingConsentCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	consentPatterns := []string{
		"consent", "einwilligung", "zustimmung",
		"opt_in", "optin", "opt_out", "optout",
		"data_processing_agreement", "dpa",
		"has_consent", "check_consent", "verify_consent",
		"consent_given", "accepted_terms",
	}

	hasConsent := false
	for _, file := range scope.Files {
		if ctx.Err() != nil {
			break
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		lower := strings.ToLower(string(content))
		for _, p := range consentPatterns {
			if strings.Contains(lower, p) {
				hasConsent = true
				break
			}
		}
		if hasConsent {
			break
		}
	}

	var findings []compliance.Finding
	if !hasConsent {
		piiScanner := compliance.NewPIIScanner(scope.Config.PIIFieldPatterns)
		piiFields, _ := piiScanner.ScanFiles(ctx, scope)
		if len(piiFields) > 0 {
			findings = append(findings, compliance.Finding{
				Severity:   "warning",
				Article:    "Art. 6, 7 GDPR",
				Message:    "No consent verification patterns detected in codebase that processes personal data",
				Suggestion: "Implement consent management: capture consent before PII processing, support withdrawal",
				Confidence: 0.55,
			})
		}
	}

	return findings, nil
}

// --- excessive-collection: Art. 25 — SELECT * or over-fetching ---

type excessiveCollectionCheck struct{}

func (c *excessiveCollectionCheck) ID() string       { return "excessive-collection" }
func (c *excessiveCollectionCheck) Name() string     { return "Excessive Data Collection" }
func (c *excessiveCollectionCheck) Article() string   { return "Art. 25 GDPR" }
func (c *excessiveCollectionCheck) Severity() string  { return "warning" }

func (c *excessiveCollectionCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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
				upper := strings.ToUpper(strings.TrimSpace(line))

				// Detect SELECT * patterns
				if strings.Contains(upper, "SELECT *") || strings.Contains(upper, "SELECT * FROM") {
					findings = append(findings, compliance.Finding{
						Severity:   "warning",
						Article:    "Art. 25 GDPR",
						File:       file,
						StartLine:  lineNum,
						Message:    "SELECT * may fetch more personal data than needed (data minimization violation)",
						Suggestion: "Select only the specific columns required for the operation",
						Confidence: 0.70,
					})
				}
			}
		}()
	}

	return findings, nil
}

// --- unencrypted-transport: Art. 32 — HTTP for PII ---

type unencryptedTransportCheck struct{}

func (c *unencryptedTransportCheck) ID() string       { return "unencrypted-transport" }
func (c *unencryptedTransportCheck) Name() string     { return "Unencrypted PII Transport" }
func (c *unencryptedTransportCheck) Article() string   { return "Art. 32 GDPR" }
func (c *unencryptedTransportCheck) Severity() string  { return "error" }

func (c *unencryptedTransportCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

				// Skip comments
				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "*") {
					continue
				}

				// Detect hardcoded HTTP URLs (not HTTPS) in code
				if strings.Contains(line, "http://") && !strings.Contains(line, "http://localhost") &&
					!strings.Contains(line, "http://127.0.0.1") && !strings.Contains(line, "http://0.0.0.0") &&
					!strings.Contains(line, "http://[::1]") {
					findings = append(findings, compliance.Finding{
						Severity:   "error",
						Article:    "Art. 32 GDPR",
						File:       file,
						StartLine:  lineNum,
						Message:    "Unencrypted HTTP URL detected — data in transit must be encrypted",
						Suggestion: "Use HTTPS for all data transmission, especially when handling personal data",
						Confidence: 0.75,
						CWE:        "CWE-319",
					})
				}
			}
		}()
	}

	return findings, nil
}

// --- missing-access-logging: Art. 30 — CRUD without audit trail ---

type missingAccessLoggingCheck struct{}

func (c *missingAccessLoggingCheck) ID() string       { return "missing-access-logging" }
func (c *missingAccessLoggingCheck) Name() string     { return "Missing Data Access Logging" }
func (c *missingAccessLoggingCheck) Article() string   { return "Art. 30 GDPR" }
func (c *missingAccessLoggingCheck) Severity() string  { return "warning" }

func (c *missingAccessLoggingCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	auditPatterns := []string{
		"audit_log", "auditlog", "audit_trail",
		"access_log", "accesslog",
		"data_access_log", "record_access",
		"log_access", "track_access",
		"zugriffsprot", "protokoll",
	}

	hasAuditLogging := false
	for _, file := range scope.Files {
		if ctx.Err() != nil {
			break
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		lower := strings.ToLower(string(content))
		for _, p := range auditPatterns {
			if strings.Contains(lower, p) {
				hasAuditLogging = true
				break
			}
		}
		if hasAuditLogging {
			break
		}
	}

	var findings []compliance.Finding
	if !hasAuditLogging {
		piiScanner := compliance.NewPIIScanner(scope.Config.PIIFieldPatterns)
		piiFields, _ := piiScanner.ScanFiles(ctx, scope)
		if len(piiFields) > 0 {
			findings = append(findings, compliance.Finding{
				Severity:   "warning",
				Article:    "Art. 30 GDPR",
				Message:    "No data access audit logging detected in codebase with PII processing",
				Suggestion: "Implement audit logging for all CRUD operations on personal data (who accessed what, when, why)",
				Confidence: 0.60,
			})
		}
	}

	return findings, nil
}
