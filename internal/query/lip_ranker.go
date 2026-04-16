package query

import (
	"context"
	"math"
	"path/filepath"
	"sort"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/lip"
)

// RerankConfig controls the LIP semantic re-ranking blend. The zero value is
// not valid — use DefaultRerankConfig. Surfaced as a struct so future empirical
// tuning (golden-query harness, per-repo overrides) does not require changing
// call sites.
type RerankConfig struct {
	// LexicalWeight is the weight on the 1/rank position score.
	LexicalWeight float64
	// SemanticWeight is the weight on the centroid cosine similarity.
	SemanticWeight float64
	// SeedCount is the number of top results used to build the query centroid.
	SeedCount int
	// MinCoherence is the minimum centroid-norm required to trust the semantic
	// signal. Each seed vector is L2-normalised, so the position-weighted
	// centroid norm lies in [0, 1]: 1.0 means seeds point the same direction,
	// near-0 means they cancel. Below this threshold we fall back to lexical
	// ranking to avoid amplifying noise when top lexical results are
	// semantically scattered.
	MinCoherence float64
}

// DefaultRerankConfig returns the current production defaults. These values
// predate an empirical tuning pass — treat them as a starting point, not a
// proven optimum.
func DefaultRerankConfig() RerankConfig {
	return RerankConfig{
		LexicalWeight:  0.6,
		SemanticWeight: 0.4,
		SeedCount:      5,
		MinCoherence:   0.35,
	}
}

// RerankWithLIP re-ranks results using semantic similarity from LIP embeddings.
// It is the Fast-tier counterpart of RerankWithPPR: where PPR uses graph
// proximity over the SCIP symbol graph, this function uses file embedding
// cosine similarity as the second ranking signal.
//
// Algorithm:
//  1. Fetch embeddings for all candidate files in a single batch RPC.
//  2. Build a position-weighted, L2-normalised seed centroid from the top
//     SeedCount candidates (weight 1/(rank+1) so top-1 dominates softly).
//  3. Measure centroid coherence; if seeds disagree, return results unchanged.
//  4. Score every candidate: LexicalWeight * 1/rank + SemanticWeight * cosine.
//  5. Re-sort by combined score.
//
// Degrades silently when LIP is unavailable or the signal is weak — the
// original results are returned unchanged.
func RerankWithLIP(ctx context.Context, results []SearchResultItem, repoRoot, query string) ([]SearchResultItem, error) {
	return rerankWithLIP(ctx, results, repoRoot, query, DefaultRerankConfig(), lip.GetEmbeddingsBatch)
}

// embedBatchFn matches lip.GetEmbeddingsBatch and exists so tests can inject
// synthetic embeddings without a running daemon.
type embedBatchFn func(uris []string, model string) ([][]float32, error)

func rerankWithLIP(
	_ context.Context,
	results []SearchResultItem,
	repoRoot, _ string,
	cfg RerankConfig,
	embed embedBatchFn,
) ([]SearchResultItem, error) {
	if len(results) <= 3 {
		return results, nil
	}

	uris := make([]string, len(results))
	for i, r := range results {
		uris[i] = lipFileURI(repoRoot, r)
	}

	vecs, _ := embed(uris, "")
	if vecs == nil {
		return results, nil
	}

	dims := 0
	for _, v := range vecs {
		if len(v) > 0 {
			dims = len(v)
			break
		}
	}
	if dims == 0 {
		return results, nil
	}

	centroid, coherence := buildSeedCentroid(vecs, cfg.SeedCount, dims)
	if centroid == nil || coherence < cfg.MinCoherence {
		// Seeds too scattered (or too few) to trust the semantic signal.
		return results, nil
	}

	// Score every candidate and re-sort.
	type scored struct {
		item  SearchResultItem
		score float64
	}
	out := make([]scored, len(results))
	for i, r := range results {
		posScore := 1.0 / (float64(i) + 1.0)
		semScore := cosine(vecs[i], centroid)
		out[i] = scored{item: r, score: cfg.LexicalWeight*posScore + cfg.SemanticWeight*semScore}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].score > out[j].score })

	reranked := make([]SearchResultItem, len(out))
	for i, s := range out {
		reranked[i] = s.item
	}
	return reranked, nil
}

// buildSeedCentroid builds a position-weighted centroid from the top-N seed
// vectors. Each seed is L2-normalised before weighting so the resulting
// centroid norm is a direct coherence measure in [0, 1] — 1.0 when seeds
// point the same direction, near-0 when they cancel.
//
// Returns (nil, 0) when fewer than two seeds have embeddings.
func buildSeedCentroid(vecs [][]float32, seedN, dims int) ([]float64, float64) {
	if seedN > len(vecs) {
		seedN = len(vecs)
	}

	centroid := make([]float64, dims)
	totalW := 0.0
	nSeeds := 0
	for i := 0; i < seedN; i++ {
		if len(vecs[i]) == 0 {
			continue
		}
		// L2-normalise the seed so every contribution has unit magnitude.
		var norm float64
		for _, x := range vecs[i] {
			norm += float64(x) * float64(x)
		}
		norm = math.Sqrt(norm)
		if norm == 0 {
			continue
		}
		w := 1.0 / float64(i+1)
		totalW += w
		for d, x := range vecs[i] {
			centroid[d] += w * float64(x) / norm
		}
		nSeeds++
	}
	if nSeeds < 2 || totalW == 0 {
		return nil, 0
	}

	// Normalise by total weight so the centroid lives in the unit ball.
	// With each seed a unit vector and weights summing to totalW, the
	// unweighted-normalised centroid norm is bounded by 1 — that's our
	// coherence metric.
	for d := range centroid {
		centroid[d] /= totalW
	}
	var coherence float64
	for _, x := range centroid {
		coherence += x * x
	}
	coherence = math.Sqrt(coherence)

	// Re-normalise centroid to unit length for cosine similarity scoring.
	if coherence > 0 {
		for d := range centroid {
			centroid[d] /= coherence
		}
	}
	return centroid, coherence
}

