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

// --- unpinned-dependencies: SLSA Level 2 — Version pinning ---

type unpinnedDependenciesCheck struct{}

func (c *unpinnedDependenciesCheck) ID() string       { return "unpinned-dependencies" }
func (c *unpinnedDependenciesCheck) Name() string     { return "Unpinned Dependency Versions" }
func (c *unpinnedDependenciesCheck) Article() string   { return "SLSA Level 2" }
func (c *unpinnedDependenciesCheck) Severity() string  { return "warning" }

var unpinnedPackageJSONPatterns = []*regexp.Regexp{
	regexp.MustCompile(`"[^"]+"\s*:\s*"\^`),       // "dep": "^1.0.0"
	regexp.MustCompile(`"[^"]+"\s*:\s*"~`),         // "dep": "~1.0.0"
	regexp.MustCompile(`"[^"]+"\s*:\s*"\*"`),       // "dep": "*"
	regexp.MustCompile(`"[^"]+"\s*:\s*"latest"`),   // "dep": "latest"
	regexp.MustCompile(`"[^"]+"\s*:\s*">=`),        // "dep": ">=1.0.0"
}

var unpinnedRequirementsPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^[a-zA-Z][\w\-]*\s*$`),                    // package without any version
	regexp.MustCompile(`^[a-zA-Z][\w\-]*\s*>=`),                   // package>=1.0
	regexp.MustCompile(`^[a-zA-Z][\w\-]*\s*~=`),                   // package~=1.0
}

var goModReplaceLatest = regexp.MustCompile(`(?i)replace\s+.*\s+=>\s+.*\blatest\b`)

func (c *unpinnedDependenciesCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		base := filepath.Base(file)

		switch base {
		case "package.json":
			fs := c.checkPackageJSON(scope.RepoRoot, file)
			findings = append(findings, fs...)
		case "requirements.txt":
			fs := c.checkRequirements(scope.RepoRoot, file)
			findings = append(findings, fs...)
		case "go.mod":
			fs := c.checkGoMod(scope.RepoRoot, file)
			findings = append(findings, fs...)
		}
	}

	return findings, nil
}

func (c *unpinnedDependenciesCheck) checkPackageJSON(repoRoot, file string) []compliance.Finding {
	var findings []compliance.Finding

	f, err := os.Open(filepath.Join(repoRoot, file))
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	inDeps := false

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.Contains(trimmed, "dependencies") && strings.Contains(trimmed, "{") {
			inDeps = true
			continue
		}
		if inDeps && trimmed == "}" {
			inDeps = false
			continue
		}

		if !inDeps {
			continue
		}

		for _, p := range unpinnedPackageJSONPatterns {
			if p.MatchString(line) {
				findings = append(findings, compliance.Finding{
					Severity:   "warning",
					Article:    "SLSA Level 2",
					File:       file,
					StartLine:  lineNum,
					Message:    "Unpinned dependency version range in package.json",
					Suggestion: "Pin dependencies to exact versions (remove ^, ~, *, >= prefixes) for reproducible builds",
					Confidence: 0.80,
				})
				break
			}
		}
	}

	return findings
}

func (c *unpinnedDependenciesCheck) checkRequirements(repoRoot, file string) []compliance.Finding {
	var findings []compliance.Finding

	f, err := os.Open(filepath.Join(repoRoot, file))
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "-") {
			continue
		}

		// Check if version is pinned with ==
		if strings.Contains(trimmed, "==") {
			continue
		}

		for _, p := range unpinnedRequirementsPatterns {
			if p.MatchString(trimmed) {
				findings = append(findings, compliance.Finding{
					Severity:   "warning",
					Article:    "SLSA Level 2",
					File:       file,
					StartLine:  lineNum,
					Message:    "Unpinned dependency in requirements.txt",
					Suggestion: "Pin dependencies to exact versions using == (e.g., package==1.2.3)",
					Confidence: 0.80,
				})
				break
			}
		}
	}

	return findings
}

func (c *unpinnedDependenciesCheck) checkGoMod(repoRoot, file string) []compliance.Finding {
	var findings []compliance.Finding

	f, err := os.Open(filepath.Join(repoRoot, file))
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if goModReplaceLatest.MatchString(line) {
			findings = append(findings, compliance.Finding{
				Severity:   "warning",
				Article:    "SLSA Level 2",
				File:       file,
				StartLine:  lineNum,
				Message:    "Go module replace directive pointing to 'latest'",
				Suggestion: "Pin replace directives to specific versions or commit hashes",
				Confidence: 0.85,
			})
		}
	}

	return findings
}

// --- missing-provenance: SLSA Level 2 — Build provenance ---

type missingProvenanceCheck struct{}

func (c *missingProvenanceCheck) ID() string       { return "missing-provenance" }
func (c *missingProvenanceCheck) Name() string     { return "Missing Build Provenance" }
func (c *missingProvenanceCheck) Article() string   { return "SLSA Level 2" }
func (c *missingProvenanceCheck) Severity() string  { return "info" }

var provenancePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bslsa[_\-]?github[_\-]?generator\b`),
	regexp.MustCompile(`(?i)\bslsa[_\-]?provenance\b`),
	regexp.MustCompile(`(?i)\bin[_\-]?toto\b`),
	regexp.MustCompile(`(?i)\bsigstore\b`),
	regexp.MustCompile(`(?i)\bcosign\b`),
	regexp.MustCompile(`(?i)\brekor\b`),
	regexp.MustCompile(`(?i)\bfulcio\b`),
	regexp.MustCompile(`(?i)\bprovenance\b`),
	regexp.MustCompile(`(?i)\battestation\b`),
}

