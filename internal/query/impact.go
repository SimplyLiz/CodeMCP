package query

import (
	"context"
	"fmt"
	"sort"
	"time"

	"ckb/internal/backends"
	"ckb/internal/compression"
	"ckb/internal/errors"
	"ckb/internal/impact"
	"ckb/internal/output"
)

// AnalyzeImpactOptions contains options for analyzeImpact.
type AnalyzeImpactOptions struct {
	SymbolId     string
	Depth        int
	IncludeTests bool
}

// AnalyzeImpactResponse is the response for analyzeImpact.
type AnalyzeImpactResponse struct {
	Symbol           *SymbolInfo        `json:"symbol"`
	Visibility       *VisibilityInfo    `json:"visibility"`
	RiskScore        *RiskScore         `json:"riskScore"`
	DirectImpact     []ImpactItem       `json:"directImpact"`
	TransitiveImpact []ImpactItem       `json:"transitiveImpact,omitempty"`
	ModulesAffected  []ModuleImpact     `json:"modulesAffected"`
	Truncated        bool               `json:"truncated,omitempty"`
	TruncationInfo   *TruncationInfo    `json:"truncationInfo,omitempty"`
	Provenance       *Provenance        `json:"provenance"`
	Drilldowns       []output.Drilldown `json:"drilldowns,omitempty"`
}

// RiskScore describes the risk of changing a symbol.
type RiskScore struct {
	Level       string       `json:"level"` // high, medium, low
	Score       float64      `json:"score"`
	Explanation string       `json:"explanation"`
	Factors     []RiskFactor `json:"factors"`
}

// RiskFactor describes a factor in the risk score.
type RiskFactor struct {
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Weight float64 `json:"weight"`
}

// ImpactItem describes an impact from changing a symbol.
type ImpactItem struct {
	StableId   string          `json:"stableId"`
	Name       string          `json:"name,omitempty"`
	Kind       string          `json:"kind"` // direct-caller, transitive-caller, type-dependency, test-dependency
	Distance   int             `json:"distance"`
	ModuleId   string          `json:"moduleId"`
	Location   *LocationInfo   `json:"location,omitempty"`
	Confidence float64         `json:"confidence"`
	Visibility *VisibilityInfo `json:"visibility,omitempty"`
}

// ModuleImpact describes the impact on a module.
type ModuleImpact struct {
	ModuleId      string `json:"moduleId"`
	Name          string `json:"name,omitempty"`
	ImpactCount   int    `json:"impactCount"`
	DirectCount   int    `json:"directCount"`
	BreakingCount int    `json:"breakingCount,omitempty"`
}

