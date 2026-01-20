package main

import (
	"encoding/json"
	"strings"
	"testing"

	"ckb/internal/secrets"
)

func TestFormatSecretsAsSARIF(t *testing.T) {
	result := &secrets.ScanResult{
		RepoRoot: "/test/repo",
		Scope:    "workdir",
		Duration: "1.5s",
		Findings: []secrets.SecretFinding{
			{
				File:       "/test/repo/config.go",
				Line:       42,
				Column:     10,
				Type:       secrets.SecretTypeAWSAccessKey,
				Severity:   secrets.SeverityCritical,
				Match:      "AKIA****************",
				Rule:       "aws_access_key_id",
				Confidence: 0.95,
				Source:     "builtin",
			},
			{
				File:       "/test/repo/auth.go",
				Line:       15,
				Column:     5,
				Type:       secrets.SecretTypeGitHubPAT,
				Severity:   secrets.SeverityHigh,
				Match:      "ghp_****",
				Rule:       "github_pat",
				Confidence: 0.90,
				Source:     "builtin",
			},
		},
		Summary: secrets.ScanSummary{
			TotalFindings:    2,
			FilesWithSecrets: 2,
			BySeverity: map[secrets.Severity]int{
				secrets.SeverityCritical: 1,
				secrets.SeverityHigh:     1,
			},
			ByType: map[secrets.SecretType]int{
				secrets.SecretTypeAWSAccessKey: 1,
				secrets.SecretTypeGitHubPAT:    1,
			},
		},
	}

	output, err := FormatSecretsAsSARIF(result, "8.1.0")
	if err != nil {
		t.Fatalf("FormatSecretsAsSARIF failed: %v", err)
	}

	// Parse and validate SARIF structure
	var sarif SARIFReport
	if err := json.Unmarshal([]byte(output), &sarif); err != nil {
		t.Fatalf("Failed to parse SARIF output: %v", err)
	}

	// Verify schema and version
	if sarif.Version != "2.1.0" {
		t.Errorf("SARIF version = %q, want 2.1.0", sarif.Version)
	}
	if !strings.Contains(sarif.Schema, "sarif-schema-2.1.0") {
		t.Errorf("SARIF schema should reference 2.1.0, got %q", sarif.Schema)
	}

	// Verify runs
	if len(sarif.Runs) != 1 {
		t.Fatalf("Expected 1 run, got %d", len(sarif.Runs))
	}

	run := sarif.Runs[0]

	// Verify tool info
	if run.Tool.Driver.Name != "CKB" {
		t.Errorf("Tool name = %q, want CKB", run.Tool.Driver.Name)
	}
	if run.Tool.Driver.Version != "8.1.0" {
		t.Errorf("Tool version = %q, want 8.1.0", run.Tool.Driver.Version)
	}

	// Verify rules
	if len(run.Tool.Driver.Rules) != 2 {
		t.Errorf("Expected 2 rules, got %d", len(run.Tool.Driver.Rules))
	}

	// Verify results
	if len(run.Results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(run.Results))
	}

	// Check first result (critical -> error)
	r1 := run.Results[0]
	if r1.Level != "error" {
		t.Errorf("Critical severity should map to 'error', got %q", r1.Level)
	}
	if r1.RuleID != "ckb/secrets/aws_access_key_id" {
		t.Errorf("RuleID = %q, want ckb/secrets/aws_access_key_id", r1.RuleID)
	}

	// Check location
	if len(r1.Locations) != 1 {
		t.Fatalf("Expected 1 location, got %d", len(r1.Locations))
	}
	loc := r1.Locations[0].PhysicalLocation
	if loc.ArtifactLocation.URI != "config.go" {
		t.Errorf("URI = %q, want config.go", loc.ArtifactLocation.URI)
	}
	if loc.Region.StartLine != 42 {
		t.Errorf("StartLine = %d, want 42", loc.Region.StartLine)
	}

	// Check fingerprint exists
	if len(r1.Fingerprints) == 0 {
		t.Error("Expected fingerprints to be set")
	}
}

func TestSeverityToSARIFLevel(t *testing.T) {
	testCases := []struct {
		severity secrets.Severity
		want     string
	}{
		{secrets.SeverityCritical, "error"},
		{secrets.SeverityHigh, "error"},
		{secrets.SeverityMedium, "warning"},
		{secrets.SeverityLow, "note"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.severity), func(t *testing.T) {
			got := severityToSARIFLevel(tc.severity)
			if got != tc.want {
				t.Errorf("severityToSARIFLevel(%s) = %q, want %q", tc.severity, got, tc.want)
			}
		})
	}
}

