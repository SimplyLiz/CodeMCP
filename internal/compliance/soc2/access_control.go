package soc2

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- missing-auth-middleware: CC6.1 — HTTP handlers without auth ---

type missingAuthMiddlewareCheck struct{}

func (c *missingAuthMiddlewareCheck) ID() string       { return "missing-auth-middleware" }
func (c *missingAuthMiddlewareCheck) Name() string     { return "Missing Authentication Middleware" }
func (c *missingAuthMiddlewareCheck) Article() string   { return "CC6.1 SOC 2" }
func (c *missingAuthMiddlewareCheck) Severity() string  { return "error" }

var routeRegistrationPatterns = []*regexp.Regexp{
	// Go
	regexp.MustCompile(`(?i)\.HandleFunc\(\s*["']`),
	regexp.MustCompile(`(?i)\.Handle\(\s*["']`),
	regexp.MustCompile(`(?i)router\.(GET|POST|PUT|DELETE|PATCH)\(`),
	regexp.MustCompile(`(?i)\.Group\(\s*["']`),
	// Node/Express
	regexp.MustCompile(`(?i)app\.(get|post|put|delete|patch)\(\s*["']`),
	regexp.MustCompile(`(?i)router\.(get|post|put|delete|patch)\(\s*["']`),
	// Python/Flask/Django
	regexp.MustCompile(`(?i)@app\.route\(`),
	regexp.MustCompile(`(?i)path\(\s*["']`),
	// Java/Spring
	regexp.MustCompile(`(?i)@(Get|Post|Put|Delete|Patch)Mapping`),
	regexp.MustCompile(`(?i)@RequestMapping`),
}

var authMiddlewareIndicators = []string{
	"auth", "middleware", "jwt", "bearer", "token",
	"session", "authenticated", "authorize", "permission",
	"guard", "interceptor", "login_required", "requires_auth",
}

func (c *missingAuthMiddlewareCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
			continue
		}

		// Skip test fixture directories
		if strings.Contains(file, "testdata/") || strings.Contains(file, "fixtures/") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		text := string(content)
		textLower := strings.ToLower(text)

		// Only flag files that are actual HTTP handler files:
		// must import "net/http" or contain http handler registration patterns.
		if !isHTTPHandlerFile(text) {
			continue
		}

		// Check if this file has route registrations
		hasRoutes := false
		for _, pattern := range routeRegistrationPatterns {
			if pattern.MatchString(text) {
				hasRoutes = true
				break
			}
		}

		if !hasRoutes {
			continue
		}

		// Check if file also has auth middleware references
		hasAuth := false
		for _, indicator := range authMiddlewareIndicators {
			if strings.Contains(textLower, indicator) {
				hasAuth = true
				break
			}
		}

		// Skip if server binds to localhost only — not exposed externally
		if !hasAuth && (strings.Contains(text, "localhost") || strings.Contains(text, "127.0.0.1")) {
			continue
		}

		if !hasAuth {
			// Find the first route line for reporting
			lines := strings.Split(text, "\n")
			for i, line := range lines {
				for _, pattern := range routeRegistrationPatterns {
					if pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "CC6.1 SOC 2",
							File:       file,
							StartLine:  i + 1,
							Message:    "HTTP route registration without visible authentication middleware",
							Suggestion: "Apply authentication middleware to all routes; use middleware wrappers or route groups with auth guards",
							Confidence: 0.60,
						})
						goto nextFile
					}
				}
			}
		}
	nextFile:
	}

	return findings, nil
}

// isHTTPHandlerFile returns true if the file content indicates it's an HTTP handler file.
func isHTTPHandlerFile(text string) bool {
	if strings.Contains(text, `"net/http"`) {
		return true
	}
	// Check for common HTTP handler registration patterns
	if strings.Contains(text, "http.Handle") || strings.Contains(text, "http.HandleFunc") ||
		strings.Contains(text, "router.GET") || strings.Contains(text, "router.POST") ||
		strings.Contains(text, "router.PUT") || strings.Contains(text, "router.DELETE") ||
		strings.Contains(text, "app.get(") || strings.Contains(text, "app.post(") ||
		strings.Contains(text, "@app.route") || strings.Contains(text, "@GetMapping") ||
		strings.Contains(text, "@PostMapping") || strings.Contains(text, "@RequestMapping") {
		return true
	}
	return false
}

// --- insecure-tls-config: CC6.7 — TLS verification disabled ---

type insecureTLSConfigCheck struct{}

func (c *insecureTLSConfigCheck) ID() string       { return "insecure-tls-config" }
func (c *insecureTLSConfigCheck) Name() string     { return "Insecure TLS Configuration" }
func (c *insecureTLSConfigCheck) Article() string   { return "CC6.7 SOC 2" }
func (c *insecureTLSConfigCheck) Severity() string  { return "error" }

var insecureTLSPatterns = []*regexp.Regexp{
	// Go
	regexp.MustCompile(`InsecureSkipVerify\s*:\s*true`),
	// Python
	regexp.MustCompile(`(?i)verify\s*=\s*False`),
	// Node.js
	regexp.MustCompile(`(?i)NODE_TLS_REJECT_UNAUTHORIZED`),
	regexp.MustCompile(`(?i)rejectUnauthorized\s*:\s*false`),
	// Java
	regexp.MustCompile(`(?i)TrustAllCerts`),
	regexp.MustCompile(`(?i)ALLOW_ALL_HOSTNAME_VERIFIER`),
	// Ruby
	regexp.MustCompile(`(?i)verify_mode\s*=\s*OpenSSL::SSL::VERIFY_NONE`),
	// General
	regexp.MustCompile(`(?i)ssl[_-]?verify\s*[:=]\s*(?:false|0|no|off)`),
}

func (c *insecureTLSConfigCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				trimmed := strings.TrimSpace(line)

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
					continue
				}

				for _, pattern := range insecureTLSPatterns {
					if pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "CC6.7 SOC 2",
							File:       file,
							StartLine:  lineNum,
							Message:    "TLS certificate verification disabled — connections are vulnerable to MITM attacks",
							Suggestion: "Enable TLS certificate verification; use proper CA certificates instead of disabling verification",
							Confidence: 0.90,
							CWE:        "CWE-295",
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}
