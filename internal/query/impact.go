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
	"ckb/internal/telemetry"
)

// AnalyzeImpactOptions contains options for analyzeImpact.
type AnalyzeImpactOptions struct {
	SymbolId            string
	Depth               int
	IncludeTests        bool
	IncludeTelemetry    bool   // Include observed telemetry data
	TelemetryPeriod     string // Time period for telemetry ("7d", "30d", "90d")
}

// AnalyzeImpactResponse is the response for analyzeImpact.
type AnalyzeImpactResponse struct {
	Symbol            *SymbolInfo          `json:"symbol"`
	Visibility        *VisibilityInfo      `json:"visibility"`
	RiskScore         *RiskScore           `json:"riskScore"`
	DirectImpact      []ImpactItem         `json:"directImpact"`
	TransitiveImpact  []ImpactItem         `json:"transitiveImpact,omitempty"`
	ModulesAffected   []ModuleImpact       `json:"modulesAffected"`
	ObservedUsage     *ObservedUsageSummary `json:"observedUsage,omitempty"`
	BlendedConfidence float64              `json:"blendedConfidence,omitempty"`
	Truncated         bool                 `json:"truncated,omitempty"`
	TruncationInfo    *TruncationInfo      `json:"truncationInfo,omitempty"`
	Provenance        *Provenance          `json:"provenance"`
	Drilldowns        []output.Drilldown   `json:"drilldowns,omitempty"`
}

