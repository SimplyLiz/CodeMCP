package export

import (
	"fmt"
	"sort"
	"strings"
)

// Organizer structures export output for better LLM comprehension.
// It adds:
// 1. Module map (overview of all modules with counts)
// 2. Cross-module bridges (key connections between modules)
// 3. Clustered output by module with importance ordering
type Organizer struct {
	export *LLMExport
}

// NewOrganizer creates a new organizer.
func NewOrganizer(export *LLMExport) *Organizer {
	return &Organizer{export: export}
}

// ModuleSummary represents a high-level module overview.
type ModuleSummary struct {
	Path        string   `json:"path"`
	SymbolCount int      `json:"symbolCount"`
	FileCount   int      `json:"fileCount"`
	TopSymbols  []string `json:"topSymbols,omitempty"`
	Owner       string   `json:"owner,omitempty"`
}

// CrossModuleBridge represents a connection between modules.
type CrossModuleBridge struct {
	FromModule string `json:"fromModule"`
	ToModule   string `json:"toModule"`
	CallCount  int    `json:"callCount"`
	TopCaller  string `json:"topCaller,omitempty"`
	TopCallee  string `json:"topCallee,omitempty"`
}

// OrganizedExport contains the structured output.
type OrganizedExport struct {
	// Header section
	ModuleMap []ModuleSummary `json:"moduleMap"`

	// Connection section
	Bridges []CrossModuleBridge `json:"bridges,omitempty"`

	// Details section (clustered by module)
	Modules []ExportModule `json:"modules"`

	// Metadata
	TotalSymbols int `json:"totalSymbols"`
	TotalModules int `json:"totalModules"`
	TotalFiles   int `json:"totalFiles"`
}

// Organize structures the export for better LLM consumption.
func (o *Organizer) Organize() *OrganizedExport {
	if o.export == nil {
		return &OrganizedExport{}
	}

	result := &OrganizedExport{
		ModuleMap:    make([]ModuleSummary, 0, len(o.export.Modules)),
		Bridges:      make([]CrossModuleBridge, 0),
		Modules:      o.export.Modules,
		TotalModules: len(o.export.Modules),
	}

	// Build module map with summaries
	for _, mod := range o.export.Modules {
		symbolCount := 0
		topSymbols := make([]string, 0, 3)

		// Collect top symbols (by importance/name)
		var allSymbols []ExportSymbol
		for _, file := range mod.Files {
			symbolCount += len(file.Symbols)
			result.TotalFiles++
			allSymbols = append(allSymbols, file.Symbols...)
		}
		result.TotalSymbols += symbolCount

		// Sort by importance, take top 3
		sort.Slice(allSymbols, func(i, j int) bool {
			return allSymbols[i].Importance > allSymbols[j].Importance
		})
		for i := 0; i < min(3, len(allSymbols)); i++ {
			topSymbols = append(topSymbols, allSymbols[i].Name)
		}

		summary := ModuleSummary{
			Path:        mod.Path,
			SymbolCount: symbolCount,
			FileCount:   len(mod.Files),
			TopSymbols:  topSymbols,
			Owner:       mod.Owner,
		}
		result.ModuleMap = append(result.ModuleMap, summary)
	}

	// Sort module map by symbol count (most important first)
	sort.Slice(result.ModuleMap, func(i, j int) bool {
		return result.ModuleMap[i].SymbolCount > result.ModuleMap[j].SymbolCount
	})

	// Detect cross-module bridges (simplified - based on naming patterns)
	// In a full implementation, this would use actual call graph data
	result.Bridges = o.detectBridges()

	return result
}

// detectBridges identifies cross-module connections.
// This is a heuristic based on naming patterns.
// Full implementation would use call graph data.
func (o *Organizer) detectBridges() []CrossModuleBridge {
	// Build a map of module -> symbols
	moduleSymbols := make(map[string][]string)
	for _, mod := range o.export.Modules {
		for _, file := range mod.Files {
			for _, sym := range file.Symbols {
				moduleSymbols[mod.Path] = append(moduleSymbols[mod.Path], sym.Name)
			}
		}
	}

	// Find potential bridges (modules that likely connect based on naming)
	bridges := make([]CrossModuleBridge, 0)

	// Common integration patterns
	integrationPatterns := map[string][]string{
		"query":    {"backends", "storage", "mcp"},
		"mcp":      {"query"},
		"api":      {"query", "storage"},
		"backends": {"scip", "lsp", "git"},
	}

	for from, targets := range integrationPatterns {
		for _, to := range targets {
			// Check if both modules exist
			_, fromExists := moduleSymbols[from]
			_, toExists := moduleSymbols[to]

			if !fromExists || !toExists {
				// Try with internal/ prefix
				_, fromExists = moduleSymbols["internal/"+from]
				_, toExists = moduleSymbols["internal/"+to]

				if fromExists && toExists {
					from = "internal/" + from
					to = "internal/" + to
				} else {
					continue
				}
			}

			bridges = append(bridges, CrossModuleBridge{
				FromModule: from,
				ToModule:   to,
				CallCount:  1, // Placeholder
			})
		}
	}

	return bridges
}

// FormatOrganizedText generates LLM-friendly text from organized export.
func FormatOrganizedText(org *OrganizedExport, opts ExportOptions) string {
	var sb strings.Builder

	// Title
	sb.WriteString("# Codebase Structure\n\n")

	// Module Map (overview)
	sb.WriteString("## Module Map\n\n")
	sb.WriteString("| Module | Symbols | Files | Key Exports |\n")
	sb.WriteString("|--------|---------|-------|-------------|\n")

	for _, mod := range org.ModuleMap {
		topExports := strings.Join(mod.TopSymbols, ", ")
		if topExports == "" {
			topExports = "-"
		}
		fmt.Fprintf(&sb, "| %s | %d | %d | %s |\n",
			mod.Path, mod.SymbolCount, mod.FileCount, topExports)
	}
	sb.WriteString("\n")

	// Cross-Module Connections
	if len(org.Bridges) > 0 {
		sb.WriteString("## Cross-Module Connections\n\n")
		for _, bridge := range org.Bridges {
			fmt.Fprintf(&sb, "- %s → %s\n", bridge.FromModule, bridge.ToModule)
		}
		sb.WriteString("\n")
	}

	// Details by Module
	sb.WriteString("## Module Details\n\n")

	for _, mod := range org.Modules {
		fmt.Fprintf(&sb, "### %s/\n\n", mod.Path)

		// Group files
		for _, file := range mod.Files {
			fmt.Fprintf(&sb, "**%s**\n", file.Name)

			for _, sym := range file.Symbols {
				prefix := "#"
				if sym.Type == SymbolTypeClass || sym.Type == "struct" {
					prefix = "$"
				}

				line := fmt.Sprintf("  %s %s", prefix, sym.Name)
				if sym.Type == SymbolTypeFunction || sym.Type == SymbolTypeMethod {
					line += "()"
				}

				// Add complexity if available
				if opts.IncludeComplexity && sym.Complexity > 0 {
					line += fmt.Sprintf(" [c=%d]", sym.Complexity)
				}

				// Add importance indicator
				if sym.Importance > 0 {
					line += " " + strings.Repeat("★", sym.Importance)
				}

				sb.WriteString(line + "\n")
			}
			sb.WriteString("\n")
		}
	}

	// Summary footer
	fmt.Fprintf(&sb, "---\n")
	fmt.Fprintf(&sb, "Total: %d modules, %d files, %d symbols\n",
		org.TotalModules, org.TotalFiles, org.TotalSymbols)

	return sb.String()
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