func TestSeverityToScore(t *testing.T) {
	testCases := []struct {
		severity secrets.Severity
		want     float64
	}{
		{secrets.SeverityCritical, 9.0},
		{secrets.SeverityHigh, 7.0},
		{secrets.SeverityMedium, 5.0},
		{secrets.SeverityLow, 3.0},
	}

	for _, tc := range testCases {
		t.Run(string(tc.severity), func(t *testing.T) {
			got := severityToScore(tc.severity)
			if got != tc.want {
				t.Errorf("severityToScore(%s) = %f, want %f", tc.severity, got, tc.want)
			}
		})
	}
}

func TestGenerateFingerprint(t *testing.T) {
	f1 := secrets.SecretFinding{
		File: "config.go",
		Line: 42,
		Rule: "aws_access_key_id",
	}
	f2 := secrets.SecretFinding{
		File: "config.go",
		Line: 42,
		Rule: "aws_access_key_id",
	}
	f3 := secrets.SecretFinding{
		File: "other.go",
		Line: 42,
		Rule: "aws_access_key_id",
	}

	// Same finding should produce same fingerprint
	fp1 := generateFingerprint(f1)
	fp2 := generateFingerprint(f2)
	if fp1 != fp2 {
		t.Error("Same findings should produce same fingerprint")
	}

	// Different finding should produce different fingerprint
	fp3 := generateFingerprint(f3)
	if fp1 == fp3 {
		t.Error("Different findings should produce different fingerprints")
	}

	// Fingerprint should be 16 chars (first 16 hex chars of SHA256)
	if len(fp1) != 16 {
		t.Errorf("Fingerprint length = %d, want 16", len(fp1))
	}
}

func TestToRelativeURI(t *testing.T) {
	testCases := []struct {
		path string
		base string
		want string
	}{
		{"/test/repo/src/config.go", "/test/repo", "src/config.go"},
		{"/test/repo/config.go", "/test/repo", "config.go"},
		{"/other/path/file.go", "/test/repo", "../../other/path/file.go"},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			got := toRelativeURI(tc.path, tc.base)
			if got != tc.want {
				t.Errorf("toRelativeURI(%q, %q) = %q, want %q", tc.path, tc.base, got, tc.want)
			}
		})
	}
}

func TestFormatSecretsAsSARIFEmpty(t *testing.T) {
	result := &secrets.ScanResult{
		RepoRoot: "/test/repo",
		Scope:    "workdir",
		Duration: "0.1s",
		Findings: []secrets.SecretFinding{},
		Summary: secrets.ScanSummary{
			TotalFindings:    0,
			FilesWithSecrets: 0,
			BySeverity:       map[secrets.Severity]int{},
			ByType:           map[secrets.SecretType]int{},
		},
	}

	output, err := FormatSecretsAsSARIF(result, "8.1.0")
	if err != nil {
		t.Fatalf("FormatSecretsAsSARIF failed: %v", err)
	}

	var sarif SARIFReport
	if err := json.Unmarshal([]byte(output), &sarif); err != nil {
		t.Fatalf("Failed to parse SARIF output: %v", err)
	}

	// Should still have valid structure with empty results
	if len(sarif.Runs) != 1 {
		t.Fatalf("Expected 1 run, got %d", len(sarif.Runs))
	}
	if len(sarif.Runs[0].Results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(sarif.Runs[0].Results))
	}
}

func TestFormatSecretsAsSARIFWithCommitInfo(t *testing.T) {
	result := &secrets.ScanResult{
		RepoRoot: "/test/repo",
		Scope:    "history",
		Duration: "5.0s",
		Findings: []secrets.SecretFinding{
			{
				File:       "/test/repo/config.go",
				Line:       10,
				Type:       secrets.SecretTypeAWSAccessKey,
				Severity:   secrets.SeverityCritical,
				Match:      "AKIA****",
				Rule:       "aws_access_key_id",
				Confidence: 0.95,
				Source:     "builtin",
				Commit:     "abc123def",
				Author:     "developer@example.com",
				CommitDate: "2024-01-15T10:30:00Z",
			},
		},
		Summary: secrets.ScanSummary{
			TotalFindings:    1,
			FilesWithSecrets: 1,
			BySeverity: map[secrets.Severity]int{
				secrets.SeverityCritical: 1,
			},
			ByType: map[secrets.SecretType]int{
				secrets.SecretTypeAWSAccessKey: 1,
			},
		},
	}

	output, err := FormatSecretsAsSARIF(result, "8.1.0")
	if err != nil {
		t.Fatalf("FormatSecretsAsSARIF failed: %v", err)
	}

	var sarif SARIFReport
	if err := json.Unmarshal([]byte(output), &sarif); err != nil {
		t.Fatalf("Failed to parse SARIF output: %v", err)
	}

	// Check that commit info is in properties
	r := sarif.Runs[0].Results[0]
	if r.Properties["commit"] != "abc123def" {
		t.Errorf("Expected commit in properties, got %v", r.Properties["commit"])
	}
	if r.Properties["author"] != "developer@example.com" {
		t.Errorf("Expected author in properties, got %v", r.Properties["author"])
	}
}
