package main

import (
	"strings"
	"testing"

	"ckb/internal/query"
)

func TestConvertImpactResponse(t *testing.T) {
	resp := &query.AnalyzeImpactResponse{
		Symbol: &query.SymbolInfo{
			StableId: "sym123",
			Name:     "MyFunc",
			Kind:     "function",
			Visibility: &query.VisibilityInfo{
				Visibility: "public",
				Confidence: 0.9,
			},
		},
		RiskScore: &query.RiskScore{
			Level:       "medium",
			Score:       0.5,
			Explanation: "Moderate risk",
			Factors: []query.RiskFactor{
				{Name: "visibility", Value: 0.6, Weight: 0.3},
			},
		},
		DirectImpact: []query.ImpactItem{
			{
				StableId:   "caller1",
				Name:       "Caller1",
				Kind:       "direct-caller",
				Distance:   1,
				ModuleId:   "mod1",
				Confidence: 0.9,
				Location: &query.LocationInfo{
					FileId:      "caller.go",
					StartLine:   10,
					StartColumn: 5,
				},
			},
		},
		TransitiveImpact: []query.ImpactItem{
			{
				StableId:   "trans1",
				Name:       "Trans1",
				Kind:       "transitive-caller",
				Distance:   2,
				ModuleId:   "mod2",
				Confidence: 0.7,
			},
		},
		ModulesAffected: []query.ModuleImpact{
			{ModuleId: "mod1", Name: "Module One", ImpactCount: 5, DirectCount: 3},
		},
		Provenance: &query.Provenance{
			RepoStateId:     "abc123",
			RepoStateDirty:  false,
			QueryDurationMs: 100,
		},
	}

	result := convertImpactResponse("sym123", resp)

	// Check basic fields
	if result.SymbolID != "sym123" {
		t.Errorf("SymbolID: got %q, want %q", result.SymbolID, "sym123")
	}

	// Check symbol info
	if result.Symbol == nil {
		t.Fatal("Symbol is nil")
	}
	if result.Symbol.Name != "MyFunc" {
		t.Errorf("Symbol.Name: got %q, want %q", result.Symbol.Name, "MyFunc")
	}
	if result.Symbol.Visibility != "public" {
		t.Errorf("Symbol.Visibility: got %q, want %q", result.Symbol.Visibility, "public")
	}

	// Check risk score
	if result.RiskScore == nil {
		t.Fatal("RiskScore is nil")
	}
	if result.RiskScore.Level != "medium" {
		t.Errorf("RiskScore.Level: got %q, want %q", result.RiskScore.Level, "medium")
	}
	if len(result.RiskScore.Factors) != 1 {
		t.Errorf("RiskScore.Factors: got %d, want 1", len(result.RiskScore.Factors))
	}

	// Check direct impact
	if len(result.DirectImpact) != 1 {
		t.Fatalf("DirectImpact: got %d, want 1", len(result.DirectImpact))
	}
	if result.DirectImpact[0].StableID != "caller1" {
		t.Errorf("DirectImpact[0].StableID: got %q, want %q", result.DirectImpact[0].StableID, "caller1")
	}
	if result.DirectImpact[0].Location == nil {
		t.Error("DirectImpact[0].Location is nil")
	}

	// Check transitive impact
	if len(result.TransitiveImpact) != 1 {
		t.Fatalf("TransitiveImpact: got %d, want 1", len(result.TransitiveImpact))
	}

	// Check modules affected
	if len(result.ModulesAffected) != 1 {
		t.Fatalf("ModulesAffected: got %d, want 1", len(result.ModulesAffected))
	}

	// Check provenance
	if result.Provenance == nil {
		t.Fatal("Provenance is nil")
	}
	if result.Provenance.RepoStateId != "abc123" {
		t.Errorf("Provenance.RepoStateId: got %q, want %q", result.Provenance.RepoStateId, "abc123")
	}
}

func TestConvertImpactResponse_NilFields(t *testing.T) {
	// Test with minimal/nil fields
	resp := &query.AnalyzeImpactResponse{
		DirectImpact:     []query.ImpactItem{},
		TransitiveImpact: nil,
		ModulesAffected:  nil,
	}

	result := convertImpactResponse("sym", resp)

	if result.Symbol != nil {
		t.Error("expected nil Symbol")
	}
	if result.RiskScore != nil {
		t.Error("expected nil RiskScore")
	}
	if result.Provenance != nil {
		t.Error("expected nil Provenance")
	}
	if len(result.DirectImpact) != 0 {
		t.Errorf("DirectImpact should be empty, got %d", len(result.DirectImpact))
	}
}

