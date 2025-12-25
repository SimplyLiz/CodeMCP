package envelope

import (
	"encoding/json"
	"fmt"
	"testing"

	"ckb/internal/output"
	"ckb/internal/query"
)

func TestScoreToTier(t *testing.T) {
	tests := []struct {
		score float64
		want  ConfidenceTier
	}{
		{1.0, TierHigh},
		{0.95, TierHigh},
		{0.94, TierMedium},
		{0.70, TierMedium},
		{0.69, TierLow},
		{0.30, TierLow},
		{0.29, TierSpeculative},
		{0.0, TierSpeculative},
	}

	for _, tt := range tests {
		got := ScoreToTier(tt.score)
		if got != tt.want {
			t.Errorf("ScoreToTier(%v) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestTierFromContext(t *testing.T) {
	tests := []struct {
		name        string
		hasSCIP     bool
		isSCIPFresh bool
		isCrossRepo bool
		want        ConfidenceTier
	}{
		{"cross-repo always speculative", true, true, true, TierSpeculative},
		{"SCIP fresh is high", true, true, false, TierHigh},
		{"SCIP stale is medium", true, false, false, TierMedium},
		{"no SCIP is low", false, false, false, TierLow},
		{"no SCIP fresh ignored", false, true, false, TierLow},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TierFromContext(tt.hasSCIP, tt.isSCIPFresh, tt.isCrossRepo)
			if got != tt.want {
				t.Errorf("TierFromContext(%v, %v, %v) = %q, want %q",
					tt.hasSCIP, tt.isSCIPFresh, tt.isCrossRepo, got, tt.want)
			}
		})
	}
}

func TestBuilderBasic(t *testing.T) {
	resp := New().
		Data(map[string]string{"key": "value"}).
		Build()

	if resp.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", resp.SchemaVersion, CurrentSchemaVersion)
	}

	data, ok := resp.Data.(map[string]string)
	if !ok {
		t.Fatalf("Data type = %T, want map[string]string", resp.Data)
	}
	if data["key"] != "value" {
		t.Errorf("Data[key] = %q, want %q", data["key"], "value")
	}
}

func TestBuilderFromProvenance(t *testing.T) {
	prov := &query.Provenance{
		Backends: []query.BackendContribution{
			{BackendId: "scip", Used: true},
			{BackendId: "git", Used: true},
			{BackendId: "lsp", Used: false},
		},
		RepoStateId: "abc123",
		Completeness: query.CompletenessInfo{
			Score:  0.85,
			Reason: "SCIP primary",
		},
		Warnings: []string{"some warning"},
	}

	resp := New().
		Data(nil).
		FromProvenance(prov).
		Build()

	if resp.Meta == nil {
		t.Fatal("Meta should not be nil")
	}

	// Check provenance
	if resp.Meta.Provenance == nil {
		t.Fatal("Meta.Provenance should not be nil")
	}
	if len(resp.Meta.Provenance.Backends) != 2 {
		t.Errorf("Backends count = %d, want 2", len(resp.Meta.Provenance.Backends))
	}
	if resp.Meta.Provenance.RepoStateID != "abc123" {
		t.Errorf("RepoStateID = %q, want %q", resp.Meta.Provenance.RepoStateID, "abc123")
	}

	// Check confidence
	if resp.Meta.Confidence == nil {
		t.Fatal("Meta.Confidence should not be nil")
	}
	if resp.Meta.Confidence.Score != 0.85 {
		t.Errorf("Confidence.Score = %v, want 0.85", resp.Meta.Confidence.Score)
	}
	if resp.Meta.Confidence.Tier != TierMedium {
		t.Errorf("Confidence.Tier = %q, want %q", resp.Meta.Confidence.Tier, TierMedium)
	}
	if len(resp.Meta.Confidence.Reasons) != 1 || resp.Meta.Confidence.Reasons[0] != "SCIP primary" {
		t.Errorf("Confidence.Reasons = %v, want [SCIP primary]", resp.Meta.Confidence.Reasons)
	}

	// Check warnings
	if len(resp.Warnings) != 1 || resp.Warnings[0].Message != "some warning" {
		t.Errorf("Warnings = %v, want [{Message: some warning}]", resp.Warnings)
	}
}

func TestBuilderFromProvenanceNil(t *testing.T) {
	resp := New().
		Data(nil).
		FromProvenance(nil).
		Build()

	// Should not panic, meta should be nil
	if resp.Meta != nil {
		t.Error("Meta should be nil when provenance is nil")
	}
}

