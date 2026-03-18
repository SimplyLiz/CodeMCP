package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SimplyLiz/CodeMCP/internal/query"
)

func testResponse() *query.ReviewPRResponse {
	return &query.ReviewPRResponse{
		CkbVersion:    "8.2.0",
		SchemaVersion: "8.2",
		Tool:          "reviewPR",
		Verdict:       "warn",
		Score:         72,
		Summary: query.ReviewSummary{
			TotalFiles:      10,
			TotalChanges:    200,
			ReviewableFiles: 8,
			GeneratedFiles:  2,
			CriticalFiles:   1,
			ChecksPassed:    3,
			ChecksWarned:    2,
			ChecksFailed:    1,
			Languages:       []string{"Go", "TypeScript"},
			ModulesChanged:  2,
		},
		Checks: []query.ReviewCheck{
			{Name: "breaking", Status: "fail", Severity: "error", Summary: "2 breaking changes"},
			{Name: "secrets", Status: "pass", Severity: "error", Summary: "No secrets"},
			{Name: "complexity", Status: "warn", Severity: "warning", Summary: "+5 cyclomatic"},
		},
		Findings: []query.ReviewFinding{
			{
				Check:    "breaking",
				Severity: "error",
				File:     "api/handler.go",
				StartLine: 42,
				Message:  "Removed public function HandleAuth()",
				Category: "breaking",
				RuleID:   "ckb/breaking/removed-symbol",
			},
			{
				Check:      "complexity",
				Severity:   "warning",
				File:       "internal/query/engine.go",
				StartLine:  155,
				Message:    "Complexity 12→20 in parseQuery()",
				Category:   "complexity",
				RuleID:     "ckb/complexity/increase",
				Suggestion: "Consider extracting helper functions",
			},
			{
				Check:    "risk",
				Severity: "info",
				File:     "config.go",
				Message:  "High churn file",
				Category: "risk",
				RuleID:   "ckb/risk/high-score",
			},
		},
		Reviewers: []query.SuggestedReview{
			{Owner: "alice", Coverage: 0.85},
		},
	}
}

// --- SARIF Tests ---

func TestFormatSARIF_ValidJSON(t *testing.T) {
	resp := testResponse()
	output, err := formatReviewSARIF(resp)
	if err != nil {
		t.Fatalf("formatReviewSARIF error: %v", err)
	}

	var sarif sarifLog
	if err := json.Unmarshal([]byte(output), &sarif); err != nil {
		t.Fatalf("invalid SARIF JSON: %v", err)
	}

	if sarif.Version != "2.1.0" {
		t.Errorf("version = %q, want %q", sarif.Version, "2.1.0")
	}
}

func TestFormatSARIF_HasRuns(t *testing.T) {
	resp := testResponse()
	output, _ := formatReviewSARIF(resp)

	var sarif sarifLog
	json.Unmarshal([]byte(output), &sarif)

	if len(sarif.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(sarif.Runs))
	}

	run := sarif.Runs[0]
	if run.Tool.Driver.Name != "CKB" {
		t.Errorf("tool name = %q, want %q", run.Tool.Driver.Name, "CKB")
	}
}

func TestFormatSARIF_Results(t *testing.T) {
	resp := testResponse()
	output, _ := formatReviewSARIF(resp)

	var sarif sarifLog
	json.Unmarshal([]byte(output), &sarif)

	results := sarif.Runs[0].Results
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}

	// Check first result
	r := results[0]
	if r.RuleID != "ckb/breaking/removed-symbol" {
		t.Errorf("ruleId = %q, want %q", r.RuleID, "ckb/breaking/removed-symbol")
	}
	if r.Level != "error" {
		t.Errorf("level = %q, want %q", r.Level, "error")
	}
	if len(r.Locations) == 0 {
		t.Fatal("expected locations")
	}
	if r.Locations[0].PhysicalLocation.Region.StartLine != 42 {
		t.Errorf("startLine = %d, want 42", r.Locations[0].PhysicalLocation.Region.StartLine)
	}
}

