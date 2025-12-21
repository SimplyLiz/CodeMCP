// Package tier provides analysis tier detection and capability gating.
// CKB operates in three tiers based on available backends:
//   - Fast: Tree-sitter only (immediate, no setup required)
//   - Standard: SCIP index available (precise cross-references)
//   - Full: SCIP + Telemetry (runtime usage data)
package tier

import "fmt"

// AnalysisTier represents the current analysis capability level.
type AnalysisTier int

const (
	// TierBasic provides tree-sitter based analysis without SCIP.
	// Available immediately with no setup required.
	// User-facing name: "fast"
	TierBasic AnalysisTier = iota

	// TierEnhanced adds SCIP-based precise cross-references.
	// Requires running a SCIP indexer.
	// User-facing name: "standard"
	TierEnhanced

	// TierFull adds telemetry and runtime usage data.
	// Requires OpenTelemetry integration.
	// User-facing name: "full"
	TierFull
)

// TierMode represents the requested tier mode from CLI/env/config.
type TierMode string

const (
	// TierModeAuto uses the highest available tier (default behavior)
	TierModeAuto TierMode = "auto"
	// TierModeFast uses tree-sitter only, ignoring SCIP if available
	TierModeFast TierMode = "fast"
	// TierModeStandard requires SCIP index
	TierModeStandard TierMode = "standard"
	// TierModeFull requires SCIP + telemetry
	TierModeFull TierMode = "full"
)

// String returns the tier name (user-facing).
func (t AnalysisTier) String() string {
	switch t {
	case TierBasic:
		return "Fast"
	case TierEnhanced:
		return "Standard"
	case TierFull:
		return "Full"
	default:
		return "Unknown"
	}
}

// ModeName returns the CLI mode name for the tier.
func (t AnalysisTier) ModeName() string {
	switch t {
	case TierBasic:
		return "fast"
	case TierEnhanced:
		return "standard"
	case TierFull:
		return "full"
	default:
		return "unknown"
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

// tierFromMode converts a TierMode to an AnalysisTier.
func tierFromMode(mode TierMode) AnalysisTier {
	switch mode {
	case TierModeFast:
		return TierBasic
	case TierModeStandard:
		return TierEnhanced
	case TierModeFull:
		return TierFull
	default:
		return TierBasic
	}
}

// tierRequirement returns a human-readable requirement for a tier.
func tierRequirement(tier AnalysisTier) string {
	switch tier {
	case TierEnhanced:
		return "SCIP index. Run 'ckb index' first."
	case TierFull:
		return "telemetry. See 'ckb help telemetry' for setup."
	default:
		return ""
	}
}

// ValidTierModes returns all valid tier mode strings.
func ValidTierModes() []string {
	return []string{"auto", "fast", "standard", "full"}
}

// ParseTierMode parses a string into a TierMode, returning an error for invalid values.
func ParseTierMode(s string) (TierMode, error) {
	switch s {
	case "", "auto":
		return TierModeAuto, nil
	case "fast":
		return TierModeFast, nil
	case "standard":
		return TierModeStandard, nil
	case "full":
		return TierModeFull, nil
	default:
		return TierModeAuto, fmt.Errorf("invalid tier '%s': must be one of: auto, fast, standard, full", s)
	}
}

// TierInfo contains detailed information about the current tier.
type TierInfo struct {
	Current          AnalysisTier `json:"current"`
	CurrentName      string       `json:"currentName"`
	Description      string       `json:"description"`
	Mode             string       `json:"mode"`                       // "auto" or explicit mode name
	Explicit         bool         `json:"explicit"`                   // true if tier was explicitly set
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
	requestedMode      TierMode // Explicitly requested tier mode
}

// NewDetector creates a new tier detector.
func NewDetector() *Detector {
	return &Detector{
		requestedMode: TierModeAuto,
	}
}

// SetRequestedMode sets the explicitly requested tier mode.
func (d *Detector) SetRequestedMode(mode TierMode) {
	d.requestedMode = mode
}

// GetRequestedMode returns the currently requested tier mode.
func (d *Detector) GetRequestedMode() TierMode {
	return d.requestedMode
}

// IsExplicitMode returns true if the tier was explicitly set (not auto).
func (d *Detector) IsExplicitMode() bool {
	return d.requestedMode != TierModeAuto
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

// ResolveTier resolves the effective tier based on requested mode and availability.
// If mode is auto, returns the highest available tier.
// If mode is explicit, validates that requirements are met and returns the requested tier.
// Returns an error if the requested tier's requirements are not met.
func (d *Detector) ResolveTier() (AnalysisTier, error) {
	if d.requestedMode == TierModeAuto {
		return d.DetectTier(), nil
	}

	available := d.DetectTier()
	requested := tierFromMode(d.requestedMode)

	// For "fast" mode, always allow (downgrade from available)
	if d.requestedMode == TierModeFast {
		return TierBasic, nil
	}

	// For other modes, check if requirements are met
	if requested > available {
		return available, fmt.Errorf("tier '%s' requires %s", d.requestedMode, tierRequirement(requested))
	}

	return requested, nil
}

// EffectiveTier returns the tier that will be used for operations.
// Unlike ResolveTier, this doesn't return an error - it falls back to available tier.
func (d *Detector) EffectiveTier() AnalysisTier {
	tier, err := d.ResolveTier()
	if err != nil {
		return d.DetectTier()
	}
	return tier
}

// GetTierInfo returns detailed information about the current tier.
func (d *Detector) GetTierInfo() TierInfo {
	// Use effective tier (respects explicit mode)
	current := d.EffectiveTier()

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

	// Generate upgrade hint (only if not in explicit mode or at max tier)
	var hint string
	if !d.IsExplicitMode() {
		switch current {
		case TierBasic:
			hint = "Run 'ckb index' to unlock findReferences, getCallGraph, and analyzeImpact"
		case TierEnhanced:
			hint = "Configure OpenTelemetry for runtime usage insights"
		}
	}

	// Determine mode string
	var modeStr string
	if d.requestedMode == TierModeAuto {
		modeStr = "auto-detected"
	} else {
		modeStr = string(d.requestedMode)
	}

	return TierInfo{
		Current:          current,
		CurrentName:      current.String(),
		Description:      current.Description(),
		Mode:             modeStr,
		Explicit:         d.IsExplicitMode(),
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
