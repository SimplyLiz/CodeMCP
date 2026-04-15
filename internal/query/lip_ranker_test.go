package query

import (
	"context"
	"math"
	"testing"
)

// mkResults builds n stub SearchResultItems with deterministic StableIds and
// FilePaths, suitable for feeding into rerankWithLIP.
func mkResults(n int) []SearchResultItem {
	out := make([]SearchResultItem, n)
	for i := 0; i < n; i++ {
		out[i] = SearchResultItem{
			StableId: string(rune('A' + i)),
			Name:     string(rune('A' + i)),
			Location: &LocationInfo{FileId: string(rune('a'+i)) + ".go"},
		}
	}
	return out
}

// unit returns a dims-length unit vector with value 1 at index hot, 0 elsewhere.
func unit(dims, hot int) []float32 {
	v := make([]float32, dims)
	v[hot] = 1
	return v
}

// constEmbed returns an embedBatchFn that yields the supplied vectors in order.
func constEmbed(vecs [][]float32) embedBatchFn {
	return func(uris []string, _ string) ([][]float32, error) {
		// Pad or truncate to len(uris) so test doesn't need to match exactly.
		out := make([][]float32, len(uris))
		for i := range uris {
			if i < len(vecs) {
				out[i] = vecs[i]
			}
		}
		return out, nil
	}
}

// TestRerankWithLIP_CoherentSeeds_PromotesAlignedResult: when the top seeds
// all point at the same axis and a later candidate also points at that axis,
// the later candidate should be promoted.
func TestRerankWithLIP_CoherentSeeds_PromotesAlignedResult(t *testing.T) {
	dims := 4
	// Seeds (positions 0–4) aligned on axis 0. Candidate at position 7 also
	// on axis 0 — semantically it belongs near the top, even though lexical
	// rank is low.
	vecs := [][]float32{
		unit(dims, 0), // rank 0 — on-axis seed
		unit(dims, 0), // rank 1 — on-axis seed
		unit(dims, 0), // rank 2 — on-axis seed
		unit(dims, 1), // rank 3 — off-axis
		unit(dims, 2), // rank 4 — off-axis
		unit(dims, 3), // rank 5
		unit(dims, 1), // rank 6
		unit(dims, 0), // rank 7 — on-axis, should rise
	}

	results := mkResults(8)
	got, err := rerankWithLIP(context.Background(), results, "/repo", "q", DefaultRerankConfig(), constEmbed(vecs))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 8 {
		t.Fatalf("len = %d, want 8", len(got))
	}

	// Rank 7 (StableId "H") should now be in the top 4.
	topFour := map[string]bool{}
	for i := 0; i < 4; i++ {
		topFour[got[i].StableId] = true
	}
	if !topFour["H"] {
		t.Errorf("on-axis candidate H not promoted into top 4; got order: %s", stableIds(got))
	}
}

// TestRerankWithLIP_ScatteredSeeds_FallsBackToLexical: when seeds point in
// orthogonal directions (coherence ≈ 0), the rerank should refuse to amplify
// noise and return the lexical order unchanged.
func TestRerankWithLIP_ScatteredSeeds_FallsBackToLexical(t *testing.T) {
	dims := 8
	// Each seed on a different axis → centroid norm → 0 after averaging.
	vecs := [][]float32{
		unit(dims, 0),
		unit(dims, 1),
		unit(dims, 2),
		unit(dims, 3),
		unit(dims, 4),
		unit(dims, 5),
		unit(dims, 6),
		unit(dims, 7),
	}
	results := mkResults(8)
	orig := stableIds(results)
	got, err := rerankWithLIP(context.Background(), results, "/repo", "q", DefaultRerankConfig(), constEmbed(vecs))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := stableIds(got); got != orig {
		t.Errorf("expected no re-order on scattered seeds, got %s (was %s)", got, orig)
	}
}

