package query

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"ckb/internal/backends"
	"ckb/internal/compression"
	"ckb/internal/errors"
	"ckb/internal/output"
)

// GetSymbolOptions contains options for getSymbol.
type GetSymbolOptions struct {
	SymbolId      string
	RepoStateMode string // "head" or "full"
}

// GetSymbolResponse is the response for getSymbol.
type GetSymbolResponse struct {
	Symbol         *SymbolInfo        `json:"symbol,omitempty"`
	Redirected     bool               `json:"redirected,omitempty"`
	RedirectedFrom string             `json:"redirectedFrom,omitempty"`
	RedirectReason string             `json:"redirectReason,omitempty"`
	Deleted        bool               `json:"deleted,omitempty"`
	DeletedAt      string             `json:"deletedAt,omitempty"`
	Provenance     *Provenance        `json:"provenance"`
	Drilldowns     []output.Drilldown `json:"drilldowns,omitempty"`
}

// SymbolInfo contains symbol metadata.
type SymbolInfo struct {
	StableId            string          `json:"stableId"`
	Name                string          `json:"name"`
	Kind                string          `json:"kind"`
	Signature           string          `json:"signature,omitempty"`
	SignatureNormalized string          `json:"signatureNormalized,omitempty"`
	Visibility          *VisibilityInfo `json:"visibility"`
	ModuleId            string          `json:"moduleId"`
	ModuleName          string          `json:"moduleName,omitempty"`
	ContainerName       string          `json:"containerName,omitempty"`
	Location            *LocationInfo   `json:"location"`
	LocationFreshness   string          `json:"locationFreshness"`
	Documentation       string          `json:"documentation,omitempty"`
}

// VisibilityInfo describes symbol visibility.
type VisibilityInfo struct {
	Visibility string  `json:"visibility"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source"`
}

// LocationInfo describes a source location.
type LocationInfo struct {
	FileId      string `json:"fileId"`
	StartLine   int    `json:"startLine"`
	StartColumn int    `json:"startColumn"`
	EndLine     int    `json:"endLine,omitempty"`
	EndColumn   int    `json:"endColumn,omitempty"`
}

// TruncationInfo describes why results were truncated.
type TruncationInfo struct {
	Reason        string `json:"reason"`
	OriginalCount int    `json:"originalCount"`
	ReturnedCount int    `json:"returnedCount"`
}

