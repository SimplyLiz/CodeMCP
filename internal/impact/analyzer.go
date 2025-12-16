package impact

import (
	"fmt"
	"sort"
)

// ImpactAnalyzer performs impact analysis on symbols
type ImpactAnalyzer struct {
	maxDepth int // Maximum depth for transitive analysis (default 2)
}

// NewImpactAnalyzer creates a new ImpactAnalyzer with the specified max depth
func NewImpactAnalyzer(maxDepth int) *ImpactAnalyzer {
	if maxDepth <= 0 {
		maxDepth = 2 // Default to 2 levels of transitive analysis
	}
	return &ImpactAnalyzer{
		maxDepth: maxDepth,
	}
}

// ImpactAnalysisResult contains the complete results of an impact analysis
type ImpactAnalysisResult struct {
	Symbol           *Symbol          // The analyzed symbol
	Visibility       *VisibilityInfo  // Visibility information
	RiskScore        *RiskScore       // Risk assessment
	DirectImpact     []ImpactItem     // Direct references (distance = 1)
	TransitiveImpact []ImpactItem     // Transitive references (distance > 1)
	ModulesAffected  []ModuleSummary  // Summary by module
	AnalysisLimits   *AnalysisLimits  // Limitations of the analysis
}

// ModuleSummary provides a summary of impact for a single module
type ModuleSummary struct {
	ModuleId    string    // Module identifier
	Name        string    // Module name
	ImpactCount int       // Number of impact items in this module
	MaxRisk     RiskLevel // Highest risk level in this module
}

// Analyze performs a complete impact analysis on the given symbol
func (a *ImpactAnalyzer) Analyze(symbol *Symbol, refs []Reference) (*ImpactAnalysisResult, error) {
	if symbol == nil {
		return nil, fmt.Errorf("symbol cannot be nil")
	}

	// Initialize result
	result := &ImpactAnalysisResult{
		Symbol:           symbol,
		DirectImpact:     make([]ImpactItem, 0),
		TransitiveImpact: make([]ImpactItem, 0),
		ModulesAffected:  make([]ModuleSummary, 0),
		AnalysisLimits:   NewAnalysisLimits(),
	}

	// Derive visibility
	result.Visibility = DeriveVisibility(symbol, refs)

	// Determine type context
	result.AnalysisLimits.TypeContext = DetermineTypeContext(symbol, refs)

	// Process direct references
	directImpact := a.processDirectReferences(symbol, refs, result.Visibility)
	result.DirectImpact = directImpact

	// Note: Transitive analysis would require additional reference data
	// For now, we only process direct references
	if a.maxDepth > 1 {
		result.AnalysisLimits.AddNote("Transitive impact analysis requires additional reference data")
	}

	// Combine all impact items for risk calculation
	allImpact := append([]ImpactItem{}, result.DirectImpact...)
	allImpact = append(allImpact, result.TransitiveImpact...)

	// Calculate risk score
	result.RiskScore = ComputeRiskScore(symbol, allImpact)

	// Generate module summaries
	result.ModulesAffected = a.generateModuleSummaries(allImpact)

	return result, nil
}

// processDirectReferences converts references into impact items
func (a *ImpactAnalyzer) processDirectReferences(symbol *Symbol, refs []Reference, symbolVisibility *VisibilityInfo) []ImpactItem {
	items := make([]ImpactItem, 0, len(refs))

	for _, ref := range refs {
		// Classify the impact
		kind, confidence := ClassifyImpactWithConfidence(&ref, symbol)

		// Create impact item
		item := ImpactItem{
			StableId:   ref.FromSymbol,
			Name:       extractNameFromStableId(ref.FromSymbol),
			Kind:       kind,
			Confidence: confidence,
			ModuleId:   ref.FromModule,
			ModuleName: extractModuleNameFromId(ref.FromModule),
			Location:   ref.Location,
			Visibility: symbolVisibility, // Use the same visibility as the symbol
			Distance:   1,                 // Direct reference
		}

		items = append(items, item)
	}

	// Sort by confidence (descending) and then by name
	sort.Slice(items, func(i, j int) bool {
		if items[i].Confidence != items[j].Confidence {
			return items[i].Confidence > items[j].Confidence
		}
		return items[i].Name < items[j].Name
	})

	return items
}

