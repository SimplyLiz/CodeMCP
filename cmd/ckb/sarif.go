package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"

	"ckb/internal/secrets"
)

// SARIF 2.1.0 schema types
// See: https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html

// SARIFReport is the top-level SARIF document.
type SARIFReport struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []SARIFRun `json:"runs"`
}

// SARIFRun represents a single analysis run.
type SARIFRun struct {
	Tool        SARIFTool         `json:"tool"`
	Results     []SARIFResult     `json:"results,omitempty"`
	Invocations []SARIFInvocation `json:"invocations,omitempty"`
}

// SARIFTool describes the analysis tool.
type SARIFTool struct {
	Driver SARIFDriver `json:"driver"`
}

// SARIFDriver describes the primary analysis component.
type SARIFDriver struct {
	Name            string      `json:"name"`
	Version         string      `json:"version,omitempty"`
	InformationURI  string      `json:"informationUri,omitempty"`
	Rules           []SARIFRule `json:"rules,omitempty"`
	SemanticVersion string      `json:"semanticVersion,omitempty"`
}

// SARIFRule describes a rule that detected an issue.
type SARIFRule struct {
	ID                   string                  `json:"id"`
	Name                 string                  `json:"name,omitempty"`
	ShortDescription     *SARIFMessage           `json:"shortDescription,omitempty"`
	FullDescription      *SARIFMessage           `json:"fullDescription,omitempty"`
	DefaultConfiguration *SARIFRuleConfiguration `json:"defaultConfiguration,omitempty"`
	HelpURI              string                  `json:"helpUri,omitempty"`
	Properties           map[string]interface{}  `json:"properties,omitempty"`
}

// SARIFRuleConfiguration describes the default configuration for a rule.
type SARIFRuleConfiguration struct {
	Level string `json:"level,omitempty"` // error, warning, note, none
}

// SARIFResult represents a single finding.
type SARIFResult struct {
	RuleID              string                 `json:"ruleId"`
	RuleIndex           int                    `json:"ruleIndex,omitempty"`
	Level               string                 `json:"level,omitempty"` // error, warning, note, none
	Message             SARIFMessage           `json:"message"`
	Locations           []SARIFLocation        `json:"locations,omitempty"`
	Fingerprints        map[string]string      `json:"fingerprints,omitempty"`
	PartialFingerprints map[string]string      `json:"partialFingerprints,omitempty"`
	Properties          map[string]interface{} `json:"properties,omitempty"`
}

// SARIFMessage contains text in various formats.
type SARIFMessage struct {
	Text     string `json:"text,omitempty"`
	Markdown string `json:"markdown,omitempty"`
}

// SARIFLocation describes where a result was found.
type SARIFLocation struct {
	PhysicalLocation *SARIFPhysicalLocation `json:"physicalLocation,omitempty"`
}

// SARIFPhysicalLocation identifies a file and region.
type SARIFPhysicalLocation struct {
	ArtifactLocation *SARIFArtifactLocation `json:"artifactLocation,omitempty"`
	Region           *SARIFRegion           `json:"region,omitempty"`
}

// SARIFArtifactLocation identifies a file.
type SARIFArtifactLocation struct {
	URI       string `json:"uri,omitempty"`
	URIBaseID string `json:"uriBaseId,omitempty"`
}