// GetSymbol retrieves symbol information by ID.
func (e *Engine) GetSymbol(ctx context.Context, opts GetSymbolOptions) (*GetSymbolResponse, error) {
	startTime := time.Now()

	// Default to head mode
	if opts.RepoStateMode == "" {
		opts.RepoStateMode = "head"
	}

	// Get repo state
	repoState, err := e.GetRepoState(ctx, opts.RepoStateMode)
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	// Resolve symbol ID through aliases
	resolved, err := e.resolver.ResolveSymbolId(opts.SymbolId)
	if err != nil {
		// Check if it's a known error type
		if ckbErr, ok := err.(*errors.CkbError); ok {
			completeness := CompletenessInfo{Score: 0.0, Reason: "symbol-not-found"}
			return &GetSymbolResponse{
				Provenance: e.buildProvenance(repoState, opts.RepoStateMode, startTime, nil, completeness),
				Drilldowns: []output.Drilldown{
					{Label: "Search for similar symbols", Query: fmt.Sprintf("searchSymbols %s", opts.SymbolId)},
				},
			}, ckbErr
		}
		return nil, e.wrapError(err, errors.SymbolNotFound)
	}

	// Handle deleted symbols
	if resolved.Deleted {
		completeness := CompletenessInfo{Score: 1.0, Reason: "symbol-deleted"}
		return &GetSymbolResponse{
			Deleted:    true,
			DeletedAt:  resolved.DeletedAt,
			Provenance: e.buildProvenance(repoState, opts.RepoStateMode, startTime, nil, completeness),
		}, nil
	}

	// Build response from resolved symbol
	response := &GetSymbolResponse{
		Redirected:     resolved.Redirected,
		RedirectedFrom: resolved.RedirectedFrom,
		RedirectReason: string(resolved.RedirectReason),
	}

	var backendContribs []BackendContribution
	var completeness CompletenessInfo

	if resolved.Symbol != nil {
		// Query SCIP backend for symbol data
		if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
			result, err := e.scipAdapter.GetSymbol(ctx, resolved.Symbol.StableId)
			if err == nil && result != nil {
				// Build symbol info from backend result
				response.Symbol = &SymbolInfo{
					StableId:            result.StableID,
					Name:                result.Name,
					Kind:                result.Kind,
					Signature:           result.SignatureFull,
					SignatureNormalized: result.SignatureNormalized,
					ContainerName:       result.ContainerName,
					ModuleId:            result.ModuleID,
					Documentation:       result.Documentation,
					LocationFreshness:   e.getLocationFreshness(repoState),
					Visibility: &VisibilityInfo{
						Visibility: result.Visibility,
						Confidence: result.VisibilityConfidence,
						Source:     "scip",
					},
					Location: &LocationInfo{
						FileId:      result.Location.Path,
						StartLine:   result.Location.Line,
						StartColumn: result.Location.Column,
						EndLine:     result.Location.EndLine,
						EndColumn:   result.Location.EndColumn,
					},
				}
				backendContribs = append(backendContribs, BackendContribution{
					BackendId:    "scip",
					Available:    true,
					Used:         true,
					ResultCount:  1,
					Completeness: result.Completeness.Score,
				})
				completeness = CompletenessInfo{
					Score:  result.Completeness.Score,
					Reason: string(result.Completeness.Reason),
				}
			}
		}

		// Fallback to identity data if no backend result
		if response.Symbol == nil && resolved.Symbol.Fingerprint != nil {
			response.Symbol = &SymbolInfo{
				StableId:          resolved.Symbol.StableId,
				Name:              resolved.Symbol.Fingerprint.Name,
				Kind:              string(resolved.Symbol.Fingerprint.Kind),
				ContainerName:     resolved.Symbol.Fingerprint.QualifiedContainer,
				LocationFreshness: e.getLocationFreshness(repoState),
				Visibility: &VisibilityInfo{
					Visibility: "unknown",
					Confidence: 0.3,
					Source:     "default",
				},
			}
			if resolved.Symbol.Location != nil {
				response.Symbol.Location = &LocationInfo{
					FileId:      resolved.Symbol.Location.Path,
					StartLine:   resolved.Symbol.Location.Line,
					StartColumn: resolved.Symbol.Location.Column,
				}
			}
			completeness = CompletenessInfo{
				Score:  0.5,
				Reason: "identity-only",
			}
		}
	}

	response.Provenance = e.buildProvenance(repoState, opts.RepoStateMode, startTime, backendContribs, completeness)
	response.Drilldowns = e.generateDrilldowns(nil, completeness, opts.SymbolId, nil)

	return response, nil
}

// getLocationFreshness determines location freshness based on repo state.
func (e *Engine) getLocationFreshness(repoState *RepoState) string {
	if repoState.Dirty {
		return "may-be-stale"
	}
	return "fresh"
}

// SearchSymbolsOptions contains options for searchSymbols.
type SearchSymbolsOptions struct {
	Query string
	Scope string
	Kinds []string
	Limit int
}

// SearchSymbolsResponse is the response for searchSymbols.
type SearchSymbolsResponse struct {
	Symbols        []SearchResultItem `json:"symbols"`
	TotalCount     int                `json:"totalCount"`
	Truncated      bool               `json:"truncated"`
	TruncationInfo *TruncationInfo    `json:"truncationInfo,omitempty"`
	Provenance     *Provenance        `json:"provenance"`
	Drilldowns     []output.Drilldown `json:"drilldowns,omitempty"`
}

// RankingV52 contains v5.2 ranking signals for auditable, deterministic ordering.
type RankingV52 struct {
	Score         float64                `json:"score"`
	Signals       map[string]interface{} `json:"signals"`
	PolicyVersion string                 `json:"policyVersion"`
}

