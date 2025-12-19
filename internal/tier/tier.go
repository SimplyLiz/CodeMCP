// Package tier provides analysis tier detection and capability gating.
// CKB operates in three tiers based on available backends:
//   - Basic: Tree-sitter only (immediate, no setup required)
//   - Enhanced: SCIP index available (precise cross-references)
//   - Full: SCIP + Telemetry (runtime usage data)
package tier

// AnalysisTier represents the current analysis capability level.
type AnalysisTier int

const (
	// TierBasic provides tree-sitter based analysis without SCIP.
	// Available immediately with no setup required.
	TierBasic AnalysisTier = iota

	// TierEnhanced adds SCIP-based precise cross-references.
	// Requires running a SCIP indexer.
	TierEnhanced

	// TierFull adds telemetry and runtime usage data.
	// Requires OpenTelemetry integration.
	TierFull
)

// String returns the tier name.
func (t AnalysisTier) String() string {
	switch t {
	case TierBasic:
		return "Basic"
	case TierEnhanced:
		return "Enhanced"
	case TierFull:
		return "Full"
	default:
		return "Unknown"
	}
}

// Description returns a human-readable description of the tier.
func (t AnalysisTier) Description() string {
	switch t {
	case TierBasic:
		return "tree-sitter"
	case TierEnhanced:
		return "SCIP index"
	case TierFull:
		return "SCIP + Telemetry"
	default:
		return "unknown"
	}
}

// TierInfo contains detailed information about the current tier.
type TierInfo struct {
	Current          AnalysisTier `json:"current"`
	CurrentName      string       `json:"currentName"`
	Description      string       `json:"description"`
	AvailableTools   []string     `json:"availableTools"`
	UnavailableTools []string     `json:"unavailableTools"`
	UpgradeHint      string       `json:"upgradeHint,omitempty"`
}

// ToolRequirement defines what tier a tool requires.
type ToolRequirement struct {
	Name        string       `json:"name"`
	MinimumTier AnalysisTier `json:"minimumTier"`
	Fallback    bool         `json:"fallback"` // Has degraded mode at lower tier
}

// AllTools returns the complete list of tools with their tier requirements.
func AllTools() []ToolRequirement {
	return []ToolRequirement{
		// Basic tier tools (always available)
		{Name: "getStatus", MinimumTier: TierBasic, Fallback: false},
		{Name: "doctor", MinimumTier: TierBasic, Fallback: false},
		{Name: "searchSymbols", MinimumTier: TierBasic, Fallback: true},
		{Name: "getArchitecture", MinimumTier: TierBasic, Fallback: true},
		{Name: "getModuleOverview", MinimumTier: TierBasic, Fallback: true},
		{Name: "getOwnership", MinimumTier: TierBasic, Fallback: false},
		{Name: "explainFile", MinimumTier: TierBasic, Fallback: false},
		{Name: "explainPath", MinimumTier: TierBasic, Fallback: false},
		{Name: "getHotspots", MinimumTier: TierBasic, Fallback: false},
		{Name: "getCoupling", MinimumTier: TierBasic, Fallback: false},
		{Name: "getFileComplexity", MinimumTier: TierBasic, Fallback: false},
		{Name: "listEntrypoints", MinimumTier: TierBasic, Fallback: false},

		// Enhanced tier tools (require SCIP)
		{Name: "getSymbol", MinimumTier: TierEnhanced, Fallback: false},
		{Name: "findReferences", MinimumTier: TierEnhanced, Fallback: false},
		{Name: "getCallGraph", MinimumTier: TierEnhanced, Fallback: false},
		{Name: "analyzeImpact", MinimumTier: TierEnhanced, Fallback: false},
		{Name: "traceUsage", MinimumTier: TierEnhanced, Fallback: false},
		{Name: "explainSymbol", MinimumTier: TierEnhanced, Fallback: false},
		{Name: "justifySymbol", MinimumTier: TierEnhanced, Fallback: false},
		{Name: "getTransitiveDeps", MinimumTier: TierEnhanced, Fallback: false},
		{Name: "getContracts", MinimumTier: TierEnhanced, Fallback: false},
		{Name: "checkContractCompliance", MinimumTier: TierEnhanced, Fallback: false},

		// Full tier tools (require telemetry)
		{Name: "getObservedUsage", MinimumTier: TierFull, Fallback: false},
		{Name: "getTelemetry", MinimumTier: TierFull, Fallback: false},
	}
}

