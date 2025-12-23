package mcp

import (
	"encoding/json"
	"testing"

	"ckb/internal/envelope"
	"ckb/internal/output"
	"ckb/internal/query"
)

func TestNewToolResponse(t *testing.T) {
	tr := NewToolResponse()
	if tr == nil {
		t.Fatal("NewToolResponse() returned nil")
	}
	if tr.builder == nil {
		t.Fatal("NewToolResponse().builder is nil")
	}
}

func TestToolResponseData(t *testing.T) {
	data := map[string]int{"count": 42}
	resp := NewToolResponse().
		Data(data).
		Build()

	if resp.SchemaVersion != envelope.CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", resp.SchemaVersion, envelope.CurrentSchemaVersion)
	}

	got, ok := resp.Data.(map[string]int)
	if !ok {
		t.Fatalf("Data type = %T, want map[string]int", resp.Data)
	}
	if got["count"] != 42 {
		t.Errorf("Data[count] = %d, want 42", got["count"])
	}
}

func TestToolResponseWithProvenance(t *testing.T) {
	prov := &query.Provenance{
		Backends: []query.BackendContribution{
			{BackendId: "scip", Used: true},
		},
		RepoStateId: "test-state",
		Completeness: query.CompletenessInfo{
			Score: 0.95,
		},
	}

	resp := NewToolResponse().
		Data(nil).
		WithProvenance(prov).
		Build()

	if resp.Meta == nil {
		t.Fatal("Meta should not be nil")
	}
	if resp.Meta.Provenance == nil {
		t.Fatal("Meta.Provenance should not be nil")
	}
	if len(resp.Meta.Provenance.Backends) != 1 {
		t.Errorf("Backends count = %d, want 1", len(resp.Meta.Provenance.Backends))
	}
	if resp.Meta.Provenance.Backends[0] != "scip" {
		t.Errorf("Backends[0] = %q, want %q", resp.Meta.Provenance.Backends[0], "scip")
	}

	if resp.Meta.Confidence == nil {
		t.Fatal("Meta.Confidence should not be nil")
	}
	if resp.Meta.Confidence.Tier != envelope.TierHigh {
		t.Errorf("Confidence.Tier = %q, want %q", resp.Meta.Confidence.Tier, envelope.TierHigh)
	}
}

func TestToolResponseWithProvenanceNil(t *testing.T) {
	// Should not panic when provenance is nil
	resp := NewToolResponse().
		Data(nil).
		WithProvenance(nil).
		Build()

	// Meta may or may not be nil, but should not panic
	_ = resp
}

func TestToolResponseWithTruncation(t *testing.T) {
	// Not truncated
	resp := NewToolResponse().
		Data(nil).
		WithTruncation(false, 10, 10, "").
		Build()

	if resp.Meta != nil && resp.Meta.Truncation != nil {
		t.Error("Truncation should not be set when not truncated")
	}

	// Truncated
	resp = NewToolResponse().
		Data(nil).
		WithTruncation(true, 50, 500, "max-results").
		Build()

	if resp.Meta == nil || resp.Meta.Truncation == nil {
		t.Fatal("Meta.Truncation should not be nil when truncated")
	}
	if !resp.Meta.Truncation.IsTruncated {
		t.Error("IsTruncated should be true")
	}
	if resp.Meta.Truncation.Shown != 50 {
		t.Errorf("Shown = %d, want 50", resp.Meta.Truncation.Shown)
	}
	if resp.Meta.Truncation.Total != 500 {
		t.Errorf("Total = %d, want 500", resp.Meta.Truncation.Total)
	}
}

func TestToolResponseWithFreshness(t *testing.T) {
	resp := NewToolResponse().
		Data(nil).
		WithFreshness(3, "minor-drift").
		Build()

	if resp.Meta == nil || resp.Meta.Freshness == nil {
		t.Fatal("Meta.Freshness should not be nil")
	}
	if resp.Meta.Freshness.IndexAge.CommitsBehind != 3 {
		t.Errorf("CommitsBehind = %d, want 3", resp.Meta.Freshness.IndexAge.CommitsBehind)
	}
}