// NewRankingV52 creates a new v5.2 ranking with the given score and signals.
func NewRankingV52(score float64, signals map[string]interface{}) *RankingV52 {
	return &RankingV52{
		Score:         score,
		Signals:       signals,
		PolicyVersion: "5.2",
	}
}

// SearchResultItem represents a symbol search result.
type SearchResultItem struct {
	StableId   string          `json:"stableId"`
	Name       string          `json:"name"`
	Kind       string          `json:"kind"`
	ModuleId   string          `json:"moduleId"`
	ModuleName string          `json:"moduleName,omitempty"`
	Location   *LocationInfo   `json:"location,omitempty"`
	Visibility *VisibilityInfo `json:"visibility,omitempty"`
	Score      float64         `json:"score"`
	Ranking    *RankingV52     `json:"ranking,omitempty"`
}

// SearchSymbols searches for symbols by name.
func (e *Engine) SearchSymbols(ctx context.Context, opts SearchSymbolsOptions) (*SearchSymbolsResponse, error) {
	startTime := time.Now()

	// Default limit
	if opts.Limit <= 0 {
		opts.Limit = 20
	}

	// Get repo state
	repoState, err := e.GetRepoState(ctx, "head")
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	var results []SearchResultItem
	var backendContribs []BackendContribution
	var completeness CompletenessInfo

	// Try SCIP first
	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		searchOpts := backends.SearchOptions{
			MaxResults:   opts.Limit * 2, // Request more to allow for ranking
			IncludeTests: true,
			Scope:        parseScope(opts.Scope),
			Kind:         opts.Kinds,
		}
		searchResult, err := e.scipAdapter.SearchSymbols(ctx, opts.Query, searchOpts)
		if err == nil && searchResult != nil {
			for _, sym := range searchResult.Symbols {
				results = append(results, SearchResultItem{
					StableId: sym.StableID,
					Name:     sym.Name,
					Kind:     sym.Kind,
					ModuleId: sym.ModuleID,
					Location: &LocationInfo{
						FileId:      sym.Location.Path,
						StartLine:   sym.Location.Line,
						StartColumn: sym.Location.Column,
						EndLine:     sym.Location.EndLine,
						EndColumn:   sym.Location.EndColumn,
					},
					Visibility: &VisibilityInfo{
						Visibility: sym.Visibility,
						Confidence: sym.VisibilityConfidence,
						Source:     "scip",
					},
				})
			}
			backendContribs = append(backendContribs, BackendContribution{
				BackendId:    "scip",
				Available:    true,
				Used:         true,
				ResultCount:  len(searchResult.Symbols),
				Completeness: searchResult.Completeness.Score,
			})
			completeness = CompletenessInfo{
				Score:  searchResult.Completeness.Score,
				Reason: string(searchResult.Completeness.Reason),
			}
		}
	}

	// If no results, return empty response
	if len(results) == 0 {
		completeness = CompletenessInfo{Score: 0.0, Reason: "no-results"}
		return &SearchSymbolsResponse{
			Symbols:    []SearchResultItem{},
			TotalCount: 0,
			Truncated:  false,
			Provenance: e.buildProvenance(repoState, "head", startTime, backendContribs, completeness),
		}, nil
	}

	// Apply ranking
	rankSearchResults(results, opts.Query)

	// Sort by score
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Apply limit and track truncation
	totalCount := len(results)
	var truncationInfo *TruncationInfo
	if len(results) > opts.Limit {
		truncationInfo = &TruncationInfo{
			Reason:        "max-results",
			OriginalCount: totalCount,
			ReturnedCount: opts.Limit,
		}
		results = results[:opts.Limit]
	}

	// Build provenance
	provenance := e.buildProvenance(repoState, "head", startTime, backendContribs, completeness)

	// Generate drilldowns
	var compTrunc *compression.TruncationInfo
	if truncationInfo != nil {
		compTrunc = &compression.TruncationInfo{
			Reason:        compression.TruncMaxSymbols,
			OriginalCount: truncationInfo.OriginalCount,
			ReturnedCount: truncationInfo.ReturnedCount,
		}
	}
	drilldowns := e.generateDrilldowns(compTrunc, completeness, "", nil)

	return &SearchSymbolsResponse{
		Symbols:        results,
		TotalCount:     totalCount,
		Truncated:      truncationInfo != nil,
		TruncationInfo: truncationInfo,
		Provenance:     provenance,
		Drilldowns:     drilldowns,
	}, nil
}