// cosine returns the cosine similarity between a float32 vector and a
// unit-length float64 centroid. Returns 0 when v has no length.
func cosine(v []float32, centroid []float64) float64 {
	if len(v) == 0 || len(v) != len(centroid) {
		return 0
	}
	var dot, norm float64
	for d, x := range v {
		f := float64(x)
		dot += f * centroid[d]
		norm += f * f
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return 0
	}
	return dot / norm
}

// SemanticSearchWithLIP queries LIP's nearest-neighbour index for files matching
// the query text, then resolves the file URIs to SearchResultItems using the
// provided symbol lookup function. It is a Fast-tier complement to FTS5 search:
// where FTS5 matches symbol names lexically, this finds semantically related
// files even when the query terms don't appear literally in the code.
//
// filter is an optional glob pattern to restrict candidates (e.g. "*_test.go");
// minScore is an optional minimum cosine similarity threshold (0 = disabled).
//
// fn receives the full slice of URIs returned by LIP in a single call and should
// return a map from URI to SearchResultItems for that file. This allows callers to
// batch all symbol lookups into one round-trip instead of N.
//
// Results are ordered by LIP similarity score (highest first) and deduplicated by
// symbol ID.
//
// Returns nil (not an error) when LIP is unavailable or returns no results.
func SemanticSearchWithLIP(query string, topK int, filter string, minScore float32, fn func(fileURIs []string) map[string][]SearchResultItem) []SearchResultItem {
	return semanticSearchWithLIP(query, topK, filter, minScore, false, fn)
}

// SemanticSearchWithLIPExplained is SemanticSearchWithLIP plus evidence
// attachment: for the top `explainTopK` hits, it calls LIP's `explain_match`
// RPC and attaches the returned chunks to every SearchResultItem sourced
// from that file. Makes semantic hits auditable — the caller can surface
// "matched at lines 42–60 with cosine 0.71" instead of a bare file URL.
//
// Gate this on `Engine.lipSupports("explain_match")` — older daemons will
// return UnknownMessage, which `ExplainMatch` silently swallows but costs
// a round-trip per hit regardless.
func SemanticSearchWithLIPExplained(query string, topK int, filter string, minScore float32, fn func(fileURIs []string) map[string][]SearchResultItem) []SearchResultItem {
	return semanticSearchWithLIP(query, topK, filter, minScore, true, fn)
}

// explainTopK bounds how many semantic hits get a follow-up `explain_match`
// round-trip. Explanation is pure overhead for hits the caller never reads,
// so we only explain the top few — matching the realistic display budget.
const explainTopK = 5

// explainChunkLines caps the lines-per-chunk the daemon returns. 40 is
// enough to carry a function body or small block without bloating the
// response.
const explainChunkLines = 40

func semanticSearchWithLIP(query string, topK int, filter string, minScore float32, explain bool, fn func(fileURIs []string) map[string][]SearchResultItem) []SearchResultItem {
	hits, _ := lip.NearestByTextFiltered(query, topK, filter, minScore, "")
	if len(hits) == 0 {
		return nil
	}

	// Collect all URIs in hit order and resolve them in a single batch call.
	uris := make([]string, len(hits))
	for i, h := range hits {
		uris[i] = h.URI
	}
	byURI := fn(uris)

	// Optionally fetch explanations for the top hits in parallel — one RPC
	// per URI, but bounded by explainTopK and only when requested.
	evidence := map[string][]SemanticEvidenceChunk{}
	if explain {
		limit := min(len(hits), explainTopK)
		for i := 0; i < limit; i++ {
			chunks, _, _ := lip.ExplainMatch(query, hits[i].URI, 2, explainChunkLines, "")
			if len(chunks) == 0 {
				continue
			}
			out := make([]SemanticEvidenceChunk, len(chunks))
			for j, c := range chunks {
				out[j] = SemanticEvidenceChunk{
					StartLine: c.StartLine,
					EndLine:   c.EndLine,
					Text:      c.ChunkText,
					Score:     c.Score,
				}
			}
			evidence[hits[i].URI] = out
		}
	}

	seen := make(map[string]struct{}, topK*4)
	var out []SearchResultItem
	for _, h := range hits {
		for _, item := range byURI[h.URI] {
			id := item.StableId
			if id == "" {
				id = item.Name
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			// Blend LIP score into result Score so downstream re-ranking has a signal.
			item.Score = float64(h.Score)
			if chunks, ok := evidence[h.URI]; ok {
				item.SemanticEvidence = chunks
			}
			out = append(out, item)
		}
	}
	return out
}

// lipFileURI returns the file:// URI for a result's source file, suitable for
// LIP embedding requests. Returns "" when the result has no location.
//
// Handles three input shapes for `FileId`:
//   - repo-relative path (the common case): joined onto repoRoot.
//   - absolute filesystem path: used as-is, repoRoot ignored.
//   - already a `file://` URI: returned unchanged.
//
// Backends today return relative paths, but the SearchResultItem contract does
// not forbid the other two shapes — this guard keeps a misbehaving backend
// from producing malformed URIs like `file:///repo//abs/path`.
func lipFileURI(repoRoot string, r SearchResultItem) string {
	if r.Location == nil || r.Location.FileId == "" {
		return ""
	}
	id := r.Location.FileId
	if strings.HasPrefix(id, "file://") {
		return id
	}
	if filepath.IsAbs(id) {
		return "file://" + id
	}
	return "file://" + filepath.Join(repoRoot, id)
}
