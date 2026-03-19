package main

import (
	"crypto/md5" // #nosec G501 — MD5 used for fingerprinting, not security
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/SimplyLiz/CodeMCP/internal/query"
)

// Code Climate JSON format for GitLab Code Quality
// https://docs.gitlab.com/ee/ci/testing/code_quality.html

type codeClimateIssue struct {
	Type        string              `json:"type"`
	CheckName   string              `json:"check_name"`
	Description string              `json:"description"`
	Content     *codeClimateContent `json:"content,omitempty"`
	Categories  []string            `json:"categories"`
	Location    codeClimateLocation `json:"location"`
	Severity    string              `json:"severity"` // blocker, critical, major, minor, info
	Fingerprint string              `json:"fingerprint"`
}

type codeClimateContent struct {
	Body string `json:"body"`
}

type codeClimateLocation struct {
	Path  string            `json:"path"`
	Lines *codeClimateLines `json:"lines,omitempty"`
}

type codeClimateLines struct {
	Begin int `json:"begin"`
	End   int `json:"end,omitempty"`
}

// formatReviewCodeClimate generates Code Climate JSON for GitLab.
func formatReviewCodeClimate(resp *query.ReviewPRResponse) (string, error) {
	issues := make([]codeClimateIssue, 0, len(resp.Findings))

	for _, f := range resp.Findings {
		issue := codeClimateIssue{
			Type:        "issue",
			CheckName:   f.RuleID,
			Description: f.Message,
			Categories:  ccCategories(f.Category),
			Severity:    ccSeverity(f.Severity),
			Fingerprint: ccFingerprint(f),
			Location: codeClimateLocation{
				Path: f.File,
			},
		}

		if issue.CheckName == "" {
			issue.CheckName = fmt.Sprintf("ckb/%s", f.Check)
		}

		if f.File == "" {
			issue.Location.Path = "."
		}

		if f.StartLine > 0 {
			issue.Location.Lines = &codeClimateLines{
				Begin: f.StartLine,
			}
			if f.EndLine > 0 {
				issue.Location.Lines.End = f.EndLine
			}
		}

		if f.Detail != "" {
			issue.Content = &codeClimateContent{Body: f.Detail}
		} else if f.Suggestion != "" {
			issue.Content = &codeClimateContent{Body: f.Suggestion}
		}

		issues = append(issues, issue)
	}

	data, err := json.MarshalIndent(issues, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal CodeClimate: %w", err)
	}
	return string(data), nil
}

func ccSeverity(severity string) string {
	switch severity {
	case "error":
		return "critical"
	case "warning":
		return "major"
	default:
		return "minor"
	}
}

func ccCategories(category string) []string {
	switch category {
	case "security":
		return []string{"Security"}
	case "breaking":
		return []string{"Compatibility"}
	case "complexity":
		return []string{"Complexity"}
	case "testing":
		return []string{"Bug Risk"}
	case "coupling":
		return []string{"Duplication"} // closest CC category for coupling
	case "risk":
		return []string{"Bug Risk"}
	case "critical":
		return []string{"Security", "Bug Risk"}
	case "compliance":
		return []string{"Style"} // closest CC category for compliance
	case "health":
		return []string{"Complexity"}
	default:
		return []string{"Bug Risk"}
	}
}

func ccFingerprint(f query.ReviewFinding) string {
	h := md5.New() // #nosec G401 — MD5 for fingerprinting, not security
	h.Write([]byte(f.RuleID))
	h.Write([]byte{0})
	h.Write([]byte(f.File))
	h.Write([]byte{0})
	h.Write([]byte(f.Message))
	return hex.EncodeToString(h.Sum(nil))
}
