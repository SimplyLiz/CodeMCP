package iso27701

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- no-consent-mechanism: A.7.2.2 — No consent verification ---

type noConsentMechanismCheck struct{}

func (c *noConsentMechanismCheck) ID() string       { return "no-consent-mechanism" }
func (c *noConsentMechanismCheck) Name() string     { return "Missing Consent Mechanism" }
func (c *noConsentMechanismCheck) Article() string  { return "A.7.2.2 ISO 27701" }
func (c *noConsentMechanismCheck) Severity() string { return "warning" }

func (c *noConsentMechanismCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	consentPatterns := []string{
		"consent", "einwilligung", "opt_in", "optin",
		"has_consent", "check_consent", "verify_consent",
		"consent_given", "accepted_terms", "privacy_policy",
	}

	hasConsent := false
	hasPII := false

	piiScanner := compliance.NewPIIScanner(scope.Config.PIIFieldPatterns)
	piiFields, _ := piiScanner.ScanFiles(ctx, scope)
	hasPII = len(piiFields) > 0

	if !hasPII {
		return nil, nil
	}

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
		findings = append(findings, compliance.Finding{
			Severity:   "warning",
			Article:    "A.7.2.2 ISO 27701",
			Message:    "No consent verification mechanism detected for PII processing",
			Suggestion: "Implement consent capture, storage, and withdrawal mechanisms before processing personal data",
			Confidence: 0.55,
		})
	}
	return findings, nil
}

// --- no-deletion-endpoint: A.7.3.6 — Missing data erasure ---

type noDeletionEndpointCheck struct{}

func (c *noDeletionEndpointCheck) ID() string       { return "no-deletion-endpoint" }
func (c *noDeletionEndpointCheck) Name() string     { return "Missing Data Erasure Endpoint" }
func (c *noDeletionEndpointCheck) Article() string  { return "A.7.3.6 ISO 27701" }
func (c *noDeletionEndpointCheck) Severity() string { return "warning" }

func (c *noDeletionEndpointCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	deletionPatterns := []string{
		"delete_user", "deleteuser", "remove_user", "erase_data",
		"purge_user", "anonymize_user", "gdpr_delete",
		"right_to_erasure", "data_deletion",
	}

	piiScanner := compliance.NewPIIScanner(scope.Config.PIIFieldPatterns)
	piiFields, _ := piiScanner.ScanFiles(ctx, scope)
	if len(piiFields) == 0 {
		return nil, nil
	}

	hasDelete := false
	for _, file := range scope.Files {
		if ctx.Err() != nil {
			break
		}
		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}
		lower := strings.ToLower(string(content))
		for _, p := range deletionPatterns {
			if strings.Contains(lower, p) {
				hasDelete = true
				break
			}
		}
		if hasDelete {
			break
		}
	}

	var findings []compliance.Finding
	if !hasDelete {
		findings = append(findings, compliance.Finding{
			Severity:   "warning",
			Article:    "A.7.3.6 ISO 27701",
			Message:    "No data erasure capability detected for PII principals",
			Suggestion: "Implement an endpoint or function to delete/anonymize all personal data for a given user",
			Confidence: 0.60,
		})
	}
	return findings, nil
}

// --- no-access-endpoint: A.7.3.6 — Missing data access endpoint ---

type noAccessEndpointCheck struct{}

func (c *noAccessEndpointCheck) ID() string       { return "no-access-endpoint" }
func (c *noAccessEndpointCheck) Name() string     { return "Missing Data Access Endpoint" }
func (c *noAccessEndpointCheck) Article() string  { return "A.7.3.6 ISO 27701" }
func (c *noAccessEndpointCheck) Severity() string { return "warning" }

func (c *noAccessEndpointCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	accessPatterns := []string{
		"export_data", "export_user", "download_data",
		"data_export", "user_data_export", "data_portability",
		"get_my_data", "personal_data_request", "data_access_request",
		"subject_access", "dsar", "sar_request",
	}

	piiScanner := compliance.NewPIIScanner(scope.Config.PIIFieldPatterns)
	piiFields, _ := piiScanner.ScanFiles(ctx, scope)
	if len(piiFields) == 0 {
		return nil, nil
	}

	hasAccess := false
	for _, file := range scope.Files {
		if ctx.Err() != nil {
			break
		}
		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}
		lower := strings.ToLower(string(content))
		for _, p := range accessPatterns {
			if strings.Contains(lower, p) {
				hasAccess = true
				break
			}
		}
		if hasAccess {
			break
		}
	}

	var findings []compliance.Finding
	if !hasAccess {
		findings = append(findings, compliance.Finding{
			Severity:   "warning",
			Article:    "A.7.3.6 ISO 27701",
			Message:    "No data access/export endpoint detected for PII principals",
			Suggestion: "Implement a data export endpoint so users can request all their personal data",
			Confidence: 0.55,
		})
	}
	return findings, nil
}

// --- no-data-portability: A.7.3.6 — No data export ---

type noDataPortabilityCheck struct{}

func (c *noDataPortabilityCheck) ID() string       { return "no-data-portability" }
func (c *noDataPortabilityCheck) Name() string     { return "Missing Data Portability" }
func (c *noDataPortabilityCheck) Article() string  { return "A.7.3.6 ISO 27701" }
func (c *noDataPortabilityCheck) Severity() string { return "info" }

func (c *noDataPortabilityCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	portabilityPatterns := []string{
		"export_json", "export_csv", "to_json", "to_csv",
		"data_portability", "machine_readable", "structured_format",
	}

	piiScanner := compliance.NewPIIScanner(scope.Config.PIIFieldPatterns)
	piiFields, _ := piiScanner.ScanFiles(ctx, scope)
	if len(piiFields) == 0 {
		return nil, nil
	}

	hasPortability := false
	for _, file := range scope.Files {
		if ctx.Err() != nil {
			break
		}
		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}
		lower := strings.ToLower(string(content))
		for _, p := range portabilityPatterns {
			if strings.Contains(lower, p) {
				hasPortability = true
				break
			}
		}
		if hasPortability {
			break
		}
	}

	var findings []compliance.Finding
	if !hasPortability {
		findings = append(findings, compliance.Finding{
			Severity:   "info",
			Article:    "A.7.3.6 ISO 27701",
			Message:    "No data portability (structured export) capability detected",
			Suggestion: "Provide data export in machine-readable formats (JSON, CSV) for data portability",
			Confidence: 0.50,
		})
	}
	return findings, nil
}
