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

// SCIPSymbolToURI translates a SCIP symbol string (space-separated
// `<scheme> <manager> <package> <version> <descriptor>`) into the LIP URI
// form LIP's daemon uses for SCIP-imported symbols:
// `lip://<scheme>/<manager>/<package>@<version>/<descriptor>`.
//
// Returns the input unchanged when it already looks like a LIP URI (starts
// with `lip://`) or doesn't parse as a 5-field SCIP symbol. Mirrors
// `scip_symbol_to_lip_uri` in LIP's import.rs.
func SCIPSymbolToURI(sym string) string {
	if sym == "" {
		return ""
	}
	if strings.HasPrefix(sym, "lip://") {
		return sym
	}
	parts := strings.SplitN(sym, " ", 5)
	if len(parts) != 5 {
		return sym
	}
	scheme, manager, pkg, version, descriptor := parts[0], parts[1], parts[2], parts[3], parts[4]
	descPath := strings.ReplaceAll(descriptor, " ", "/")
	return "lip://" + scheme + "/" + manager + "/" + pkg + "@" + version + "/" + descPath
}

// EntryToExternal converts a BlastRadiusEntry to impact.ExternalBlastRadius.
func EntryToExternal(entry *BlastRadiusEntry) *impact.ExternalBlastRadius {
	ebr := &impact.ExternalBlastRadius{
		RiskLevel:   entry.RiskLevel,
		EdgesSource: entry.EdgesSource,
	}
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

// OutgoingEntryToExternal converts an OutgoingImpactEntry to
// impact.ExternalBlastRadius so callers can reuse the same fold and merge
// machinery built for incoming blast radius.
//
// The shared Go type does not imply shared semantics: direct_items here are
// callees (symbols the target invokes), not callers. Consumers must classify
// folded items with DirectCallee / TransitiveCallee kinds rather than
// DirectCaller / TransitiveCaller.
//
// RiskLevel is intentionally left empty — outgoing impact doesn't carry its
// own risk classification; CKB derives one from the unioned callee set on
// receipt.
func OutgoingEntryToExternal(entry *OutgoingImpactEntry) *impact.ExternalBlastRadius {
	if entry == nil {
		return nil
	}
	ebr := &impact.ExternalBlastRadius{
		EdgesSource: entry.EdgesSource,
	}
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

