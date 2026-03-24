package hipaa

import (
	"context"
	"fmt"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// phiExtraPatterns are HIPAA's 18 identifiers beyond standard PII.
var phiExtraPatterns = []string{
	"patient_name", "medical_record_number", "mrn", "diagnosis", "icd_code",
	"treatment", "prescription", "insurance_id", "beneficiary", "health_plan",
	"provider_npi", "npi_number", "date_of_service", "admission_date",
	"discharge_date", "lab_result", "vital_sign", "allergy", "immunization",
	"procedure_code", "cpt_code", "drg", "patient_id",
}

// --- phi-detection: §164.514(b) — Detect PHI in data models ---

type phiDetectionCheck struct{}

func (c *phiDetectionCheck) ID() string       { return "phi-detection" }
func (c *phiDetectionCheck) Name() string     { return "PHI Field Detection" }
func (c *phiDetectionCheck) Article() string   { return "§164.514(b) HIPAA" }
func (c *phiDetectionCheck) Severity() string  { return "info" }

func (c *phiDetectionCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	extraPatterns := append(scope.Config.PIIFieldPatterns, phiExtraPatterns...)
	scanner := compliance.NewPIIScanner(extraPatterns)
	fields, err := scanner.ScanFiles(ctx, scope)
	if err != nil {
		return nil, err
	}

	var findings []compliance.Finding
	for _, f := range fields {
		// Only report PHI-specific fields as PHI; standard PII is handled by GDPR/other frameworks
		if !isPHIField(f.Name) {
			continue
		}

		msg := fmt.Sprintf("PHI field '%s' (%s) detected", f.Name, f.PIIType)
		if f.Container != "" {
			msg += fmt.Sprintf(" in %s", f.Container)
		}

		findings = append(findings, compliance.Finding{
			Severity:   "info",
			Article:    "§164.514(b) HIPAA",
			File:       f.File,
			StartLine:  f.Line,
			Message:    msg,
			Suggestion: "Ensure this PHI field has appropriate safeguards: encryption, access controls, audit logging, and minimum necessary access",
			Confidence: f.Confidence,
		})
	}

	return findings, nil
}

func isPHIField(name string) bool {
	lower := strings.ToLower(name)
	for _, p := range phiExtraPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// --- phi-in-logs: §164.312(b) — PHI in log statements ---

type phiInLogsCheck struct{}

func (c *phiInLogsCheck) ID() string       { return "phi-in-logs" }
func (c *phiInLogsCheck) Name() string     { return "PHI in Log Statements" }
func (c *phiInLogsCheck) Article() string   { return "§164.312(b) HIPAA" }
func (c *phiInLogsCheck) Severity() string  { return "error" }

func (c *phiInLogsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	extraPatterns := append(scope.Config.PIIFieldPatterns, phiExtraPatterns...)
	scanner := compliance.NewPIIScanner(extraPatterns)
	findings, err := scanner.CheckPIIInLogs(ctx, scope)
	if err != nil {
		return nil, err
	}

	// Tag with HIPAA-specific metadata
	for i := range findings {
		findings[i].Article = "§164.312(b) HIPAA"
		findings[i].CWE = "CWE-532"
	}

	return findings, nil
}
