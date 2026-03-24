package nist80053

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

// --- non-fips-crypto: SC-13 — Non-FIPS-approved cryptographic algorithms ---

type nonFIPSCryptoCheck struct{}

func (c *nonFIPSCryptoCheck) ID() string       { return "non-fips-crypto" }
func (c *nonFIPSCryptoCheck) Name() string     { return "Non-FIPS Cryptographic Algorithm" }
func (c *nonFIPSCryptoCheck) Article() string   { return "SC-13 NIST 800-53" }
func (c *nonFIPSCryptoCheck) Severity() string  { return "error" }

var nonFIPSAlgorithms = []struct {
	pattern *regexp.Regexp
	name    string
}{
	{regexp.MustCompile(`(?i)\bcrypto/md5\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bmd5\.New\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bhashlib\.md5\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bDigestUtils\.md5\b`), "MD5"},
	{regexp.MustCompile(`(?i)\bMessageDigest\.getInstance\(["']MD5["']\)`), "MD5"},
	{regexp.MustCompile(`(?i)\bcrypto/sha1\b`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bsha1\.New\b`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bhashlib\.sha1\b`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bDigestUtils\.sha1\b`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bMessageDigest\.getInstance\(["']SHA-?1["']\)`), "SHA-1"},
	{regexp.MustCompile(`(?i)\bcrypto/des\b`), "DES"},
	{regexp.MustCompile(`(?i)\bdes\.NewCipher\b`), "DES"},
	{regexp.MustCompile(`(?i)\bcreateCipheriv\(["']des\b`), "DES"},
	{regexp.MustCompile(`(?i)\bTripleDES\b`), "3DES"},
	{regexp.MustCompile(`(?i)\b3des\b`), "3DES"},
	{regexp.MustCompile(`(?i)\bcrypto/rc4\b`), "RC4"},
	{regexp.MustCompile(`(?i)\brc4\.NewCipher\b`), "RC4"},
	{regexp.MustCompile(`(?i)\bcreateCipheriv\(["']rc4\b`), "RC4"},
	{regexp.MustCompile(`(?i)\bBlowfish\b`), "Blowfish"},
}

func (c *nonFIPSCryptoCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

			for _, algo := range nonFIPSAlgorithms {
				if algo.pattern.MatchString(line) {
					findings = append(findings, compliance.Finding{
						Severity:   "error",
						Article:    "SC-13 NIST 800-53",
						File:       file,
						StartLine:  lineNum,
						Message:    fmt.Sprintf("Non-FIPS-approved cryptographic algorithm '%s' detected", algo.name),
						Suggestion: "Use FIPS 140-2 approved algorithms: AES (128/192/256), SHA-2 (256/384/512), RSA (2048+), ECDSA",
						Confidence: 0.90,
						CWE:        "CWE-327",
					})
					break
				}
			}
		}
		f.Close()
	}

	return findings, nil
}
