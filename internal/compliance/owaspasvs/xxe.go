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

// --- xxe: V5.5.2 ASVS — XML External Entity (XXE) prevention ---

type xxeCheck struct{}

func (c *xxeCheck) ID() string       { return "xxe" }
func (c *xxeCheck) Name() string     { return "XML External Entity (XXE) Risk" }
func (c *xxeCheck) Article() string  { return "V5.5.2 ASVS" }
func (c *xxeCheck) Severity() string { return "warning" }

var xxePatterns = []struct {
	pattern *regexp.Regexp
	desc    string
}{
	{regexp.MustCompile(`(?i)xml\.NewDecoder\(`), "Go xml.NewDecoder without entity restriction"},
	{regexp.MustCompile(`(?i)etree\.Parse\(`), "Go etree XML parsing"},
	{regexp.MustCompile(`(?i)XMLReaderFactory\.createXMLReader`), "Java XMLReader (check entity resolution settings)"},
	{regexp.MustCompile(`(?i)DocumentBuilderFactory\.newInstance`), "Java DocumentBuilderFactory (check entity resolution settings)"},
	{regexp.MustCompile(`(?i)SAXParserFactory\.newInstance`), "Java SAXParserFactory (check entity resolution settings)"},
	{regexp.MustCompile(`(?i)lxml\.etree\.parse\(`), "Python lxml.etree.parse (check resolve_entities setting)"},
	{regexp.MustCompile(`(?i)xml\.etree\.ElementTree\.parse\(`), "Python stdlib XML parsing"},
	{regexp.MustCompile(`(?i)DOMParser\(\)`), "JavaScript DOMParser"},
}

func (c *xxeCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

				for _, xxe := range xxePatterns {
					if xxe.pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "warning",
							Article:    "V5.5.2 ASVS",
							File:       file,
							StartLine:  lineNum,
							Message:    "Potential XXE vulnerability: " + xxe.desc,
							Suggestion: "Disable external entity resolution in XML parsers; use defusedxml (Python), set FEATURE_SECURE_PROCESSING (Java), or restrict entity resolution (Go)",
							Confidence: 0.60,
							CWE:        "CWE-611",
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}