// parseScope converts scope string to slice.
func parseScope(scope string) []string {
	if scope == "" {
		return nil
	}
	return []string{scope}
}

// rankSearchResults applies ranking to search results with v5.2 signals.
func rankSearchResults(results []SearchResultItem, query string) {
	queryLower := strings.ToLower(query)

	for i := range results {
		score := 0.0
		var matchType string

		// Determine match type and apply score
		nameLower := strings.ToLower(results[i].Name)
		if strings.EqualFold(results[i].Name, query) {
			matchType = "exact"
			score += 100
		} else if strings.HasPrefix(nameLower, queryLower) {
			matchType = "partial"
			score += 50
		} else if strings.Contains(nameLower, queryLower) {
			matchType = "partial"
			score += 25
		} else {
			matchType = "fuzzy"
			score += 10
		}

		// Visibility weight
		if results[i].Visibility != nil {
			switch results[i].Visibility.Visibility {
			case "public":
				score += 30
			case "internal":
				score += 20
			case "private":
				score += 10
			default:
				score += 5
			}
		}

		// Kind weight
		switch results[i].Kind {
		case "class", "interface":
			score += 25
		case "function":
			score += 20
		case "method":
			score += 15
		case "property":
			score += 10
		default:
			score += 5
		}

		results[i].Score = score

		// Build v5.2 ranking signals
		scope := ""
		if results[i].Location != nil && results[i].Location.FileId != "" {
			scope = results[i].Location.FileId
		}
		if results[i].ModuleId != "" {
			scope = results[i].ModuleId
		}

		results[i].Ranking = NewRankingV52(score, map[string]interface{}{
			"matchType": matchType,
			"kind":      results[i].Kind,
			"scope":     scope,
		})
	}
}

// FindReferencesOptions contains options for findReferences.
type FindReferencesOptions struct {
	SymbolId     string
	Scope        string
	IncludeTests bool
	Limit        int
}

// FindReferencesResponse is the response for findReferences.
type FindReferencesResponse struct {
	References     []ReferenceInfo    `json:"references"`
	TotalCount     int                `json:"totalCount"`
	Truncated      bool               `json:"truncated"`
	TruncationInfo *TruncationInfo    `json:"truncationInfo,omitempty"`
	Provenance     *Provenance        `json:"provenance"`
	Drilldowns     []output.Drilldown `json:"drilldowns,omitempty"`
}

// ReferenceInfo describes a reference to a symbol.
type ReferenceInfo struct {
	Location *LocationInfo `json:"location"`
	Kind     string        `json:"kind"`
	Context  string        `json:"context,omitempty"`
	IsTest   bool          `json:"isTest,omitempty"`
}

