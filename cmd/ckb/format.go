package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

// OutputFormat represents the output format type
type OutputFormat string

const (
	FormatJSON  OutputFormat = "json"
	FormatHuman OutputFormat = "human"
)

// FormatResponse formats a response according to the specified format
func FormatResponse(resp interface{}, format OutputFormat) (string, error) {
	switch format {
	case FormatJSON:
		return formatJSON(resp)
	case FormatHuman:
		return formatHuman(resp)
	default:
		return "", fmt.Errorf("unsupported format: %s", format)
	}
}

// formatJSON formats the response as JSON
func formatJSON(resp interface{}) (string, error) {
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return string(data), nil
}

// formatHuman formats the response in human-readable format
func formatHuman(resp interface{}) (string, error) {
	switch v := resp.(type) {
	case *Response:
		return formatResponseHuman(v)
	case *StatusResponseCLI:
		return formatStatusHuman(v)
	case *DoctorResponseCLI:
		return formatDoctorHuman(v)
	case *SearchResponseCLI:
		return formatSearchHuman(v)
	case *SymbolResponseCLI:
		return formatSymbolHuman(v)
	case *ReferencesResponseCLI:
		return formatRefsHuman(v)
	case *ArchitectureResponseCLI:
		return formatArchHuman(v)
	case *ImpactResponseCLI:
		return formatImpactHuman(v)
	default:
		// For unknown types, fall back to JSON
		return formatJSON(resp)
	}
}

// formatResponseHuman formats a generic Response in human-readable format
func formatResponseHuman(resp *Response) (string, error) {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("CKB v%s\n", resp.CkbVersion))
	b.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Provenance
	if resp.Provenance != nil {
		b.WriteString("Provenance:\n")
		repoStateId := resp.Provenance.RepoStateId
		if len(repoStateId) > 12 {
			repoStateId = repoStateId[:12]
		}
		b.WriteString(fmt.Sprintf("  Repo State: %s (dirty: %v, mode: %s)\n",
			repoStateId,
			resp.Provenance.RepoStateDirty,
			resp.Provenance.RepoStateMode))

		if resp.Provenance.Completeness != nil {
			b.WriteString(fmt.Sprintf("  Completeness: %.1f%% (%s)\n",
				resp.Provenance.Completeness.Score*100,
				resp.Provenance.Completeness.Source))
		}

		if len(resp.Provenance.Backends) > 0 {
			b.WriteString("  Backends:\n")
			for _, backend := range resp.Provenance.Backends {
				b.WriteString(fmt.Sprintf("    - %s (%dms)\n", backend.BackendId, backend.DurationMs))
			}
		}

		if len(resp.Provenance.Warnings) > 0 {
			b.WriteString("  Warnings:\n")
			for _, w := range resp.Provenance.Warnings {
				b.WriteString(fmt.Sprintf("    ! %s\n", w))
			}
		}

		b.WriteString(fmt.Sprintf("  Query Duration: %dms\n", resp.Provenance.QueryDurationMs))
		b.WriteString("\n")
	}

	// Facts - this will be type-specific, so we just indicate there are facts
	b.WriteString("Facts: (see JSON output for details)\n\n")

	// Drilldowns
	if len(resp.Drilldowns) > 0 {
		b.WriteString("Suggested Follow-ups:\n")
		for i, d := range resp.Drilldowns {
			b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, d.Label))
			b.WriteString(fmt.Sprintf("     $ ckb %s\n", d.Query))
		}
	}

	return b.String(), nil
}

