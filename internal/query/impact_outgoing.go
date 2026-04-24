package query

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/errors"
	"github.com/SimplyLiz/CodeMCP/internal/impact"
	"github.com/SimplyLiz/CodeMCP/internal/lip"
)

// AnalyzeOutgoingImpactOptions configures the forward-direction call-graph
// query — "what does this symbol call?" — the mirror of AnalyzeImpact.
type AnalyzeOutgoingImpactOptions struct {
	SymbolId string
	// MinScore is the cosine-similarity threshold for semantic callees.
	// 0 disables semantic enrichment entirely; typical non-zero value 0.6.
	MinScore float32
}

// AnalyzeOutgoingImpactResponse carries the forward call graph: callees
// reached at distance 1 (direct) and 2..N (transitive), optionally enriched
// with embedding-similar coupled symbols.
type AnalyzeOutgoingImpactResponse struct {
	Symbol            *SymbolInfo          `json:"symbol"`
	Visibility        *VisibilityInfo      `json:"visibility,omitempty"`
	DirectCallees     []ImpactItem         `json:"directCallees"`
	TransitiveCallees []ImpactItem         `json:"transitiveCallees,omitempty"`
	SemanticCallees   []SemanticCalleeInfo `json:"semanticCallees,omitempty"`
	// EdgesSource is LIP's provenance tag: "tier1", "scip_with_tier1_edges",
	// "scip_only", "empty". Helps consumers judge completeness.
	EdgesSource string      `json:"edgesSource,omitempty"`
	Truncated   bool        `json:"truncated,omitempty"`
	Provenance  *Provenance `json:"provenance"`
}

// SemanticCalleeInfo is a coupled symbol discovered by embedding similarity
// rather than the static call graph. `Source` is "semantic" (similarity-only)
// or "both" (also confirmed by static edges).
type SemanticCalleeInfo struct {
	SymbolURI  string  `json:"symbolUri,omitempty"`
	FileURI    string  `json:"fileUri"`
	Similarity float32 `json:"similarity"`
	Source     string  `json:"source"`
}

// AnalyzeOutgoingImpact asks LIP for the forward call graph of a symbol.
// Requires a daemon advertising `query_outgoing_impact` (LIP v2.3.5+ for
// Go method symbols — earlier versions hit the name-bridge asymmetry and
// return empty). When LIP is unavailable or the RPC isn't supported the
// response carries the symbol metadata with empty callee lists and a
// provenance warning; no error is returned.
func (e *Engine) AnalyzeOutgoingImpact(ctx context.Context, opts AnalyzeOutgoingImpactOptions) (*AnalyzeOutgoingImpactResponse, error) {
	startTime := time.Now()

	repoState, err := e.GetRepoState(ctx, "full")
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	symbolInfo, completeness, backendContribs := resolveSymbolForImpactHook(e, ctx, opts.SymbolId)
	if symbolInfo == nil {
		return nil, errors.NewCkbError(
			errors.SymbolNotFound,
			fmt.Sprintf("Symbol not found: %s", opts.SymbolId),
			nil, nil, nil,
		)
	}

	resp := &AnalyzeOutgoingImpactResponse{
		Symbol:     symbolInfo,
		Visibility: symbolInfo.Visibility,
	}

	provenance := e.buildProvenance(repoState, "full", startTime, backendContribs, completeness)

	if !e.lipSupports("query_outgoing_impact") {
		provenance.Warnings = append(provenance.Warnings,
			"LIP daemon unavailable or does not advertise query_outgoing_impact — outgoing call graph skipped")
		resp.Provenance = provenance
		return resp, nil
	}

	symURI := buildLIPSymbolURI(symbolInfo)
	if symURI == "" {
		provenance.Warnings = append(provenance.Warnings,
			"symbol has no resolvable LIP URI — outgoing call graph skipped")
		resp.Provenance = provenance
		return resp, nil
	}

	entry, err := lip.QueryOutgoingImpact(symURI, opts.MinScore)
	if err != nil {
		provenance.Warnings = append(provenance.Warnings,
			fmt.Sprintf("LIP query_outgoing_impact failed: %v", err))
		resp.Provenance = provenance
		return resp, nil
	}
	if entry == nil {
		// LIP reached the daemon but returned no result for this symbol.
		// Typically means the symbol isn't indexed or has no outgoing edges.
		resp.Provenance = provenance
		return resp, nil
	}

	external := lip.OutgoingEntryToExternal(entry)
	direct, transitive := impact.FoldExternalCalleeItems(nil, nil, external, e.repoRoot)

	resp.DirectCallees = convertImpactItems(direct)
	resp.TransitiveCallees = convertImpactItems(transitive)
	resp.EdgesSource = entry.EdgesSource
	resp.Truncated = entry.Truncated

	if external != nil {
		for _, si := range external.SemanticItems {
			resp.SemanticCallees = append(resp.SemanticCallees, SemanticCalleeInfo{
				SymbolURI:  si.SymbolURI,
				FileURI:    si.FileURI,
				Similarity: si.Similarity,
				Source:     si.Source,
			})
		}
	}

	sortImpactItems(resp.DirectCallees)
	sortImpactItems(resp.TransitiveCallees)
	sort.Slice(resp.SemanticCallees, func(i, j int) bool {
		return resp.SemanticCallees[i].Similarity > resp.SemanticCallees[j].Similarity
	})

	if resp.Truncated {
		provenance.Warnings = append(provenance.Warnings,
			"LIP hit its 200-node BFS cap — callee set is incomplete")
	}
	resp.Provenance = provenance
	return resp, nil
}

