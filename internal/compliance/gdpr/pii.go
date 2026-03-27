package gdpr

import (
	"context"
	"fmt"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- pii-detection: Art. 4(1) — find PII fields in data models ---

type piiDetectionCheck struct{}

func (c *piiDetectionCheck) ID() string       { return "pii-detection" }
func (c *piiDetectionCheck) Name() string     { return "PII Field Detection" }
func (c *piiDetectionCheck) Article() string  { return "Art. 4(1) GDPR" }
func (c *piiDetectionCheck) Severity() string { return "info" }

func (c *piiDetectionCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	scanner := compliance.NewPIIScanner(scope.Config.PIIFieldPatterns)
	fields, err := scanner.ScanFiles(ctx, scope)
	if err != nil {
		return nil, err
	}

	var findings []compliance.Finding
	for _, f := range fields {
		msg := fmt.Sprintf("PII field '%s' (%s) detected", f.Name, f.PIIType)
		if f.Container != "" {
			msg += fmt.Sprintf(" in %s", f.Container)
		}

		findings = append(findings, compliance.Finding{
			Severity:   "info",
			Article:    "Art. 4(1) GDPR",
			File:       f.File,
			StartLine:  f.Line,
			Message:    msg,
			Suggestion: "Ensure this PII field has appropriate protection: encryption at rest, access controls, retention policy, and deletion capability",
			Confidence: f.Confidence,
		})
	}

	return findings, nil
}

// --- pii-in-logs: Art. 25, 32 — PII in log statements ---

type piiInLogsCheck struct{}

func (c *piiInLogsCheck) ID() string       { return "pii-in-logs" }
func (c *piiInLogsCheck) Name() string     { return "PII in Log Statements" }
func (c *piiInLogsCheck) Article() string  { return "Art. 25, 32 GDPR" }
func (c *piiInLogsCheck) Severity() string { return "error" }

func (c *piiInLogsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	scanner := compliance.NewPIIScanner(scope.Config.PIIFieldPatterns)
	findings, err := scanner.CheckPIIInLogs(ctx, scope)
	if err != nil {
		return nil, err
	}

	// Tag with GDPR-specific metadata
	for i := range findings {
		findings[i].Article = "Art. 25, 32 GDPR"
		findings[i].CWE = "CWE-532"
	}

	return findings, nil
}

// --- pii-in-errors: Art. 25 — PII in error messages ---

type piiInErrorsCheck struct{}

func (c *piiInErrorsCheck) ID() string       { return "pii-in-errors" }
func (c *piiInErrorsCheck) Name() string     { return "PII in Error Messages" }
func (c *piiInErrorsCheck) Article() string  { return "Art. 25 GDPR" }
func (c *piiInErrorsCheck) Severity() string { return "error" }

func (c *piiInErrorsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	scanner := compliance.NewPIIScanner(scope.Config.PIIFieldPatterns)
	findings, err := scanner.CheckPIIInErrors(ctx, scope)
	if err != nil {
		return nil, err
	}

	for i := range findings {
		findings[i].Article = "Art. 25 GDPR"
	}

	return findings, nil
}