// generateModuleSummaries creates a summary of impacts grouped by module
func (a *ImpactAnalyzer) generateModuleSummaries(allImpact []ImpactItem) []ModuleSummary {
	moduleMap := make(map[string]*ModuleSummary)

	// Aggregate by module
	for _, item := range allImpact {
		if item.ModuleId == "" {
			continue
		}

		summary, exists := moduleMap[item.ModuleId]
		if !exists {
			summary = &ModuleSummary{
				ModuleId:    item.ModuleId,
				Name:        item.ModuleName,
				ImpactCount: 0,
				MaxRisk:     RiskLow,
			}
			moduleMap[item.ModuleId] = summary
		}

		summary.ImpactCount++

		// Update max risk (this is simplified - in a real implementation,
		// we'd calculate per-item risk)
		// For now, use a heuristic based on impact kind
		itemRisk := estimateItemRisk(item)
		if isHigherRisk(itemRisk, summary.MaxRisk) {
			summary.MaxRisk = itemRisk
		}
	}

	// Convert map to slice
	summaries := make([]ModuleSummary, 0, len(moduleMap))
	for _, summary := range moduleMap {
		summaries = append(summaries, *summary)
	}

	// Sort by impact count (descending) and then by name
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].ImpactCount != summaries[j].ImpactCount {
			return summaries[i].ImpactCount > summaries[j].ImpactCount
		}
		return summaries[i].Name < summaries[j].Name
	})

	return summaries
}

// extractNameFromStableId extracts a readable name from a stable identifier
func extractNameFromStableId(stableId string) string {
	// This is a simplified implementation
	// In a real system, this would parse the stable ID format
	if stableId == "" {
		return "unknown"
	}
	return stableId
}

// extractModuleNameFromId extracts a readable module name from a module ID
func extractModuleNameFromId(moduleId string) string {
	// This is a simplified implementation
	// In a real system, this would parse the module ID format
	if moduleId == "" {
		return "unknown"
	}
	return moduleId
}

// estimateItemRisk estimates the risk level of an individual impact item
func estimateItemRisk(item ImpactItem) RiskLevel {
	// High risk: public direct callers, interface implementations
	if item.Visibility != nil && item.Visibility.Visibility == VisibilityPublic {
		if item.Kind == DirectCaller || item.Kind == ImplementsInterface {
			return RiskHigh
		}
	}

	// Medium risk: internal direct callers, transitive callers
	if item.Kind == DirectCaller || item.Kind == TransitiveCaller {
		return RiskMedium
	}

	// Low risk: type dependencies, test dependencies
	return RiskLow
}

// isHigherRisk compares two risk levels
func isHigherRisk(a, b RiskLevel) bool {
	riskOrder := map[RiskLevel]int{
		RiskLow:    1,
		RiskMedium: 2,
		RiskHigh:   3,
	}
	return riskOrder[a] > riskOrder[b]
}

// AnalyzeWithOptions performs impact analysis with custom options
type AnalyzeOptions struct {
	MaxDepth             int  // Override analyzer's default max depth
	IncludeTests         bool // Include test dependencies in analysis
	OnlyBreakingChanges  bool // Only include potentially breaking changes
}

// AnalyzeWithOptions performs analysis with custom options
func (a *ImpactAnalyzer) AnalyzeWithOptions(symbol *Symbol, refs []Reference, opts AnalyzeOptions) (*ImpactAnalysisResult, error) {
	// Apply options
	originalMaxDepth := a.maxDepth
	if opts.MaxDepth > 0 {
		a.maxDepth = opts.MaxDepth
	}
	defer func() { a.maxDepth = originalMaxDepth }()

	// Filter references based on options
	filteredRefs := refs
	if !opts.IncludeTests {
		filteredRefs = make([]Reference, 0)
		for _, ref := range refs {
			if !ref.IsTest {
				filteredRefs = append(filteredRefs, ref)
			}
		}
	}

	// Perform standard analysis
	return a.Analyze(symbol, filteredRefs)
}