func TestFormatSARIF_Fingerprints(t *testing.T) {
	resp := testResponse()
	output, _ := formatReviewSARIF(resp)

	var sarif sarifLog
	json.Unmarshal([]byte(output), &sarif)

	for _, r := range sarif.Runs[0].Results {
		if r.PartialFingerprints == nil {
			t.Error("expected partialFingerprints")
		}
		if _, ok := r.PartialFingerprints["ckb/v1"]; !ok {
			t.Error("expected ckb/v1 fingerprint")
		}
	}
}

func TestFormatSARIF_Rules(t *testing.T) {
	resp := testResponse()
	output, _ := formatReviewSARIF(resp)

	var sarif sarifLog
	json.Unmarshal([]byte(output), &sarif)

	rules := sarif.Runs[0].Tool.Driver.Rules
	if len(rules) != 3 {
		t.Errorf("rules = %d, want 3", len(rules))
	}
}

func TestFormatSARIF_Fixes(t *testing.T) {
	resp := testResponse()
	output, _ := formatReviewSARIF(resp)

	var sarif sarifLog
	json.Unmarshal([]byte(output), &sarif)

	// The complexity finding has a suggestion
	hasFix := false
	for _, r := range sarif.Runs[0].Results {
		if len(r.Fixes) > 0 {
			hasFix = true
			if r.Fixes[0].Description.Text != "Consider extracting helper functions" {
				t.Errorf("fix description = %q", r.Fixes[0].Description.Text)
			}
		}
	}
	if !hasFix {
		t.Error("expected at least one result with fixes")
	}
}

func TestFormatSARIF_EmptyFindings(t *testing.T) {
	resp := &query.ReviewPRResponse{
		CkbVersion: "8.2.0",
		Verdict:    "pass",
		Score:      100,
	}
	output, err := formatReviewSARIF(resp)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.Contains(output, `"results": []`) {
		t.Error("expected empty results array")
	}
}

// --- CodeClimate Tests ---

func TestFormatCodeClimate_ValidJSON(t *testing.T) {
	resp := testResponse()
	output, err := formatReviewCodeClimate(resp)
	if err != nil {
		t.Fatalf("formatReviewCodeClimate error: %v", err)
	}

	var issues []codeClimateIssue
	if err := json.Unmarshal([]byte(output), &issues); err != nil {
		t.Fatalf("invalid CodeClimate JSON: %v", err)
	}

	if len(issues) != 3 {
		t.Fatalf("issues = %d, want 3", len(issues))
	}
}

func TestFormatCodeClimate_Severity(t *testing.T) {
	resp := testResponse()
	output, _ := formatReviewCodeClimate(resp)

	var issues []codeClimateIssue
	json.Unmarshal([]byte(output), &issues)

	severities := make(map[string]int)
	for _, i := range issues {
		severities[i.Severity]++
	}

	if severities["critical"] != 1 {
		t.Errorf("critical = %d, want 1", severities["critical"])
	}
	if severities["major"] != 1 {
		t.Errorf("major = %d, want 1", severities["major"])
	}
	if severities["minor"] != 1 {
		t.Errorf("minor = %d, want 1", severities["minor"])
	}
}

func TestFormatCodeClimate_Fingerprints(t *testing.T) {
	resp := testResponse()
	output, _ := formatReviewCodeClimate(resp)

	var issues []codeClimateIssue
	json.Unmarshal([]byte(output), &issues)

	fps := make(map[string]bool)
	for _, i := range issues {
		if i.Fingerprint == "" {
			t.Error("empty fingerprint")
		}
		if fps[i.Fingerprint] {
			t.Errorf("duplicate fingerprint: %s", i.Fingerprint)
		}
		fps[i.Fingerprint] = true
	}
}

