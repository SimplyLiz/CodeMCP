package lip

import (
	"context"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/impact"
)

// BlastRadiusEnricher adapts LIP's QueryBlastRadiusBatch into the
// impact.BlastRadiusEnricher interface. Safe for concurrent use (stateless
// adapter over the LIP socket RPC).
type BlastRadiusEnricher struct {
	// MinScore is the cosine similarity threshold for semantic hits.
	// Zero means static-only (no semantic items). Typical: 0.6.
	MinScore float32
}

// EnrichBatch implements impact.BlastRadiusEnricher.
func (e *BlastRadiusEnricher) EnrichBatch(ctx context.Context, changedFileURIs []string) (map[string]*impact.ExternalBlastRadius, error) {
	if len(changedFileURIs) == 0 {
		return nil, nil
	}

	result, err := QueryBlastRadiusBatch(changedFileURIs, e.MinScore)
	if result == nil {
		return nil, err
	}

	out := make(map[string]*impact.ExternalBlastRadius, len(result.Entries))
	for symbolURI, entry := range result.Entries {
		out[symbolURI] = EntryToExternal(&entry)
	}
	return out, nil
}

// LookupSymbol finds the blast radius entry for a symbol within a pre-fetched
// result map. LIP keys entries by "lip://local/<file>#<name>" — tries exact
// match first, then falls back to scanning entries whose URI shares the file
// prefix and contains the symbol name. The fallback handles C++ mangled names
// and template specialisations where LIP's symbol URI diverges from CKB's
// stable ID.
func LookupSymbol(entries map[string]*impact.ExternalBlastRadius, file, name string) (*impact.ExternalBlastRadius, bool) {
	key := "lip://local/" + file + "#" + name
	if ebr, ok := entries[key]; ok {
		return ebr, true
	}
	prefix := "lip://local/" + file + "#"
	for uri, ebr := range entries {
		if strings.HasPrefix(uri, prefix) && strings.Contains(uri[len(prefix):], name) {
			return ebr, true
		}
	}
	return nil, false
}

// EntryToExternal converts a BlastRadiusEntry to impact.ExternalBlastRadius.
func EntryToExternal(entry *BlastRadiusEntry) *impact.ExternalBlastRadius {
	ebr := &impact.ExternalBlastRadius{RiskLevel: entry.RiskLevel}
	for _, di := range entry.DirectItems {
		ebr.DirectItems = append(ebr.DirectItems, impact.ExternalItem{
			FileURI: di.FileURI, SymbolURI: di.SymbolURI,
			Distance: di.Distance, Confidence: di.Confidence,
		})
	}
	for _, ti := range entry.TransitiveItems {
		ebr.TransitiveItems = append(ebr.TransitiveItems, impact.ExternalItem{
			FileURI: ti.FileURI, SymbolURI: ti.SymbolURI,
			Distance: ti.Distance, Confidence: ti.Confidence,
		})
	}
	for _, si := range entry.SemanticItems {
		ebr.SemanticItems = append(ebr.SemanticItems, impact.ExternalSemanticItem{
			FileURI: si.FileURI, SymbolURI: si.SymbolURI,
			Similarity: si.Similarity, Source: si.Source,
		})
	}
	return ebr
}

