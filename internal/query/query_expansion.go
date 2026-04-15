package query

import (
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/lip"
)

// maxExpansionTerms caps how many LIP-suggested terms we append to the
// user's query. Beyond ~5 the FTS5 result set starts diluting — the expansion
// is meant to rescue misses, not to replace the original ranking.
const maxExpansionTerms = 5

// expandQueryViaLIP returns `query` with up to `maxExpansionTerms` related
// terms appended, fetched from LIP's query_expansion RPC. Only fires for
// short queries (≤ 2 tokens) — longer queries already carry enough lexical
// signal that expansion tends to hurt precision without helping recall.
//
// Silently falls back to the raw query when:
//   - the query is empty or already compound,
//   - the daemon is unavailable, mixed-models, or predates v1.6,
//   - expansion returns no terms.
//
// Expansion is lexical (additional search tokens), so the mixed-models
// gate is relevant: a query embedding produced against one model that then
// matches vocabulary indexed under another will return garbage. We re-use
// `lipSemanticAvailable` as the gate rather than introducing a second one.
func (e *Engine) expandQueryViaLIP(query string) string {
	q := strings.TrimSpace(query)
	if q == "" {
		return query
	}
	if strings.Count(q, " ") >= 2 {
		return query
	}
	if !e.lipSemanticAvailable() {
		return query
	}
	// Gate on handshake: older daemons that lack query_expansion would
	// respond with UnknownMessage. Engines that haven't completed the
	// handshake yet fall through unchanged.
	if !e.lipSupports("query_expansion") {
		return query
	}

	terms, _ := lip.QueryExpansion(q, maxExpansionTerms)
	if len(terms) == 0 {
		return query
	}

	// Drop terms that duplicate the original query (case-insensitive) so we
	// don't bias FTS ranking by repeating the input.
	qLower := strings.ToLower(q)
	kept := make([]string, 0, len(terms))
	for _, t := range terms {
		t = strings.TrimSpace(t)
		if t == "" || strings.ToLower(t) == qLower {
			continue
		}
		kept = append(kept, t)
	}
	if len(kept) == 0 {
		return query
	}
	return q + " " + strings.Join(kept, " ")
}
