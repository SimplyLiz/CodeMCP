package eucra

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- missing-sbom: Art. 13(6) — No SBOM generation tooling ---

type missingSBOMCheck struct{}

func (c *missingSBOMCheck) ID() string       { return "missing-sbom" }
func (c *missingSBOMCheck) Name() string     { return "Missing SBOM Generation" }
func (c *missingSBOMCheck) Article() string   { return "Art. 13(6) EU CRA" }
func (c *missingSBOMCheck) Severity() string  { return "warning" }

var sbomIndicators = []string{
	"cyclonedx", "spdx", "syft", "sbom",
	"bom.json", "bom.xml", "sbom.json", "sbom.xml",
	"software-bill-of-materials",
}

var ciFileIndicators = []string{
	".github/workflows", ".gitlab-ci", "Jenkinsfile",
	".circleci", "azure-pipelines", ".travis",
}

func (c *missingSBOMCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	hasSBOM := false

	// Check for SBOM config or tooling references
	for _, file := range scope.Files {
		if ctx.Err() != nil {
			break
		}

		lower := strings.ToLower(file)
		for _, indicator := range sbomIndicators {
			if strings.Contains(lower, indicator) {
				hasSBOM = true
				break
			}
		}
		if hasSBOM {
			break
		}

		// Check CI files for SBOM generation steps
		isCI := false
		for _, ciPattern := range ciFileIndicators {
			if strings.Contains(lower, ciPattern) {
				isCI = true
				break
			}
		}

		if isCI {
			content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
			if err != nil {
				continue
			}
			contentLower := strings.ToLower(string(content))
			for _, indicator := range sbomIndicators {
				if strings.Contains(contentLower, indicator) {
					hasSBOM = true
					break
				}
			}
			if hasSBOM {
				break
			}
		}
	}

	var findings []compliance.Finding
	if !hasSBOM {
		findings = append(findings, compliance.Finding{
			Severity:   "warning",
			Article:    "Art. 13(6) EU CRA",
			Message:    "No SBOM (Software Bill of Materials) generation tooling detected",
			Suggestion: "EU CRA requires machine-readable SBOM in CycloneDX or SPDX format; integrate syft, cyclonedx-cli, or similar into your CI pipeline",
			Confidence: 0.80,
		})
	}

	return findings, nil
}

// --- missing-update-mechanism: Annex I, Part I(3) — No update/migration mechanism ---

type missingUpdateMechanismCheck struct{}

func (c *missingUpdateMechanismCheck) ID() string       { return "missing-update-mechanism" }
func (c *missingUpdateMechanismCheck) Name() string     { return "Missing Update Mechanism" }
func (c *missingUpdateMechanismCheck) Article() string   { return "Annex I, Part I(3) EU CRA" }
func (c *missingUpdateMechanismCheck) Severity() string  { return "info" }

var updateMechanismIndicators = []string{
	"auto_update", "autoupdate", "self_update", "selfupdate",
	"check_update", "checkupdate", "check_version", "checkversion",
	"version_check", "update_available", "upgrade",
	"migration", "migrate", "schema_version", "db_version",
	"alembic", "flyway", "liquibase", "goose", "migrate",
	"/update", "/upgrade", "/version",
}

func (c *missingUpdateMechanismCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	hasUpdateMechanism := false

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			break
		}

		lower := strings.ToLower(file)
		for _, indicator := range updateMechanismIndicators {
			if strings.Contains(lower, indicator) {
				hasUpdateMechanism = true
				break
			}
		}
		if hasUpdateMechanism {
			break
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		contentLower := strings.ToLower(string(content))
		for _, indicator := range updateMechanismIndicators {
			if strings.Contains(contentLower, indicator) {
				hasUpdateMechanism = true
				break
			}
		}
		if hasUpdateMechanism {
			break
		}
	}

	var findings []compliance.Finding
	if !hasUpdateMechanism {
		findings = append(findings, compliance.Finding{
			Severity:   "info",
			Article:    "Annex I, Part I(3) EU CRA",
			Message:    "No software update or migration mechanism detected",
			Suggestion: "EU CRA requires products to support secure updates; implement version checking, auto-update, or migration tooling",
			Confidence: 0.55,
		})
	}

	return findings, nil
}