func TestBuilderWithTruncation(t *testing.T) {
	// Not truncated - should not add metadata
	resp := New().
		Data(nil).
		WithTruncation(false, 10, 10, "").
		Build()
	if resp.Meta != nil && resp.Meta.Truncation != nil {
		t.Error("Truncation should not be set when not truncated")
	}

	// Truncated - should add metadata
	resp = New().
		Data(nil).
		WithTruncation(true, 10, 100, "max-symbols").
		Build()

	if resp.Meta == nil || resp.Meta.Truncation == nil {
		t.Fatal("Meta.Truncation should not be nil")
	}
	if !resp.Meta.Truncation.IsTruncated {
		t.Error("IsTruncated should be true")
	}
	if resp.Meta.Truncation.Shown != 10 {
		t.Errorf("Shown = %d, want 10", resp.Meta.Truncation.Shown)
	}
	if resp.Meta.Truncation.Total != 100 {
		t.Errorf("Total = %d, want 100", resp.Meta.Truncation.Total)
	}
	if resp.Meta.Truncation.Reason != "max-symbols" {
		t.Errorf("Reason = %q, want %q", resp.Meta.Truncation.Reason, "max-symbols")
	}
}

func TestBuilderWithFreshness(t *testing.T) {
	// No staleness - should not add metadata
	resp := New().
		Data(nil).
		WithFreshness(0, "").
		Build()
	if resp.Meta != nil && resp.Meta.Freshness != nil {
		t.Error("Freshness should not be set when fresh")
	}

	// Stale - should add metadata
	resp = New().
		Data(nil).
		WithFreshness(10, "behind-head").
		Build()

	if resp.Meta == nil || resp.Meta.Freshness == nil {
		t.Fatal("Meta.Freshness should not be nil")
	}
	if resp.Meta.Freshness.IndexAge.CommitsBehind != 10 {
		t.Errorf("CommitsBehind = %d, want 10", resp.Meta.Freshness.IndexAge.CommitsBehind)
	}
	if resp.Meta.Freshness.IndexAge.StaleReason != "behind-head" {
		t.Errorf("StaleReason = %q, want %q", resp.Meta.Freshness.IndexAge.StaleReason, "behind-head")
	}
}

func TestBuilderWithFreshnessDowngradesConfidence(t *testing.T) {
	// Start with high confidence
	prov := &query.Provenance{
		Completeness: query.CompletenessInfo{Score: 1.0},
	}

	resp := New().
		Data(nil).
		FromProvenance(prov).
		WithFreshness(10, "behind-head"). // >5 commits behind
		Build()

	if resp.Meta.Confidence.Tier != TierMedium {
		t.Errorf("Confidence.Tier = %q, want %q (downgraded due to staleness)",
			resp.Meta.Confidence.Tier, TierMedium)
	}
	if len(resp.Meta.Confidence.Reasons) == 0 {
		t.Error("Should have added index-stale reason")
	}
}

func TestBuilderCrossRepo(t *testing.T) {
	resp := New().
		Data(nil).
		CrossRepo().
		Build()

	if resp.Meta == nil || resp.Meta.Confidence == nil {
		t.Fatal("Meta.Confidence should not be nil")
	}
	if resp.Meta.Confidence.Tier != TierSpeculative {
		t.Errorf("Confidence.Tier = %q, want %q", resp.Meta.Confidence.Tier, TierSpeculative)
	}

	found := false
	for _, r := range resp.Meta.Confidence.Reasons {
		if r == "cross-repo-query" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Should have cross-repo-query reason")
	}
}

func TestBuilderWarning(t *testing.T) {
	resp := New().
		Data(nil).
		Warning("first warning").
		WarningWithCode("W001", "coded warning").
		Build()

	if len(resp.Warnings) != 2 {
		t.Fatalf("Warnings count = %d, want 2", len(resp.Warnings))
	}

	if resp.Warnings[0].Message != "first warning" {
		t.Errorf("Warnings[0].Message = %q, want %q", resp.Warnings[0].Message, "first warning")
	}
	if resp.Warnings[0].Code != "" {
		t.Errorf("Warnings[0].Code = %q, want empty", resp.Warnings[0].Code)
	}

	if resp.Warnings[1].Code != "W001" {
		t.Errorf("Warnings[1].Code = %q, want %q", resp.Warnings[1].Code, "W001")
	}
	if resp.Warnings[1].Message != "coded warning" {
		t.Errorf("Warnings[1].Message = %q, want %q", resp.Warnings[1].Message, "coded warning")
	}
}

func TestBuilderError(t *testing.T) {
	resp := New().
		Data(nil).
		Error(nil).
		Build()
	if resp.Error != nil {
		t.Error("Error should be nil when no error passed")
	}

	testErr := fmt.Errorf("symbol not found")
	resp = New().
		Data(nil).
		Error(testErr).
		Build()
	if resp.Error == nil {
		t.Fatal("Error should not be nil")
	}
	if *resp.Error != "symbol not found" {
		t.Errorf("Error = %q, want %q", *resp.Error, "symbol not found")
	}
}

