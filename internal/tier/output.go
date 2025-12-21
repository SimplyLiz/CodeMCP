package tier

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// OutputFormat represents the output format.
type OutputFormat string

const (
	FormatHuman OutputFormat = "human"
	FormatJSON  OutputFormat = "json"
)

// TierSummaryOutput renders a tier summary to the writer.
func TierSummaryOutput(w io.Writer, result ValidationResult, format OutputFormat) error {
	switch format {
	case FormatJSON:
		return tierSummaryJSON(w, result)
	default:
		return tierSummaryHuman(w, result)
	}
}

func tierSummaryHuman(w io.Writer, result ValidationResult) error {
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Tier Summary")
	_, _ = fmt.Fprintln(w, strings.Repeat("=", 60))
	_, _ = fmt.Fprintln(w, "")

	// Header
	_, _ = fmt.Fprintf(w, "%-12s %-10s %-10s %s\n", "Language", "Effective", "Requested", "Status")
	_, _ = fmt.Fprintln(w, strings.Repeat("-", 60))

	// Sort languages for deterministic output
	validations := SortedLanguageValidations(result)

	for _, validation := range validations {
		effectiveTier := GetEffectiveTier(validation)
		statusStr := formatValidationStatus(validation)

		_, _ = fmt.Fprintf(w, "%-12s %-10s %-10s %s\n",
			validation.DisplayName,
			tierDisplayName(effectiveTier),
			tierDisplayName(validation.RequestedTier),
			statusStr,
		)
	}

	_, _ = fmt.Fprintln(w, "")

	// Capability matrix
	_, _ = fmt.Fprintln(w, "Capabilities")
	_, _ = fmt.Fprintln(w, strings.Repeat("-", 60))

	// Header row
	caps := AllCapabilities()
	capHeaders := make([]string, len(caps))
	for i, cap := range caps {
		capHeaders[i] = truncate(string(cap), 6)
	}
	_, _ = fmt.Fprintf(w, "%-12s %s\n", "", strings.Join(capHeaders, "  "))

	// Capability rows
	for _, validation := range validations {
		capValues := make([]string, len(caps))
		for i, cap := range caps {
			if validation.Capabilities[string(cap)] {
				capValues[i] = "  Y   "
			} else {
				capValues[i] = "  -   "
			}
		}
		_, _ = fmt.Fprintf(w, "%-12s %s\n", validation.DisplayName, strings.Join(capValues, ""))
	}

	_, _ = fmt.Fprintln(w, "")

	// Show missing tools if any
	hasMissing := false
	for _, validation := range validations {
		if len(validation.Missing) > 0 {
			hasMissing = true
			break
		}
	}

	if hasMissing {
		_, _ = fmt.Fprintln(w, "Missing Tools")
		_, _ = fmt.Fprintln(w, strings.Repeat("-", 60))

		for _, validation := range validations {
			if len(validation.Missing) > 0 {
				_, _ = fmt.Fprintf(w, "%s:\n", validation.DisplayName)
				for _, missing := range SortedTools(validation.Missing) {
					_, _ = fmt.Fprintf(w, "  - %s\n", missing.Name)
					if missing.InstallCmd != "" {
						_, _ = fmt.Fprintf(w, "    Install: %s\n", missing.InstallCmd)
					}
				}
			}
		}
		_, _ = fmt.Fprintln(w, "")
	}

	// Footer hint
	if result.Degraded {
		_, _ = fmt.Fprintln(w, "Run 'ckb doctor --tier <tier>' for detailed diagnostics.")
	}

	return nil
}

func formatValidationStatus(validation LanguageValidation) string {
	if validation.Satisfied {
		// Show which tools are providing the tier
		var providers []string
		for _, tool := range validation.Tools {
			if tool.Found {
				providers = append(providers, tool.Name)
			}
		}
		if len(providers) > 0 {
			return fmt.Sprintf("Y (%s)", strings.Join(providers, ", "))
		}
		return "Y"
	}

	// Show what's missing
	var missing []string
	for _, m := range validation.Missing {
		missing = append(missing, m.Name)
	}
	if len(missing) > 0 {
		return fmt.Sprintf("N (missing: %s)", strings.Join(missing, ", "))
	}
	return "N"
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s + strings.Repeat(" ", maxLen-len(s))
	}
	return s[:maxLen]
}

