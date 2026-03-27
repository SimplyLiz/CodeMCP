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

// --- hardcoded-secret: A.8.4 — Secrets in source code ---

type hardcodedSecretCheck struct{}

func (c *hardcodedSecretCheck) ID() string       { return "hardcoded-secret" }
func (c *hardcodedSecretCheck) Name() string     { return "Hardcoded Secrets" }
func (c *hardcodedSecretCheck) Article() string  { return "A.8.4 ISO 27001:2022" }
func (c *hardcodedSecretCheck) Severity() string { return "error" }

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|apikey)\s*[:=]\s*["'][\w\-]{16,}`),
	regexp.MustCompile(`(?i)(secret[_-]?key|secretkey)\s*[:=]\s*["'][\w\-]{16,}`),
	regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*["'][^"']{8,}`),
	regexp.MustCompile(`(?i)(access[_-]?token|auth[_-]?token)\s*[:=]\s*["'][\w\-\.]{20,}`),
	regexp.MustCompile(`(?i)(private[_-]?key)\s*[:=]\s*["']`),
	regexp.MustCompile(`(?i)-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----`),
	regexp.MustCompile(`(?i)(aws[_-]?secret|aws[_-]?access)\s*[:=]\s*["']`),
	regexp.MustCompile(`(?i)(database[_-]?url|db[_-]?url|connection[_-]?string)\s*[:=]\s*["'][^"']*[:@]`),
}

func (c *hardcodedSecretCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Skip test files and config examples
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

				for _, pattern := range secretPatterns {
					if pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "A.8.4 ISO 27001:2022",
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

// --- pii-in-logs: A.8.12 — Data leakage via logs ---

type piiInLogsCheck struct{}

func (c *piiInLogsCheck) ID() string       { return "pii-in-logs" }
func (c *piiInLogsCheck) Name() string     { return "PII Data Leakage in Logs" }
func (c *piiInLogsCheck) Article() string  { return "A.8.12 ISO 27001:2022" }
func (c *piiInLogsCheck) Severity() string { return "error" }

func (c *piiInLogsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	scanner := compliance.NewPIIScanner(scope.Config.PIIFieldPatterns)
	findings, err := scanner.CheckPIIInLogs(ctx, scope)
	if err != nil {
		return nil, err
	}

	for i := range findings {
		findings[i].Article = "A.8.12 ISO 27001:2022"
	}

	return findings, nil
}
