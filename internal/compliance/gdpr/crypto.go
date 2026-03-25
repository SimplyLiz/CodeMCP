package gdpr

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

// --- weak-pii-crypto: Art. 32 — Weak crypto on personal data ---

type weakPIICryptoCheck struct{}

func (c *weakPIICryptoCheck) ID() string       { return "weak-pii-crypto" }
func (c *weakPIICryptoCheck) Name() string     { return "Weak Cryptography on PII" }
func (c *weakPIICryptoCheck) Article() string   { return "Art. 32 GDPR" }
func (c *weakPIICryptoCheck) Severity() string  { return "error" }

// weakCryptoPatterns detects use of deprecated/insecure algorithms.
var weakCryptoPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bmd5\b`),
	regexp.MustCompile(`(?i)\bsha1\b`),
	regexp.MustCompile(`(?i)\bsha[-_]?1\b`),
	regexp.MustCompile(`(?i)\bdes\b\.`),
	regexp.MustCompile(`(?i)\b3des\b`),
	regexp.MustCompile(`(?i)\brc4\b`),
	regexp.MustCompile(`(?i)\brc2\b`),
	regexp.MustCompile(`(?i)\bblowfish\b`),
	regexp.MustCompile(`(?i)cipher\.NewCFBEncrypter`),
	regexp.MustCompile(`(?i)ECB`),
}

// weakCryptoNames maps pattern index to algorithm name.
var weakCryptoNames = []string{
	"MD5", "SHA-1", "SHA-1", "DES", "3DES", "RC4", "RC2", "Blowfish", "CFB without authentication", "ECB mode",
}

func (c *weakPIICryptoCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

				// Skip comments and imports
				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") ||
					strings.HasPrefix(trimmed, "import") || strings.HasPrefix(trimmed, "require") {
					continue
				}

				for i, pattern := range weakCryptoPatterns {
					if pattern.MatchString(line) {
						algoName := weakCryptoNames[i]
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "Art. 32 GDPR",
							File:       file,
							StartLine:  lineNum,
							Message:    fmt.Sprintf("Weak/deprecated cryptographic algorithm '%s' detected", algoName),
							Suggestion: "Use AES-256-GCM, SHA-256+, or bcrypt/argon2 for password hashing",
							Confidence: 0.85,
							CWE:        "CWE-327",
						})
						break // One finding per line
					}
				}
			}
		}()
	}

	return findings, nil
}

// --- plaintext-pii: Art. 32 — PII stored without encryption indicators ---

type plaintextPIICheck struct{}

func (c *plaintextPIICheck) ID() string       { return "plaintext-pii" }
func (c *plaintextPIICheck) Name() string     { return "Plaintext PII Storage" }
func (c *plaintextPIICheck) Article() string   { return "Art. 32 GDPR" }
func (c *plaintextPIICheck) Severity() string  { return "warning" }

// dbStoragePatterns detects database write patterns.
var dbStoragePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)INSERT\s+INTO`),
	regexp.MustCompile(`(?i)\.Create\(`),
	regexp.MustCompile(`(?i)\.Save\(`),
	regexp.MustCompile(`(?i)\.Insert\(`),
	regexp.MustCompile(`(?i)db\.Exec\(`),
	regexp.MustCompile(`(?i)\.execute\(`),
	regexp.MustCompile(`(?i)\.query\(`),
	regexp.MustCompile(`(?i)UPDATE\s+\w+\s+SET`),
}

func (c *plaintextPIICheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	piiScanner := compliance.NewPIIScanner(scope.Config.PIIFieldPatterns)
	piiFields, err := piiScanner.ScanFiles(ctx, scope)
	if err != nil {
		return nil, err
	}

	// Build set of files with PII
	piiByFile := make(map[string][]compliance.PIIField)
	for _, f := range piiFields {
		piiByFile[f.File] = append(piiByFile[f.File], f)
	}

	var findings []compliance.Finding

	// For files containing PII, check for DB writes without encryption indicators
	for file, fields := range piiByFile {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		text := string(content)
		textLower := strings.ToLower(text)

		// Check if file has DB operations
		hasDBOps := false
		for _, pattern := range dbStoragePatterns {
			if pattern.MatchString(text) {
				hasDBOps = true
				break
			}
		}

		if !hasDBOps {
			continue
		}

		// Check if file has encryption indicators
		hasEncryption := strings.Contains(textLower, "encrypt") ||
			strings.Contains(textLower, "cipher") ||
			strings.Contains(textLower, "aes") ||
			strings.Contains(textLower, "bcrypt") ||
			strings.Contains(textLower, "argon2") ||
			strings.Contains(textLower, "scrypt") ||
			strings.Contains(textLower, "hash")

		if !hasEncryption {
			// Report one finding per file listing the PII fields
			fieldNames := make([]string, 0, len(fields))
			seen := make(map[string]bool)
			for _, f := range fields {
				if !seen[f.Name] {
					fieldNames = append(fieldNames, f.Name)
					seen[f.Name] = true
				}
			}
			if len(fieldNames) > 5 {
				fieldNames = append(fieldNames[:5], "...")
			}

			findings = append(findings, compliance.Finding{
				Severity:   "warning",
				Article:    "Art. 32 GDPR",
				File:       file,
				Message:    fmt.Sprintf("Database operations with PII fields (%s) but no encryption detected", strings.Join(fieldNames, ", ")),
				Suggestion: "Consider encrypting PII at rest using column-level encryption or application-layer encryption",
				Confidence: 0.60,
			})
		}
	}

	return findings, nil
}
