package lip

import (
	"context"

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

	entries, err := QueryBlastRadiusBatch(changedFileURIs, e.MinScore)
	if entries == nil {
		return nil, err
	}

	out := make(map[string]*impact.ExternalBlastRadius, len(entries))
	for symbolURI, entry := range entries {
		ebr := &impact.ExternalBlastRadius{
			RiskLevel: entry.RiskLevel,
		}

		for _, di := range entry.DirectItems {
			ebr.DirectItems = append(ebr.DirectItems, impact.ExternalItem{
				FileURI:    di.FileURI,
				SymbolURI:  di.SymbolURI,
				Distance:   di.Distance,
				Confidence: di.Confidence,
			})
		}

		for _, ti := range entry.TransitiveItems {
			ebr.TransitiveItems = append(ebr.TransitiveItems, impact.ExternalItem{
				FileURI:    ti.FileURI,
				SymbolURI:  ti.SymbolURI,
				Distance:   ti.Distance,
				Confidence: ti.Confidence,
			})
		}

		for _, si := range entry.SemanticItems {
			ebr.SemanticItems = append(ebr.SemanticItems, impact.ExternalSemanticItem{
				FileURI:    si.FileURI,
				SymbolURI:  si.SymbolURI,
				Similarity: si.Similarity,
				Source:     si.Source,
			})
		}

		out[symbolURI] = ebr
	}

	return out, nil
}
