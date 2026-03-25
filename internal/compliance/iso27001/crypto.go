package iso27001

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

// --- weak-crypto: A.8.24 — Deprecated cryptographic algorithms ---

type weakCryptoCheck struct{}

func (c *weakCryptoCheck) ID() string       { return "weak-crypto" }
func (c *weakCryptoCheck) Name() string     { return "Weak Cryptographic Algorithms" }
func (c *weakCryptoCheck) Article() string   { return "A.8.24 ISO 27001:2022" }
func (c *weakCryptoCheck) Severity() string  { return "error" }

var weakAlgorithms = []struct {
	pattern *regexp.Regexp
	name    string
}{
	{regexp.MustCompile(`(?i)\bcrypto/md5\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bmd5\.New\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bcrypto/sha1\b`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bsha1\.New\b`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bcrypto/des\b`), "DES"},
	{regexp.MustCompile(`(?i)\bdes\.NewCipher\b`), "DES"},
	{regexp.MustCompile(`(?i)\bcrypto/rc4\b`), "RC4"},
	{regexp.MustCompile(`(?i)\brc4\.NewCipher\b`), "RC4"},
	{regexp.MustCompile(`(?i)\bNewECBEncrypter\b`), "ECB mode"},
	{regexp.MustCompile(`(?i)\bNewECBDecrypter\b`), "ECB mode"},
	{regexp.MustCompile(`(?i)\bcreateCipheriv\(['"]des\b`), "DES"},
	{regexp.MustCompile(`(?i)\bcreateCipheriv\(['"]rc4\b`), "RC4"},
	{regexp.MustCompile(`(?i)\bhashlib\.md5\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bhashlib\.sha1\b`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bDigestUtils\.md5\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bDigestUtils\.sha1\b`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bMessageDigest\.getInstance\(['"]MD5['"]\)`), "MD5"},
	{regexp.MustCompile(`(?i)\bMessageDigest\.getInstance\(['"]SHA-?1['"]\)`), "SHA-1"},
}

func (c *weakCryptoCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

				for _, algo := range weakAlgorithms {
					if algo.pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "A.8.24 ISO 27001:2022",
							File:       file,
							StartLine:  lineNum,
							Message:    fmt.Sprintf("Deprecated cryptographic algorithm '%s' detected", algo.name),
							Suggestion: "Use SHA-256+, AES-256-GCM, or bcrypt/argon2 for password hashing",
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

// --- insecure-random: A.8.24 — Non-cryptographic random for security ---

type insecureRandomCheck struct{}

func (c *insecureRandomCheck) ID() string       { return "insecure-random" }
func (c *insecureRandomCheck) Name() string     { return "Insecure Random Number Generator" }
func (c *insecureRandomCheck) Article() string   { return "A.8.24 ISO 27001:2022" }
func (c *insecureRandomCheck) Severity() string  { return "error" }

var insecureRandomPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bmath/rand\b`),                                     // Go: math/rand import
	regexp.MustCompile(`\brand\.New\b`),                                     // Go: rand.New
	regexp.MustCompile(`\brand\.(Int|Intn|Float|Read)\b`),                   // Go: rand.Int etc.
	regexp.MustCompile(`\bMath\.random\(\)`),                                // JavaScript
	regexp.MustCompile(`\brandom\.random\(\)`),                              // Python
	regexp.MustCompile(`\brandom\.randint\(`),                               // Python
	regexp.MustCompile(`\bjava\.util\.Random\b`),                            // Java
	regexp.MustCompile(`\bnew Random\(\)`),                                  // Java
}

func (c *insecureRandomCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Skip test files — insecure random is fine in tests
		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") || strings.Contains(file, ".spec.") {
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

				for _, pattern := range insecureRandomPatterns {
					if pattern.MatchString(line) {
						// Check context: is this used for security-related purposes?
						lower := strings.ToLower(line)
						securityContext := strings.Contains(lower, "token") || strings.Contains(lower, "secret") ||
							strings.Contains(lower, "key") || strings.Contains(lower, "nonce") ||
							strings.Contains(lower, "salt") || strings.Contains(lower, "session") ||
							strings.Contains(lower, "password") || strings.Contains(lower, "auth")

						confidence := 0.60
						if securityContext {
							confidence = 0.90
						}

						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "A.8.24 ISO 27001:2022",
							File:       file,
							StartLine:  lineNum,
							Message:    "Non-cryptographic random number generator used",
							Suggestion: "Use crypto/rand (Go), crypto.getRandomValues (JS), or secrets module (Python) for security purposes",
							Confidence: confidence,
							CWE:        "CWE-338",
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}
