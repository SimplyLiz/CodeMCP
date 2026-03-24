package pcidss

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- weak-password-policy: Req 8.3.6 — Password minimum length < 12 ---

type weakPasswordPolicyCheck struct{}

func (c *weakPasswordPolicyCheck) ID() string       { return "weak-password-policy" }
func (c *weakPasswordPolicyCheck) Name() string     { return "Weak Password Policy" }
func (c *weakPasswordPolicyCheck) Article() string   { return "Req 8.3.6 PCI DSS 4.0" }
func (c *weakPasswordPolicyCheck) Severity() string  { return "warning" }

var weakPasswordPatterns = []*regexp.Regexp{
	// Password min length constants or checks < 12
	regexp.MustCompile(`(?i)(password|passwd|pwd).*min.*len.*[=<:]\s*([1-9]|1[01])\b`),
	regexp.MustCompile(`(?i)min.*(password|passwd|pwd).*len.*[=:]\s*([1-9]|1[01])\b`),
	// Regex patterns for password validation with low length
	regexp.MustCompile(`(?i)(password|passwd).*\.\{([1-9]|1[01]),`),
	// Validation constants
	regexp.MustCompile(`(?i)(MIN_PASSWORD_LENGTH|PASSWORD_MIN_LEN|MINIMUM_PASSWORD)\s*[=:]\s*([1-9]|1[01])\b`),
	regexp.MustCompile(`(?i)len\((password|passwd|pwd)\)\s*(<|>=?\s*)([1-9]|1[01])\b`),
}

func (c *weakPasswordPolicyCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

			for _, pattern := range weakPasswordPatterns {
				if pattern.MatchString(line) {
					findings = append(findings, compliance.Finding{
						Severity:   "warning",
						Article:    "Req 8.3.6 PCI DSS 4.0",
						File:       file,
						StartLine:  lineNum,
						Message:    "Password policy with minimum length below 12 characters detected",
						Suggestion: "PCI DSS 4.0 requires minimum 12-character passwords; update password validation accordingly",
						Confidence: 0.70,
					})
					break
				}
			}
		}
		f.Close()
	}

	return findings, nil
}

// --- hardcoded-credentials: Req 8.6.2 — Hardcoded passwords/keys ---

type hardcodedCredentialsCheck struct{}

func (c *hardcodedCredentialsCheck) ID() string       { return "hardcoded-credentials" }
func (c *hardcodedCredentialsCheck) Name() string     { return "Hardcoded Credentials" }
func (c *hardcodedCredentialsCheck) Article() string   { return "Req 8.6.2 PCI DSS 4.0" }
func (c *hardcodedCredentialsCheck) Severity() string  { return "error" }

var pciSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*["'][\w\-]{16,}`),
	regexp.MustCompile(`(?i)(secret[_-]?key|secretkey)\s*[:=]\s*["'][\w\-]{16,}`),
	regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*["'][^"']{8,}`),
	regexp.MustCompile(`(?i)(access[_-]?token|auth[_-]?token)\s*[:=]\s*["'][\w\-\.]{20,}`),
	regexp.MustCompile(`(?i)(private[_-]?key)\s*[:=]\s*["']`),
	regexp.MustCompile(`(?i)-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----`),
	regexp.MustCompile(`(?i)(aws[_-]?secret|aws[_-]?access)\s*[:=]\s*["']`),
	regexp.MustCompile(`(?i)(database[_-]?url|db[_-]?url|connection[_-]?string)\s*[:=]\s*["'][^"']*[:@]`),
}

func (c *hardcodedCredentialsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		lower := strings.ToLower(file)
		if strings.Contains(lower, "_test.") || strings.Contains(lower, ".test.") ||
			strings.Contains(lower, "example") || strings.Contains(lower, "sample") ||
			strings.Contains(lower, "fixture") || strings.Contains(lower, "mock") {
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

			for _, pattern := range pciSecretPatterns {
				if pattern.MatchString(line) {
					findings = append(findings, compliance.Finding{
						Severity:   "error",
						Article:    "Req 8.6.2 PCI DSS 4.0",
						File:       file,
						StartLine:  lineNum,
						Message:    "Potential hardcoded credential/secret detected",
						Suggestion: "Use environment variables, secret managers (Vault, AWS Secrets Manager), or encrypted configuration",
						Confidence: 0.80,
						CWE:        "CWE-798",
					})
					break
				}
			}
		}
		f.Close()
	}

	return findings, nil
}
