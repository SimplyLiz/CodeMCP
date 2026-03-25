package pcidss

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- pan-in-source: Req 3.4 — Detect card numbers in source code ---

type panInSourceCheck struct{}

func (c *panInSourceCheck) ID() string       { return "pan-in-source" }
func (c *panInSourceCheck) Name() string     { return "PAN in Source Code" }
func (c *panInSourceCheck) Article() string   { return "Req 3.4 PCI DSS 4.0" }
func (c *panInSourceCheck) Severity() string  { return "error" }

var panPattern = regexp.MustCompile(`\b[0-9]{13,19}\b`)

// Common test card numbers that indicate PAN handling in code.
var testCardNumbers = []string{
	"4111111111111111", "4012888888881881", "4222222222222",
	"5500000000000004", "5105105105105100",
	"340000000000009", "371449635398431",
	"6011000000000004", "6011111111111117",
	"3530111333300000", "3566002020360505",
	"30569309025904", "38520000023237",
}

// regexDefinitionPattern detects lines that are defining regex patterns themselves.
var regexDefinitionPattern = regexp.MustCompile(`(?i)(regexp|regex|pattern|re\.compile|MustCompile)`)

func (c *panInSourceCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

				// Skip lines that are regex pattern definitions
				if regexDefinitionPattern.MatchString(line) {
					continue
				}

				// Check for known test card numbers first
				for _, card := range testCardNumbers {
					if strings.Contains(line, card) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "Req 3.4 PCI DSS 4.0",
							File:       file,
							StartLine:  lineNum,
							Message:    "Known test card number detected in source code",
							Suggestion: "Remove card numbers from source code; use tokenization or references to a secure vault",
							Confidence: 0.90,
							CWE:        "CWE-312",
						})
						break
					}
				}

				// Check for PAN-like patterns in string literals or comments
				if (strings.Contains(line, `"`) || strings.Contains(line, `'`)) && panPattern.MatchString(line) {
					matches := panPattern.FindAllString(line, -1)
					for _, m := range matches {
						// Filter out common non-PAN numbers (timestamps, IDs, etc.)
						if len(m) >= 13 && len(m) <= 19 {
							// Skip if it's already caught as test card
							isTestCard := false
							for _, card := range testCardNumbers {
								if m == card {
									isTestCard = true
									break
								}
							}
							if isTestCard {
								continue
							}

							findings = append(findings, compliance.Finding{
								Severity:   "error",
								Article:    "Req 3.4 PCI DSS 4.0",
								File:       file,
								StartLine:  lineNum,
								Message:    "Potential PAN (Primary Account Number) detected in source code",
								Suggestion: "Never store full PAN in source code; use tokenization, truncation, or masking",
								Confidence: 0.70,
								CWE:        "CWE-312",
							})
							break
						}
					}
				}
			}
		}()
	}

	return findings, nil
}

// --- pan-in-logs: Req 3.3.1 — Card data in log statements ---

type panInLogsCheck struct{}

func (c *panInLogsCheck) ID() string       { return "pan-in-logs" }
func (c *panInLogsCheck) Name() string     { return "Card Data in Logs" }
func (c *panInLogsCheck) Article() string   { return "Req 3.3.1 PCI DSS 4.0" }
func (c *panInLogsCheck) Severity() string  { return "error" }

var cardFieldPatterns = regexp.MustCompile(`(?i)(card_?number|card_?num|pan[^a-z]|credit_?card|ccn|card_?holder|cvv|cvc|expir(y|ation)_?date|track_?data|magnetic_?stripe)`)

func (c *panInLogsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

				// Check if line is a log statement
				isLog := false
				lower := strings.ToLower(line)
				for _, lp := range compliance.LogFunctionPatterns {
					if strings.Contains(lower, lp) {
						isLog = true
						break
					}
				}

				if !isLog {
					continue
				}

				// Check if log statement contains card-related fields
				if cardFieldPatterns.MatchString(line) {
					findings = append(findings, compliance.Finding{
						Severity:   "error",
						Article:    "Req 3.3.1 PCI DSS 4.0",
						File:       file,
						StartLine:  lineNum,
						Message:    "Card data field name referenced in log statement",
						Suggestion: "Never log card numbers, CVV, or track data; mask or omit payment card fields in logs",
						Confidence: 0.85,
						CWE:        "CWE-532",
					})
				}
			}
		}()
	}

	return findings, nil
}
