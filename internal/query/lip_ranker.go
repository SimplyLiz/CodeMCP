package query

import (
	"context"
	"math"
	"path/filepath"
	"sort"

	"github.com/SimplyLiz/CodeMCP/internal/lip"
)

// lipSeedN is the number of top-ranked results used to build the query centroid.
// More seeds → more stable centroid; fewer → faster. Five is the sweet spot for
// typical search result sets (10–50 candidates).
const lipSeedN = 5

// RerankWithLIP re-ranks results using semantic similarity from LIP embeddings.
// It is the Fast-tier counterpart of RerankWithPPR: where PPR uses graph
// proximity over the SCIP symbol graph, this function uses file embedding
// dot-product similarity as the second ranking signal.
//
// Algorithm:
//  1. Fetch embeddings for all candidate files in a single batch RPC.
//  2. Average the top-lipSeedN seed vectors → L2-normalised query centroid.
//  3. Score every candidate: 0.6 * lexical_position + 0.4 * dot_product(vec, centroid).
//  4. Re-sort by combined score.
//
// Degrades silently when LIP is unavailable — the original results are returned
// unchanged, so callers never need to handle the failure path specially.
func RerankWithLIP(_ context.Context, results []SearchResultItem, repoRoot, _ string) ([]SearchResultItem, error) {
	if len(results) <= 3 {
		return results, nil
	}

	// Build URI list, preserving index correspondence with results.
	uris := make([]string, len(results))
	for i, r := range results {
		uris[i] = lipFileURI(repoRoot, r)
	}

	// Single batch RPC instead of N individual round-trips.
	batchVecs, _ := lip.GetEmbeddingsBatch(uris, "")
	vecs := batchVecs
	if vecs == nil {
		// LIP not running — allocate a nil slice so the rest of the function is uniform.
		vecs = make([][]float32, len(results))
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

	// Build centroid from the top-N seeds (lexical ordering).
	seedN := min(lipSeedN, len(results))
	centroid := make([]float64, dims)
	nSeeds := 0
	for i := 0; i < seedN; i++ {
		if vecs[i] == nil {
			continue
		}
		for d, x := range vecs[i] {
			centroid[d] += float64(x)
		}
		nSeeds++
	}
	if nSeeds < 2 {
		// Not enough seed embeddings to form a meaningful centroid.
		return results, nil
	}
	for d := range centroid {
		centroid[d] /= float64(nSeeds)
	}
	// L2-normalise so dot products are cosine similarities.
	var norm float64
	for _, x := range centroid {
		norm += x * x
	}
	if norm = math.Sqrt(norm); norm > 0 {
		for d := range centroid {
			centroid[d] /= norm
		}
	}

	// Score every candidate and re-sort.
	type scored struct {
		item  SearchResultItem
		score float64
	}
	out := make([]scored, len(results))
	for i, r := range results {
		// Lexical position score: decays as 1/rank (same shape as PPR's positionScore).
		posScore := 1.0 / (float64(i) + 1.0)

		// Semantic similarity: dot product with normalised centroid.
		semScore := 0.0
		if vecs[i] != nil {
			for d, x := range vecs[i] {
				semScore += float64(x) * centroid[d]
			}
		}

		out[i] = scored{item: r, score: 0.6*posScore + 0.4*semScore}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].score > out[j].score })

	reranked := make([]SearchResultItem, len(out))
	for i, s := range out {
		reranked[i] = s.item
	}
	return reranked, nil
}

// SemanticSearchWithLIP queries LIP's nearest-neighbour index for files matching
// the query text, then resolves the file URIs to SearchResultItems using the
// provided symbol lookup function. It is a Fast-tier complement to FTS5 search:
// where FTS5 matches symbol names lexically, this finds semantically related
// files even when the query terms don't appear literally in the code.
//
// fn is called with each URI returned by LIP; it should return any SearchResultItems
// associated with that file (e.g. all symbols defined there). Results are ordered by
// LIP similarity score (highest first) and deduplicated by symbol ID.
//
// Returns nil (not an error) when LIP is unavailable or returns no results.
func SemanticSearchWithLIP(query string, topK int, fn func(fileURI string) []SearchResultItem) []SearchResultItem {
	hits, _ := lip.NearestByText(query, topK)
	if len(hits) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, topK*4)
	var out []SearchResultItem
	for _, h := range hits {
		items := fn(h.URI)
		for _, item := range items {
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
			out = append(out, item)
		}
	}
	return out
}

// lipFileURI returns the file:// URI for a result's source file, suitable for
// LIP embedding requests. Returns "" when the result has no location.
func lipFileURI(repoRoot string, r SearchResultItem) string {
	if r.Location == nil || r.Location.FileId == "" {
		return ""
	}
	return "file://" + filepath.Join(repoRoot, r.Location.FileId)
}
