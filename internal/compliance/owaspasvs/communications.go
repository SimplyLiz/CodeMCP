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

// --- missing-tls: V9.1.1 ASVS — TLS for all connections ---

type missingTLSCheck struct{}

func (c *missingTLSCheck) ID() string       { return "missing-tls" }
func (c *missingTLSCheck) Name() string     { return "Missing TLS for Sensitive Data" }
func (c *missingTLSCheck) Article() string   { return "V9.1.1 ASVS" }
func (c *missingTLSCheck) Severity() string  { return "error" }

func (c *missingTLSCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

				if strings.Contains(line, "http://") {
					lower := strings.ToLower(line)
					if strings.Contains(lower, "http://localhost") || strings.Contains(lower, "http://127.0.0.1") ||
						strings.Contains(lower, "http://0.0.0.0") || strings.Contains(lower, "http://[::1]") ||
						strings.Contains(lower, "http://example") {
						continue
					}
					// Skip print/log (displaying URLs, not connecting)
					if strings.Contains(lower, "printf") || strings.Contains(lower, "println") ||
						strings.Contains(lower, "log.") || strings.Contains(lower, "slog.") ||
						strings.Contains(lower, "fmt.") {
						continue
					}
					// Skip URL validation/parsing
					if strings.Contains(lower, "hasprefix") || strings.Contains(lower, "starts_with") ||
						strings.Contains(lower, "startswith") || strings.Contains(lower, "must start with") {
						continue
					}

					findings = append(findings, compliance.Finding{
						Severity:   "error",
						Article:    "V9.1.1 ASVS",
						File:       file,
						StartLine:  lineNum,
						Message:    "Unencrypted HTTP connection detected — all data in transit must use TLS",
						Suggestion: "Replace http:// with https:// or configure TLS for all connections carrying sensitive data",
						Confidence: 0.80,
						CWE:        "CWE-319",
					})
				}
			}
		}()
	}

	return findings, nil
}

// --- tls-bypass: V9.2.1 ASVS — TLS certificate validation ---

type tlsBypassCheck struct{}

func (c *tlsBypassCheck) ID() string       { return "tls-bypass" }
func (c *tlsBypassCheck) Name() string     { return "TLS Certificate Validation Bypass" }
func (c *tlsBypassCheck) Article() string   { return "V9.2.1 ASVS" }
func (c *tlsBypassCheck) Severity() string  { return "error" }

var tlsBypassPatterns = []struct {
	pattern *regexp.Regexp
	desc    string
}{
	{regexp.MustCompile(`InsecureSkipVerify\s*:\s*true`), "Go InsecureSkipVerify set to true"},
	{regexp.MustCompile(`(?i)verify\s*[=:]\s*(?:False|false|0)`), "TLS verification disabled (verify=False)"},
	{regexp.MustCompile(`rejectUnauthorized\s*:\s*false`), "Node.js rejectUnauthorized set to false"},
	{regexp.MustCompile(`NODE_TLS_REJECT_UNAUTHORIZED\s*=\s*['"]?0`), "NODE_TLS_REJECT_UNAUTHORIZED disabled"},
	{regexp.MustCompile(`(?i)ssl_verify\s*[=:]\s*(?:false|0)`), "SSL verification disabled"},
	{regexp.MustCompile(`(?i)CURLOPT_SSL_VERIFYPEER\s*,\s*(?:false|0)`), "PHP CURLOPT_SSL_VERIFYPEER disabled"},
}

func (c *tlsBypassCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Skip test files
		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") || strings.Contains(file, ".spec.") ||
			strings.Contains(file, "testdata/") || strings.Contains(file, "testutil/") {
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

				// Skip comments
				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "*") {
					continue
				}

				// Skip #nosec/nolint annotations
				if strings.Contains(line, "#nosec") || strings.Contains(line, "nolint:gosec") {
					continue
				}

				for _, tls := range tlsBypassPatterns {
					if tls.pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "V9.2.1 ASVS",
							File:       file,
							StartLine:  lineNum,
							Message:    "TLS certificate validation bypass: " + tls.desc,
							Suggestion: "Enable TLS certificate verification; use proper CA certificates instead of disabling validation",
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