// AnalyzeImpact analyzes the impact of changing a symbol.
func (e *Engine) AnalyzeImpact(ctx context.Context, opts AnalyzeImpactOptions) (*AnalyzeImpactResponse, error) {
	startTime := time.Now()

	// Default options
	if opts.Depth <= 0 {
		opts.Depth = 2
	}

	// Get repo state (full mode for impact analysis)
	repoState, err := e.GetRepoState(ctx, "full")
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	// Resolve symbol ID
	resolved, err := e.resolver.ResolveSymbolId(opts.SymbolId)
	if err != nil || resolved.Symbol == nil {
		return nil, errors.NewCkbError(
			errors.SymbolNotFound,
			fmt.Sprintf("Symbol not found: %s", opts.SymbolId),
			nil, nil, nil,
		)
	}

	// Get symbol info from backend
	var symbolInfo *SymbolInfo
	var backendContribs []BackendContribution
	var completeness CompletenessInfo

	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		result, err := e.scipAdapter.GetSymbol(ctx, resolved.Symbol.StableId)
		if err == nil && result != nil {
			symbolInfo = &SymbolInfo{
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
			backendContribs = append(backendContribs, BackendContribution{
				BackendId:    "scip",
				Available:    true,
				Used:         true,
				Completeness: result.Completeness.Score,
			})
			completeness = CompletenessInfo{
				Score:  result.Completeness.Score,
				Reason: string(result.Completeness.Reason),
			}
		}
	}

	// Fallback to identity data
	if symbolInfo == nil && resolved.Symbol.Fingerprint != nil {
		symbolInfo = &SymbolInfo{
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
	}

	// Find references for impact analysis
	var refs []impact.Reference
	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		refOpts := backends.RefOptions{
			MaxResults:   500,
			IncludeTests: opts.IncludeTests,
		}
		refsResult, err := e.scipAdapter.FindReferences(ctx, resolved.Symbol.StableId, refOpts)
		if err == nil && refsResult != nil {
			for _, ref := range refsResult.References {
				impactRef := impact.Reference{
					Kind: impact.ReferenceKind(ref.Kind),
					Location: &impact.Location{
						FileId:    ref.Location.Path,
						StartLine: ref.Location.Line,
					},
				}
				refs = append(refs, impactRef)
			}
		}
	}

	// Filter test references if not included
	if !opts.IncludeTests {
		refs = filterTestReferences(refs)
	}

	// Create impact analyzer and run analysis
	analyzer := impact.NewImpactAnalyzer(opts.Depth)

	impactSymbol := &impact.Symbol{
		StableId: resolved.Symbol.StableId,
		Name:     symbolInfo.Name,
		Kind:     impact.SymbolKind(symbolInfo.Kind),
		ModuleId: symbolInfo.ModuleId,
	}
	if symbolInfo.Visibility != nil {
		impactSymbol.Modifiers = []string{symbolInfo.Visibility.Visibility}
	}

	result, err := analyzer.Analyze(impactSymbol, refs)
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	// Convert results
	directImpact := convertImpactItems(result.DirectImpact)
	transitiveImpact := convertImpactItems(result.TransitiveImpact)
	modulesAffected := convertModuleImpacts(result.ModulesAffected)

	// Apply budget
	budget := e.compressor.GetBudget()
	var truncationInfo *TruncationInfo
	totalItems := len(directImpact) + len(transitiveImpact)
	if totalItems > budget.MaxImpactItems {
		truncationInfo = &TruncationInfo{
			Reason:        "max-items",
			OriginalCount: totalItems,
			ReturnedCount: budget.MaxImpactItems,
		}
		// Truncate transitive first, then direct
		if len(directImpact) >= budget.MaxImpactItems {
			directImpact = directImpact[:budget.MaxImpactItems]
			transitiveImpact = nil
		} else {
			remaining := budget.MaxImpactItems - len(directImpact)
			if len(transitiveImpact) > remaining {
				transitiveImpact = transitiveImpact[:remaining]
			}
		}
	}

	// Sort by impact priority
	sortImpactItems(directImpact)
	sortImpactItems(transitiveImpact)

	// Sort modules by impact count
	sort.Slice(modulesAffected, func(i, j int) bool {
		return modulesAffected[i].ImpactCount > modulesAffected[j].ImpactCount
	})

	// Limit modules
	if len(modulesAffected) > budget.MaxModules {
		modulesAffected = modulesAffected[:budget.MaxModules]
	}

	// Convert visibility and risk score
	visibility := symbolInfo.Visibility
	riskScore := convertRiskScore(result.RiskScore)

	// Build provenance
	provenance := e.buildProvenance(repoState, "full", startTime, backendContribs, completeness)
	if result.AnalysisLimits != nil && result.AnalysisLimits.HasLimitations() {
		provenance.Warnings = append(provenance.Warnings, result.AnalysisLimits.Notes...)
	}

	// Generate drilldowns
	var compTrunc *compression.TruncationInfo
	if truncationInfo != nil {
		compTrunc = &compression.TruncationInfo{
			Reason:        compression.TruncMaxItems,
			OriginalCount: truncationInfo.OriginalCount,
			ReturnedCount: truncationInfo.ReturnedCount,
		}
	}

	var topModule *output.Module
	if len(modulesAffected) > 0 {
		topModule = &output.Module{
			ModuleId: modulesAffected[0].ModuleId,
			Name:     modulesAffected[0].Name,
		}
	}

	drilldowns := e.generateDrilldowns(compTrunc, completeness, opts.SymbolId, topModule)

	return &AnalyzeImpactResponse{
		Symbol:           symbolInfo,
		Visibility:       visibility,
		RiskScore:        riskScore,
		DirectImpact:     directImpact,
		TransitiveImpact: transitiveImpact,
		ModulesAffected:  modulesAffected,
		Truncated:        truncationInfo != nil,
		TruncationInfo:   truncationInfo,
		Provenance:       provenance,
		Drilldowns:       drilldowns,
	}, nil
}

