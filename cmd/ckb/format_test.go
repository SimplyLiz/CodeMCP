package main

import (
	"strings"
	"testing"
)

func TestFormatResponse_JSON(t *testing.T) {
	resp := map[string]interface{}{
		"key": "value",
		"num": 42,
	}

	result, err := FormatResponse(resp, FormatJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, `"key": "value"`) {
		t.Error("JSON output missing expected key")
	}
	if !strings.Contains(result, `"num": 42`) {
		t.Error("JSON output missing expected number")
	}
}

func TestFormatResponse_UnsupportedFormat(t *testing.T) {
	resp := map[string]string{"key": "value"}

	_, err := FormatResponse(resp, "xml")
	if err == nil {
		t.Error("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("error should mention unsupported format, got: %v", err)
	}
}

func TestFormatJSON(t *testing.T) {
	resp := struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}{
		Name:  "test",
		Value: 123,
	}

	result, err := formatJSON(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, `"name": "test"`) {
		t.Error("missing name field")
	}
	if !strings.Contains(result, `"value": 123`) {
		t.Error("missing value field")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1048576, "1.0 MiB"},
		{1073741824, "1.0 GiB"},
		{1099511627776, "1.0 TiB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, result, tt.expected)
			}
		})
	}
}

func TestFormatHuman_UnknownType(t *testing.T) {
	// For unknown types, should fall back to JSON with a note
	resp := struct {
		Foo string `json:"foo"`
	}{Foo: "bar"}

	result, err := formatHuman(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Human format not available") {
		t.Error("missing fallback message")
	}
	if !strings.Contains(result, `"foo": "bar"`) {
		t.Error("missing JSON content")
	}
}

func TestFormatStatusHuman(t *testing.T) {
	resp := &StatusResponseCLI{
		CkbVersion: "7.5.0",
		Healthy:    true,
		Backends: []BackendStatusCLI{
			{ID: "scip", Available: true, Details: "100 symbols"},
		},
		Cache: CacheStatusCLI{
			QueryCount: 50,
			HitRate:    0.85,
			SizeBytes:  2048,
		},
	}

	result, err := formatStatusHuman(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "CKB v7.5.0") {
		t.Error("missing version")
	}
	if !strings.Contains(result, "✓ System: Healthy") {
		t.Error("missing health status")
	}
	if !strings.Contains(result, "✓ scip") {
		t.Error("missing backend")
	}
	if !strings.Contains(result, "50 queries") {
		t.Error("missing cache stats")
	}
}

func TestFormatStatusHuman_Unhealthy(t *testing.T) {
	resp := &StatusResponseCLI{
		CkbVersion: "7.5.0",
		Healthy:    false,
		Backends: []BackendStatusCLI{
			{ID: "scip", Available: false},
		},
	}

	result, err := formatStatusHuman(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "✗ System: Issues detected") {
		t.Error("missing unhealthy status")
	}
	if !strings.Contains(result, "✗ scip") {
		t.Error("missing unavailable backend")
	}
}

func TestFormatDoctorHuman(t *testing.T) {
	resp := &DoctorResponseCLI{
		Healthy: true,
		Checks: []DoctorCheckCLI{
			{Name: "scip-index", Status: "pass", Message: "Index found"},
			{Name: "git-repo", Status: "warn", Message: "Dirty", SuggestedFixes: []FixActionCLI{{Command: "git stash"}}},
			{Name: "config", Status: "fail", Message: "Missing config"},
		},
	}

	result, err := formatDoctorHuman(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "CKB Doctor") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "✓ scip-index") {
		t.Error("missing pass check")
	}
	if !strings.Contains(result, "⚠ git-repo") {
		t.Error("missing warn check")
	}
	if !strings.Contains(result, "✗ config") {
		t.Error("missing fail check")
	}
	if !strings.Contains(result, "git stash") {
		t.Error("missing suggested fix")
	}
	if !strings.Contains(result, "All checks passed") {
		t.Error("missing healthy message")
	}
}

func TestFormatSearchHuman(t *testing.T) {
	resp := &SearchResponseCLI{
		Query:        "Engine",
		TotalMatches: 2,
		Symbols: []SearchSymbolCLI{
			{
				Name:           "Engine",
				Kind:           "struct",
				StableID:       "sym1",
				ModuleID:       "query",
				RelevanceScore: 0.95,
				Location:       &LocationCLI{FileID: "engine.go", StartLine: 10},
			},
		},
	}

	result, err := formatSearchHuman(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Search Results for: Engine") {
		t.Error("missing query")
	}
	if !strings.Contains(result, "Found 2 matches") {
		t.Error("missing match count")
	}
	if !strings.Contains(result, "Engine (struct)") {
		t.Error("missing symbol")
	}
}

func TestFormatImpactHuman(t *testing.T) {
	resp := &ImpactResponseCLI{
		SymbolID: "sym123",
		RiskScore: &RiskScoreCLI{
			Level:       "medium",
			Score:       0.5,
			Explanation: "Moderate impact",
		},
		DirectImpact: []ImpactItemCLI{
			{Name: "Caller1", Kind: "direct-caller"},
			{Name: "Caller2", Kind: "direct-caller"},
		},
		TransitiveImpact: []ImpactItemCLI{
			{Name: "Trans1", Kind: "transitive-caller"},
		},
		ModulesAffected: []ModuleImpactCLI{
			{ModuleID: "mod1", ImpactCount: 3},
		},
	}

	result, err := formatImpactHuman(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Impact Analysis: sym123") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "Risk Level: medium") {
		t.Error("missing risk level")
	}
	if !strings.Contains(result, "Direct Impact: 2 symbols") {
		t.Error("missing direct impact count")
	}
	if !strings.Contains(result, "Transitive Impact: 1 symbols") {
		t.Error("missing transitive impact count")
	}
	if !strings.Contains(result, "mod1: 3 symbols") {
		t.Error("missing module impact")
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{5, 5, 5},
		{0, 10, 0},
		{-5, 5, -5},
	}

	for _, tt := range tests {
		result := min(tt.a, tt.b)
		if result != tt.want {
			t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, result, tt.want)
		}
	}
}
