package query

import (
	"context"
	"time"

	"ckb/internal/breaking"
)

// CompareAPIOptions configures API comparison
type CompareAPIOptions struct {
	BaseRef       string   `json:"baseRef"`       // Git ref for base version (default: HEAD~1)
	TargetRef     string   `json:"targetRef"`     // Git ref for target version (default: HEAD)
	Scope         []string `json:"scope"`         // Limit to specific packages/paths
	IncludeMinor  bool     `json:"includeMinor"`  // Include non-breaking changes in output
	IgnorePrivate bool     `json:"ignorePrivate"` // Only compare exported symbols (default: true)
}

// CompareAPIResponse contains the API comparison result
type CompareAPIResponse struct {
	BaseRef            string            `json:"baseRef"`
	TargetRef          string            `json:"targetRef"`
	Changes            []APIChangeItem   `json:"changes"`
	Summary            *APIChangeSummary `json:"summary"`
	SemverAdvice       string            `json:"semverAdvice,omitempty"`
	TotalBaseSymbols   int               `json:"totalBaseSymbols"`
	TotalTargetSymbols int               `json:"totalTargetSymbols"`
	Provenance         *Provenance       `json:"provenance,omitempty"`
}

// APIChangeItem represents a single API change
type APIChangeItem struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	SymbolName   string `json:"symbolName"`
	SymbolKind   string `json:"symbolKind"`
	Package      string `json:"package"`
	FilePath     string `json:"filePath"`
	LineNumber   int    `json:"lineNumber,omitempty"`
	Description  string `json:"description"`
	OldValue     string `json:"oldValue,omitempty"`
	NewValue     string `json:"newValue,omitempty"`
	Suggestion   string `json:"suggestion,omitempty"`
	AffectsUsers bool   `json:"affectsUsers"`
}

// APIChangeSummary provides an overview of API changes
type APIChangeSummary struct {
	TotalChanges    int            `json:"totalChanges"`
	BreakingChanges int            `json:"breakingChanges"`
	Warnings        int            `json:"warnings"`
	Additions       int            `json:"additions"`
	ByKind          map[string]int `json:"byKind"`
	ByPackage       map[string]int `json:"byPackage,omitempty"`
}

// CompareAPI compares API surfaces between two git refs
func (e *Engine) CompareAPI(ctx context.Context, opts CompareAPIOptions) (*CompareAPIResponse, error) {
	startTime := time.Now()

	// Apply defaults
	if opts.BaseRef == "" {
		opts.BaseRef = "HEAD~1"
	}
	if opts.TargetRef == "" {
		opts.TargetRef = "HEAD"
	}

	// Get repo state
	repoState, _ := e.GetRepoState(ctx, "fast")

	// Create analyzer
	analyzer := breaking.NewAnalyzer(e.scipAdapter, e.repoRoot, e.logger)

	// Compare
	compareOpts := breaking.CompareOptions{
		BaseRef:       opts.BaseRef,
		TargetRef:     opts.TargetRef,
		Scope:         opts.Scope,
		IncludeMinor:  opts.IncludeMinor,
		IgnorePrivate: opts.IgnorePrivate,
	}

	result, err := analyzer.Compare(ctx, compareOpts)
	if err != nil {
		return nil, err
	}

	// Build provenance
	completeness := CompletenessInfo{Score: 0.8, Reason: "static-analysis"}
	provenance := e.buildProvenance(repoState, "fast", startTime, nil, completeness)

	// Convert to response format
	response := &CompareAPIResponse{
		BaseRef:            result.BaseRef,
		TargetRef:          result.TargetRef,
		SemverAdvice:       result.SemverAdvice,
		TotalBaseSymbols:   result.TotalBaseSymbols,
		TotalTargetSymbols: result.TotalTargetSymbols,
		Provenance:         provenance,
	}

	// Convert changes
	response.Changes = make([]APIChangeItem, len(result.Changes))
	for i, change := range result.Changes {
		response.Changes[i] = APIChangeItem{
			Kind:         string(change.Kind),
			Severity:     string(change.Severity),
			SymbolName:   change.SymbolName,
			SymbolKind:   change.SymbolKind,
			Package:      change.Package,
			FilePath:     change.FilePath,
			LineNumber:   change.LineNumber,
			Description:  change.Description,
			OldValue:     change.OldValue,
			NewValue:     change.NewValue,
			Suggestion:   change.Suggestion,
			AffectsUsers: change.AffectsUsers,
		}
	}

	// Convert summary
	if result.Summary != nil {
		response.Summary = &APIChangeSummary{
			TotalChanges:    result.Summary.TotalChanges,
			BreakingChanges: result.Summary.BreakingChanges,
			Warnings:        result.Summary.Warnings,
			Additions:       result.Summary.Additions,
			ByKind:          result.Summary.ByKind,
			ByPackage:       result.Summary.ByPackage,
		}
	}

	return response, nil
}
