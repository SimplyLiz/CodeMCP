package fda21cfr11

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- missing-input-validation: §11.10(a) — input validation for regulated data ---

type missingInputValidationCheck struct{}

func (c *missingInputValidationCheck) ID() string       { return "missing-input-validation" }
func (c *missingInputValidationCheck) Name() string     { return "Missing Input Validation" }
func (c *missingInputValidationCheck) Article() string  { return "§11.10(a) 21 CFR Part 11" }
func (c *missingInputValidationCheck) Severity() string { return "warning" }

// Patterns for form/API input handling
var inputPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(request\.body|req\.body|request\.form|request\.params|request\.query)`),
	regexp.MustCompile(`(?i)(r\.FormValue|r\.PostFormValue|r\.URL\.Query|c\.Bind|c\.ShouldBind)`),
	regexp.MustCompile(`(?i)(getParameter|getRequestBody|@RequestBody|@FormParam|@QueryParam)`),
	regexp.MustCompile(`(?i)(request\.data|request\.POST|request\.GET|request\.FILES)`),
}

var validationPatterns = regexp.MustCompile(`(?i)(validate|validator|validation|sanitize|sanitizer|schema\.parse|zod\.|joi\.|yup\.|is_valid|clean\(|strip_tags)`)

func (c *missingInputValidationCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, "test_") {
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
			hasInput := false
			hasValidation := false
			inputLine := 0

			// Simple function-scope tracking
			braceDepth := 0

			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				trimmed := strings.TrimSpace(line)

				if strings.HasPrefix(trimmed, "//") {
					continue
				}

				prevDepth := braceDepth
				braceDepth += strings.Count(line, "{") - strings.Count(line, "}")

				// Reset at function boundaries
				if braceDepth <= 0 && prevDepth > 0 {
					if hasInput && !hasValidation {
						findings = append(findings, compliance.Finding{
							CheckID:    "missing-input-validation",
							Framework:  compliance.FrameworkFDAPart11,
							Severity:   "warning",
							Article:    "§11.10(a) 21 CFR Part 11",
							File:       file,
							StartLine:  inputLine,
							Message:    "Form/API input handling without input validation",
							Suggestion: "Add input validation and sanitization for all user-submitted data per 21 CFR Part 11",
							Confidence: 0.60,
						})
					}
					hasInput = false
					hasValidation = false
				}

				for _, pattern := range inputPatterns {
					if pattern.MatchString(line) {
						hasInput = true
						inputLine = lineNum
						break
					}
				}

				if validationPatterns.MatchString(line) {
					hasValidation = true
				}
			}

			// Handle last function in file
			if hasInput && !hasValidation {
				findings = append(findings, compliance.Finding{
					CheckID:    "missing-input-validation",
					Framework:  compliance.FrameworkFDAPart11,
					Severity:   "warning",
					Article:    "§11.10(a) 21 CFR Part 11",
					File:       file,
					StartLine:  inputLine,
					Message:    "Form/API input handling without input validation",
					Suggestion: "Add input validation and sanitization for all user-submitted data per 21 CFR Part 11",
					Confidence: 0.60,
				})
			}

		}()
	}

	return findings, nil
}
