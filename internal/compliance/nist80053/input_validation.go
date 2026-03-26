package nist80053

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- missing-input-validation: SI-10 — HTTP handlers without input validation ---

type missingInputValidationCheck struct{}

func (c *missingInputValidationCheck) ID() string       { return "missing-input-validation" }
func (c *missingInputValidationCheck) Name() string     { return "Missing Input Validation" }
func (c *missingInputValidationCheck) Article() string   { return "SI-10 NIST 800-53" }
func (c *missingInputValidationCheck) Severity() string  { return "warning" }

var inputReadPatterns = []*regexp.Regexp{
	// Go
	regexp.MustCompile(`(?i)r\.Body`),
	regexp.MustCompile(`(?i)r\.FormValue\(`),
	regexp.MustCompile(`(?i)r\.URL\.Query\(\)`),
	regexp.MustCompile(`(?i)r\.ParseForm\(`),
	regexp.MustCompile(`(?i)json\.NewDecoder\(r\.Body\)`),
	regexp.MustCompile(`(?i)c\.Bind\(`),
	regexp.MustCompile(`(?i)c\.ShouldBind\(`),
	// Node/Express
	regexp.MustCompile(`(?i)req\.body\b`),
	regexp.MustCompile(`(?i)req\.params\b`),
	regexp.MustCompile(`(?i)req\.query\b`),
	// Python/Flask
	regexp.MustCompile(`(?i)request\.form\b`),
	regexp.MustCompile(`(?i)request\.json\b`),
	regexp.MustCompile(`(?i)request\.args\b`),
	regexp.MustCompile(`(?i)request\.get_json\(`),
	// Java/Spring
	regexp.MustCompile(`(?i)@RequestBody`),
	regexp.MustCompile(`(?i)@RequestParam`),
	regexp.MustCompile(`(?i)@PathVariable`),
}

var validationIndicators = []string{
	"validate", "sanitize", "schema", "validator",
	"binding:", "required", "min:", "max:",
	"regexp", "regex", "pattern", "constraint",
	"joi.", "yup.", "zod.", "class-validator",
	"@valid", "@notempty", "@notblank", "@size",
	// Go validation patterns (lowercase for case-insensitive matching)
	"strconv.", "parseint", "parsefloat", "parsebool", "atoi",
	"json.unmarshal", "json.newdecoder", "json.decode",
	"statusbadrequest", "http.error", "badrequest",
	"limitreader", "maxbytesreader",
	"filepath.clean", "filepath.abs",
}

func (c *missingInputValidationCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		text := string(content)

		// Only flag actual HTTP handler files (importing "net/http"), not internal
		// packages that happen to read from io.Reader or other non-HTTP sources.
		if !strings.Contains(text, `"net/http"`) && !strings.Contains(text, "req.body") &&
			!strings.Contains(text, "request.form") && !strings.Contains(text, "request.json") &&
			!strings.Contains(text, "@RequestBody") && !strings.Contains(text, "@RequestParam") {
			continue
		}

		// Check if file reads user input
		hasInputRead := false
		var firstInputLine int
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			for _, pattern := range inputReadPatterns {
				if pattern.MatchString(line) {
					hasInputRead = true
					if firstInputLine == 0 {
						firstInputLine = i + 1
					}
					break
				}
			}
			if hasInputRead && firstInputLine > 0 {
				break
			}
		}

		if !hasInputRead {
			continue
		}

		// Check for validation indicators
		textLower := strings.ToLower(text)
		hasValidation := false
		for _, indicator := range validationIndicators {
			if strings.Contains(textLower, indicator) {
				hasValidation = true
				break
			}
		}

		if !hasValidation {
			// Scan for the actual input read lines to report
			sc := bufio.NewScanner(strings.NewReader(text))
			lineNum := 0
			reported := false

			for sc.Scan() {
				lineNum++
				line := sc.Text()

				for _, pattern := range inputReadPatterns {
					if pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "warning",
							Article:    "SI-10 NIST 800-53",
							File:       file,
							StartLine:  lineNum,
							Message:    "HTTP request input read without visible input validation",
							Suggestion: "Validate and sanitize all user input: check types, lengths, ranges, and formats before processing",
							Confidence: 0.60,
						})
						reported = true
						break
					}
				}
				if reported {
					break
				}
			}
		}
	}

	return findings, nil
}