// ObservedUsageSummary contains telemetry-based usage information
type ObservedUsageSummary struct {
	HasTelemetry      bool    `json:"hasTelemetry"`
	TotalCalls        int64   `json:"totalCalls,omitempty"`
	LastObserved      string  `json:"lastObserved,omitempty"`
	MatchQuality      string  `json:"matchQuality,omitempty"`
	ObservedConfidence float64 `json:"observedConfidence,omitempty"`
	Trend             string  `json:"trend,omitempty"`
	CallerServices    []string `json:"callerServices,omitempty"`
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

	// Resolve symbol ID - try resolver first, then fall back to SCIP directly
	resolved, _ := e.resolver.ResolveSymbolId(opts.SymbolId)

	// Get symbol info from backend
	var symbolInfo *SymbolInfo
	var backendContribs []BackendContribution
	var completeness CompletenessInfo

	// Determine the symbol ID to use for SCIP lookup
	symbolIdForLookup := opts.SymbolId
	if resolved != nil && resolved.Symbol != nil {
		symbolIdForLookup = resolved.Symbol.StableId
	}

	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		result, symbolErr := e.scipAdapter.GetSymbol(ctx, symbolIdForLookup)
		if symbolErr == nil && result != nil {
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
	if symbolInfo == nil && resolved != nil && resolved.Symbol != nil && resolved.Symbol.Fingerprint != nil {
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

	// If we still don't have symbol info, return not found
	if symbolInfo == nil {
		return nil, errors.NewCkbError(
			errors.SymbolNotFound,
			fmt.Sprintf("Symbol not found: %s", opts.SymbolId),
			nil, nil, nil,
		)
	}

	// Find references for impact analysis
	var refs []impact.Reference
	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		refOpts := backends.RefOptions{
			MaxResults:   500,
			IncludeTests: opts.IncludeTests,
		}
		refsResult, refsErr := e.scipAdapter.FindReferences(ctx, symbolIdForLookup, refOpts)
		if refsErr == nil && refsResult != nil {
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
		StableId: symbolInfo.StableId,
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

	// Get telemetry data if enabled
	var observedUsage *ObservedUsageSummary
	var blendedConfidence float64

	if opts.IncludeTelemetry && e.config != nil && e.config.Telemetry.Enabled && e.db != nil {
		observedUsage, blendedConfidence = e.getObservedUsageForImpact(symbolIdForLookup, opts.TelemetryPeriod)

		// Add telemetry factors to risk score if we have observed data
		if observedUsage != nil && observedUsage.HasTelemetry && riskScore != nil {
			riskScore = e.enhanceRiskScoreWithTelemetry(riskScore, observedUsage)
		}
	} else {
		// Static-only confidence
		blendedConfidence = completeness.Score * 0.79
	}

	return &AnalyzeImpactResponse{
		Symbol:            symbolInfo,
		Visibility:        visibility,
		RiskScore:         riskScore,
		DirectImpact:      directImpact,
		TransitiveImpact:  transitiveImpact,
		ModulesAffected:   modulesAffected,
		ObservedUsage:     observedUsage,
		BlendedConfidence: blendedConfidence,
		Truncated:         truncationInfo != nil,
		TruncationInfo:    truncationInfo,
		Provenance:        provenance,
		Drilldowns:        drilldowns,
	}, nil
}

// getObservedUsageForImpact fetches telemetry data for impact analysis
func (e *Engine) getObservedUsageForImpact(symbolID string, period string) (*ObservedUsageSummary, float64) {
	storage := telemetry.NewStorage(e.db.Conn())

	// Compute period filter
	periodFilter := computeTelemetryPeriodFilter(period)

	// Get usage data
	usages, err := storage.GetObservedUsage(symbolID, periodFilter)
	if err != nil || len(usages) == 0 {
		return &ObservedUsageSummary{HasTelemetry: false}, 0.79 // Static-only confidence
	}

	// Calculate totals
	var totalCalls int64
	var lastObserved time.Time
	var matchQuality telemetry.MatchQuality

	for i, u := range usages {
		totalCalls += u.CallCount
		if i == 0 {
			lastObserved = u.IngestedAt
			matchQuality = u.MatchQuality
		}
	}

	// Get callers
	var callerServices []string
	if e.config.Telemetry.Aggregation.StoreCallers {
		callers, err := storage.GetObservedCallers(symbolID, 5)
		if err == nil {
			for _, c := range callers {
				callerServices = append(callerServices, c.CallerService)
			}
		}
	}

	// Compute trend
	trend := computeUsageTrend(usages)

	// Build summary
	summary := &ObservedUsageSummary{
		HasTelemetry:       true,
		TotalCalls:         totalCalls,
		LastObserved:       lastObserved.Format(time.RFC3339),
		MatchQuality:       string(matchQuality),
		ObservedConfidence: matchQuality.Confidence(),
		Trend:              trend,
		CallerServices:     callerServices,
	}

	// Compute blended confidence
	staticConfidence := 0.79
	observedConfidence := matchQuality.Confidence()
	blendedConfidence := computeBlendedConfidenceScore(staticConfidence, observedConfidence)

	return summary, blendedConfidence
}

// enhanceRiskScoreWithTelemetry adds telemetry factors to risk assessment
func (e *Engine) enhanceRiskScoreWithTelemetry(riskScore *RiskScore, usage *ObservedUsageSummary) *RiskScore {
	// Copy existing factors
	enhanced := &RiskScore{
		Level:       riskScore.Level,
		Score:       riskScore.Score,
		Explanation: riskScore.Explanation,
		Factors:     make([]RiskFactor, len(riskScore.Factors)),
	}
	copy(enhanced.Factors, riskScore.Factors)

	// Add observed usage factor
	usageFactor := RiskFactor{
		Name:   "observed_usage",
		Weight: 0.2,
	}

	if usage.TotalCalls == 0 {
		usageFactor.Value = 0.0 // No observed usage - lower risk
	} else if usage.TotalCalls < 100 {
		usageFactor.Value = 0.3 // Low usage
	} else if usage.TotalCalls < 1000 {
		usageFactor.Value = 0.6 // Medium usage
	} else {
		usageFactor.Value = 1.0 // High usage - higher risk
	}
	enhanced.Factors = append(enhanced.Factors, usageFactor)

	// Add caller diversity factor
	if len(usage.CallerServices) > 0 {
		diversityFactor := RiskFactor{
			Name:   "caller_diversity",
			Weight: 0.15,
		}
		if len(usage.CallerServices) >= 5 {
			diversityFactor.Value = 1.0 // Many callers - higher risk
		} else if len(usage.CallerServices) >= 3 {
			diversityFactor.Value = 0.6
		} else {
			diversityFactor.Value = 0.3
		}
		enhanced.Factors = append(enhanced.Factors, diversityFactor)
	}

	// Add trend factor
	if usage.Trend != "" {
		trendFactor := RiskFactor{
			Name:   "usage_trend",
			Weight: 0.1,
		}
		switch usage.Trend {
		case "increasing":
			trendFactor.Value = 0.8 // Growing usage - higher risk
		case "decreasing":
			trendFactor.Value = 0.2 // Declining usage - lower risk
		default:
			trendFactor.Value = 0.5 // Stable
		}
		enhanced.Factors = append(enhanced.Factors, trendFactor)
	}

	// Recalculate score with new factors
	var totalWeight, weightedSum float64
	for _, f := range enhanced.Factors {
		totalWeight += f.Weight
		weightedSum += f.Value * f.Weight
	}

	if totalWeight > 0 {
		enhanced.Score = weightedSum / totalWeight
	}

	// Update level
	if enhanced.Score >= 0.7 {
		enhanced.Level = "high"
	} else if enhanced.Score >= 0.4 {
		enhanced.Level = "medium"
	} else {
		enhanced.Level = "low"
	}

	// Update explanation with telemetry info
	if usage.TotalCalls > 0 {
		enhanced.Explanation = fmt.Sprintf("%s Observed %d calls from %d services.",
			enhanced.Explanation, usage.TotalCalls, len(usage.CallerServices))
	} else {
		enhanced.Explanation = fmt.Sprintf("%s No runtime calls observed in telemetry window.",
			enhanced.Explanation)
	}

	return enhanced
}

// computeTelemetryPeriodFilter converts period string to date filter
func computeTelemetryPeriodFilter(period string) string {
	now := time.Now()
	switch period {
	case "7d":
		return now.AddDate(0, 0, -7).Format("2006-01-02")
	case "30d":
		return now.AddDate(0, 0, -30).Format("2006-01-02")
	case "90d":
		return now.AddDate(0, 0, -90).Format("2006-01-02")
	case "all":
		return ""
	default:
		return now.AddDate(0, 0, -90).Format("2006-01-02")
	}
}

// computeUsageTrend calculates the usage trend from observed data
func computeUsageTrend(usages []telemetry.ObservedUsage) string {
	if len(usages) < 2 {
		return "stable"
	}

	mid := len(usages) / 2
	var recentCalls, olderCalls int64
	for i, u := range usages {
		if i < mid {
			recentCalls += u.CallCount
		} else {
			olderCalls += u.CallCount
		}
	}

	if olderCalls == 0 {
		if recentCalls > 0 {
			return "increasing"
		}
		return "stable"
	}

	ratio := float64(recentCalls) / float64(olderCalls)
	if ratio > 1.2 {
		return "increasing"
	} else if ratio < 0.8 {
		return "decreasing"
	}
	return "stable"
}

// computeBlendedConfidenceScore combines static and observed confidence
func computeBlendedConfidenceScore(staticConfidence, observedConfidence float64) float64 {
	// Take higher, with small boost if both agree
	base := staticConfidence
	if observedConfidence > base {
		base = observedConfidence
	}

	agreementBoost := 0.0
	if staticConfidence > 0.5 && observedConfidence > 0.5 {
		agreementBoost = 0.03
	}

	result := base + agreementBoost
	if result > 1.0 {
		result = 1.0
	}
	return result
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
