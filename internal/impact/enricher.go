package impact

import "context"

// BlastRadiusEnricher supplements SCIP-derived blast radius with external data
// (e.g., LIP embedding-based semantic coupling). Implementations must be safe
// for concurrent use and degrade gracefully — returning nil signals "unavailable".
type BlastRadiusEnricher interface {
	// EnrichBatch takes changed file URIs and returns per-symbol blast radius
	// from the external source. The map key is the symbol URI (e.g.,
	// "lip://local/src/auth.rs#validate_token"). Returns nil when the source
	// is unavailable.
	EnrichBatch(ctx context.Context, changedFileURIs []string) (map[string]*ExternalBlastRadius, error)
}

// ExternalBlastRadius is what an enricher returns per symbol.
type ExternalBlastRadius struct {
	// DirectItems are callers the external source found via static analysis.
	// These overlap with SCIP's results and are used to confirm edges.
	DirectItems []ExternalItem
	// TransitiveItems are transitive callers from the external source.
	TransitiveItems []ExternalItem
	// SemanticItems are callers found via embedding similarity that may not
	// appear in any static call graph (dynamic dispatch, macros, etc.).
	SemanticItems []ExternalSemanticItem
	// RiskLevel is the external source's own risk assessment.
	RiskLevel string
}

// ExternalItem is a static caller from an external blast radius source.
type ExternalItem struct {
	FileURI    string
	SymbolURI  string
	Distance   int
	Confidence float64
}

// ExternalSemanticItem is a semantically coupled symbol from an enricher.
type ExternalSemanticItem struct {
	FileURI    string
	SymbolURI  string
	Similarity float32 // cosine similarity
	Source     string  // "semantic" or "both"
}

// MergeBlastRadius blends SCIP-derived blast radius with enricher data.
//
// Design invariant: UniqueCallerCount stays SCIP-only so that reviewPR
// thresholds (callerCount >= 3, callerCount > maxFanOut) are never inflated
// by embedding noise. Semantic callers are additive in SemanticCallerCount
// and SemanticCallers — they inform humans, not thresholds.
//
// Items with source=="both" confirm that a SCIP static edge also has embedding
// evidence. These bump ConfirmedCount but don't change UniqueCallerCount.
func MergeBlastRadius(static *BlastRadius, external *ExternalBlastRadius) *BlastRadius {
	if static == nil {
		return nil
	}
	if external == nil {
		return static
	}

	merged := *static // shallow copy
	merged.StaticCallerCount = static.UniqueCallerCount

	// Build a set of files SCIP already knows about (from static callers).
	// We use file URIs because SCIP symbol IDs and LIP symbol URIs use different
	// schemes — file URI is the stable join key.
	staticFiles := make(map[string]struct{})
	for _, item := range external.DirectItems {
		staticFiles[item.FileURI] = struct{}{}
	}
	for _, item := range external.TransitiveItems {
		staticFiles[item.FileURI] = struct{}{}
	}

	var semanticCallers []EnrichedCaller
	confirmed := 0
	seen := make(map[string]struct{}) // dedup by file URI

	for _, si := range external.SemanticItems {
		if _, dup := seen[si.FileURI]; dup {
			continue
		}
		seen[si.FileURI] = struct{}{}

		switch si.Source {
		case "both":
			// Confirms a SCIP edge — record but don't inflate counts
			confirmed++
			semanticCallers = append(semanticCallers, EnrichedCaller{
				SymbolURI:  si.SymbolURI,
				FileURI:    si.FileURI,
				Tier:       CouplingBoth,
				Confidence: 0.95,
				Similarity: si.Similarity,
			})
		case "semantic":
			// New coupling not in SCIP — advisory
			semanticCallers = append(semanticCallers, EnrichedCaller{
				SymbolURI:  si.SymbolURI,
				FileURI:    si.FileURI,
				Tier:       CouplingSemantic,
				Confidence: float64(si.Similarity), // cosine similarity as confidence proxy
				Similarity: si.Similarity,
			})
		}
	}

	// Count only pure semantic (not "both") as additional callers
	pureSemanticCount := 0
	for _, c := range semanticCallers {
		if c.Tier == CouplingSemantic {
			pureSemanticCount++
		}
	}

	merged.SemanticCallerCount = pureSemanticCount
	merged.ConfirmedCount = confirmed
	merged.SemanticCallers = semanticCallers
	// RiskLevel stays SCIP-derived. Semantic coupling informs the human, not the threshold.
	return &merged
}