// resolveSymbolForImpactHook is the indirection tests hook to bypass the
// real SCIP/resolver path. Production callers never reassign it.
var resolveSymbolForImpactHook = func(e *Engine, ctx context.Context, symbolId string) (*SymbolInfo, CompletenessInfo, []BackendContribution) {
	return e.resolveSymbolForImpact(ctx, symbolId)
}

// resolveSymbolForImpact replicates the first half of AnalyzeImpact's symbol
// lookup: SCIP backend first, then identity-resolver fallback. Kept private
// to this package — callers outside query/ should use the engine methods.
func (e *Engine) resolveSymbolForImpact(ctx context.Context, symbolId string) (*SymbolInfo, CompletenessInfo, []BackendContribution) {
	var completeness CompletenessInfo
	var contribs []BackendContribution

	resolved, _ := e.resolver.ResolveSymbolId(symbolId)
	lookupId := symbolId
	if resolved != nil && resolved.Symbol != nil {
		lookupId = resolved.Symbol.StableId
	}

	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		result, err := e.scipAdapter.GetSymbol(ctx, lookupId)
		if err == nil && result != nil {
			info := &SymbolInfo{
				StableId:      result.StableID,
				Name:          result.Name,
				Kind:          result.Kind,
				ContainerName: result.ContainerName,
				ModuleId:      result.ModuleID,
				Visibility: &VisibilityInfo{
					Visibility: result.Visibility,
					Confidence: result.VisibilityConfidence,
					Source:     "scip",
				},
				Location: &LocationInfo{
					FileId:      result.Location.Path,
					StartLine:   result.Location.Line,
					StartColumn: result.Location.Column,
				},
			}
			contribs = append(contribs, BackendContribution{
				BackendId:    "scip",
				Available:    true,
				Used:         true,
				Completeness: result.Completeness.Score,
			})
			completeness = CompletenessInfo{
				Score:  result.Completeness.Score,
				Reason: string(result.Completeness.Reason),
			}
			return info, completeness, contribs
		}
	}

	if resolved != nil && resolved.Symbol != nil && resolved.Symbol.Fingerprint != nil {
		info := &SymbolInfo{
			StableId:      resolved.Symbol.StableId,
			Name:          resolved.Symbol.Fingerprint.Name,
			Kind:          string(resolved.Symbol.Fingerprint.Kind),
			ContainerName: resolved.Symbol.Fingerprint.QualifiedContainer,
			Visibility: &VisibilityInfo{
				Visibility: "unknown",
				Confidence: 0.3,
				Source:     "default",
			},
		}
		completeness = CompletenessInfo{Score: 0.5, Reason: "identity-only"}
		return info, completeness, contribs
	}

	return nil, completeness, contribs
}

// buildLIPSymbolURI translates a CKB symbol into the URI LIP expects on its
// query_* RPCs. Prefers the SCIP-form URI when the stable ID is a SCIP
// symbol (the common case post-index), falls back to the tier-1 local form
// keyed by file path + name.
func buildLIPSymbolURI(symbolInfo *SymbolInfo) string {
	if symbolInfo == nil {
		return ""
	}
	if sym := lip.SCIPSymbolToURI(symbolInfo.StableId); sym != "" && strings.HasPrefix(sym, "lip://") {
		return sym
	}
	if symbolInfo.Location != nil && symbolInfo.Location.FileId != "" && symbolInfo.Name != "" {
		return "lip://local/" + symbolInfo.Location.FileId + "#" + symbolInfo.Name
	}
	return ""
}