// SARIFRegion identifies a region within a file.
type SARIFRegion struct {
	StartLine   int `json:"startLine,omitempty"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// SARIFInvocation describes a single invocation of the tool.
type SARIFInvocation struct {
	ExecutionSuccessful bool                   `json:"executionSuccessful"`
	CommandLine         string                 `json:"commandLine,omitempty"`
	WorkingDirectory    *SARIFArtifactLocation `json:"workingDirectory,omitempty"`
	Machine             string                 `json:"machine,omitempty"`
}

// FormatSecretsAsSARIF converts a secrets scan result to SARIF format.
func FormatSecretsAsSARIF(result *secrets.ScanResult, version string) (string, error) {
	// Build rules from findings (deduplicated)
	ruleMap := make(map[string]SARIFRule)
	ruleIndex := make(map[string]int)

	for _, f := range result.Findings {
		ruleID := fmt.Sprintf("ckb/secrets/%s", f.Rule)
		if _, exists := ruleMap[ruleID]; !exists {
			rule := SARIFRule{
				ID:   ruleID,
				Name: f.Rule,
				ShortDescription: &SARIFMessage{
					Text: fmt.Sprintf("Potential %s detected", string(f.Type)),
				},
				FullDescription: &SARIFMessage{
					Text: fmt.Sprintf("CKB detected a potential %s secret. This may expose sensitive credentials.", string(f.Type)),
				},
				DefaultConfiguration: &SARIFRuleConfiguration{
					Level: severityToSARIFLevel(f.Severity),
				},
				Properties: map[string]interface{}{
					"security-severity": severityToScore(f.Severity),
					"tags":              []string{"security", "secrets", string(f.Type)},
				},
			}
			ruleIndex[ruleID] = len(ruleMap)
			ruleMap[ruleID] = rule
		}
	}

	// Convert map to slice in stable order
	rules := make([]SARIFRule, len(ruleMap))
	for id, rule := range ruleMap {
		rules[ruleIndex[id]] = rule
	}

	// Build results
	results := make([]SARIFResult, 0, len(result.Findings))
	for _, f := range result.Findings {
		ruleID := fmt.Sprintf("ckb/secrets/%s", f.Rule)

		// Generate fingerprint for deduplication
		fingerprint := generateFingerprint(f)

		sarifResult := SARIFResult{
			RuleID:    ruleID,
			RuleIndex: ruleIndex[ruleID],
			Level:     severityToSARIFLevel(f.Severity),
			Message: SARIFMessage{
				Text: fmt.Sprintf("Potential %s found: %s", string(f.Type), f.Match),
			},
			Locations: []SARIFLocation{
				{
					PhysicalLocation: &SARIFPhysicalLocation{
						ArtifactLocation: &SARIFArtifactLocation{
							URI:       toRelativeURI(f.File, result.RepoRoot),
							URIBaseID: "%SRCROOT%",
						},
						Region: &SARIFRegion{
							StartLine:   f.Line,
							StartColumn: f.Column,
						},
					},
				},
			},
			Fingerprints: map[string]string{
				"ckb/v1": fingerprint,
			},
			Properties: map[string]interface{}{
				"confidence": f.Confidence,
				"source":     f.Source,
			},
		}

		// Add commit info if available
		if f.Commit != "" {
			sarifResult.Properties["commit"] = f.Commit
			sarifResult.Properties["author"] = f.Author
			sarifResult.Properties["commitDate"] = f.CommitDate
		}

		results = append(results, sarifResult)
	}

	// Build the complete report
	report := SARIFReport{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []SARIFRun{
			{
				Tool: SARIFTool{
					Driver: SARIFDriver{
						Name:            "CKB",
						Version:         version,
						SemanticVersion: version,
						InformationURI:  "https://github.com/SimplyLiz/CodeMCP",
						Rules:           rules,
					},
				},
				Results: results,
				Invocations: []SARIFInvocation{
					{
						ExecutionSuccessful: true,
						WorkingDirectory: &SARIFArtifactLocation{
							URI: result.RepoRoot,
						},
						Machine: runtime.GOOS + "/" + runtime.GOARCH,
					},
				},
			},
		},
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal SARIF: %w", err)
	}
	return string(data), nil
}

// severityToSARIFLevel converts CKB severity to SARIF level.
func severityToSARIFLevel(s secrets.Severity) string {
	switch s {
	case secrets.SeverityCritical, secrets.SeverityHigh:
		return "error"
	case secrets.SeverityMedium:
		return "warning"
	case secrets.SeverityLow:
		return "note"
	default:
		return "warning"
	}
}

// severityToScore converts CKB severity to security-severity score (0-10).
func severityToScore(s secrets.Severity) float64 {
	switch s {
	case secrets.SeverityCritical:
		return 9.0
	case secrets.SeverityHigh:
		return 7.0
	case secrets.SeverityMedium:
		return 5.0
	case secrets.SeverityLow:
		return 3.0
	default:
		return 5.0
	}
}

// generateFingerprint creates a stable fingerprint for deduplication.
func generateFingerprint(f secrets.SecretFinding) string {
	// Use file + line + rule for fingerprinting
	data := fmt.Sprintf("%s:%d:%s", f.File, f.Line, f.Rule)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])[:16]
}

// toRelativeURI converts an absolute path to a relative URI.
func toRelativeURI(path, base string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	// Convert to forward slashes for URI
	return filepath.ToSlash(rel)
}
