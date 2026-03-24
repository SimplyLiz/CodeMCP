package ccpa

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- sensitive-pi-exposure: §1798.121 CCPA — Sensitive personal information ---

type sensitivePIExposureCheck struct{}

func (c *sensitivePIExposureCheck) ID() string       { return "sensitive-pi-exposure" }
func (c *sensitivePIExposureCheck) Name() string     { return "Sensitive Personal Information Exposure" }
func (c *sensitivePIExposureCheck) Article() string   { return "§1798.121 CCPA" }
func (c *sensitivePIExposureCheck) Severity() string  { return "warning" }

// CCPA-defined sensitive personal information categories
var sensitivePIPatterns = []struct {
	pattern  *regexp.Regexp
	category string
}{
	// Government IDs
	{regexp.MustCompile(`(?i)\bssn\b`), "Social Security Number"},
	{regexp.MustCompile(`(?i)\bsocial[_\-]?security\b`), "Social Security Number"},
	{regexp.MustCompile(`(?i)\bdriver[_\-]?license\b`), "Driver's License"},
	{regexp.MustCompile(`(?i)\bpassport[_\-]?(number|num|no)?\b`), "Passport"},
	{regexp.MustCompile(`(?i)\bstate[_\-]?id\b`), "State ID"},

	// Financial credentials
	{regexp.MustCompile(`(?i)\baccount[_\-]?number\b.*\b(pin|credential|login)\b`), "Financial Account + Credentials"},
	{regexp.MustCompile(`(?i)\bdebit[_\-]?card\b`), "Financial Account"},
	{regexp.MustCompile(`(?i)\bcredit[_\-]?card\b`), "Financial Account"},
	{regexp.MustCompile(`(?i)\bcard[_\-]?number\b`), "Financial Account"},
	{regexp.MustCompile(`(?i)\bcvv\b`), "Financial Account Credential"},

	// Precise geolocation
	{regexp.MustCompile(`(?i)\bprecise[_\-]?geolocation\b`), "Precise Geolocation"},
	{regexp.MustCompile(`(?i)\bgps[_\-]?coordinate\b`), "Precise Geolocation"},
	{regexp.MustCompile(`(?i)\blatitude\b.*\blongitude\b`), "Precise Geolocation"},

	// Racial/ethnic origin
	{regexp.MustCompile(`(?i)\bracial[_\-]?origin\b`), "Racial/Ethnic Origin"},
	{regexp.MustCompile(`(?i)\bethnic[_\-]?origin\b`), "Racial/Ethnic Origin"},
	{regexp.MustCompile(`(?i)\bethnicity\b`), "Racial/Ethnic Origin"},
	{regexp.MustCompile(`(?i)\brace\b`), "Racial/Ethnic Origin"},

	// Religious beliefs
	{regexp.MustCompile(`(?i)\breligion\b`), "Religious Beliefs"},
	{regexp.MustCompile(`(?i)\breligious[_\-]?belief\b`), "Religious Beliefs"},
	{regexp.MustCompile(`(?i)\bfaith\b`), "Religious Beliefs"},

	// Biometric data
	{regexp.MustCompile(`(?i)\bbiometric\b`), "Biometric Data"},
	{regexp.MustCompile(`(?i)\bfingerprint\b`), "Biometric Data"},
	{regexp.MustCompile(`(?i)\bface[_\-]?id\b`), "Biometric Data"},
	{regexp.MustCompile(`(?i)\bretina[_\-]?scan\b`), "Biometric Data"},
	{regexp.MustCompile(`(?i)\bvoice[_\-]?print\b`), "Biometric Data"},

	// Health data
	{regexp.MustCompile(`(?i)\bhealth[_\-]?data\b`), "Health Data"},
	{regexp.MustCompile(`(?i)\bmedical[_\-]?record\b`), "Health Data"},
	{regexp.MustCompile(`(?i)\bdiagnosis\b`), "Health Data"},
	{regexp.MustCompile(`(?i)\bprescription\b`), "Health Data"},

	// Sexual orientation
	{regexp.MustCompile(`(?i)\bsexual[_\-]?orientation\b`), "Sexual Orientation"},
	{regexp.MustCompile(`(?i)\bgender[_\-]?identity\b`), "Sexual Orientation/Gender Identity"},
}

var useLimitationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)use[_\-]?limit`),
	regexp.MustCompile(`(?i)purpose[_\-]?limit`),
	regexp.MustCompile(`(?i)sensitive[_\-]?data[_\-]?policy`),
	regexp.MustCompile(`(?i)data[_\-]?classification`),
	regexp.MustCompile(`(?i)access[_\-]?control.*sensitive`),
}

func (c *sensitivePIExposureCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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
		foundCategories := make(map[string]bool) // Avoid duplicate categories per file

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
				continue
			}

			for _, spi := range sensitivePIPatterns {
				if spi.pattern.MatchString(line) {
					if foundCategories[spi.category] {
						continue
					}
					foundCategories[spi.category] = true

					findings = append(findings, compliance.Finding{
						Severity:   "warning",
						Article:    "§1798.121 CCPA",
						File:       file,
						StartLine:  lineNum,
						Message:    "CCPA sensitive personal information detected: " + spi.category,
						Suggestion: "Ensure use limitation is enforced for sensitive PI; consumers must be able to limit use per CCPA §1798.121",
						Confidence: 0.65,
					})
					break
				}
			}
		}
		f.Close()
	}

	return findings, nil
}
