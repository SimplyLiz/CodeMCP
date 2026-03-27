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

// --- cors-wildcard: V14.5.3 ASVS — CORS origin validation ---

type asvsCORSWildcardCheck struct{}

func (c *asvsCORSWildcardCheck) ID() string       { return "asvs-cors-wildcard" }
func (c *asvsCORSWildcardCheck) Name() string     { return "CORS Wildcard Origin" }
func (c *asvsCORSWildcardCheck) Article() string  { return "V14.5.3 ASVS" }
func (c *asvsCORSWildcardCheck) Severity() string { return "warning" }

var asvsCORSWildcardPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)Access-Control-Allow-Origin.*\*`),
	regexp.MustCompile(`(?i)AllowOrigins.*\*`),
	regexp.MustCompile(`(?i)cors.*origin.*\*`),
	regexp.MustCompile(`(?i)allow_origins.*\[["']\*["']\]`),
}

func (c *asvsCORSWildcardCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

				// Skip flag/option definitions (documenting '*' as a choice, not setting it)
				lower := strings.ToLower(line)
				if strings.Contains(lower, "flag") || strings.Contains(lower, "option") ||
					strings.Contains(lower, "usage") || strings.Contains(lower, "help") ||
					strings.Contains(lower, "description") {
					continue
				}

				for _, pattern := range asvsCORSWildcardPatterns {
					if pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "warning",
							Article:    "V14.5.3 ASVS",
							File:       file,
							StartLine:  lineNum,
							Message:    "CORS wildcard origin (*) allows any website to make cross-origin requests",
							Suggestion: "Restrict Access-Control-Allow-Origin to specific trusted domains; avoid wildcard origins on authenticated endpoints",
							Confidence: 0.85,
							CWE:        "CWE-346",
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}
