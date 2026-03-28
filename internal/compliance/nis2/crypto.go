package nis2

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- deprecated-crypto: Art. 21(2)(j) NIS2 — Weak cryptographic algorithms ---

type deprecatedCryptoCheck struct{}

func (c *deprecatedCryptoCheck) ID() string       { return "deprecated-crypto" }
func (c *deprecatedCryptoCheck) Name() string     { return "Deprecated Cryptographic Algorithm" }
func (c *deprecatedCryptoCheck) Article() string  { return "Art. 21(2)(j) NIS2" }
func (c *deprecatedCryptoCheck) Severity() string { return "error" }

var nis2WeakAlgorithms = []struct {
	pattern *regexp.Regexp
	name    string
}{
	{regexp.MustCompile(`(?i)\bcrypto/md5\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bmd5\.New\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bhashlib\.md5\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bDigestUtils\.md5\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bMessageDigest\.getInstance\(['"]MD5['"]\)`), "MD5"},
	{regexp.MustCompile(`(?i)\bcrypto/sha1\b`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bsha1\.New\b`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bhashlib\.sha1\b`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bDigestUtils\.sha1\b`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bMessageDigest\.getInstance\(['"]SHA-?1['"]\)`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bcrypto/des\b`), "DES"},
	{regexp.MustCompile(`(?i)\bdes\.NewCipher\b`), "DES"},
	{regexp.MustCompile(`(?i)\bcreateCipheriv\(['"]des\b`), "DES"},
	{regexp.MustCompile(`(?i)\bcrypto/rc4\b`), "RC4"},
	{regexp.MustCompile(`(?i)\brc4\.NewCipher\b`), "RC4"},
	{regexp.MustCompile(`(?i)\bcreateCipheriv\(['"]rc4\b`), "RC4"},
	{regexp.MustCompile(`(?i)\bNewECBEncrypter\b`), "ECB mode"},
	{regexp.MustCompile(`(?i)\bNewECBDecrypter\b`), "ECB mode"},
}

func (c *deprecatedCryptoCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
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

				for _, algo := range nis2WeakAlgorithms {
					if algo.pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "Art. 21(2)(j) NIS2",
							File:       file,
							StartLine:  lineNum,
							Message:    fmt.Sprintf("Deprecated cryptographic algorithm '%s' detected", algo.name),
							Suggestion: "Use SHA-256+, AES-256-GCM, or bcrypt/argon2 for password hashing per NIS2 cryptography requirements",
							Confidence: 0.90,
							CWE:        "CWE-327",
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}

// --- hardcoded-secrets: Art. 21(2)(g) NIS2 — Hardcoded credentials ---

type hardcodedSecretsCheck struct{}

func (c *hardcodedSecretsCheck) ID() string       { return "hardcoded-secrets" }
func (c *hardcodedSecretsCheck) Name() string     { return "Hardcoded Secrets/Credentials" }
func (c *hardcodedSecretsCheck) Article() string  { return "Art. 21(2)(g) NIS2" }
func (c *hardcodedSecretsCheck) Severity() string { return "error" }

var nis2SecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*["'][\w\-]{16,}`),
	regexp.MustCompile(`(?i)(secret[_-]?key|secretkey)\s*[:=]\s*["'][\w\-]{16,}`),
	regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*["'][^"']{8,}`),
	regexp.MustCompile(`(?i)(access[_-]?token|auth[_-]?token)\s*[:=]\s*["'][\w\-\.]{20,}`),
	regexp.MustCompile(`(?i)(private[_-]?key)\s*[:=]\s*["']`),
	regexp.MustCompile(`(?i)-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----`),
	regexp.MustCompile(`(?i)(aws[_-]?secret|aws[_-]?access)\s*[:=]\s*["']`),
	regexp.MustCompile(`(?i)(database[_-]?url|db[_-]?url|connection[_-]?string)\s*[:=]\s*["'][^"']*[:@]`),
}

func (c *hardcodedSecretsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

				for _, pattern := range nis2SecretPatterns {
					if pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "Art. 21(2)(g) NIS2",
							File:       file,
							StartLine:  lineNum,
							Message:    "Potential hardcoded secret/credential detected",
							Suggestion: "Use environment variables, secret managers (Vault, AWS Secrets Manager), or .env files (gitignored)",
							Confidence: 0.80,
							CWE:        "CWE-798",
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}
