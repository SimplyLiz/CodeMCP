package sbom

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- missing-sbom-generation: EO 14028 §4(e) — SBOM generation ---

type missingSBOMGenerationCheck struct{}

func (c *missingSBOMGenerationCheck) ID() string       { return "missing-sbom-generation" }
func (c *missingSBOMGenerationCheck) Name() string     { return "Missing SBOM Generation" }
func (c *missingSBOMGenerationCheck) Article() string   { return "EO 14028 §4(e)" }
func (c *missingSBOMGenerationCheck) Severity() string  { return "warning" }

var sbomToolPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bcyclonedx\b`),
	regexp.MustCompile(`(?i)\bspdx\b`),
	regexp.MustCompile(`(?i)\bsyft\b`),
	regexp.MustCompile(`(?i)\btrivy\b.*\bsbom\b`),
	regexp.MustCompile(`(?i)\bsbom[_\-]?tool\b`),
	regexp.MustCompile(`(?i)\bsbom[_\-]?generate\b`),
	regexp.MustCompile(`(?i)\bgenerate[_\-]?sbom\b`),
	regexp.MustCompile(`(?i)\bcdxgen\b`),
}

var sbomFilePatterns = []string{
	"bom.json", "bom.xml",
	"sbom.json", "sbom.xml",
	".spdx", ".spdx.json",
	"cyclonedx.json", "cyclonedx.xml",
}

var sbomCIFiles = []string{
	".github/workflows",
	".gitlab-ci",
	"Jenkinsfile",
	".circleci",
	"Makefile",
	"makefile",
	"Taskfile",
}

func (c *missingSBOMGenerationCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	hasSBOMFile := false
	hasSBOMTool := false

	// Check for SBOM artifact files
	for _, file := range scope.Files {
		base := strings.ToLower(filepath.Base(file))
		for _, pattern := range sbomFilePatterns {
			if base == pattern || strings.HasSuffix(base, pattern) {
				hasSBOMFile = true
				break
			}
		}
	}

	// Check for SBOM tool references in CI/CD and build files
	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		isRelevant := false
		for _, ciFile := range sbomCIFiles {
			if strings.Contains(file, ciFile) {
				isRelevant = true
				break
			}
		}
		ext := filepath.Ext(file)
		if ext == ".sh" || ext == ".bash" || ext == ".ps1" {
			isRelevant = true
		}

		if !isRelevant {
			continue
		}

		f, err := os.Open(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()

			for _, p := range sbomToolPatterns {
				if p.MatchString(line) {
					hasSBOMTool = true
				}
			}
		}
		f.Close()
	}

	if !hasSBOMFile && !hasSBOMTool {
		return []compliance.Finding{
			{
				Severity:   "warning",
				Article:    "EO 14028 §4(e)",
				File:       "",
				Message:    "No SBOM generation tooling or SBOM artifacts found in the project",
				Suggestion: "Integrate SBOM generation (CycloneDX, SPDX, Syft) into your build/CI pipeline per Executive Order 14028",
				Confidence: 0.75,
			},
		}, nil
	}

	return nil, nil
}

// --- missing-lock-file: SLSA Level 1 — Dependency lock files ---

type missingLockFileCheck struct{}

func (c *missingLockFileCheck) ID() string       { return "missing-lock-file" }
func (c *missingLockFileCheck) Name() string     { return "Missing Dependency Lock File" }
func (c *missingLockFileCheck) Article() string   { return "SLSA Level 1" }
func (c *missingLockFileCheck) Severity() string  { return "warning" }

type manifestLockPair struct {
	manifest string
	lockFile string
}

var manifestLockPairs = []manifestLockPair{
	{"go.mod", "go.sum"},
	{"package.json", "package-lock.json"},
	{"Pipfile", "Pipfile.lock"},
	{"pyproject.toml", "poetry.lock"},
	{"Cargo.toml", "Cargo.lock"},
	{"Gemfile", "Gemfile.lock"},
}

// Alternative lock files for package.json (yarn/pnpm)
var altJSLockFiles = []string{"yarn.lock", "pnpm-lock.yaml"}

func (c *missingLockFileCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	manifests := make(map[string]bool)
	lockFilesFound := make(map[string]bool)

	for _, file := range scope.Files {
		base := filepath.Base(file)
		manifests[base] = true
		lockFilesFound[base] = true
	}

	// Also check repo root
	for _, pair := range manifestLockPairs {
		lockPath := filepath.Join(scope.RepoRoot, pair.lockFile)
		if _, err := os.Stat(lockPath); err == nil {
			lockFilesFound[pair.lockFile] = true
		}
	}
	for _, alt := range altJSLockFiles {
		altPath := filepath.Join(scope.RepoRoot, alt)
		if _, err := os.Stat(altPath); err == nil {
			lockFilesFound[alt] = true
		}
	}

	for _, pair := range manifestLockPairs {
		if !manifests[pair.manifest] {
			continue
		}

		hasLock := lockFilesFound[pair.lockFile]

		// For package.json, also check yarn.lock / pnpm-lock.yaml
		if pair.manifest == "package.json" && !hasLock {
			for _, alt := range altJSLockFiles {
				if lockFilesFound[alt] {
					hasLock = true
					break
				}
			}
		}

		if !hasLock {
			findings = append(findings, compliance.Finding{
				Severity:   "warning",
				Article:    "SLSA Level 1",
				File:       pair.manifest,
				Message:    "Dependency manifest '" + pair.manifest + "' without lock file '" + pair.lockFile + "'",
				Suggestion: "Generate and commit a lock file for reproducible builds and supply chain integrity",
				Confidence: 0.90,
			})
		}
	}

	return findings, nil
}
