package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/SimplyLiz/CodeMCP/internal/query"
	"github.com/SimplyLiz/CodeMCP/internal/version"
)

// SARIF v2.1.0 types (subset needed for CKB output)

type sarifLog struct {
	Version string     `json:"version"`
	Schema  string     `json:"$schema"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name            string      `json:"name"`
	Version         string      `json:"version"`
	InformationURI  string      `json:"informationUri"`
	Rules           []sarifRule `json:"rules"`
	SemanticVersion string      `json:"semanticVersion"`
}

type sarifRule struct {
	ID               string              `json:"id"`
	ShortDescription sarifMessage        `json:"shortDescription"`
	DefaultConfig    *sarifConfiguration `json:"defaultConfiguration,omitempty"`
}

type sarifConfiguration struct {
	Level string `json:"level"` // "error", "warning", "note"
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifResult struct {
	RuleID              string            `json:"ruleId"`
	Level               string            `json:"level"` // "error", "warning", "note"
	Message             sarifMessage      `json:"message"`
	Locations           []sarifLocation   `json:"locations,omitempty"`
	PartialFingerprints map[string]string `json:"partialFingerprints,omitempty"`
	RelatedLocations    []sarifRelatedLoc `json:"relatedLocations,omitempty"`
	Fixes               []sarifFix        `json:"fixes,omitempty"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine int `json:"startLine,omitempty"`
	EndLine   int `json:"endLine,omitempty"`
}

type sarifRelatedLoc struct {
	ID               int                   `json:"id"`
	Message          sarifMessage          `json:"message"`
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifFix struct {
	Description sarifMessage          `json:"description"`
	Changes     []sarifArtifactChange `json:"artifactChanges"`
}

type sarifArtifactChange struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
}

// formatReviewSARIF generates SARIF v2.1.0 output for GitHub Code Scanning.
func formatReviewSARIF(resp *query.ReviewPRResponse) (string, error) {
	// Collect unique rules
	ruleMap := make(map[string]sarifRule)
	for _, f := range resp.Findings {
		ruleID := f.RuleID
		if ruleID == "" {
			ruleID = fmt.Sprintf("ckb/%s/unknown", f.Check)
		}
		if _, exists := ruleMap[ruleID]; !exists {
			level := sarifLevel(f.Severity)
			ruleMap[ruleID] = sarifRule{
				ID:               ruleID,
				ShortDescription: sarifMessage{Text: ruleID},
				DefaultConfig:    &sarifConfiguration{Level: level},
			}
		}
	}

	rules := make([]sarifRule, 0, len(ruleMap))
	for _, r := range ruleMap {
		rules = append(rules, r)
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })

	// Build results
	results := make([]sarifResult, 0, len(resp.Findings))
	for _, f := range resp.Findings {
		ruleID := f.RuleID
		if ruleID == "" {
			ruleID = fmt.Sprintf("ckb/%s/unknown", f.Check)
		}

		result := sarifResult{
			RuleID:  ruleID,
			Level:   sarifLevel(f.Severity),
			Message: sarifMessage{Text: f.Message},
			PartialFingerprints: map[string]string{
				"ckb/v1": sarifFingerprint(f),
			},
		}

		if f.File != "" {
			loc := sarifLocation{
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: f.File},
				},
			}
			if f.StartLine > 0 {
				loc.PhysicalLocation.Region = &sarifRegion{
					StartLine: f.StartLine,
				}
				if f.EndLine > 0 {
					loc.PhysicalLocation.Region.EndLine = f.EndLine
				}
			}
			result.Locations = []sarifLocation{loc}
		}

		if f.Suggestion != "" {
			// Add suggestion as a related location message rather than a Fix,
			// since SARIF v2.1.0 requires Fixes to include artifactChanges.
			result.RelatedLocations = append(result.RelatedLocations, sarifRelatedLoc{
				ID:      1,
				Message: sarifMessage{Text: "Suggestion: " + f.Suggestion},
				PhysicalLocation: sarifPhysicalLocation{
					ArtifactLocation: sarifArtifactLocation{URI: f.File},
				},
			})
		}

		results = append(results, result)
	}

	log := sarifLog{
		Version: "2.1.0",
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/main/sarif-2.1/schema/sarif-schema-2.1.0.json",
		Runs: []sarifRun{
			{
				Tool: sarifTool{
					Driver: sarifDriver{
						Name:            "CKB",
						Version:         version.Version,
						SemanticVersion: version.Version,
						InformationURI:  "https://github.com/SimplyLiz/CodeMCP",
						Rules:           rules,
					},
				},
				Results: results,
			},
		},
	}

	data, err := json.MarshalIndent(log, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal SARIF: %w", err)
	}
	return string(data), nil
}

func sarifLevel(severity string) string {
	switch severity {
	case "error":
		return "error"
	case "warning":
		return "warning"
	default:
		return "note"
	}
}

func sarifFingerprint(f query.ReviewFinding) string {
	h := sha256.New()
	h.Write([]byte(f.RuleID))
	h.Write([]byte{0})
	h.Write([]byte(f.File))
	h.Write([]byte{0})
	h.Write([]byte(f.Message))
	return hex.EncodeToString(h.Sum(nil))[:16]
}