// formatStatusHuman formats a StatusResponseCLI in human-readable format
func formatStatusHuman(resp *StatusResponseCLI) (string, error) {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("CKB Status - v%s\n", resp.CkbVersion))
	b.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Overall health
	healthIcon := "✓"
	if !resp.Healthy {
		healthIcon = "✗"
	}
	healthText := "Healthy"
	if !resp.Healthy {
		healthText = "Unhealthy"
	}
	b.WriteString(fmt.Sprintf("%s System Health: %s\n\n", healthIcon, healthText))

	// Repository State
	if resp.RepoState != nil {
		b.WriteString("Repository State:\n")
		headCommit := resp.RepoState.HeadCommit
		if len(headCommit) > 12 {
			headCommit = headCommit[:12]
		}
		b.WriteString(fmt.Sprintf("  Head Commit: %s\n", headCommit))
		b.WriteString(fmt.Sprintf("  Dirty: %v\n", resp.RepoState.Dirty))
		b.WriteString(fmt.Sprintf("  Computed: %s\n\n", resp.RepoState.ComputedAt))
	}

	// Backends
	b.WriteString("Backends:\n")
	for _, backend := range resp.Backends {
		status := "✓"
		if !backend.Available {
			status = "✗"
		}
		availText := "Available"
		if !backend.Available {
			availText = "Unavailable"
		}
		b.WriteString(fmt.Sprintf("  %s %s: %s\n", status, backend.ID, availText))
		if len(backend.Capabilities) > 0 {
			b.WriteString(fmt.Sprintf("     Capabilities: %s\n", strings.Join(backend.Capabilities, ", ")))
		}
		if backend.Details != "" {
			b.WriteString(fmt.Sprintf("     %s\n", backend.Details))
		}
	}
	b.WriteString("\n")

	// Cache
	b.WriteString("Cache:\n")
	b.WriteString(fmt.Sprintf("  Queries Cached: %d\n", resp.Cache.QueryCount))
	b.WriteString(fmt.Sprintf("  Views Cached: %d\n", resp.Cache.ViewCount))
	b.WriteString(fmt.Sprintf("  Hit Rate: %.1f%%\n", resp.Cache.HitRate*100))
	b.WriteString(fmt.Sprintf("  Size: %s\n", formatBytes(resp.Cache.SizeBytes)))

	return b.String(), nil
}

// formatDoctorHuman formats a DoctorResponseCLI in human-readable format
func formatDoctorHuman(resp *DoctorResponseCLI) (string, error) {
	var b strings.Builder

	b.WriteString("CKB Doctor\n")
	b.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Overall health
	healthIcon := "✓"
	healthText := "All checks passed"
	if !resp.Healthy {
		healthIcon = "✗"
		healthText = "Issues found"
	}
	b.WriteString(fmt.Sprintf("%s %s\n\n", healthIcon, healthText))

	// Checks
	for _, check := range resp.Checks {
		var icon string
		switch check.Status {
		case "pass":
			icon = "✓"
		case "warn":
			icon = "⚠"
		case "fail":
			icon = "✗"
		default:
			icon = "?"
		}

		b.WriteString(fmt.Sprintf("%s %s: %s\n", icon, check.Name, check.Message))

		// Suggested fixes
		if len(check.SuggestedFixes) > 0 {
			b.WriteString("  Suggested fixes:\n")
			for _, fix := range check.SuggestedFixes {
				b.WriteString(fmt.Sprintf("    - %s\n", fix.Description))
				if fix.Command != "" {
					b.WriteString(fmt.Sprintf("      $ %s\n", fix.Command))
				}
			}
		}
		b.WriteString("\n")
	}

	return b.String(), nil
}

// formatSearchHuman formats a SearchResponseCLI in human-readable format
func formatSearchHuman(resp *SearchResponseCLI) (string, error) {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Search Results for: %s\n", resp.Query))
	b.WriteString(strings.Repeat("=", 60) + "\n\n")
	b.WriteString(fmt.Sprintf("Found %d matches\n\n", resp.TotalMatches))

	for i, sym := range resp.Symbols {
		b.WriteString(fmt.Sprintf("%d. %s (%s)\n", i+1, sym.Name, sym.Kind))
		b.WriteString(fmt.Sprintf("   ID: %s\n", sym.StableID))
		b.WriteString(fmt.Sprintf("   Module: %s\n", sym.ModuleID))
		if sym.Location != nil {
			b.WriteString(fmt.Sprintf("   Location: %s:%d\n", sym.Location.FileID, sym.Location.StartLine))
		}
		b.WriteString(fmt.Sprintf("   Relevance: %.2f\n\n", sym.RelevanceScore))
	}

	return b.String(), nil
}

// formatSymbolHuman formats a SymbolResponseCLI in human-readable format
func formatSymbolHuman(resp *SymbolResponseCLI) (string, error) {
	var b strings.Builder

	b.WriteString("Symbol Details\n")
	b.WriteString(strings.Repeat("=", 60) + "\n\n")

	b.WriteString(fmt.Sprintf("Name: %s\n", resp.Symbol.Name))
	b.WriteString(fmt.Sprintf("Kind: %s\n", resp.Symbol.Kind))
	b.WriteString(fmt.Sprintf("ID: %s\n", resp.Symbol.StableID))
	b.WriteString(fmt.Sprintf("Visibility: %s (confidence: %.2f)\n", resp.Symbol.Visibility, resp.Symbol.VisibilityConfidence))

	if resp.Symbol.ContainerName != "" {
		b.WriteString(fmt.Sprintf("Container: %s\n", resp.Symbol.ContainerName))
	}

	if resp.Location != nil {
		b.WriteString(fmt.Sprintf("\nLocation: %s:%d:%d\n", resp.Location.FileID, resp.Location.StartLine, resp.Location.StartColumn))
	}

	if resp.Module != nil {
		b.WriteString(fmt.Sprintf("\nModule: %s\n", resp.Module.ModuleID))
	}

	return b.String(), nil
}

