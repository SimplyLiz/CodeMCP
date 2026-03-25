package fda21cfr11

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- missing-authority-check: §11.10(d) — authority checks required ---

type missingAuthorityCheckCheck struct{}

func (c *missingAuthorityCheckCheck) ID() string       { return "missing-authority-check" }
func (c *missingAuthorityCheckCheck) Name() string     { return "Missing Authority Check" }
func (c *missingAuthorityCheckCheck) Article() string   { return "§11.10(d) 21 CFR Part 11" }
func (c *missingAuthorityCheckCheck) Severity() string  { return "warning" }

var modificationCallPattern = regexp.MustCompile(`(?i)\.(save|create|update|delete|destroy|remove|put|post)\s*\(`)
var authCheckPattern = regexp.MustCompile(`(?i)(auth|permission|role|authorize|authorized|is_admin|has_permission|check_access|access_control|rbac|acl)`)

func (c *missingAuthorityCheckCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, "test_") {
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
			// Track if we've seen an auth check in the current function context
			authCheckSeen := false
			braceDepth := 0

			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				trimmed := strings.TrimSpace(line)

				if strings.HasPrefix(trimmed, "//") {
					continue
				}

				prevDepth := braceDepth
				braceDepth += strings.Count(line, "{") - strings.Count(line, "}")

				// Reset auth tracking at function boundaries
				if braceDepth <= 0 && prevDepth > 0 {
					authCheckSeen = false
				}

				// Track auth checks
				if authCheckPattern.MatchString(line) {
					authCheckSeen = true
				}

				// Detect modification calls without preceding auth check
				if modificationCallPattern.MatchString(line) && !authCheckSeen {
					findings = append(findings, compliance.Finding{
						CheckID:    "missing-authority-check",
						Framework:  compliance.FrameworkFDAPart11,
						Severity:   "warning",
						Article:    "§11.10(d) 21 CFR Part 11",
						File:       file,
						StartLine:  lineNum,
						Message:    "Data modification operation without preceding authorization check",
						Suggestion: "Add authorization/permission check before data modification operations",
						Confidence: 0.55,
					})
				}
			}
		}()
	}

	return findings, nil
}

// --- missing-esignature: §11.50 — electronic signatures for regulated records ---

type missingESignatureCheck struct{}

func (c *missingESignatureCheck) ID() string       { return "missing-esignature" }
func (c *missingESignatureCheck) Name() string     { return "Missing Electronic Signature Support" }
func (c *missingESignatureCheck) Article() string   { return "§11.50 21 CFR Part 11" }
func (c *missingESignatureCheck) Severity() string  { return "info" }

var approvalWorkflowPattern = regexp.MustCompile(`(?i)(approval|approve|approved|review|workflow|submit_for_review|pending_approval|approval_status)`)
var eSignaturePattern = regexp.MustCompile(`(?i)(e_signature|esignature|digital_signature|sign_off|signoff|signer|signatory|electronic_signature)`)

func (c *missingESignatureCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	hasApprovalWorkflow := false
	hasESignature := false
	var approvalFiles []string

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

		contentStr := string(content)

		if approvalWorkflowPattern.MatchString(contentStr) {
			hasApprovalWorkflow = true
			approvalFiles = append(approvalFiles, file)
		}

		if eSignaturePattern.MatchString(contentStr) {
			hasESignature = true
		}
	}

	// If approval workflows exist but no e-signature patterns, flag it
	if hasApprovalWorkflow && !hasESignature {
		// Only flag the first few files to avoid noise
		maxFiles := 3
		if len(approvalFiles) < maxFiles {
			maxFiles = len(approvalFiles)
		}
		for _, file := range approvalFiles[:maxFiles] {
			findings = append(findings, compliance.Finding{
				CheckID:    "missing-esignature",
				Framework:  compliance.FrameworkFDAPart11,
				Severity:   "info",
				Article:    "§11.50 21 CFR Part 11",
				File:       file,
				StartLine:  1,
				Message:    "Approval workflow found without electronic signature implementation",
				Suggestion: "Implement electronic signatures (21 CFR Part 11 compliant) for regulated record approvals",
				Confidence: 0.50,
			})
		}
	}

	return findings, nil
}
