package nis2

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- unverified-dependencies: Art. 21(2)(d) NIS2 — Dependency lock files ---

type unverifiedDependenciesCheck struct{}

func (c *unverifiedDependenciesCheck) ID() string       { return "unverified-dependencies" }
func (c *unverifiedDependenciesCheck) Name() string     { return "Unverified Dependencies" }
func (c *unverifiedDependenciesCheck) Article() string  { return "Art. 21(2)(d) NIS2" }
func (c *unverifiedDependenciesCheck) Severity() string { return "warning" }

type lockFileMapping struct {
	manifest string
	lockFile string
}

var lockFileMappings = []lockFileMapping{
	{"go.mod", "go.sum"},
	{"package.json", "package-lock.json"},
	{"yarn.lock", "yarn.lock"}, // yarn uses yarn.lock as manifest marker too
	{"Pipfile", "Pipfile.lock"},
	{"Cargo.toml", "Cargo.lock"},
	{"Gemfile", "Gemfile.lock"},
	{"pnpm-lock.yaml", "pnpm-lock.yaml"},
	{"requirements.txt", "requirements.txt"}, // pip has no lock file, just pinning
	{"pyproject.toml", "poetry.lock"},
}

var wildcardVersionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`"[^"]*":\s*"\*"`),          // package.json: "dep": "*"
	regexp.MustCompile(`"[^"]*":\s*"latest"`),      // package.json: "dep": "latest"
	regexp.MustCompile(`>=\s*\d+\.\d+\.\d+,?\s*$`), // open-ended ranges
}

func (c *unverifiedDependenciesCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	manifests := make(map[string]bool) // Set of manifest files found
	lockFiles := make(map[string]bool) // Set of lock files found

	for _, file := range scope.Files {
		base := filepath.Base(file)
		for _, m := range lockFileMappings {
			if base == m.manifest {
				manifests[m.manifest] = true
			}
			if base == m.lockFile {
				lockFiles[m.lockFile] = true
			}
		}
	}

	// Also check repo root for lock files that may not be in scope.Files
	for _, m := range lockFileMappings {
		if manifests[m.manifest] {
			lockPath := filepath.Join(scope.RepoRoot, m.lockFile)
			if _, err := os.Stat(lockPath); err == nil {
				lockFiles[m.lockFile] = true
			}
		}
	}

	// Check each manifest for its corresponding lock file
	for _, m := range lockFileMappings {
		if manifests[m.manifest] && !lockFiles[m.lockFile] && m.manifest != m.lockFile {
			findings = append(findings, compliance.Finding{
				Severity:   "warning",
				Article:    "Art. 21(2)(d) NIS2",
				File:       m.manifest,
				Message:    "Dependency manifest '" + m.manifest + "' found without lock file '" + m.lockFile + "'",
				Suggestion: "Generate and commit a lock file to ensure reproducible builds and verified dependency resolution",
				Confidence: 0.90,
			})
		}
	}

	// Check for wildcard version ranges in package.json
	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		base := filepath.Base(file)
		if base != "package.json" {
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

			for scanner.Scan() {
				lineNum++
				line := scanner.Text()

				for _, p := range wildcardVersionPatterns {
					if p.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "warning",
							Article:    "Art. 21(2)(d) NIS2",
							File:       file,
							StartLine:  lineNum,
							Message:    "Wildcard or unpinned dependency version range detected",
							Suggestion: "Pin dependencies to specific versions or use lock files to ensure supply chain integrity",
							Confidence: 0.80,
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}

// --- missing-integrity-check: Art. 21(2)(d) NIS2 — Checksum verification ---

type missingIntegrityCheckCheck struct{}

func (c *missingIntegrityCheckCheck) ID() string       { return "missing-integrity-check" }
func (c *missingIntegrityCheckCheck) Name() string     { return "Missing Integrity Verification" }
func (c *missingIntegrityCheckCheck) Article() string  { return "Art. 21(2)(d) NIS2" }
func (c *missingIntegrityCheckCheck) Severity() string { return "warning" }

var downloadPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bcurl\b.*https?://`),
	regexp.MustCompile(`(?i)\bwget\b.*https?://`),
	regexp.MustCompile(`(?i)\bInvoke-WebRequest\b`),
	regexp.MustCompile(`(?i)ADD\s+https?://`), // Dockerfile ADD
	regexp.MustCompile(`(?i)RUN\s+.*curl\b`),  // Dockerfile RUN curl
	regexp.MustCompile(`(?i)RUN\s+.*wget\b`),  // Dockerfile RUN wget
}

var integrityPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bsha256sum\b`),
	regexp.MustCompile(`(?i)\bsha512sum\b`),
	regexp.MustCompile(`(?i)\bchecksum\b`),
	regexp.MustCompile(`(?i)\bverify\b.*hash`),
	regexp.MustCompile(`(?i)\bgpg\b.*--verify`),
	regexp.MustCompile(`(?i)\bcosign\b.*verify`),
	regexp.MustCompile(`(?i)\bminisign\b`),
}

func (c *missingIntegrityCheckCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
			continue
		}

		// Focus on Dockerfiles, shell scripts, Makefiles, CI files
		base := strings.ToLower(filepath.Base(file))
		ext := strings.ToLower(filepath.Ext(file))
		isRelevant := base == "dockerfile" || base == "makefile" ||
			ext == ".sh" || ext == ".bash" || ext == ".ps1" ||
			strings.Contains(file, ".github/") || strings.Contains(file, ".gitlab-ci")
		if !isRelevant {
			continue
		}

		hasDownload := false
		hasIntegrity := false
		var downloadLine int

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

				for _, p := range downloadPatterns {
					if p.MatchString(line) {
						hasDownload = true
						if downloadLine == 0 {
							downloadLine = lineNum
						}
					}
				}

				for _, p := range integrityPatterns {
					if p.MatchString(line) {
						hasIntegrity = true
					}
				}
			}
		}()

		if hasDownload && !hasIntegrity {
			findings = append(findings, compliance.Finding{
				Severity:   "warning",
				Article:    "Art. 21(2)(d) NIS2",
				File:       file,
				StartLine:  downloadLine,
				Message:    "External resource download without checksum/signature verification",
				Suggestion: "Verify checksums (sha256sum) or cryptographic signatures for all downloaded resources",
				Confidence: 0.70,
			})
		}
	}

	return findings, nil
}
