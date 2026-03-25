package eucra

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- insecure-defaults: Art. 13 — Default passwords and insecure defaults ---

type insecureDefaultsCheck struct{}

func (c *insecureDefaultsCheck) ID() string       { return "insecure-defaults" }
func (c *insecureDefaultsCheck) Name() string     { return "Insecure Default Configuration" }
func (c *insecureDefaultsCheck) Article() string   { return "Art. 13 EU CRA" }
func (c *insecureDefaultsCheck) Severity() string  { return "error" }

var insecureDefaultPatterns = []*regexp.Regexp{
	// Default passwords
	regexp.MustCompile(`(?i)default.*(password|passwd|pwd|secret)\s*[:=]\s*["'][^"']+["']`),
	regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*["'](admin|password|root|default|123456|changeme)["']`),
	// Overly permissive defaults
	regexp.MustCompile(`(?i)0\.0\.0\.0`),
	// Wildcard CORS
	regexp.MustCompile(`(?i)Access-Control-Allow-Origin.*\*`),
	regexp.MustCompile(`(?i)AllowOrigins.*\*`),
	regexp.MustCompile(`(?i)cors.*origin.*\*`),
	// Debug mode as default
	regexp.MustCompile(`(?i)debug\s*[:=]\s*true`),
}

// Patterns that make 0.0.0.0 acceptable (e.g., in comments, docs, or env lookups).
var bindExclusions = []string{
	"//", "#", "env", "flag", "config", "getenv", "os.environ",
}

func (c *insecureDefaultsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		lower := strings.ToLower(file)
		if strings.Contains(lower, "_test.") || strings.Contains(lower, ".test.") ||
			strings.Contains(lower, "example") || strings.Contains(lower, "sample") {
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
				trimmed := strings.TrimSpace(line)

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
					continue
				}

				for _, pattern := range insecureDefaultPatterns {
					if pattern.MatchString(line) {
						// Special handling for 0.0.0.0 — skip if it's in env/config context
						if strings.Contains(line, "0.0.0.0") {
							excluded := false
							lowerLine := strings.ToLower(line)
							for _, excl := range bindExclusions {
								if strings.Contains(lowerLine, excl) {
									excluded = true
									break
								}
							}
							if excluded {
								continue
							}
						}

						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "Art. 13 EU CRA",
							File:       file,
							StartLine:  lineNum,
							Message:    "Insecure default configuration detected (default credential, permissive binding, or debug mode)",
							Suggestion: "EU CRA requires secure-by-default configuration; remove default credentials and restrict default network exposure",
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

// --- unnecessary-attack-surface: Annex I, Part I(1) — Unnecessary exposure ---

type unnecessaryAttackSurfaceCheck struct{}

func (c *unnecessaryAttackSurfaceCheck) ID() string       { return "unnecessary-attack-surface" }
func (c *unnecessaryAttackSurfaceCheck) Name() string     { return "Unnecessary Attack Surface" }
func (c *unnecessaryAttackSurfaceCheck) Article() string   { return "Annex I, Part I(1) EU CRA" }
func (c *unnecessaryAttackSurfaceCheck) Severity() string  { return "warning" }

var attackSurfacePatterns = []*regexp.Regexp{
	// Admin/debug endpoints without restriction
	regexp.MustCompile(`(?i)(["']/admin|["']/debug|["']/internal|["']/metrics|["']/pprof|["']/healthz)`),
	// Multiple port listeners
	regexp.MustCompile(`(?i)\.Listen\(\s*["']:?\d+["']`),
	regexp.MustCompile(`(?i)listen\s*[:=]\s*["']:?\d+`),
	// Unrestricted file serving
	regexp.MustCompile(`(?i)FileServer\(`),
	regexp.MustCompile(`(?i)static\s*\(\s*["']/`),
	regexp.MustCompile(`(?i)express\.static\(`),
}

var restrictionIndicators = []string{
	"auth", "middleware", "restrict", "internal", "private",
	"localhost", "127.0.0.1", "allowlist", "whitelist",
}

func (c *unnecessaryAttackSurfaceCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

			scanner := bufio.NewScanner(f)
			lineNum := 0
			// Track context: look at surrounding lines for restriction indicators
			var recentLines []string

			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				trimmed := strings.TrimSpace(line)

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
					continue
				}

				recentLines = append(recentLines, strings.ToLower(line))
				if len(recentLines) > 10 {
					recentLines = recentLines[1:]
				}

				for _, pattern := range attackSurfacePatterns {
					if pattern.MatchString(line) {
						// Check if there are restriction indicators nearby
						context := strings.Join(recentLines, " ")
						hasRestriction := false
						for _, indicator := range restrictionIndicators {
							if strings.Contains(context, indicator) {
								hasRestriction = true
								break
							}
						}

						if !hasRestriction {
							findings = append(findings, compliance.Finding{
								Severity:   "warning",
								Article:    "Annex I, Part I(1) EU CRA",
								File:       file,
								StartLine:  lineNum,
								Message:    "Potentially unnecessary attack surface: exposed endpoint or service without visible access restriction",
								Suggestion: "Minimize attack surface: restrict admin/debug endpoints, limit network listeners, and apply authentication to all exposed services",
								Confidence: 0.55,
							})
						}
						break
					}
				}
			}
		}()
	}

	return findings, nil
}