func TestBuilderSuggestCalls(t *testing.T) {
	drilldowns := []output.Drilldown{
		{Label: "View references", Query: "findReferences sym123"},
		{Label: "Module details", Query: "getModuleOverview --path=internal/query"},
	}

	resp := New().
		Data(nil).
		SuggestCalls(drilldowns).
		Build()

	if len(resp.SuggestedNextCalls) != 2 {
		t.Fatalf("SuggestedNextCalls count = %d, want 2", len(resp.SuggestedNextCalls))
	}

	// Check first call
	call := resp.SuggestedNextCalls[0]
	if call.Tool != "findReferences" {
		t.Errorf("SuggestedNextCalls[0].Tool = %q, want %q", call.Tool, "findReferences")
	}
	if call.Params["symbolId"] != "sym123" {
		t.Errorf("SuggestedNextCalls[0].Params[symbolId] = %v, want %q", call.Params["symbolId"], "sym123")
	}
	if call.Reason != "View references" {
		t.Errorf("SuggestedNextCalls[0].Reason = %q, want %q", call.Reason, "View references")
	}

	// Check second call (flag parameter)
	call = resp.SuggestedNextCalls[1]
	if call.Tool != "getModuleOverview" {
		t.Errorf("SuggestedNextCalls[1].Tool = %q, want %q", call.Tool, "getModuleOverview")
	}
	if call.Params["path"] != "internal/query" {
		t.Errorf("SuggestedNextCalls[1].Params[path] = %v, want %q", call.Params["path"], "internal/query")
	}
}

func TestParseDrilldown(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		want   *SuggestedCall
		params map[string]interface{}
	}{
		{
			name:  "simple positional",
			query: "getSymbol sym123",
			want:  &SuggestedCall{Tool: "getSymbol"},
			params: map[string]interface{}{
				"symbolId": "sym123",
			},
		},
		{
			name:  "flag parameter",
			query: "searchSymbols --query=Foo",
			want:  &SuggestedCall{Tool: "searchSymbols"},
			params: map[string]interface{}{
				"query": "Foo",
			},
		},
		{
			name:  "mixed params",
			query: "explainFile path/to/file.go --verbose=true",
			want:  &SuggestedCall{Tool: "explainFile"},
			params: map[string]interface{}{
				"filePath": "path/to/file.go",
				"verbose":  "true",
			},
		},
		{
			name:  "empty query",
			query: "",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := output.Drilldown{Query: tt.query, Label: "test"}
			got := ParseDrilldown(d)

			if tt.want == nil {
				if got != nil {
					t.Errorf("ParseDrilldown(%q) = %v, want nil", tt.query, got)
				}
				return
			}

			if got == nil {
				t.Fatalf("ParseDrilldown(%q) = nil, want non-nil", tt.query)
			}

			if got.Tool != tt.want.Tool {
				t.Errorf("Tool = %q, want %q", got.Tool, tt.want.Tool)
			}

			for k, v := range tt.params {
				if got.Params[k] != v {
					t.Errorf("Params[%q] = %v, want %v", k, got.Params[k], v)
				}
			}
		})
	}
}

func TestOperational(t *testing.T) {
	data := map[string]bool{"healthy": true}
	resp := Operational(data)

	if resp.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", resp.SchemaVersion, CurrentSchemaVersion)
	}

	if resp.Meta == nil || resp.Meta.Confidence == nil {
		t.Fatal("Meta.Confidence should not be nil")
	}
	if resp.Meta.Confidence.Score != 1.0 {
		t.Errorf("Confidence.Score = %v, want 1.0", resp.Meta.Confidence.Score)
	}
	if resp.Meta.Confidence.Tier != TierHigh {
		t.Errorf("Confidence.Tier = %q, want %q", resp.Meta.Confidence.Tier, TierHigh)
	}
}

func TestResponseJSONSerialization(t *testing.T) {
	resp := New().
		Data(map[string]string{"foo": "bar"}).
		Warning("test warning").
		CrossRepo().
		Build()

	jsonBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed Response
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if parsed.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", parsed.SchemaVersion, CurrentSchemaVersion)
	}

	if len(parsed.Warnings) != 1 {
		t.Errorf("Warnings count = %d, want 1", len(parsed.Warnings))
	}

	if parsed.Meta == nil || parsed.Meta.Confidence == nil {
		t.Fatal("Meta.Confidence should not be nil")
	}
	if parsed.Meta.Confidence.Tier != TierSpeculative {
		t.Errorf("Confidence.Tier = %q, want %q", parsed.Meta.Confidence.Tier, TierSpeculative)
	}
}

func TestBuilderChaining(t *testing.T) {
	// Test that builder methods return the same builder for chaining
	builder := New()
	b1 := builder.Data(nil)
	if b1 != builder {
		t.Error("Data() should return same builder")
	}

	b2 := builder.Warning("test")
	if b2 != builder {
		t.Error("Warning() should return same builder")
	}

	b3 := builder.CrossRepo()
	if b3 != builder {
		t.Error("CrossRepo() should return same builder")
	}
}