func TestToolResponseWithDrilldowns(t *testing.T) {
	drilldowns := []output.Drilldown{
		{Label: "See callers", Query: "getCallGraph sym1"},
		{Label: "View module", Query: "getModuleOverview internal/query"},
	}

	resp := NewToolResponse().
		Data(nil).
		WithDrilldowns(drilldowns).
		Build()

	if len(resp.SuggestedNextCalls) != 2 {
		t.Fatalf("SuggestedNextCalls count = %d, want 2", len(resp.SuggestedNextCalls))
	}

	if resp.SuggestedNextCalls[0].Tool != "getCallGraph" {
		t.Errorf("SuggestedNextCalls[0].Tool = %q, want %q",
			resp.SuggestedNextCalls[0].Tool, "getCallGraph")
	}
	if resp.SuggestedNextCalls[0].Reason != "See callers" {
		t.Errorf("SuggestedNextCalls[0].Reason = %q, want %q",
			resp.SuggestedNextCalls[0].Reason, "See callers")
	}
}

func TestToolResponseWithDrilldownsEmpty(t *testing.T) {
	resp := NewToolResponse().
		Data(nil).
		WithDrilldowns(nil).
		Build()

	if len(resp.SuggestedNextCalls) != 0 {
		t.Errorf("SuggestedNextCalls should be empty, got %d", len(resp.SuggestedNextCalls))
	}
}

func TestToolResponseWarning(t *testing.T) {
	resp := NewToolResponse().
		Data(nil).
		Warning("some warning").
		Warning("another warning").
		Build()

	if len(resp.Warnings) != 2 {
		t.Fatalf("Warnings count = %d, want 2", len(resp.Warnings))
	}
	if resp.Warnings[0].Message != "some warning" {
		t.Errorf("Warnings[0].Message = %q, want %q", resp.Warnings[0].Message, "some warning")
	}
}

func TestToolResponseCrossRepo(t *testing.T) {
	resp := NewToolResponse().
		Data(nil).
		CrossRepo().
		Build()

	if resp.Meta == nil || resp.Meta.Confidence == nil {
		t.Fatal("Meta.Confidence should not be nil")
	}
	if resp.Meta.Confidence.Tier != envelope.TierSpeculative {
		t.Errorf("Confidence.Tier = %q, want %q",
			resp.Meta.Confidence.Tier, envelope.TierSpeculative)
	}
}

func TestToolResponseChaining(t *testing.T) {
	// Verify all methods return *ToolResponse for chaining
	tr := NewToolResponse()
	t1 := tr.Data(nil)
	if t1 != tr {
		t.Error("Data() should return same ToolResponse")
	}

	t2 := tr.WithProvenance(nil)
	if t2 != tr {
		t.Error("WithProvenance() should return same ToolResponse")
	}

	t3 := tr.WithTruncation(false, 0, 0, "")
	if t3 != tr {
		t.Error("WithTruncation() should return same ToolResponse")
	}

	t4 := tr.WithFreshness(0, "")
	if t4 != tr {
		t.Error("WithFreshness() should return same ToolResponse")
	}

	t5 := tr.WithDrilldowns(nil)
	if t5 != tr {
		t.Error("WithDrilldowns() should return same ToolResponse")
	}

	t6 := tr.Warning("test")
	if t6 != tr {
		t.Error("Warning() should return same ToolResponse")
	}

	t7 := tr.CrossRepo()
	if t7 != tr {
		t.Error("CrossRepo() should return same ToolResponse")
	}
}

func TestOperationalResponse(t *testing.T) {
	data := map[string]interface{}{
		"healthy":  true,
		"backends": []string{"scip", "git"},
	}

	resp := OperationalResponse(data)

	if resp.SchemaVersion != envelope.CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", resp.SchemaVersion, envelope.CurrentSchemaVersion)
	}

	if resp.Meta == nil || resp.Meta.Confidence == nil {
		t.Fatal("Meta.Confidence should not be nil for operational response")
	}
	if resp.Meta.Confidence.Score != 1.0 {
		t.Errorf("Confidence.Score = %v, want 1.0", resp.Meta.Confidence.Score)
	}
	if resp.Meta.Confidence.Tier != envelope.TierHigh {
		t.Errorf("Confidence.Tier = %q, want %q",
			resp.Meta.Confidence.Tier, envelope.TierHigh)
	}

	// Data should be preserved
	got, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Data type = %T, want map[string]interface{}", resp.Data)
	}
	if got["healthy"] != true {
		t.Errorf("Data[healthy] = %v, want true", got["healthy"])
	}
}