// FindReferences finds all references to a symbol.
func (e *Engine) FindReferences(ctx context.Context, opts FindReferencesOptions) (*FindReferencesResponse, error) {
	startTime := time.Now()

	// Default options
	if opts.Limit <= 0 {
		opts.Limit = 100
	}

	// Get repo state (full mode for references)
	repoState, err := e.GetRepoState(ctx, "full")
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	// Resolve symbol ID - first try the identity resolver
	var symbolIdToQuery string
	resolved, err := e.resolver.ResolveSymbolId(opts.SymbolId)
	if err == nil && resolved.Symbol != nil {
		symbolIdToQuery = resolved.Symbol.StableId
	} else {
		// Fall back to using the raw symbol ID directly (for SCIP symbols not in SQLite)
		symbolIdToQuery = opts.SymbolId
	}

	var refs []ReferenceInfo
	var backendContribs []BackendContribution
	var completeness CompletenessInfo

	// Query SCIP for references
	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		refOpts := backends.RefOptions{
			MaxResults:         opts.Limit * 2,
			IncludeTests:       opts.IncludeTests,
			IncludeDeclaration: true,
			Scope:              parseScope(opts.Scope),
		}
		refsResult, err := e.scipAdapter.FindReferences(ctx, symbolIdToQuery, refOpts)
		if err == nil && refsResult != nil {
			for _, ref := range refsResult.References {
				refs = append(refs, ReferenceInfo{
					Location: &LocationInfo{
						FileId:      ref.Location.Path,
						StartLine:   ref.Location.Line,
						StartColumn: ref.Location.Column,
						EndLine:     ref.Location.EndLine,
						EndColumn:   ref.Location.EndColumn,
					},
					Kind:    ref.Kind,
					Context: ref.Context,
				})
			}
			backendContribs = append(backendContribs, BackendContribution{
				BackendId:    "scip",
				Available:    true,
				Used:         true,
				ResultCount:  len(refsResult.References),
				Completeness: refsResult.Completeness.Score,
			})
			completeness = CompletenessInfo{
				Score:  refsResult.Completeness.Score,
				Reason: string(refsResult.Completeness.Reason),
			}
		}
	}

	// If no results and symbol wasn't found in identity system, return not found
	if len(refs) == 0 && (resolved == nil || resolved.Symbol == nil) {
		return nil, errors.NewCkbError(
			errors.SymbolNotFound,
			fmt.Sprintf("Symbol not found: %s", opts.SymbolId),
			nil, nil, nil,
		)
	}

	// Deduplicate
	refs = deduplicateReferences(refs)

	// Sort deterministically
	sortReferences(refs)

	// Apply limit and track truncation
	totalCount := len(refs)
	var truncationInfo *TruncationInfo
	if len(refs) > opts.Limit {
		truncationInfo = &TruncationInfo{
			Reason:        "max-refs",
			OriginalCount: totalCount,
			ReturnedCount: opts.Limit,
		}
		refs = refs[:opts.Limit]
	}

	// Build provenance
	provenance := e.buildProvenance(repoState, "full", startTime, backendContribs, completeness)

	// Generate drilldowns
	var compTrunc *compression.TruncationInfo
	if truncationInfo != nil {
		compTrunc = &compression.TruncationInfo{
			Reason:        compression.TruncMaxRefs,
			OriginalCount: truncationInfo.OriginalCount,
			ReturnedCount: truncationInfo.ReturnedCount,
		}
	}
	drilldowns := e.generateDrilldowns(compTrunc, completeness, opts.SymbolId, nil)

	return &FindReferencesResponse{
		References:     refs,
		TotalCount:     totalCount,
		Truncated:      truncationInfo != nil,
		TruncationInfo: truncationInfo,
		Provenance:     provenance,
		Drilldowns:     drilldowns,
	}, nil
}

// deduplicateReferences removes duplicate references.
func deduplicateReferences(refs []ReferenceInfo) []ReferenceInfo {
	seen := make(map[string]bool)
	result := make([]ReferenceInfo, 0, len(refs))

	for _, ref := range refs {
		if ref.Location == nil {
			continue
		}
		key := fmt.Sprintf("%s:%d:%d", ref.Location.FileId, ref.Location.StartLine, ref.Location.StartColumn)
		if !seen[key] {
			seen[key] = true
			result = append(result, ref)
		}
	}

	return result
}

// sortReferences sorts references deterministically.
func sortReferences(refs []ReferenceInfo) {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Location.FileId != refs[j].Location.FileId {
			return refs[i].Location.FileId < refs[j].Location.FileId
		}
		if refs[i].Location.StartLine != refs[j].Location.StartLine {
			return refs[i].Location.StartLine < refs[j].Location.StartLine
		}
		return refs[i].Location.StartColumn < refs[j].Location.StartColumn
	})
}