// formatRefsHuman formats a ReferencesResponseCLI in human-readable format
func formatRefsHuman(resp *ReferencesResponseCLI) (string, error) {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("References to: %s\n", resp.SymbolID))
	b.WriteString(strings.Repeat("=", 60) + "\n\n")
	b.WriteString(fmt.Sprintf("Total references: %d\n\n", resp.TotalReferences))

	if len(resp.ByModule) > 0 {
		b.WriteString("By Module:\n")
		for _, m := range resp.ByModule {
			b.WriteString(fmt.Sprintf("  %s: %d references\n", m.ModuleID, m.Count))
		}
		b.WriteString("\n")
	}

	b.WriteString("References:\n")
	for i, ref := range resp.References {
		if ref.Location != nil {
			testMarker := ""
			if ref.IsTest {
				testMarker = " [test]"
			}
			b.WriteString(fmt.Sprintf("  %d. %s:%d (%s)%s\n", i+1, ref.Location.FileID, ref.Location.StartLine, ref.Kind, testMarker))
		}
	}

	return b.String(), nil
}

// formatArchHuman formats an ArchitectureResponseCLI in human-readable format
func formatArchHuman(resp *ArchitectureResponseCLI) (string, error) {
	var b strings.Builder

	b.WriteString("Architecture Overview\n")
	b.WriteString(strings.Repeat("=", 60) + "\n\n")

	b.WriteString(fmt.Sprintf("Modules: %d\n", len(resp.Modules)))
	b.WriteString(fmt.Sprintf("Dependencies: %d\n", len(resp.DependencyGraph)))
	b.WriteString(fmt.Sprintf("Entrypoints: %d\n\n", len(resp.Entrypoints)))

	b.WriteString("Modules:\n")
	for _, m := range resp.Modules {
		b.WriteString(fmt.Sprintf("  %s (%s)\n", m.Name, m.Language))
		b.WriteString(fmt.Sprintf("    Path: %s\n", m.RootPath))
		b.WriteString(fmt.Sprintf("    Files: %d, Symbols: %d\n", m.FileCount, m.SymbolCount))
		b.WriteString(fmt.Sprintf("    Deps: %d incoming, %d outgoing\n\n", m.IncomingEdges, m.OutgoingEdges))
	}

	if len(resp.Entrypoints) > 0 {
		b.WriteString("Entrypoints:\n")
		for _, ep := range resp.Entrypoints {
			b.WriteString(fmt.Sprintf("  %s (%s) - %s\n", ep.Name, ep.Kind, ep.FileID))
		}
	}

	return b.String(), nil
}

// formatImpactHuman formats an ImpactResponseCLI in human-readable format
func formatImpactHuman(resp *ImpactResponseCLI) (string, error) {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Impact Analysis: %s\n", resp.SymbolID))
	b.WriteString(strings.Repeat("=", 60) + "\n\n")

	if resp.RiskScore != nil {
		b.WriteString(fmt.Sprintf("Risk Level: %s (score: %.2f)\n", resp.RiskScore.Level, resp.RiskScore.Score))
		b.WriteString(fmt.Sprintf("Explanation: %s\n\n", resp.RiskScore.Explanation))
	}

	b.WriteString(fmt.Sprintf("Direct Impact: %d symbols\n", len(resp.DirectImpact)))
	b.WriteString(fmt.Sprintf("Transitive Impact: %d symbols\n", len(resp.TransitiveImpact)))
	b.WriteString(fmt.Sprintf("Modules Affected: %d\n\n", len(resp.ModulesAffected)))

	if len(resp.ModulesAffected) > 0 {
		b.WriteString("Affected Modules:\n")
		for _, m := range resp.ModulesAffected {
			b.WriteString(fmt.Sprintf("  %s: %d symbols\n", m.ModuleID, m.ImpactCount))
		}
		b.WriteString("\n")
	}

	if len(resp.DirectImpact) > 0 {
		b.WriteString("Direct Dependencies:\n")
		for _, item := range resp.DirectImpact[:min(10, len(resp.DirectImpact))] {
			b.WriteString(fmt.Sprintf("  - %s (%s)\n", item.Name, item.Kind))
		}
		if len(resp.DirectImpact) > 10 {
			b.WriteString(fmt.Sprintf("  ... and %d more\n", len(resp.DirectImpact)-10))
		}
	}

	return b.String(), nil
}

// formatBytes formats byte size in human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