func TestFullToolResponseFlow(t *testing.T) {
	// Simulate a typical tool response construction
	prov := &query.Provenance{
		Backends: []query.BackendContribution{
			{BackendId: "scip", Used: true},
			{BackendId: "git", Used: true},
		},
		RepoStateId: "abc123def456",
		Completeness: query.CompletenessInfo{
			Score:  0.92,
			Reason: "SCIP+git hybrid",
		},
		Warnings: []string{"index slightly stale"},
	}

	drilldowns := []output.Drilldown{
		{Label: "Get references", Query: "findReferences sym:abc"},
	}

	resp := NewToolResponse().
		Data(map[string]interface{}{
			"symbols": []string{"foo", "bar", "baz"},
			"count":   3,
		}).
		WithProvenance(prov).
		WithTruncation(true, 3, 100, "max-symbols").
		WithDrilldowns(drilldowns).
		Build()

	// Verify all metadata was set
	if resp.SchemaVersion != envelope.CurrentSchemaVersion {
		t.Errorf("SchemaVersion = %q, want %q", resp.SchemaVersion, envelope.CurrentSchemaVersion)
	}

	if resp.Meta == nil {
		t.Fatal("Meta should not be nil")
	}

	// Confidence
	if resp.Meta.Confidence == nil {
		t.Fatal("Meta.Confidence should not be nil")
	}
	if resp.Meta.Confidence.Score != 0.92 {
		t.Errorf("Confidence.Score = %v, want 0.92", resp.Meta.Confidence.Score)
	}
	// 0.92 -> medium tier
	if resp.Meta.Confidence.Tier != envelope.TierMedium {
		t.Errorf("Confidence.Tier = %q, want %q", resp.Meta.Confidence.Tier, envelope.TierMedium)
	}

	// Provenance
	if resp.Meta.Provenance == nil {
		t.Fatal("Meta.Provenance should not be nil")
	}
	if len(resp.Meta.Provenance.Backends) != 2 {
		t.Errorf("Backends count = %d, want 2", len(resp.Meta.Provenance.Backends))
	}

	// Truncation
	if resp.Meta.Truncation == nil {
		t.Fatal("Meta.Truncation should not be nil")
	}
	if !resp.Meta.Truncation.IsTruncated {
		t.Error("IsTruncated should be true")
	}
	if resp.Meta.Truncation.Shown != 3 {
		t.Errorf("Shown = %d, want 3", resp.Meta.Truncation.Shown)
	}

	// Warnings
	if len(resp.Warnings) != 1 {
		t.Fatalf("Warnings count = %d, want 1", len(resp.Warnings))
	}

	// Suggested calls
	if len(resp.SuggestedNextCalls) != 1 {
		t.Fatalf("SuggestedNextCalls count = %d, want 1", len(resp.SuggestedNextCalls))
	}

	// Verify JSON serialization works
	jsonBytes, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed envelope.Response
	if err := json.Unmarshal(jsonBytes, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Verify parsed response matches original
	if parsed.SchemaVersion != resp.SchemaVersion {
		t.Errorf("Parsed SchemaVersion = %q, want %q",
			parsed.SchemaVersion, resp.SchemaVersion)
	}
	if parsed.Meta.Confidence.Tier != resp.Meta.Confidence.Tier {
		t.Errorf("Parsed Confidence.Tier = %q, want %q",
			parsed.Meta.Confidence.Tier, resp.Meta.Confidence.Tier)
	}
}

func TestCrossRepoWithProvenance(t *testing.T) {
	// When both provenance and CrossRepo are set,
	// CrossRepo should override to speculative tier
	prov := &query.Provenance{
		Completeness: query.CompletenessInfo{
			Score: 0.95, // Would normally be high tier
		},
	}

	resp := NewToolResponse().
		Data(nil).
		WithProvenance(prov).
		CrossRepo().
		Build()

	// CrossRepo should override the high confidence from provenance
	if resp.Meta.Confidence.Tier != envelope.TierSpeculative {
		t.Errorf("Confidence.Tier = %q, want %q (CrossRepo should override)",
			resp.Meta.Confidence.Tier, envelope.TierSpeculative)
	}
}
