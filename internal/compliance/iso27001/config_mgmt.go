package iso27001

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- hardcoded-config: A.8.9 — Hardcoded configuration values ---

type hardcodedConfigCheck struct{}

func (c *hardcodedConfigCheck) ID() string       { return "hardcoded-config" }
func (c *hardcodedConfigCheck) Name() string     { return "Hardcoded Configuration" }
func (c *hardcodedConfigCheck) Article() string   { return "A.8.9 ISO 27001:2022" }
func (c *hardcodedConfigCheck) Severity() string  { return "warning" }

var hardcodedConfigPatterns = []*regexp.Regexp{
	// Hardcoded hostnames/IPs (not localhost)
	regexp.MustCompile(`["'](?:https?://)?(?:\d{1,3}\.){3}\d{1,3}(?::\d+)?["']`),
	// Hardcoded non-standard ports
	regexp.MustCompile(`(?i)(port|listen)\s*[:=]\s*["']?\d{4,5}["']?`),
	// Hardcoded database connection strings
	regexp.MustCompile(`(?i)(postgres|mysql|mongodb|redis)://[^"'\s]+`),
}

// Excluded patterns: localhost, 127.0.0.1, 0.0.0.0, test fixtures
var configExclusions = []string{
	"localhost", "127.0.0.1", "0.0.0.0", "::1",
	"example.com", "example.org",
}

func (c *hardcodedConfigCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Skip test files and config files
		lower := strings.ToLower(file)
		if strings.Contains(lower, "_test.") || strings.Contains(lower, ".test.") ||
			strings.Contains(lower, "config") || strings.Contains(lower, "fixture") ||
			strings.Contains(lower, "example") || strings.Contains(lower, "mock") ||
			strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".yaml") ||
			strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".toml") {
			continue
		}

		f, err := os.Open(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		lineNum := 0

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
				continue
			}

			for _, pattern := range hardcodedConfigPatterns {
				if pattern.MatchString(line) {
					// Check exclusions
					excluded := false
					for _, excl := range configExclusions {
						if strings.Contains(strings.ToLower(line), excl) {
							excluded = true
							break
						}
					}
					if excluded {
						continue
					}

					findings = append(findings, compliance.Finding{
						Severity:   "warning",
						Article:    "A.8.9 ISO 27001:2022",
						File:       file,
						StartLine:  lineNum,
						Message:    "Hardcoded configuration value detected (hostname, port, or connection string)",
						Suggestion: "Use environment variables or configuration files for environment-specific values",
						Confidence: 0.65,
					})
					break
				}
			}
		}
		f.Close()
	}

	return findings, nil
}

// --- missing-tls: A.8.20 — Unencrypted network connections ---

type missingTLSCheck struct{}

func (c *missingTLSCheck) ID() string       { return "missing-tls" }
func (c *missingTLSCheck) Name() string     { return "Missing TLS Encryption" }
func (c *missingTLSCheck) Article() string   { return "A.8.20 ISO 27001:2022" }
func (c *missingTLSCheck) Severity() string  { return "error" }

var httpPatterns = []*regexp.Regexp{
	regexp.MustCompile(`http://[^/\s"']+`),
}

func (c *missingTLSCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
			continue
		}

		f, err := os.Open(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		lineNum := 0

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
				continue
			}

			if strings.Contains(line, "http://") {
				// Exclude localhost/loopback
				lower := strings.ToLower(line)
				if strings.Contains(lower, "http://localhost") || strings.Contains(lower, "http://127.0.0.1") ||
					strings.Contains(lower, "http://0.0.0.0") || strings.Contains(lower, "http://[::1]") ||
					strings.Contains(lower, "http://example") {
					continue
				}

				findings = append(findings, compliance.Finding{
					Severity:   "error",
					Article:    "A.8.20 ISO 27001:2022",
					File:       file,
					StartLine:  lineNum,
					Message:    "Unencrypted HTTP connection detected — use TLS for data in transit",
					Suggestion: "Replace http:// with https:// or use TLS configuration",
					Confidence: 0.80,
					CWE:        "CWE-319",
				})
			}
		}
		f.Close()
	}

	return findings, nil
}

// --- cors-wildcard: A.8.27 — CORS wildcard on authenticated endpoints ---

type corsWildcardCheck struct{}

func (c *corsWildcardCheck) ID() string       { return "cors-wildcard" }
func (c *corsWildcardCheck) Name() string     { return "CORS Wildcard Origin" }
func (c *corsWildcardCheck) Article() string   { return "A.8.27 ISO 27001:2022" }
func (c *corsWildcardCheck) Severity() string  { return "warning" }

var corsWildcardPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)Access-Control-Allow-Origin.*\*`),
	regexp.MustCompile(`(?i)AllowOrigins.*\*`),
	regexp.MustCompile(`(?i)cors.*origin.*\*`),
	regexp.MustCompile(`(?i)allow_origins.*\[["']\*["']\]`),
}

func (c *corsWildcardCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		f, err := os.Open(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		lineNum := 0

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			for _, pattern := range corsWildcardPatterns {
				if pattern.MatchString(line) {
					findings = append(findings, compliance.Finding{
						Severity:   "warning",
						Article:    "A.8.27 ISO 27001:2022",
						File:       file,
						StartLine:  lineNum,
						Message:    "CORS wildcard origin (*) allows any website to make requests",
						Suggestion: "Restrict CORS origins to specific trusted domains",
						Confidence: 0.85,
					})
					break
				}
			}
		}
		f.Close()
	}

	return findings, nil
}
