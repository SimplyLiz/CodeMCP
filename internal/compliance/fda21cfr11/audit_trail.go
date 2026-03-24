package fda21cfr11

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- missing-audit-trail: §11.10(e) — audit trail required for data modifications ---

type missingAuditTrailCheck struct{}

func (c *missingAuditTrailCheck) ID() string       { return "missing-audit-trail" }
func (c *missingAuditTrailCheck) Name() string     { return "Missing Audit Trail" }
func (c *missingAuditTrailCheck) Article() string   { return "§11.10(e) 21 CFR Part 11" }
func (c *missingAuditTrailCheck) Severity() string  { return "error" }

var dataModificationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(INSERT\s+INTO|UPDATE\s+\w+\s+SET|DELETE\s+FROM)\b`),
	regexp.MustCompile(`(?i)\.(Create|Save|Update|Delete|Destroy|Remove)\s*\(`),
}

var auditTrailPatterns = regexp.MustCompile(`(?i)(audit_trail|audit_log|change_log|history_log|event_log|audit\.log|auditlog|changelog)`)

func (c *missingAuditTrailCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	// First pass: check if the codebase has any audit trail infrastructure
	hasAuditInfra := false
	hasDataModification := false
	var modificationFiles []string

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Skip test files
		if strings.Contains(file, "_test.") || strings.Contains(file, "test_") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		contentStr := string(content)

		if auditTrailPatterns.MatchString(contentStr) {
			hasAuditInfra = true
		}

		for _, pattern := range dataModificationPatterns {
			if pattern.MatchString(contentStr) {
				hasDataModification = true
				modificationFiles = append(modificationFiles, file)
				break
			}
		}
	}

	// If there are data modifications but no audit infrastructure, flag it
	if hasDataModification && !hasAuditInfra {
		for _, file := range modificationFiles {
			findings = append(findings, compliance.Finding{
				CheckID:    "missing-audit-trail",
				Framework:  compliance.FrameworkFDAPart11,
				Severity:   "error",
				Article:    "§11.10(e) 21 CFR Part 11",
				File:       file,
				StartLine:  1,
				Message:    "Data modification operations found without audit trail logging infrastructure",
				Suggestion: "Implement audit trail logging for all data creation, modification, and deletion operations",
				Confidence: 0.70,
			})
		}
	}

	return findings, nil
}

// --- mutable-audit-records: §11.10(e) — audit records must be immutable ---

type mutableAuditRecordsCheck struct{}

func (c *mutableAuditRecordsCheck) ID() string       { return "mutable-audit-records" }
func (c *mutableAuditRecordsCheck) Name() string     { return "Mutable Audit Records" }
func (c *mutableAuditRecordsCheck) Article() string   { return "§11.10(e) 21 CFR Part 11" }
func (c *mutableAuditRecordsCheck) Severity() string  { return "warning" }

// Detect UPDATE/DELETE on audit/log tables
var auditTableMutationPattern = regexp.MustCompile(`(?i)(UPDATE|DELETE\s+FROM)\s+\S*(audit|_log|_history|audit_trail)\b`)

func (c *mutableAuditRecordsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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
		for i, line := range lines {
			if auditTableMutationPattern.MatchString(line) {
				findings = append(findings, compliance.Finding{
					CheckID:    "mutable-audit-records",
					Framework:  compliance.FrameworkFDAPart11,
					Severity:   "warning",
					Article:    "§11.10(e) 21 CFR Part 11",
					File:       file,
					StartLine:  i + 1,
					Message:    "UPDATE or DELETE operation on audit/log table — audit records must be immutable",
					Suggestion: "Audit trail records must be append-only; remove any UPDATE/DELETE operations on audit tables",
					Confidence: 0.85,
				})
			}
		}
	}

	return findings, nil
}