// GetToolsForTier returns tools available at the given tier.
func GetToolsForTier(tier AnalysisTier) []ToolRequirement {
	var available []ToolRequirement
	for _, tool := range AllTools() {
		if tool.MinimumTier <= tier || tool.Fallback {
			available = append(available, tool)
		}
	}
	return available
}

// GetUnavailableTools returns tools that require a higher tier.
func GetUnavailableTools(tier AnalysisTier) []ToolRequirement {
	var unavailable []ToolRequirement
	for _, tool := range AllTools() {
		if tool.MinimumTier > tier && !tool.Fallback {
			unavailable = append(unavailable, tool)
		}
	}
	return unavailable
}

// Detector detects the current analysis tier based on backend availability.
type Detector struct {
	scipAvailable      bool
	telemetryAvailable bool
}

// NewDetector creates a new tier detector.
func NewDetector() *Detector {
	return &Detector{}
}

// SetScipAvailable sets whether SCIP is available.
func (d *Detector) SetScipAvailable(available bool) {
	d.scipAvailable = available
}

// SetTelemetryAvailable sets whether telemetry is available.
func (d *Detector) SetTelemetryAvailable(available bool) {
	d.telemetryAvailable = available
}

// DetectTier returns the current analysis tier based on backend availability.
func (d *Detector) DetectTier() AnalysisTier {
	if d.telemetryAvailable && d.scipAvailable {
		return TierFull
	}
	if d.scipAvailable {
		return TierEnhanced
	}
	return TierBasic
}

// GetTierInfo returns detailed information about the current tier.
func (d *Detector) GetTierInfo() TierInfo {
	current := d.DetectTier()

	// Get available and unavailable tools
	availableReqs := GetToolsForTier(current)
	unavailableReqs := GetUnavailableTools(current)

	available := make([]string, 0, len(availableReqs))
	for _, req := range availableReqs {
		available = append(available, req.Name)
	}

	unavailable := make([]string, 0, len(unavailableReqs))
	for _, req := range unavailableReqs {
		unavailable = append(unavailable, req.Name)
	}

	// Generate upgrade hint
	var hint string
	switch current {
	case TierBasic:
		hint = "Run 'ckb index' to unlock findReferences, getCallGraph, and analyzeImpact"
	case TierEnhanced:
		hint = "Configure OpenTelemetry for runtime usage insights"
	}

	return TierInfo{
		Current:          current,
		CurrentName:      current.String(),
		Description:      current.Description(),
		AvailableTools:   available,
		UnavailableTools: unavailable,
		UpgradeHint:      hint,
	}
}

// IsToolAvailable checks if a tool is available at the current tier.
func (d *Detector) IsToolAvailable(toolName string) bool {
	current := d.DetectTier()
	for _, tool := range AllTools() {
		if tool.Name == toolName {
			return tool.MinimumTier <= current || tool.Fallback
		}
	}
	// Unknown tools are allowed (future-proofing)
	return true
}

// GetToolTierMessage returns a message if a tool is running in degraded mode.
// Returns empty string if tool is running at full capability.
func (d *Detector) GetToolTierMessage(toolName string) string {
	current := d.DetectTier()
	for _, tool := range AllTools() {
		if tool.Name == toolName {
			if tool.Fallback && tool.MinimumTier > current {
				return "Running in degraded mode (tree-sitter fallback). Run 'ckb index' for precise results."
			}
			if tool.MinimumTier > current {
				return "This tool requires " + tool.MinimumTier.Description() + ". Run 'ckb index' to unlock."
			}
		}
	}
	return ""
}