func TestConvertChangeSetResponse(t *testing.T) {
	resp := &query.AnalyzeChangeSetResponse{
		Summary: &query.ChangeSummary{
			FilesChanged:         5,
			SymbolsChanged:       10,
			DirectlyAffected:     15,
			TransitivelyAffected: 25,
			EstimatedRisk:        "high",
		},
		ChangedSymbols: []query.ChangedSymbolInfo{
			{
				SymbolID:   "sym1",
				Name:       "Foo",
				File:       "foo.go",
				ChangeType: "modified",
				Lines:      []int{10, 20, 30},
				Confidence: 0.9,
			},
		},
		AffectedSymbols: []query.ImpactItem{
			{
				StableId:   "affected1",
				Name:       "Bar",
				Kind:       "direct-caller",
				Distance:   1,
				ModuleId:   "mod1",
				Confidence: 0.8,
			},
		},
		ModulesAffected: []query.ModuleImpact{
			{ModuleId: "mod1", Name: "Module One", ImpactCount: 5, DirectCount: 3},
		},
		BlastRadius: &query.BlastRadiusSummary{
			ModuleCount:       2,
			FileCount:         10,
			UniqueCallerCount: 15,
			RiskLevel:         "high",
		},
		RiskScore: &query.RiskScore{
			Level:       "high",
			Score:       0.75,
			Explanation: "High risk change",
		},
		Recommendations: []query.Recommendation{
			{Type: "review", Severity: "warning", Message: "Needs review", Action: "Request review"},
		},
		IndexStaleness: &query.IndexStalenessInfo{
			IsStale:          true,
			CommitsBehind:    3,
			IndexedCommit:    "abc",
			HeadCommit:       "def",
			StalenessMessage: "Index is stale",
		},
		Provenance: &query.Provenance{
			RepoStateId:     "state123",
			QueryDurationMs: 150,
		},
	}

	result := convertChangeSetResponse(resp)

	// Check summary
	if result.Summary == nil {
		t.Fatal("Summary is nil")
	}
	if result.Summary.FilesChanged != 5 {
		t.Errorf("Summary.FilesChanged: got %d, want 5", result.Summary.FilesChanged)
	}
	if result.Summary.EstimatedRisk != "high" {
		t.Errorf("Summary.EstimatedRisk: got %q, want %q", result.Summary.EstimatedRisk, "high")
	}

	// Check changed symbols
	if len(result.ChangedSymbols) != 1 {
		t.Fatalf("ChangedSymbols: got %d, want 1", len(result.ChangedSymbols))
	}
	if result.ChangedSymbols[0].SymbolID != "sym1" {
		t.Errorf("ChangedSymbols[0].SymbolID: got %q, want %q", result.ChangedSymbols[0].SymbolID, "sym1")
	}

	// Check blast radius
	if result.BlastRadius == nil {
		t.Fatal("BlastRadius is nil")
	}
	if result.BlastRadius.ModuleCount != 2 {
		t.Errorf("BlastRadius.ModuleCount: got %d, want 2", result.BlastRadius.ModuleCount)
	}

	// Check risk score
	if result.RiskScore == nil {
		t.Fatal("RiskScore is nil")
	}
	if result.RiskScore.Level != "high" {
		t.Errorf("RiskScore.Level: got %q, want %q", result.RiskScore.Level, "high")
	}

	// Check recommendations
	if len(result.Recommendations) != 1 {
		t.Fatalf("Recommendations: got %d, want 1", len(result.Recommendations))
	}
	if result.Recommendations[0].Type != "review" {
		t.Errorf("Recommendations[0].Type: got %q, want %q", result.Recommendations[0].Type, "review")
	}

	// Check index staleness
	if result.IndexStaleness == nil {
		t.Fatal("IndexStaleness is nil")
	}
	if !result.IndexStaleness.IsStale {
		t.Error("IndexStaleness.IsStale should be true")
	}
	if result.IndexStaleness.CommitsBehind != 3 {
		t.Errorf("IndexStaleness.CommitsBehind: got %d, want 3", result.IndexStaleness.CommitsBehind)
	}
}

