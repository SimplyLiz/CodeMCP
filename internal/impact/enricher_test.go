package impact

import "testing"

func TestMergeBlastRadius_NilExternal(t *testing.T) {
	static := &BlastRadius{
		ModuleCount:       3,
		FileCount:         5,
		UniqueCallerCount: 8,
		RiskLevel:         "medium",
	}
	got := MergeBlastRadius(static, nil)
	if got != static {
		t.Fatal("nil external should return static unchanged")
	}
}

func TestMergeBlastRadius_NilStatic(t *testing.T) {
	got := MergeBlastRadius(nil, &ExternalBlastRadius{})
	if got != nil {
		t.Fatal("nil static should return nil")
	}
}

func TestMergeBlastRadius_SemanticOnly(t *testing.T) {
	static := &BlastRadius{
		ModuleCount:       2,
		FileCount:         3,
		UniqueCallerCount: 4,
		RiskLevel:         "low",
	}
	external := &ExternalBlastRadius{
		SemanticItems: []ExternalSemanticItem{
			{FileURI: "file:///src/a.rs", SymbolURI: "sym:a", Similarity: 0.85, Source: "semantic"},
			{FileURI: "file:///src/b.rs", SymbolURI: "sym:b", Similarity: 0.72, Source: "semantic"},
		},
	}

	got := MergeBlastRadius(static, external)

	// UniqueCallerCount must stay SCIP-only
	if got.UniqueCallerCount != 4 {
		t.Errorf("UniqueCallerCount = %d, want 4 (SCIP-only)", got.UniqueCallerCount)
	}
	if got.StaticCallerCount != 4 {
		t.Errorf("StaticCallerCount = %d, want 4", got.StaticCallerCount)
	}
	if got.SemanticCallerCount != 2 {
		t.Errorf("SemanticCallerCount = %d, want 2", got.SemanticCallerCount)
	}
	if got.ConfirmedCount != 0 {
		t.Errorf("ConfirmedCount = %d, want 0", got.ConfirmedCount)
	}
	if len(got.SemanticCallers) != 2 {
		t.Fatalf("SemanticCallers len = %d, want 2", len(got.SemanticCallers))
	}
	for _, sc := range got.SemanticCallers {
		if sc.Tier != CouplingSemantic {
			t.Errorf("caller %s tier = %s, want semantic", sc.FileURI, sc.Tier)
		}
	}
	// RiskLevel stays SCIP-derived
	if got.RiskLevel != "low" {
		t.Errorf("RiskLevel = %s, want low", got.RiskLevel)
	}
}

func TestMergeBlastRadius_BothSource(t *testing.T) {
	static := &BlastRadius{
		ModuleCount:       1,
		FileCount:         2,
		UniqueCallerCount: 3,
		RiskLevel:         "low",
	}
	external := &ExternalBlastRadius{
		SemanticItems: []ExternalSemanticItem{
			{FileURI: "file:///src/confirmed.rs", Similarity: 0.91, Source: "both"},
			{FileURI: "file:///src/new.rs", Similarity: 0.78, Source: "semantic"},
		},
	}

	got := MergeBlastRadius(static, external)

	// "both" confirms a SCIP edge — doesn't inflate semantic count
	if got.SemanticCallerCount != 1 {
		t.Errorf("SemanticCallerCount = %d, want 1 (only pure semantic)", got.SemanticCallerCount)
	}
	if got.ConfirmedCount != 1 {
		t.Errorf("ConfirmedCount = %d, want 1", got.ConfirmedCount)
	}
	if len(got.SemanticCallers) != 2 {
		t.Fatalf("SemanticCallers len = %d, want 2 (both + semantic)", len(got.SemanticCallers))
	}

	// Check tiers
	tiers := map[CouplingTier]int{}
	for _, sc := range got.SemanticCallers {
		tiers[sc.Tier]++
	}
	if tiers[CouplingBoth] != 1 || tiers[CouplingSemantic] != 1 {
		t.Errorf("tier counts = %v, want both:1, semantic:1", tiers)
	}
}

func TestMergeBlastRadius_DedupByFile(t *testing.T) {
	static := &BlastRadius{
		UniqueCallerCount: 2,
		RiskLevel:         "low",
	}
	external := &ExternalBlastRadius{
		SemanticItems: []ExternalSemanticItem{
			{FileURI: "file:///src/dup.rs", Similarity: 0.80, Source: "semantic"},
			{FileURI: "file:///src/dup.rs", Similarity: 0.75, Source: "semantic"}, // same file
			{FileURI: "file:///src/other.rs", Similarity: 0.70, Source: "semantic"},
		},
	}

	got := MergeBlastRadius(static, external)

	if got.SemanticCallerCount != 2 {
		t.Errorf("SemanticCallerCount = %d, want 2 (deduped)", got.SemanticCallerCount)
	}
}
