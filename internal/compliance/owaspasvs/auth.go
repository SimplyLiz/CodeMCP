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

// --- weak-password-hash: V2.4.1 ASVS — Password storage algorithms ---

type weakPasswordHashCheck struct{}

func (c *weakPasswordHashCheck) ID() string       { return "weak-password-hash" }
func (c *weakPasswordHashCheck) Name() string     { return "Weak Password Hashing Algorithm" }
func (c *weakPasswordHashCheck) Article() string   { return "V2.4.1 ASVS" }
func (c *weakPasswordHashCheck) Severity() string  { return "error" }

var passwordContextPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)password`),
	regexp.MustCompile(`(?i)passwd`),
	regexp.MustCompile(`(?i)pass_hash`),
	regexp.MustCompile(`(?i)hash_password`),
	regexp.MustCompile(`(?i)user.*hash`),
	regexp.MustCompile(`(?i)credential`),
}

var weakHashForPasswordPatterns = []struct {
	pattern *regexp.Regexp
	name    string
}{
	{regexp.MustCompile(`(?i)\bmd5\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bsha1\b`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bsha256\b`), "SHA-256 (without salt/iterations)"},
	{regexp.MustCompile(`(?i)\bsha512\b`), "SHA-512 (without salt/iterations)"},
	{regexp.MustCompile(`(?i)\bhashlib\.md5\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bhashlib\.sha1\b`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bhashlib\.sha256\b`), "SHA-256"},
	{regexp.MustCompile(`(?i)\bcrypto/md5\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bcrypto/sha1\b`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bcrypto/sha256\b`), "SHA-256"},
	{regexp.MustCompile(`(?i)\bDigestUtils\.md5\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bDigestUtils\.sha\b`), "SHA"},
	{regexp.MustCompile(`(?i)\bMessageDigest\.getInstance\b`), "MessageDigest (likely non-password-safe)"},
}

var approvedPasswordHashPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bbcrypt\b`),
	regexp.MustCompile(`(?i)\bscrypt\b`),
	regexp.MustCompile(`(?i)\bargon2\b`),
	regexp.MustCompile(`(?i)\bpbkdf2\b`),
	regexp.MustCompile(`(?i)\bPBKDF2\b`),
}

func (c *weakPasswordHashCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

			// Check if this line is in a password context
			inPasswordContext := false
			for _, p := range passwordContextPatterns {
				if p.MatchString(line) {
					inPasswordContext = true
					break
				}
			}

			if !inPasswordContext {
				continue
			}

			// Check if an approved hash is used
			hasApproved := false
			for _, p := range approvedPasswordHashPatterns {
				if p.MatchString(line) {
					hasApproved = true
					break
				}
			}
			if hasApproved {
				continue
			}

			// Check for weak hash algorithms in password context
			for _, algo := range weakHashForPasswordPatterns {
				if algo.pattern.MatchString(line) {
					findings = append(findings, compliance.Finding{
						Severity:   "error",
						Article:    "V2.4.1 ASVS",
						File:       file,
						StartLine:  lineNum,
						Message:    "Password hashing with non-approved algorithm: " + algo.name,
						Suggestion: "Use bcrypt, scrypt, argon2, or PBKDF2 with sufficient iterations for password storage",
						Confidence: 0.85,
						CWE:        "CWE-916",
					})
					break
				}
			}
		}
		f.Close()
	}

	return findings, nil
}

// --- hardcoded-credentials: V2.10.4 ASVS — Hardcoded service credentials ---

type hardcodedCredentialsCheck struct{}

func (c *hardcodedCredentialsCheck) ID() string       { return "hardcoded-credentials" }
func (c *hardcodedCredentialsCheck) Name() string     { return "Hardcoded Credentials" }
func (c *hardcodedCredentialsCheck) Article() string   { return "V2.10.4 ASVS" }
func (c *hardcodedCredentialsCheck) Severity() string  { return "error" }

var asvsSecretPatterns = []*regexp.Regexp{
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

			for _, pattern := range asvsSecretPatterns {
				if pattern.MatchString(line) {
					findings = append(findings, compliance.Finding{
						Severity:   "error",
						Article:    "V2.10.4 ASVS",
						File:       file,
						StartLine:  lineNum,
						Message:    "Potential hardcoded credential detected",
						Suggestion: "Use environment variables, secret managers, or configuration files excluded from version control",
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