// capitalizeFirst capitalizes the first letter of a string.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func tierSummaryJSON(w io.Writer, result ValidationResult) error {
	// Create a JSON-friendly structure
	output := struct {
		AllSatisfied bool                           `json:"allSatisfied"`
		Degraded     bool                           `json:"degraded"`
		Errors       []string                       `json:"errors,omitempty"`
		Warnings     []string                       `json:"warnings,omitempty"`
		Languages    []LanguageValidationJSONOutput `json:"languages"`
	}{
		AllSatisfied: result.AllSatisfied,
		Degraded:     result.Degraded,
		Errors:       result.Errors,
		Warnings:     result.Warnings,
	}

	// Convert to sorted slice
	for _, validation := range SortedLanguageValidations(result) {
		output.Languages = append(output.Languages, toJSONOutput(validation))
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

// LanguageValidationJSONOutput is the JSON-friendly output format.
type LanguageValidationJSONOutput struct {
	Language      string               `json:"language"`
	DisplayName   string               `json:"displayName"`
	RequestedTier string               `json:"requestedTier"`
	EffectiveTier string               `json:"effectiveTier"`
	ToolTier      string               `json:"toolTier"`
	RuntimeTier   string               `json:"runtimeTier"`
	Satisfied     bool                 `json:"satisfied"`
	Tools         []ToolStatus         `json:"tools,omitempty"`
	Missing       []MissingToolOutput  `json:"missing,omitempty"`
	Prerequisites []PrerequisiteStatus `json:"prerequisites,omitempty"`
	Capabilities  map[string]bool      `json:"capabilities"`
}

// MissingToolOutput is the JSON output for a missing tool.
type MissingToolOutput struct {
	Name       string `json:"name"`
	InstallCmd string `json:"install"`
}

func toJSONOutput(v LanguageValidation) LanguageValidationJSONOutput {
	output := LanguageValidationJSONOutput{
		Language:      string(v.Language),
		DisplayName:   v.DisplayName,
		RequestedTier: tierDisplayName(v.RequestedTier),
		EffectiveTier: tierDisplayName(GetEffectiveTier(v)),
		ToolTier:      tierDisplayName(v.ToolTier),
		RuntimeTier:   tierDisplayName(v.RuntimeTier),
		Satisfied:     v.Satisfied,
		Tools:         SortedTools(v.Tools),
		Prerequisites: v.Prerequisites,
		Capabilities:  v.Capabilities,
	}

	for _, m := range SortedTools(v.Missing) {
		output.Missing = append(output.Missing, MissingToolOutput{
			Name:       m.Name,
			InstallCmd: m.InstallCmd,
		})
	}

	return output
}

// DoctorOutput renders doctor diagnostics to the writer.
func DoctorOutput(w io.Writer, result ValidationResult, requestedTier AnalysisTier, format OutputFormat) error {
	switch format {
	case FormatJSON:
		return doctorJSON(w, result, requestedTier)
	default:
		return doctorHuman(w, result, requestedTier)
	}
}

func doctorHuman(w io.Writer, result ValidationResult, requestedTier AnalysisTier) error {
	tierName := tierDisplayName(requestedTier)
	_, _ = fmt.Fprintf(w, "CKB Doctor - %s Tier Requirements\n", capitalizeFirst(tierName))
	_, _ = fmt.Fprintln(w, strings.Repeat("=", 45))
	_, _ = fmt.Fprintln(w, "")

	readyCount := 0
	totalCount := len(result.Languages)

	validations := SortedLanguageValidations(result)

	for _, validation := range validations {
		ready := validation.ToolTier >= requestedTier

		if ready {
			readyCount++
			_, _ = fmt.Fprintf(w, "%s: Y Ready\n", validation.DisplayName)
		} else {
			_, _ = fmt.Fprintf(w, "%s: N Not Ready\n", validation.DisplayName)
		}

		// Filter tools based on requested tier
		relevantTools := filterToolsForTier(validation, requestedTier)

		// Show found tools
		for _, tool := range SortedTools(relevantTools.found) {
			versionInfo := ""
			if tool.Version != "" {
				versionInfo = fmt.Sprintf(" v%s", tool.Version)
			}
			_, _ = fmt.Fprintf(w, "  Y %s%s\n", tool.Name, versionInfo)
		}

		// Show missing tools (only those needed for requested tier)
		for _, missing := range SortedTools(relevantTools.missing) {
			_, _ = fmt.Fprintf(w, "  N %s not found\n", missing.Name)
			if missing.InstallCmd != "" {
				_, _ = fmt.Fprintf(w, "    Suggested install: %s\n", missing.InstallCmd)
			}
		}

		// Show prerequisites
		if len(validation.Prerequisites) > 0 {
			for _, prereq := range validation.Prerequisites {
				if prereq.Required && !prereq.Found {
					_, _ = fmt.Fprintf(w, "  ! Missing %s\n", prereq.Name)
					if prereq.Hint != "" {
						_, _ = fmt.Fprintf(w, "    %s\n", prereq.Hint)
					}
				}
			}
		}

		_, _ = fmt.Fprintln(w)
	}

	_, _ = fmt.Fprintf(w, "Summary: %d/%d languages ready for %s tier.\n", readyCount, totalCount, tierName)

	return nil
}

type filteredTools struct {
	found   []ToolStatus
	missing []ToolStatus
}

// filterToolsForTier returns only the tools relevant to the requested tier.
func filterToolsForTier(validation LanguageValidation, requestedTier AnalysisTier) filteredTools {
	result := filteredTools{}

	// Get the tool names needed for each tier
	lang, ok := ParseLanguage(string(validation.Language))
	if !ok {
		// If we can't parse the language, return all tools
		for _, tool := range validation.Tools {
			if tool.Found {
				result.found = append(result.found, tool)
			}
		}
		result.missing = validation.Missing
		return result
	}

	// Get requirements for the requested tier level
	neededTools := make(map[string]bool)
	if requestedTier >= TierEnhanced {
		for _, req := range GetIndexerRequirements(lang, TierEnhanced) {
			neededTools[req.Name] = true
		}
	}
	if requestedTier >= TierFull {
		for _, req := range GetIndexerRequirements(lang, TierFull) {
			neededTools[req.Name] = true
		}
	}

	// Filter tools
	for _, tool := range validation.Tools {
		if neededTools[tool.Name] {
			if tool.Found {
				result.found = append(result.found, tool)
			}
		}
	}

	for _, tool := range validation.Missing {
		if neededTools[tool.Name] {
			result.missing = append(result.missing, tool)
		}
	}

	return result
}

func doctorJSON(w io.Writer, result ValidationResult, requestedTier AnalysisTier) error {
	output := struct {
		RequestedTier string                         `json:"requestedTier"`
		ReadyCount    int                            `json:"readyCount"`
		TotalCount    int                            `json:"totalCount"`
		Languages     []LanguageValidationJSONOutput `json:"languages"`
	}{
		RequestedTier: tierDisplayName(requestedTier),
		TotalCount:    len(result.Languages),
	}

	for _, validation := range SortedLanguageValidations(result) {
		if validation.ToolTier >= requestedTier {
			output.ReadyCount++
		}
		output.Languages = append(output.Languages, toJSONOutput(validation))
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

// CapabilityStatusOutput renders capability status when a command needs it.
func CapabilityStatusOutput(w io.Writer, validation LanguageValidation, capability Capability) error {
	if validation.Capabilities[string(capability)] {
		return nil // Capability is available, no output needed
	}

	_, _ = fmt.Fprintf(w, "\nCapability '%s' is not available for %s.\n", capability, validation.DisplayName)
	_, _ = fmt.Fprintf(w, "Current tier: %s (requires: ", tierDisplayName(validation.ToolTier))

	// Find which tier provides this capability
	for tier, caps := range TierCapabilities {
		for _, cap := range caps {
			if cap == capability {
				_, _ = fmt.Fprintf(w, "%s)\n", tierDisplayName(tier))
				break
			}
		}
	}

	// Show what providers could enable this
	if providers, ok := CapabilityProviders[capability]; ok {
		_, _ = fmt.Fprintf(w, "Providers: %s\n", strings.Join(providerStrings(providers), ", "))
	}

	// Show install hint
	for _, missing := range validation.Missing {
		for _, cap := range missing.Capabilities {
			if cap == string(capability) && missing.InstallCmd != "" {
				_, _ = fmt.Fprintf(w, "Install: %s\n", missing.InstallCmd)
				break
			}
		}
	}

	return nil
}

func providerStrings(providers []Provider) []string {
	strs := make([]string, len(providers))
	for i, p := range providers {
		strs[i] = string(p)
	}
	return strs
}