// filterTestReferences removes test references.
func filterTestReferences(refs []impact.Reference) []impact.Reference {
	filtered := make([]impact.Reference, 0, len(refs))
	for _, ref := range refs {
		if !ref.IsTest {
			filtered = append(filtered, ref)
		}
	}
	return filtered
}

// convertImpactItems converts impact items to response format.
func convertImpactItems(items []impact.ImpactItem) []ImpactItem {
	result := make([]ImpactItem, 0, len(items))

	for _, item := range items {
		ri := ImpactItem{
			StableId:   item.StableId,
			Name:       item.Name,
			Kind:       string(item.Kind),
			Distance:   item.Distance,
			ModuleId:   item.ModuleId,
			Confidence: item.Confidence,
		}
		if item.Location != nil {
			ri.Location = &LocationInfo{
				FileId:      item.Location.FileId,
				StartLine:   item.Location.StartLine,
				StartColumn: item.Location.StartColumn,
			}
		}
		if item.Visibility != nil {
			ri.Visibility = &VisibilityInfo{
				Visibility: string(item.Visibility.Visibility),
				Confidence: item.Visibility.Confidence,
				Source:     item.Visibility.Source,
			}
		}
		result = append(result, ri)
	}

	return result
}

// convertModuleImpacts converts module impacts to response format.
func convertModuleImpacts(modules []impact.ModuleSummary) []ModuleImpact {
	result := make([]ModuleImpact, 0, len(modules))

	for _, m := range modules {
		result = append(result, ModuleImpact{
			ModuleId:    m.ModuleId,
			Name:        m.Name,
			ImpactCount: m.ImpactCount,
		})
	}

	return result
}

// convertRiskScore converts risk score to response format.
func convertRiskScore(r *impact.RiskScore) *RiskScore {
	if r == nil {
		return &RiskScore{
			Level:       "unknown",
			Score:       0,
			Explanation: "Unable to compute risk score",
		}
	}

	factors := make([]RiskFactor, 0, len(r.Factors))
	for _, f := range r.Factors {
		factors = append(factors, RiskFactor{
			Name:   f.Name,
			Value:  f.Value,
			Weight: f.Weight,
		})
	}

	return &RiskScore{
		Level:       string(r.Level),
		Score:       r.Score,
		Explanation: r.Explanation,
		Factors:     factors,
	}
}

// sortImpactItems sorts impact items by priority.
func sortImpactItems(items []ImpactItem) {
	kindPriority := map[string]int{
		"direct-caller":     1,
		"transitive-caller": 2,
		"type-dependency":   3,
		"test-dependency":   4,
		"unknown":           5,
	}

	sort.Slice(items, func(i, j int) bool {
		pi := kindPriority[items[i].Kind]
		pj := kindPriority[items[j].Kind]
		if pi != pj {
			return pi < pj
		}
		if items[i].Confidence != items[j].Confidence {
			return items[i].Confidence > items[j].Confidence
		}
		return items[i].StableId < items[j].StableId
	})
}
