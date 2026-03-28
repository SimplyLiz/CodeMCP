package owaspasvs

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- insecure-cookie: V3.4.2/V3.4.3 ASVS — Cookie security flags ---

type insecureCookieCheck struct{}

func (c *insecureCookieCheck) ID() string       { return "insecure-cookie" }
func (c *insecureCookieCheck) Name() string     { return "Insecure Cookie Configuration" }
func (c *insecureCookieCheck) Article() string  { return "V3.4.2/V3.4.3 ASVS" }
func (c *insecureCookieCheck) Severity() string { return "warning" }

var cookieCreationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)Set-Cookie:`),
	regexp.MustCompile(`\bhttp\.Cookie\{`),
	regexp.MustCompile(`(?i)\bcookie\s*\(`),
	regexp.MustCompile(`(?i)\.set_cookie\(`),
	regexp.MustCompile(`(?i)res\.cookie\(`),
	regexp.MustCompile(`(?i)response\.set_cookie\(`),
	regexp.MustCompile(`(?i)setCookie\(`),
	regexp.MustCompile(`(?i)document\.cookie\s*=`),
	regexp.MustCompile(`(?i)Cookie\.Builder`),
	regexp.MustCompile(`(?i)new Cookie\(`),
}

var secureCookieFlags = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bSecure\b`),
	regexp.MustCompile(`(?i)\bsecure\s*[:=]\s*true`),
}

var httpOnlyFlags = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bHttpOnly\b`),
	regexp.MustCompile(`(?i)\bhttponly\s*[:=]\s*true`),
	regexp.MustCompile(`(?i)\bhttp_only\s*[:=]\s*true`),
}

var sameSiteFlags = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bSameSite\b`),
	regexp.MustCompile(`(?i)\bsamesite\b`),
	regexp.MustCompile(`(?i)\bsame_site\b`),
}

func (c *insecureCookieCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

			// Read the full file content for context-aware analysis
			scanner := bufio.NewScanner(f)
			lineNum := 0

			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				trimmed := strings.TrimSpace(line)

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
					continue
				}

				isCookieLine := false
				for _, p := range cookieCreationPatterns {
					if p.MatchString(line) {
						isCookieLine = true
						break
					}
				}

				if !isCookieLine {
					continue
				}

				// Check for missing Secure flag
				hasSecure := false
				for _, p := range secureCookieFlags {
					if p.MatchString(line) {
						hasSecure = true
						break
					}
				}

				hasHttpOnly := false
				for _, p := range httpOnlyFlags {
					if p.MatchString(line) {
						hasHttpOnly = true
						break
					}
				}

				hasSameSite := false
				for _, p := range sameSiteFlags {
					if p.MatchString(line) {
						hasSameSite = true
						break
					}
				}

				// For Go http.Cookie{}, flags are typically on separate lines — lower confidence
				isMultiLineStruct := strings.HasSuffix(trimmed, "{") || strings.Contains(line, "http.Cookie{")
				confidence := 0.80
				if isMultiLineStruct {
					confidence = 0.60 // Flags may be on subsequent lines
				}

				if !hasSecure {
					findings = append(findings, compliance.Finding{
						Severity:   "warning",
						Article:    "V3.4.2/V3.4.3 ASVS",
						File:       file,
						StartLine:  lineNum,
						Message:    "Cookie creation without Secure flag — cookie may be sent over unencrypted connections",
						Suggestion: "Set the Secure flag on all cookies to prevent transmission over HTTP",
						Confidence: confidence,
						CWE:        "CWE-614",
					})
				}

				if !hasHttpOnly && !isMultiLineStruct {
					findings = append(findings, compliance.Finding{
						Severity:   "warning",
						Article:    "V3.4.2/V3.4.3 ASVS",
						File:       file,
						StartLine:  lineNum,
						Message:    "Cookie creation without HttpOnly flag — cookie accessible via JavaScript",
						Suggestion: "Set the HttpOnly flag on session cookies to prevent XSS-based cookie theft",
						Confidence: confidence,
						CWE:        "CWE-614",
					})
				}

				if !hasSameSite && !isMultiLineStruct {
					findings = append(findings, compliance.Finding{
						Severity:   "warning",
						Article:    "V3.4.2/V3.4.3 ASVS",
						File:       file,
						StartLine:  lineNum,
						Message:    "Cookie creation without SameSite attribute — vulnerable to CSRF attacks",
						Suggestion: "Set SameSite=Lax or SameSite=Strict on cookies to mitigate CSRF",
						Confidence: confidence,
						CWE:        "CWE-614",
					})
				}
			}
		}()
	}

	return findings, nil
}
