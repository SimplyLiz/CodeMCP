package owaspasvs

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
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
