package iso27701

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- no-purpose-logging: A.7.2.1 — PII access without purpose ---

type noPurposeLoggingCheck struct{}

func (c *noPurposeLoggingCheck) ID() string       { return "no-purpose-logging" }
func (c *noPurposeLoggingCheck) Name() string     { return "Missing Purpose Logging" }
func (c *noPurposeLoggingCheck) Article() string   { return "A.7.2.1 ISO 27701" }
func (c *noPurposeLoggingCheck) Severity() string  { return "warning" }

func (c *noPurposeLoggingCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	purposePatterns := []string{
		"purpose", "processing_purpose", "data_purpose",
		"lawful_basis", "legal_basis", "processing_ground",
		"verarbeitungszweck", "rechtsgrundlage",
	}

	piiScanner := compliance.NewPIIScanner(scope.Config.PIIFieldPatterns)
	piiFields, _ := piiScanner.ScanFiles(ctx, scope)
	if len(piiFields) == 0 {
		return nil, nil
	}

	hasPurpose := false
	for _, file := range scope.Files {
		if ctx.Err() != nil {
			break
		}
		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}
		lower := strings.ToLower(string(content))
		for _, p := range purposePatterns {
			if strings.Contains(lower, p) {
				hasPurpose = true
				break
			}
		}
		if hasPurpose {
			break
		}
	}

	var findings []compliance.Finding
	if !hasPurpose {
		findings = append(findings, compliance.Finding{
			Severity:   "warning",
			Article:    "A.7.2.1 ISO 27701",
			Message:    "No processing purpose documentation or logging detected for PII operations",
			Suggestion: "Record the purpose/legal basis for each PII processing activity",
			Confidence: 0.55,
		})
	}
	return findings, nil
}
