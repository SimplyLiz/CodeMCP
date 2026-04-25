package query

import (
	"testing"

	"github.com/SimplyLiz/CodeMCP/internal/impact"
	"github.com/SimplyLiz/CodeMCP/internal/lip"
)

// TestOutgoingFoldIntegration exercises the seam between the lip and
// impact packages the way AnalyzeOutgoingImpact does: LIP wire-shape entry
// → OutgoingEntryToExternal → FoldExternalCalleeItems. This catches glue
// regressions that unit tests in either package alone would miss — e.g.
// the Kind tagging contract between the two sides, or a field rename
// breaking the conversion silently.
func TestOutgoingFoldIntegration(t *testing.T) {
	entry := &lip.OutgoingImpactEntry{
		TargetURI: "lip://local//repo/internal/foo/bar.go#DoWork",
		DirectItems: []lip.BlastRadiusItem{
			{
				FileURI:    "lip://local//repo/internal/foo/bar.go",
				SymbolURI:  "lip://local//repo/internal/foo/bar.go#helper",
				Distance:   1,
				Confidence: 0.95,
			},
			{
				FileURI:    "lip://local//repo/internal/baz/qux.go",
				SymbolURI:  "lip://local//repo/internal/baz/qux.go#validate",
				Distance:   1,
				Confidence: 0.9,
			},
		},
		TransitiveItems: []lip.BlastRadiusItem{
			{
				FileURI:    "lip://local//repo/internal/deep/x.go",
				SymbolURI:  "lip://local//repo/internal/deep/x.go#log",
				Distance:   2,
				Confidence: 0.8,
			},
		},
		SemanticItems: []lip.BlastRadiusSemanticItem{
			{
				FileURI:    "lip://local//repo/internal/sibling/y.go",
				SymbolURI:  "lip://local//repo/internal/sibling/y.go#related",
				Similarity: 0.82,
				Source:     "semantic",
			},
		},
		EdgesSource: impact.EdgesSourceScipWithTier1Edges,
		Truncated:   false,
	}

	external := lip.OutgoingEntryToExternal(entry)
	if external == nil {
		t.Fatal("OutgoingEntryToExternal returned nil for non-nil entry")
	}
	if external.EdgesSource != impact.EdgesSourceScipWithTier1Edges {
		t.Errorf("EdgesSource round-trip = %q, want %q",
			external.EdgesSource, impact.EdgesSourceScipWithTier1Edges)
	}

	direct, transitive := impact.FoldExternalCalleeItems(nil, nil, external, "/repo")

	if len(direct) != 2 {
		t.Fatalf("direct callees = %d, want 2", len(direct))
	}
	for _, d := range direct {
		if d.Kind != impact.DirectCallee {
			t.Errorf("direct kind = %q, want %q", d.Kind, impact.DirectCallee)
		}
		if d.Distance != 1 {
			t.Errorf("direct distance = %d, want 1", d.Distance)
		}
	}

	if len(transitive) != 1 {
		t.Fatalf("transitive callees = %d, want 1", len(transitive))
	}
	if transitive[0].Kind != impact.TransitiveCallee {
		t.Errorf("transitive kind = %q, want %q",
			transitive[0].Kind, impact.TransitiveCallee)
	}
	if transitive[0].Distance != 2 {
		t.Errorf("transitive distance = %d, want 2", transitive[0].Distance)
	}

	// Semantic items survive the conversion but live on the external struct —
	// the fold doesn't promote them to ImpactItems (AnalyzeOutgoingImpact
	// maps them into SemanticCalleeInfo separately).
	if len(external.SemanticItems) != 1 {
		t.Errorf("semantic items = %d, want 1", len(external.SemanticItems))
	}
	if external.SemanticItems[0].Source != "semantic" {
		t.Errorf("semantic source = %q, want semantic",
			external.SemanticItems[0].Source)
	}
}

// TestOutgoingFoldIntegration_EmptyEdgesSource verifies that an "empty"
// provenance flag bypasses the fold even when items are present. This is
// LIP's explicit signal that it has no static edge evidence to contribute.
func TestOutgoingFoldIntegration_EmptyEdgesSource(t *testing.T) {
	entry := &lip.OutgoingImpactEntry{
		DirectItems: []lip.BlastRadiusItem{
			{SymbolURI: "lip://local//repo/a.go#A", Distance: 1, Confidence: 0.9},
		},
		EdgesSource: impact.EdgesSourceEmpty,
	}
	external := lip.OutgoingEntryToExternal(entry)
	direct, _ := impact.FoldExternalCalleeItems(nil, nil, external, "/repo")
	if len(direct) != 0 {
		t.Errorf("empty EdgesSource should bypass fold, got %d items", len(direct))
	}
}