func (c *missingProvenanceCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	hasProvenance := false

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// Check CI/CD files and build configs
		isRelevant := false
		for _, ciFile := range sbomCIFiles {
			if strings.Contains(file, ciFile) {
				isRelevant = true
				break
			}
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

			for _, p := range provenancePatterns {
				if p.MatchString(line) {
					hasProvenance = true
				}
			}
		}
		f.Close()
	}

	if !hasProvenance {
		return []compliance.Finding{
			{
				Severity:   "info",
				Article:    "SLSA Level 2",
				File:       "",
				Message:    "No build provenance generation found in CI/CD configuration",
				Suggestion: "Integrate build provenance tools (SLSA GitHub generator, sigstore/cosign, in-toto) for supply chain verification",
				Confidence: 0.60,
			},
		}, nil
	}

	return nil, nil
}

// --- unsigned-commits: SLSA Level 2 — Commit signing ---

type unsignedCommitsCheck struct{}

func (c *unsignedCommitsCheck) ID() string       { return "unsigned-commits" }
func (c *unsignedCommitsCheck) Name() string     { return "Unsigned Commits Policy" }
func (c *unsignedCommitsCheck) Article() string   { return "SLSA Level 2" }
func (c *unsignedCommitsCheck) Severity() string  { return "info" }

var commitSigningPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)commit\.gpgsign`),
	regexp.MustCompile(`(?i)gpgsign\s*=\s*true`),
	regexp.MustCompile(`(?i)--verify-signatures`),
	regexp.MustCompile(`(?i)require[_\-]?signed[_\-]?commits`),
	regexp.MustCompile(`(?i)signed[_\-]?commits`),
	regexp.MustCompile(`(?i)commit[_\-]?signing`),
}

func (c *unsignedCommitsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	hasSigningPolicy := false

	// Check .gitconfig in repo root
	gitconfigPath := filepath.Join(scope.RepoRoot, ".gitconfig")
	if content, err := os.ReadFile(gitconfigPath); err == nil {
		for _, p := range commitSigningPatterns {
			if p.Match(content) {
				hasSigningPolicy = true
				break
			}
		}
	}

	// Check CI/CD files for verification
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
		base := filepath.Base(file)
		if base == ".gitconfig" || base == ".gitattributes" || strings.Contains(file, ".github/") {
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

			for _, p := range commitSigningPatterns {
				if p.MatchString(line) {
					hasSigningPolicy = true
				}
			}
		}
		f.Close()
	}

	if !hasSigningPolicy {
		return []compliance.Finding{
			{
				Severity:   "info",
				Article:    "SLSA Level 2",
				File:       "",
				Message:    "No commit signing enforcement found in repository configuration",
				Suggestion: "Enable commit signing (commit.gpgsign=true) and verify signatures in CI/CD for source integrity",
				Confidence: 0.55,
			},
		}, nil
	}

	return nil, nil
}
