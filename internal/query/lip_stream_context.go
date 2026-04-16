package query

import (
	"github.com/SimplyLiz/CodeMCP/internal/lip"
)

// streamContextMaxTokens caps the total prompt-token budget we ask LIP to
// rank within. 2048 is roughly two screens of code — enough to orient the
// LLM without dominating a typical context window.
const streamContextMaxTokens uint32 = 2048

// streamContextLimit bounds how many related symbols we retain in the
// response. The daemon ranks by relevance, so truncating here drops the
// long tail instead of promising more than the caller will use.
const streamContextLimit = 10

// relatedViaStreamContext asks LIP for symbols semantically related to the
// whole file (cursor_position spanning line 0 → lineCount). The RPC is
// v2.1+, so we gate on the handshake — older daemons would answer with
// UnknownMessage and every ExplainFile call would eat a round-trip for
// nothing.
//
// Returns an empty slice (not nil) when the daemon is unavailable, the
// feature is unsupported, or the stream yielded nothing — callers append
// to the ExplainFileFacts.Related field and a nil-vs-empty distinction
// isn't meaningful there.
func (e *Engine) relatedViaStreamContext(relPath string, lineCount int) []ExplainFileRelated {
	if !e.lipSemanticAvailable() || !e.lipSupports("stream_context") {
		return nil
	}
	if lineCount <= 0 {
		lineCount = 1
	}
	uri := "file://" + e.repoRoot + "/" + relPath

	res, _ := lip.StreamContext(uri, lip.StreamContextPosition{
		StartLine: 0,
		EndLine:   lineCount,
	}, streamContextMaxTokens, "")
	if res == nil || len(res.Symbols) == 0 {
		return nil
	}

	limit := len(res.Symbols)
	if limit > streamContextLimit {
		limit = streamContextLimit
	}
	out := make([]ExplainFileRelated, 0, limit)
	for _, s := range res.Symbols[:limit] {
		out = append(out, ExplainFileRelated{
			URI:         s.URI,
			DisplayName: s.DisplayName,
			Kind:        s.Kind,
			Relevance:   s.RelevanceScore,
			TokenCost:   s.TokenCost,
		})
	}
	return out
}
