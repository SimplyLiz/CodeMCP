package compression

import (
	"fmt"

	"ckb/internal/output"
)

// CompletenessInfo describes how complete the query results are
type CompletenessInfo struct {
	// Score is a 0-1 value indicating result completeness (1 = fully complete)
	Score float64 `json:"score"`

	// Source indicates which backend(s) provided the results
	Source string `json:"source"`

	// IsWorkspaceReady indicates if LSP workspace is fully initialized
	IsWorkspaceReady bool `json:"isWorkspaceReady"`

	// IsBestEffort indicates if results are from best-effort LSP (not fully warmed up)
	IsBestEffort bool `json:"isBestEffort"`
}

// IndexFreshness describes the freshness of the SCIP index
type IndexFreshness struct {
	// StaleAgainstHead indicates if the index is outdated compared to HEAD
	StaleAgainstHead bool `json:"staleAgainstHead"`

	// LastIndexedCommit is the commit hash the index was built from
	LastIndexedCommit string `json:"lastIndexedCommit,omitempty"`

	// HeadCommit is the current HEAD commit hash
	HeadCommit string `json:"headCommit,omitempty"`
}

// DrilldownContext provides context for generating relevant drilldown suggestions
type DrilldownContext struct {
	// TruncationReason indicates why data was truncated (if any)
	TruncationReason TruncationReason

	// Completeness describes how complete the results are
	Completeness CompletenessInfo

	// IndexFreshness describes the SCIP index freshness (if applicable)
	IndexFreshness *IndexFreshness

	// SymbolId is the symbol being queried (for reference queries)
	SymbolId string

	// TopModule is the module with the most impact (if applicable)
	TopModule *output.Module

	// Budget is the current response budget
	Budget *ResponseBudget
}

// GenerateDrilldowns creates contextual follow-up query suggestions based on context
// Returns a list of drilldowns limited by budget.MaxDrilldowns
func GenerateDrilldowns(ctx *DrilldownContext) []output.Drilldown {
	if ctx == nil || ctx.Budget == nil {
		return []output.Drilldown{}
	}

	drilldowns := []output.Drilldown{}

	// Generate drilldowns based on truncation reason
	drilldowns = append(drilldowns, generateTruncationDrilldowns(ctx)...)

	// Generate drilldowns based on completeness
	drilldowns = append(drilldowns, generateCompletenessDrilldowns(ctx)...)

	// Generate drilldowns based on index freshness
	drilldowns = append(drilldowns, generateFreshnessDrilldowns(ctx)...)

	// Limit to MaxDrilldowns
	if len(drilldowns) > ctx.Budget.MaxDrilldowns {
		drilldowns = drilldowns[:ctx.Budget.MaxDrilldowns]
	}

	return drilldowns
}

// generateTruncationDrilldowns creates drilldowns based on truncation reasons
func generateTruncationDrilldowns(ctx *DrilldownContext) []output.Drilldown {
	drilldowns := []output.Drilldown{}

	switch ctx.TruncationReason {
	case TruncMaxModules:
		// Suggest exploring the top module in detail
		if ctx.TopModule != nil {
			drilldowns = append(drilldowns, output.Drilldown{
				Label:          fmt.Sprintf("Explore top module: %s", ctx.TopModule.Name),
				Query:          fmt.Sprintf("getModuleOverview %s", ctx.TopModule.ModuleId),
				RelevanceScore: 0.9,
			})
		}

	case TruncMaxItems:
		// Suggest scoping to a specific module
		if ctx.TopModule != nil && ctx.SymbolId != "" {
			drilldowns = append(drilldowns, output.Drilldown{
				Label:          "Scope to specific module",
				Query:          fmt.Sprintf("findReferences %s --scope=%s", ctx.SymbolId, ctx.TopModule.ModuleId),
				RelevanceScore: 0.85,
			})
		}

	case TruncMaxRefs:
		// Suggest using pagination or filtering
		if ctx.SymbolId != "" {
			drilldowns = append(drilldowns, output.Drilldown{
				Label:          "Get first page of references",
				Query:          fmt.Sprintf("findReferences %s --limit=100", ctx.SymbolId),
				RelevanceScore: 0.8,
			})
		}

	case TruncTimeout:
		// Suggest trying again with stricter limits
		if ctx.SymbolId != "" {
			drilldowns = append(drilldowns, output.Drilldown{
				Label:          "Retry with faster backend",
				Query:          fmt.Sprintf("findReferences %s --backend=scip", ctx.SymbolId),
				RelevanceScore: 0.75,
			})
		}
	}

	return drilldowns
}

// generateCompletenessDrilldowns creates drilldowns based on result completeness
func generateCompletenessDrilldowns(ctx *DrilldownContext) []output.Drilldown {
	drilldowns := []output.Drilldown{}

	// If results are best-effort LSP, suggest checking workspace status
	if ctx.Completeness.IsBestEffort {
		drilldowns = append(drilldowns, output.Drilldown{
			Label:          "Check workspace status",
			Query:          "getStatus",
			RelevanceScore: 0.7,
		})
	}

	// If workspace is not ready, suggest waiting
	if !ctx.Completeness.IsWorkspaceReady && ctx.SymbolId != "" {
		drilldowns = append(drilldowns, output.Drilldown{
			Label:          "Retry after warmup",
			Query:          fmt.Sprintf("findReferences %s --wait-for-ready", ctx.SymbolId),
			RelevanceScore: 0.8,
		})
	}

	// If completeness score is low, suggest union mode for maximum results
	if ctx.Completeness.Score < 0.8 && ctx.SymbolId != "" {
		drilldowns = append(drilldowns, output.Drilldown{
			Label:          "Get maximum results (slower)",
			Query:          fmt.Sprintf("findReferences %s --merge=union", ctx.SymbolId),
			RelevanceScore: 0.65,
		})
	}

	return drilldowns
}

// generateFreshnessDrilldowns creates drilldowns based on index freshness
func generateFreshnessDrilldowns(ctx *DrilldownContext) []output.Drilldown {
	drilldowns := []output.Drilldown{}

	// If SCIP index is stale, suggest regenerating
	if ctx.IndexFreshness != nil && ctx.IndexFreshness.StaleAgainstHead {
		drilldowns = append(drilldowns, output.Drilldown{
			Label:          "Regenerate SCIP index",
			Query:          "doctor --check=scip",
			RelevanceScore: 0.6,
		})
	}

	return drilldowns
}

// SortDrilldownsByRelevance sorts drilldowns by relevance score (highest first)
func SortDrilldownsByRelevance(drilldowns []output.Drilldown) {
	// Simple bubble sort since we're dealing with small arrays (max 5 items)
	n := len(drilldowns)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if drilldowns[j].RelevanceScore < drilldowns[j+1].RelevanceScore {
				drilldowns[j], drilldowns[j+1] = drilldowns[j+1], drilldowns[j]
			}
		}
	}
}