// TestRerankWithLIP_DaemonUnavailable_ReturnsOriginal: when the embed batch
// returns nil (daemon down), order is preserved.
func TestRerankWithLIP_DaemonUnavailable_ReturnsOriginal(t *testing.T) {
	results := mkResults(8)
	orig := stableIds(results)
	embed := embedBatchFn(func(_ []string, _ string) ([][]float32, error) { return nil, nil })
	got, err := rerankWithLIP(context.Background(), results, "/repo", "q", DefaultRerankConfig(), embed)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := stableIds(got); got != orig {
		t.Errorf("expected unchanged order on daemon-down, got %s (was %s)", got, orig)
	}
}

// TestRerankWithLIP_TooFewResults_NoOp: ≤3 results → no-op.
func TestRerankWithLIP_TooFewResults_NoOp(t *testing.T) {
	results := mkResults(3)
	embed := embedBatchFn(func(_ []string, _ string) ([][]float32, error) {
		t.Fatal("embed should not be called for ≤3 results")
		return nil, nil
	})
	got, _ := rerankWithLIP(context.Background(), results, "/repo", "q", DefaultRerankConfig(), embed)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
}

// TestRerankWithLIP_ConfigOverride_RespectsWeights: bumping LexicalWeight to
// dominate semantic should keep lexical order even when semantic disagrees.
func TestRerankWithLIP_ConfigOverride_RespectsWeights(t *testing.T) {
	dims := 4
	// Coherent on-axis seeds; last candidate is anti-aligned — semantic wants
	// to push it down, but lexical dominance should keep it last anyway (which
	// is also where it started — this verifies no spurious promotion).
	vecs := [][]float32{
		unit(dims, 0), unit(dims, 0), unit(dims, 0),
		unit(dims, 0), unit(dims, 0),
		unit(dims, 1), unit(dims, 1),
		{-1, 0, 0, 0}, // anti-aligned last
	}
	cfg := DefaultRerankConfig()
	cfg.LexicalWeight = 10.0
	cfg.SemanticWeight = 0.01
	results := mkResults(8)
	got, _ := rerankWithLIP(context.Background(), results, "/repo", "q", cfg, constEmbed(vecs))
	// Rank 0 must still be top when lexical dominates.
	if got[0].StableId != "A" {
		t.Errorf("expected A at rank 0 under lexical-dominant config, got %s", got[0].StableId)
	}
}

func TestBuildSeedCentroid_CoherenceScores(t *testing.T) {
	dims := 4
	// Perfectly coherent: coherence → 1.0
	coherent := [][]float32{unit(dims, 0), unit(dims, 0), unit(dims, 0)}
	_, c := buildSeedCentroid(coherent, 3, dims)
	if math.Abs(c-1.0) > 1e-9 {
		t.Errorf("perfectly-aligned seeds coherence = %f, want ~1.0", c)
	}

	// Opposite seeds cancel: coherence drops substantially.
	opposed := [][]float32{unit(dims, 0), {-1, 0, 0, 0}}
	_, c = buildSeedCentroid(opposed, 2, dims)
	// seed0 weight 1.0, seed1 weight 0.5 → net weight-normalised vector is
	// ((1.0*1 + 0.5*-1) / 1.5, 0, 0, 0) = (0.333, 0, 0, 0) → coherence 0.333.
	if c > 0.5 {
		t.Errorf("opposed seeds coherence = %f, want <0.5", c)
	}

	// Only one usable seed → returns nil.
	one := [][]float32{unit(dims, 0), nil, nil}
	centroid, c := buildSeedCentroid(one, 3, dims)
	if centroid != nil || c != 0 {
		t.Errorf("single-seed case returned (%v, %f), want (nil, 0)", centroid, c)
	}
}

// stableIds joins the StableId fields into a comma-separated string for
// easy order comparison in tests.
func stableIds(rs []SearchResultItem) string {
	out := ""
	for i, r := range rs {
		if i > 0 {
			out += ","
		}
		out += r.StableId
	}
	return out
}
