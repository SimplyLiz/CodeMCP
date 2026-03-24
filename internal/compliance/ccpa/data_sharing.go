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

// --- missing-do-not-sell: §1798.120 CCPA — Do Not Sell opt-out ---

type missingDoNotSellCheck struct{}

func (c *missingDoNotSellCheck) ID() string       { return "missing-do-not-sell" }
func (c *missingDoNotSellCheck) Name() string     { return "Missing Do Not Sell/Share Opt-Out" }
func (c *missingDoNotSellCheck) Article() string   { return "§1798.120 CCPA" }
func (c *missingDoNotSellCheck) Severity() string  { return "warning" }

var optOutPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)do[_\-\s]?not[_\-\s]?sell`),
	regexp.MustCompile(`(?i)dns[_\-]?flag`),
	regexp.MustCompile(`(?i)sale[_\-]?opt[_\-]?out`),
	regexp.MustCompile(`(?i)opt[_\-]?out`),
	regexp.MustCompile(`(?i)doNotSell`),
	regexp.MustCompile(`(?i)do_not_share`),
	regexp.MustCompile(`(?i)sharing[_\-]?opt[_\-]?out`),
}

var thirdPartyDataPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bgoogle[_\-]?analytics\b`),
	regexp.MustCompile(`(?i)\bmixpanel\b`),
	regexp.MustCompile(`(?i)\bsegment\b`),
	regexp.MustCompile(`(?i)\bamplitude\b`),
	regexp.MustCompile(`(?i)\bfacebook[_\-]?pixel\b`),
	regexp.MustCompile(`(?i)\bgoogle[_\-]?ads\b`),
	regexp.MustCompile(`(?i)\bgoogle[_\-]?tag\b`),
	regexp.MustCompile(`(?i)\bgtag\b`),
	regexp.MustCompile(`(?i)\bhotjar\b`),
	regexp.MustCompile(`(?i)\bheap\b.*analytics`),
	regexp.MustCompile(`(?i)\bfullstory\b`),
	regexp.MustCompile(`(?i)\bintercom\b`),
	regexp.MustCompile(`(?i)\bdrift\b`),
}

func (c *missingDoNotSellCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	hasThirdPartySharing := false
	hasOptOut := false
	var sharingFile string
	var sharingLine int

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
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

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()

			for _, p := range optOutPatterns {
				if p.MatchString(line) {
					hasOptOut = true
				}
			}

			if !hasThirdPartySharing {
				for _, p := range thirdPartyDataPatterns {
					if p.MatchString(line) {
						hasThirdPartySharing = true
						sharingFile = file
						sharingLine = lineNum
					}
				}
			}
		}
		f.Close()
	}

	if hasThirdPartySharing && !hasOptOut {
		return []compliance.Finding{
			{
				Severity:   "warning",
				Article:    "§1798.120 CCPA",
				File:       sharingFile,
				StartLine:  sharingLine,
				Message:    "Third-party data sharing (analytics/tracking) detected without 'Do Not Sell/Share' opt-out mechanism",
				Suggestion: "Implement a 'Do Not Sell or Share My Personal Information' mechanism to comply with CCPA §1798.120",
				Confidence: 0.70,
			},
		}, nil
	}

	return nil, nil
}

// --- third-party-sharing: §1798.100 CCPA — Third-party data sharing detection ---

type thirdPartySharingCheck struct{}

func (c *thirdPartySharingCheck) ID() string       { return "third-party-sharing" }
func (c *thirdPartySharingCheck) Name() string     { return "Third-Party Data Sharing Detection" }
func (c *thirdPartySharingCheck) Article() string   { return "§1798.100 CCPA" }
func (c *thirdPartySharingCheck) Severity() string  { return "info" }

func (c *thirdPartySharingCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
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

		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			trimmed := strings.TrimSpace(line)

			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
				continue
			}

			for _, p := range thirdPartyDataPatterns {
				if p.MatchString(line) {
					findings = append(findings, compliance.Finding{
						Severity:   "info",
						Article:    "§1798.100 CCPA",
						File:       file,
						StartLine:  lineNum,
						Message:    "Third-party data sharing integration detected (analytics/tracking/advertising SDK)",
						Suggestion: "Ensure third-party data sharing is disclosed in your privacy policy and consumers can request information about shared data",
						Confidence: 0.75,
					})
					break // One finding per file
				}
			}
		}
		f.Close()
	}

	return findings, nil
}