func TestFormatCodeClimate_Location(t *testing.T) {
	resp := testResponse()
	output, _ := formatReviewCodeClimate(resp)

	var issues []codeClimateIssue
	json.Unmarshal([]byte(output), &issues)

	if issues[0].Location.Path != "api/handler.go" {
		t.Errorf("path = %q, want %q", issues[0].Location.Path, "api/handler.go")
	}
	if issues[0].Location.Lines == nil || issues[0].Location.Lines.Begin != 42 {
		t.Error("expected lines.begin = 42")
	}
}

func TestFormatCodeClimate_Categories(t *testing.T) {
	resp := testResponse()
	output, _ := formatReviewCodeClimate(resp)

	var issues []codeClimateIssue
	json.Unmarshal([]byte(output), &issues)

	// Breaking → Compatibility
	if len(issues[0].Categories) == 0 || issues[0].Categories[0] != "Compatibility" {
		t.Errorf("breaking category = %v, want [Compatibility]", issues[0].Categories)
	}
	// Complexity → Complexity
	if len(issues[1].Categories) == 0 || issues[1].Categories[0] != "Complexity" {
		t.Errorf("complexity category = %v, want [Complexity]", issues[1].Categories)
	}
}

func TestFormatCodeClimate_EmptyFindings(t *testing.T) {
	resp := &query.ReviewPRResponse{Verdict: "pass", Score: 100}
	output, err := formatReviewCodeClimate(resp)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if output != "[]" {
		t.Errorf("expected empty array, got %q", output)
	}
}

// --- GitHub Actions Format Tests ---

func TestFormatGitHubActions_Annotations(t *testing.T) {
	resp := testResponse()
	output := formatReviewGitHubActions(resp)

	if !strings.Contains(output, "::error file=api/handler.go,line=42::") {
		t.Error("expected error annotation with file and line")
	}
	if !strings.Contains(output, "::warning file=internal/query/engine.go,line=155::") {
		t.Error("expected warning annotation")
	}
	if !strings.Contains(output, "::notice file=config.go::") {
		t.Error("expected notice annotation")
	}
}

// --- Human Format Tests ---

func TestFormatHuman_ContainsVerdict(t *testing.T) {
	resp := testResponse()
	output := formatReviewHuman(resp)

	if !strings.Contains(output, "WARN") {
		t.Error("expected WARN in output")
	}
	if !strings.Contains(output, "72") {
		t.Error("expected score 72 in output")
	}
}

func TestFormatHuman_ContainsChecks(t *testing.T) {
	resp := testResponse()
	output := formatReviewHuman(resp)

	if !strings.Contains(output, "breaking") {
		t.Error("expected breaking check")
	}
	if !strings.Contains(output, "secrets") {
		t.Error("expected secrets check")
	}
}

// --- Markdown Format Tests ---

func TestFormatMarkdown_ContainsTable(t *testing.T) {
	resp := testResponse()
	output := formatReviewMarkdown(resp)

	if !strings.Contains(output, "| Check | Status | Detail |") {
		t.Error("expected markdown table header")
	}
	if !strings.Contains(output, "<!-- ckb-review-marker -->") {
		t.Error("expected review marker for update-in-place")
	}
}

func TestFormatMarkdown_ContainsFindings(t *testing.T) {
	resp := testResponse()
	output := formatReviewMarkdown(resp)

	if !strings.Contains(output, "Findings (3)") {
		t.Error("expected findings section with count")
	}
}

// --- Compliance Format Tests ---

func TestFormatCompliance_HasSections(t *testing.T) {
	resp := testResponse()
	output := formatReviewCompliance(resp)

	sections := []string{
		"1. CHANGE SUMMARY",
		"2. QUALITY GATE RESULTS",
		"3. TRACEABILITY",
		"4. REVIEWER INDEPENDENCE",
		"5. SAFETY-CRITICAL PATH FINDINGS",
		"6. CODE HEALTH",
		"7. COMPLETE FINDINGS",
		"END OF COMPLIANCE EVIDENCE REPORT",
	}

	for _, s := range sections {
		if !strings.Contains(output, s) {
			t.Errorf("missing section: %s", s)
		}
	}
}
