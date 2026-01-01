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

func TestFormatSymbolHuman(t *testing.T) {
	resp := &SymbolResponseCLI{
		Symbol: SymbolInfoCLI{
			StableID:             "sym123",
			Name:                 "Engine",
			Kind:                 "struct",
			Visibility:           "public",
			VisibilityConfidence: 0.95,
			ContainerName:        "query",
		},
		Location: &LocationCLI{
			FileID:      "engine.go",
			StartLine:   10,
			StartColumn: 5,
		},
		Module: &ModuleInfoCLI{
			ModuleID: "internal/query",
		},
	}

	result, err := formatSymbolHuman(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Symbol Details") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "Name: Engine") {
		t.Error("missing name")
	}
	if !strings.Contains(result, "Kind: struct") {
		t.Error("missing kind")
	}
	if !strings.Contains(result, "Container: query") {
		t.Error("missing container")
	}
	if !strings.Contains(result, "engine.go:10:5") {
		t.Error("missing location")
	}
	if !strings.Contains(result, "Module: internal/query") {
		t.Error("missing module")
	}
}

func TestFormatSymbolHuman_MinimalFields(t *testing.T) {
	resp := &SymbolResponseCLI{
		Symbol: SymbolInfoCLI{
			StableID:   "sym456",
			Name:       "Foo",
			Kind:       "function",
			Visibility: "private",
		},
	}

	result, err := formatSymbolHuman(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Name: Foo") {
		t.Error("missing name")
	}
	// Should not contain optional fields
	if strings.Contains(result, "Container:") {
		t.Error("should not have container when empty")
	}
	if strings.Contains(result, "Location:") {
		t.Error("should not have location when nil")
	}
}

func TestFormatRefsHuman(t *testing.T) {
	resp := &ReferencesResponseCLI{
		SymbolID:        "sym123",
		TotalReferences: 3,
		ByModule: []ModuleReferencesCLI{
			{ModuleID: "module1", Count: 2},
			{ModuleID: "module2", Count: 1},
		},
		References: []ReferenceCLI{
			{Location: &LocationCLI{FileID: "foo.go", StartLine: 10}, Kind: "call", IsTest: false},
			{Location: &LocationCLI{FileID: "bar.go", StartLine: 20}, Kind: "call", IsTest: true},
			{Location: &LocationCLI{FileID: "baz.go", StartLine: 30}, Kind: "import", IsTest: false},
		},
	}

	result, err := formatRefsHuman(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "References to: sym123") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "Total references: 3") {
		t.Error("missing total count")
	}
	if !strings.Contains(result, "module1: 2 references") {
		t.Error("missing module1 count")
	}
	if !strings.Contains(result, "foo.go:10 (call)") {
		t.Error("missing first reference")
	}
	if !strings.Contains(result, "[test]") {
		t.Error("missing test marker")
	}
}

func TestFormatRefsHuman_EmptyRefs(t *testing.T) {
	resp := &ReferencesResponseCLI{
		SymbolID:        "sym456",
		TotalReferences: 0,
		References:      []ReferenceCLI{},
	}

	result, err := formatRefsHuman(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Total references: 0") {
		t.Error("missing zero count")
	}
}

func TestFormatArchHuman(t *testing.T) {
	resp := &ArchitectureResponseCLI{
		Modules: []ModuleSummaryCLI{
			{
				ModuleID:      "mod1",
				Name:          "Query Engine",
				RootPath:      "internal/query",
				Language:      "go",
				FileCount:     10,
				SymbolCount:   50,
				IncomingEdges: 5,
				OutgoingEdges: 3,
			},
		},
		DependencyGraph: []DependencyEdgeCLI{
			{From: "mod1", To: "mod2", Kind: "import"},
		},
		Entrypoints: []EntrypointCLI{
			{Name: "main", Kind: "binary", FileID: "cmd/ckb/main.go"},
		},
	}

	result, err := formatArchHuman(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Architecture Overview") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "Modules: 1") {
		t.Error("missing module count")
	}
	if !strings.Contains(result, "Dependencies: 1") {
		t.Error("missing dependency count")
	}
	if !strings.Contains(result, "Entrypoints: 1") {
		t.Error("missing entrypoint count")
	}
	if !strings.Contains(result, "Query Engine (go)") {
		t.Error("missing module name with language")
	}
	if !strings.Contains(result, "Path: internal/query") {
		t.Error("missing module path")
	}
	if !strings.Contains(result, "Files: 10, Symbols: 50") {
		t.Error("missing file/symbol counts")
	}
	if !strings.Contains(result, "Deps: 5 incoming, 3 outgoing") {
		t.Error("missing edge counts")
	}
	if !strings.Contains(result, "main (binary)") {
		t.Error("missing entrypoint")
	}
}