func TestFormatImpactMarkdown(t *testing.T) {
	resp := &ChangeSetResponseCLI{
		Summary: &ChangeSummaryCLI{
			FilesChanged:         3,
			SymbolsChanged:       5,
			DirectlyAffected:     10,
			TransitivelyAffected: 15,
			EstimatedRisk:        "high",
		},
		ChangedSymbols: []ChangedSymbolCLI{
			{SymbolID: "s1", Name: "Foo", File: "foo.go", ChangeType: "modified", Confidence: 0.9},
			{SymbolID: "s2", Name: "Bar", File: "bar.go", ChangeType: "added", Confidence: 0.8},
		},
		AffectedSymbols: []ImpactItemCLI{
			{StableID: "a1", Name: "Caller1", ModuleID: "mod1", Distance: 1, Kind: "direct-caller"},
		},
		ModulesAffected: []ModuleImpactCLI{
			{ModuleID: "mod1", ModuleName: "Module One", ImpactCount: 5, DirectCount: 3},
		},
		BlastRadius: &BlastRadiusCLI{
			ModuleCount:       1,
			FileCount:         3,
			UniqueCallerCount: 5,
			RiskLevel:         "high",
		},
		Recommendations: []RecommendationCLI{
			{Type: "review", Severity: "warning", Message: "Needs review", Action: "Request review"},
		},
		IndexStaleness: &IndexStalenessCLI{
			IsStale:       true,
			CommitsBehind: 2,
		},
	}

	result := formatImpactMarkdown(resp)

	// Check header with risk emoji
	if !strings.Contains(result, "## 🟠 Change Impact Analysis") {
		t.Error("missing header with high risk emoji")
	}

	// Check summary table
	if !strings.Contains(result, "| **Risk Level** | **HIGH** 🟠 |") {
		t.Error("missing risk level row in summary")
	}
	if !strings.Contains(result, "| Files Changed | 3 |") {
		t.Error("missing files changed row")
	}

	// Check blast radius
	if !strings.Contains(result, "**Blast Radius:** 1 modules, 3 files, 5 unique callers") {
		t.Error("missing blast radius summary")
	}

	// Check changed symbols section
	if !strings.Contains(result, "📝 Changed Symbols (2)") {
		t.Error("missing changed symbols section")
	}
	if !strings.Contains(result, "`Foo`") {
		t.Error("missing changed symbol name")
	}

	// Check affected symbols section
	if !strings.Contains(result, "🎯 Affected Downstream (1)") {
		t.Error("missing affected symbols section")
	}

	// Check modules section
	if !strings.Contains(result, "📦 Modules Affected (1)") {
		t.Error("missing modules section")
	}

	// Check recommendations
	if !strings.Contains(result, "### Recommendations") {
		t.Error("missing recommendations section")
	}
	if !strings.Contains(result, "⚠️ **review**: Needs review") {
		t.Error("missing recommendation content")
	}

	// Check staleness warning
	if !strings.Contains(result, "Index is 2 commit(s) behind HEAD") {
		t.Error("missing staleness warning")
	}

	// Check footer
	if !strings.Contains(result, "Generated by") {
		t.Error("missing footer")
	}
}

func TestFormatImpactMarkdown_RiskEmojis(t *testing.T) {
	tests := []struct {
		risk  string
		emoji string
	}{
		{"critical", "🔴"},
		{"high", "🟠"},
		{"medium", "🟡"},
		{"low", "🟢"},
		{"unknown", "⚪"},
	}

	for _, tt := range tests {
		t.Run(tt.risk, func(t *testing.T) {
			resp := &ChangeSetResponseCLI{
				Summary: &ChangeSummaryCLI{
					EstimatedRisk: tt.risk,
				},
			}
			result := formatImpactMarkdown(resp)

			expectedHeader := "## " + tt.emoji + " Change Impact Analysis"
			if !strings.Contains(result, expectedHeader) {
				t.Errorf("expected header %q in output", expectedHeader)
			}
		})
	}
}

func TestFormatImpactMarkdown_Truncation(t *testing.T) {
	// Test that large lists get truncated
	changedSymbols := make([]ChangedSymbolCLI, 20)
	for i := range changedSymbols {
		changedSymbols[i] = ChangedSymbolCLI{
			SymbolID:   "sym",
			Name:       "Sym",
			File:       "file.go",
			ChangeType: "modified",
			Confidence: 0.9,
		}
	}

	resp := &ChangeSetResponseCLI{
		Summary:        &ChangeSummaryCLI{EstimatedRisk: "low"},
		ChangedSymbols: changedSymbols,
	}

	result := formatImpactMarkdown(resp)

	// Should show "+5 more" since we display max 15
	if !strings.Contains(result, "+5 more") {
		t.Error("expected truncation message for changed symbols")
	}
}

func TestFormatImpactMarkdown_EmptySections(t *testing.T) {
	// Test with minimal data
	resp := &ChangeSetResponseCLI{
		Summary: &ChangeSummaryCLI{
			EstimatedRisk: "low",
		},
		ChangedSymbols:  []ChangedSymbolCLI{},
		AffectedSymbols: []ImpactItemCLI{},
		ModulesAffected: []ModuleImpactCLI{},
		Recommendations: []RecommendationCLI{},
	}

	result := formatImpactMarkdown(resp)

	// Should not have empty sections
	if strings.Contains(result, "📝 Changed Symbols (0)") {
		t.Error("should not show empty changed symbols section")
	}
	if strings.Contains(result, "🎯 Affected Downstream (0)") {
		t.Error("should not show empty affected symbols section")
	}
	if strings.Contains(result, "### Recommendations") {
		t.Error("should not show empty recommendations section")
	}
}

func TestFormatImpactMarkdown_NoIndexStaleness(t *testing.T) {
	resp := &ChangeSetResponseCLI{
		Summary:        &ChangeSummaryCLI{EstimatedRisk: "low"},
		IndexStaleness: nil,
	}

	result := formatImpactMarkdown(resp)

	if strings.Contains(result, "Index is") {
		t.Error("should not show staleness warning when nil")
	}
}

func TestFormatImpactMarkdown_FreshIndex(t *testing.T) {
	resp := &ChangeSetResponseCLI{
		Summary: &ChangeSummaryCLI{EstimatedRisk: "low"},
		IndexStaleness: &IndexStalenessCLI{
			IsStale: false,
		},
	}

	result := formatImpactMarkdown(resp)

	if strings.Contains(result, "Index is") {
		t.Error("should not show staleness warning when fresh")
	}
}
