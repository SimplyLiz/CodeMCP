package hipaa

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

// --- missing-audit-trail: §164.312(b) — Audit controls for PHI ---

type missingAuditTrailCheck struct{}

func (c *missingAuditTrailCheck) ID() string       { return "missing-audit-trail" }
func (c *missingAuditTrailCheck) Name() string     { return "Missing HIPAA Audit Trail" }
func (c *missingAuditTrailCheck) Article() string  { return "§164.312(b) HIPAA" }
func (c *missingAuditTrailCheck) Severity() string { return "warning" }

func (c *missingAuditTrailCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	// First check if codebase has PHI
	extraPatterns := append(scope.Config.PIIFieldPatterns, phiExtraPatterns...)
	scanner := compliance.NewPIIScanner(extraPatterns)
	fields, err := scanner.ScanFiles(ctx, scope)
	if err != nil {
		return nil, err
	}

	hasPHI := false
	for _, f := range fields {
		if isPHIField(f.Name) {
			hasPHI = true
			break
		}
	}

	if !hasPHI {
		return nil, nil
	}

	// Check for audit trail patterns in codebase
	auditIndicators := []string{
		"audit_log", "auditlog", "audit_trail", "audittrail",
		"access_log", "accesslog", "hipaa_log", "hipaalog",
		"phi_access", "phiaccess", "compliance_log",
		"record_access", "log_access", "track_access",
	}

	hasAuditTrail := false
	for _, file := range scope.Files {
		if ctx.Err() != nil {
			break
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		lower := strings.ToLower(string(content))
		for _, indicator := range auditIndicators {
			if strings.Contains(lower, indicator) {
				hasAuditTrail = true
				break
			}
		}
		if hasAuditTrail {
			break
		}
	}

	var findings []compliance.Finding
	if !hasAuditTrail {
		findings = append(findings, compliance.Finding{
			Severity:   "warning",
			Article:    "§164.312(b) HIPAA",
			Message:    "No audit trail mechanisms detected in codebase that handles PHI",
			Suggestion: "Implement audit logging for all PHI access: who accessed what data, when, and from where",
			Confidence: 0.65,
		})
	}

	return findings, nil
}

// --- phi-unencrypted: §164.312(a)(2)(iv) — PHI without encryption ---

type phiUnencryptedCheck struct{}

func (c *phiUnencryptedCheck) ID() string       { return "phi-unencrypted" }
func (c *phiUnencryptedCheck) Name() string     { return "Unencrypted PHI Storage" }
func (c *phiUnencryptedCheck) Article() string  { return "§164.312(a)(2)(iv) HIPAA" }
func (c *phiUnencryptedCheck) Severity() string { return "error" }

var dbOperationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)INSERT\s+INTO`),
	regexp.MustCompile(`(?i)\.Create\(`),
	regexp.MustCompile(`(?i)\.Save\(`),
	regexp.MustCompile(`(?i)\.Insert\(`),
	regexp.MustCompile(`(?i)db\.Exec\(`),
	regexp.MustCompile(`(?i)\.execute\(`),
	regexp.MustCompile(`(?i)UPDATE\s+\w+\s+SET`),
}

func (c *phiUnencryptedCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	extraPatterns := append(scope.Config.PIIFieldPatterns, phiExtraPatterns...)
	scanner := compliance.NewPIIScanner(extraPatterns)
	fields, err := scanner.ScanFiles(ctx, scope)
	if err != nil {
		return nil, err
	}

	// Build map of files with PHI
	phiByFile := make(map[string][]string)
	for _, f := range fields {
		if isPHIField(f.Name) {
			phiByFile[f.File] = append(phiByFile[f.File], f.Name)
		}
	}

	var findings []compliance.Finding

	for file, fieldNames := range phiByFile {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		text := string(content)
		textLower := strings.ToLower(text)

		// Check if file has DB operations
		hasDBOps := false
		for _, pattern := range dbOperationPatterns {
			if pattern.MatchString(text) {
				hasDBOps = true
				break
			}
		}

		if !hasDBOps {
			continue
		}

		// Check for encryption indicators
		hasEncryption := strings.Contains(textLower, "encrypt") ||
			strings.Contains(textLower, "cipher") ||
			strings.Contains(textLower, "aes") ||
			strings.Contains(textLower, "bcrypt") ||
			strings.Contains(textLower, "argon2") ||
			strings.Contains(textLower, "scrypt") ||
			strings.Contains(textLower, "hash")

		if !hasEncryption {
			// Deduplicate field names
			seen := make(map[string]bool)
			unique := make([]string, 0, len(fieldNames))
			for _, n := range fieldNames {
				if !seen[n] {
					unique = append(unique, n)
					seen[n] = true
				}
			}
			if len(unique) > 5 {
				unique = append(unique[:5], "...")
			}

			findings = append(findings, compliance.Finding{
				Severity:   "error",
				Article:    "§164.312(a)(2)(iv) HIPAA",
				File:       file,
				Message:    fmt.Sprintf("Database operations with PHI fields (%s) but no encryption detected", strings.Join(unique, ", ")),
				Suggestion: "HIPAA requires encryption of PHI at rest; implement column-level or application-layer encryption",
				Confidence: 0.70,
				CWE:        "CWE-311",
			})
		}
	}

	return findings, nil
}

// --- minimum-necessary: §164.502(b) — SELECT * on PHI tables ---

type minimumNecessaryCheck struct{}

func (c *minimumNecessaryCheck) ID() string       { return "minimum-necessary" }
func (c *minimumNecessaryCheck) Name() string     { return "Minimum Necessary Violation" }
func (c *minimumNecessaryCheck) Article() string  { return "§164.502(b) HIPAA" }
func (c *minimumNecessaryCheck) Severity() string { return "warning" }

var selectStarPattern = regexp.MustCompile(`(?i)SELECT\s+\*\s+FROM\s+(\w+)`)

// phiTableIndicators are terms suggesting a table/model contains PHI.
var phiTableIndicators = []string{
	"patient", "medical", "health", "diagnosis", "treatment",
	"prescription", "lab", "clinical", "encounter", "admission",
	"discharge", "vital", "allergy", "immunization", "procedure",
	"insurance", "beneficiary", "provider", "claim",
}

func (c *minimumNecessaryCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

			fileScanner := bufio.NewScanner(f)
			lineNum := 0

			for fileScanner.Scan() {
				lineNum++
				line := fileScanner.Text()

				matches := selectStarPattern.FindStringSubmatch(line)
				if len(matches) < 2 {
					continue
				}

				tableName := strings.ToLower(matches[1])
				for _, indicator := range phiTableIndicators {
					if strings.Contains(tableName, indicator) {
						findings = append(findings, compliance.Finding{
							Severity:   "warning",
							Article:    "§164.502(b) HIPAA",
							File:       file,
							StartLine:  lineNum,
							Message:    fmt.Sprintf("SELECT * on PHI-bearing table '%s' violates minimum necessary principle", matches[1]),
							Suggestion: "Select only the specific PHI columns required for the operation; avoid SELECT * on tables containing protected health information",
							Confidence: 0.75,
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}