func TestFormatArchHuman_Empty(t *testing.T) {
	resp := &ArchitectureResponseCLI{
		Modules:         []ModuleSummaryCLI{},
		DependencyGraph: []DependencyEdgeCLI{},
		Entrypoints:     []EntrypointCLI{},
	}

	result, err := formatArchHuman(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Modules: 0") {
		t.Error("missing zero module count")
	}
}

func TestFormatCallgraphHuman(t *testing.T) {
	resp := &CallgraphResponseCLI{
		Root: "Engine.Search",
		Nodes: []CallgraphNodeCLI{
			{ID: "1", Name: "Caller1", Depth: -1, Role: "caller"},
			{ID: "2", Name: "Caller2", Depth: -2, Role: "caller"},
			{ID: "3", Name: "Callee1", Depth: 1, Role: "callee"},
		},
		Edges: []CallgraphEdgeCLI{
			{From: "1", To: "root"},
			{From: "root", To: "3"},
		},
	}

	result, err := formatCallgraphHuman(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Call Graph for: Engine.Search") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "Nodes: 3, Edges: 2") {
		t.Error("missing counts")
	}
	if !strings.Contains(result, "Callers (who calls this)") {
		t.Error("missing callers section")
	}
	if !strings.Contains(result, "Callees (what this calls)") {
		t.Error("missing callees section")
	}
	if !strings.Contains(result, "Caller1 (caller)") {
		t.Error("missing caller node")
	}
	if !strings.Contains(result, "Callee1 (callee)") {
		t.Error("missing callee node")
	}
}

func TestFormatCallgraphHuman_Empty(t *testing.T) {
	resp := &CallgraphResponseCLI{
		Root:  "Orphan",
		Nodes: []CallgraphNodeCLI{},
		Edges: []CallgraphEdgeCLI{},
	}

	result, err := formatCallgraphHuman(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Nodes: 0, Edges: 0") {
		t.Error("missing zero counts")
	}
}

func TestFormatHotspotsHuman(t *testing.T) {
	resp := &HotspotsResponseCLI{
		TimeWindow: "90 days",
		TotalCount: 2,
		Hotspots: []HotspotCLI{
			{
				FilePath:  "internal/query/engine.go",
				Score:     0.95,
				RiskLevel: "high",
				Churn:     HotspotChurnCLI{ChangeCount: 50, AuthorCount: 5},
			},
			{
				FilePath:  "internal/mcp/server.go",
				Score:     0.75,
				RiskLevel: "medium",
				Churn:     HotspotChurnCLI{ChangeCount: 25, AuthorCount: 3},
			},
		},
	}

	result, err := formatHotspotsHuman(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "Code Hotspots") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "Total Hotspots: 2 (period: 90 days)") {
		t.Error("missing total count and period")
	}
	if !strings.Contains(result, "1. internal/query/engine.go") {
		t.Error("missing first hotspot")
	}
	if !strings.Contains(result, "Risk: high") {
		t.Error("missing risk level")
	}
	if !strings.Contains(result, "Changes: 50, Authors: 5") {
		t.Error("missing churn stats")
	}
}

func TestFormatJustifyHuman(t *testing.T) {
	tests := []struct {
		name     string
		verdict  string
		wantIcon string
	}{
		{"keep verdict", "keep", "✓"},
		{"investigate verdict", "investigate", "⚠"},
		{"remove verdict", "remove", "✗"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &JustifyResponseCLI{
				SymbolId:   "sym123",
				Verdict:    tt.verdict,
				Confidence: 0.85,
				Reasoning:  "Test reasoning",
			}

			result, err := formatJustifyHuman(resp)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !strings.Contains(result, "Symbol Justification: sym123") {
				t.Error("missing header")
			}
			if !strings.Contains(result, tt.wantIcon) {
				t.Errorf("missing verdict icon %q", tt.wantIcon)
			}
			if !strings.Contains(result, tt.verdict) {
				t.Errorf("missing verdict %q", tt.verdict)
			}
			if !strings.Contains(result, "85%") {
				t.Error("missing confidence percentage")
			}
			if !strings.Contains(result, "Reasoning: Test reasoning") {
				t.Error("missing reasoning")
			}
		})
	}
}
