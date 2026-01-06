package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"ckb/internal/mcp"
)

var (
	toolsPresetFlag string
	toolsJSONFlag   bool
)

var toolsCmd = &cobra.Command{
	Use:   "tools [name]",
	Short: "List available MCP tools",
	Long: `List all available MCP tools organized by preset/category.

Without arguments, shows preset summary.
With a preset name, shows tools in that preset.
With a tool name, shows details about that tool.

Examples:
  ckb tools              # Show preset summary
  ckb tools core         # Show tools in core preset
  ckb tools searchSymbols  # Show details for searchSymbols
  ckb tools --preset=review  # Show review preset tools`,
	Args: cobra.MaximumNArgs(1),
	RunE: runTools,
}

func init() {
	toolsCmd.Flags().StringVar(&toolsPresetFlag, "preset", "", "Show tools for a specific preset")
	toolsCmd.Flags().BoolVar(&toolsJSONFlag, "json", false, "Output as JSON")
	rootCmd.AddCommand(toolsCmd)
}

func runTools(cmd *cobra.Command, args []string) error {
	// Get all tool definitions (need a temporary server instance)
	server := mcp.NewMCPServerForCLI()
	allTools := server.GetToolDefinitions()

	// Determine what to show
	if len(args) > 0 {
		name := args[0]

		// Check if it's a preset name
		if mcp.IsValidPreset(name) {
			return showPresetTools(allTools, name)
		}

		// Check if it's a tool name
		for _, tool := range allTools {
			if tool.Name == name {
				return showToolDetails(tool)
			}
		}

		// Not found - suggest similar
		return fmt.Errorf("unknown preset or tool: %s\n\nUse 'ckb tools' to see available presets", name)
	}

	// If --preset flag was used
	if toolsPresetFlag != "" {
		if !mcp.IsValidPreset(toolsPresetFlag) {
			return fmt.Errorf("unknown preset: %s\n\nValid presets: %s",
				toolsPresetFlag, strings.Join(mcp.ValidPresets(), ", "))
		}
		return showPresetTools(allTools, toolsPresetFlag)
	}

	// Default: show preset summary
	return showPresetSummary(allTools)
}

func showPresetSummary(allTools []mcp.Tool) error {
	infos := mcp.GetAllPresetInfo(allTools)

	if toolsJSONFlag {
		data, err := json.MarshalIndent(infos, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Println("MCP Tool Presets")
	fmt.Println("────────────────────────────────────────────────────")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PRESET\tTOOLS\tTOKENS\tDESCRIPTION")

	for _, info := range infos {
		marker := " "
		if info.IsDefault {
			marker = "*"
		}
		tokens := mcp.FormatTokens(info.TokenCount)
		fmt.Fprintf(w, "%s %s\t%d\t%s\t%s\n", marker, info.Name, info.ToolCount, tokens, info.Description)
	}
	w.Flush()

	fmt.Println()
	fmt.Println("* = default preset")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  ckb tools <preset>  Show tools in a preset")
	fmt.Println("  ckb tools <tool>    Show details for a tool")

	return nil
}

func showPresetTools(allTools []mcp.Tool, preset string) error {
	filtered := mcp.FilterAndOrderTools(allTools, preset)

	if toolsJSONFlag {
		// Just output tool names for JSON
		names := make([]string, len(filtered))
		for i, t := range filtered {
			names[i] = t.Name
		}
		data, err := json.MarshalIndent(map[string]interface{}{
			"preset": preset,
			"count":  len(filtered),
			"tools":  names,
		}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	desc := mcp.PresetDescriptions[preset]
	fmt.Printf("Preset: %s\n", preset)
	fmt.Printf("Description: %s\n", desc)
	fmt.Printf("Tools: %d\n", len(filtered))
	fmt.Println()

	// Group tools by category based on name patterns
	categories := categorizeTools(filtered)

	for _, cat := range categories {
		if len(cat.Tools) == 0 {
			continue
		}
		fmt.Printf("%s:\n", cat.Name)
		for _, tool := range cat.Tools {
			// Truncate description to 60 chars
			desc := tool.Description
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}
			fmt.Printf("  %-20s %s\n", tool.Name, desc)
		}
		fmt.Println()
	}

	return nil
}

type toolCategory struct {
	Name  string
	Tools []mcp.Tool
}

func categorizeTools(tools []mcp.Tool) []toolCategory {
	categories := map[string][]mcp.Tool{
		"Compound (v8.0)": {},
		"Discovery":       {},
		"Navigation":      {},
		"Architecture":    {},
		"Impact & Risk":   {},
		"Code Review":     {},
		"Refactoring":     {},
		"Federation":      {},
		"Documentation":   {},
		"System":          {},
		"Other":           {},
	}

	// Categorize based on tool name patterns
	for _, tool := range tools {
		name := tool.Name
		switch {
		case name == "explore" || name == "understand" || name == "prepareChange" ||
			name == "batchGet" || name == "batchSearch":
			categories["Compound (v8.0)"] = append(categories["Compound (v8.0)"], tool)

		case strings.HasPrefix(name, "search") || name == "getSymbol":
			categories["Discovery"] = append(categories["Discovery"], tool)

		case strings.HasPrefix(name, "find") || strings.HasPrefix(name, "get") && strings.Contains(name, "Call") ||
			name == "traceUsage" || name == "explainSymbol" || name == "explainFile":
			categories["Navigation"] = append(categories["Navigation"], tool)

		case strings.Contains(name, "Architecture") || strings.Contains(name, "Module") ||
			strings.Contains(name, "Concept") || strings.Contains(name, "Entrypoint"):
			categories["Architecture"] = append(categories["Architecture"], tool)

		case strings.Contains(name, "Impact") || strings.Contains(name, "Risk") ||
			strings.Contains(name, "Hotspot") || strings.Contains(name, "Coupling"):
			categories["Impact & Risk"] = append(categories["Impact & Risk"], tool)

		case strings.Contains(name, "Diff") || strings.Contains(name, "Pr") ||
			strings.Contains(name, "Ownership"):
			categories["Code Review"] = append(categories["Code Review"], tool)

		case strings.Contains(name, "Dead") || strings.Contains(name, "justify") ||
			strings.Contains(name, "Affected") || strings.Contains(name, "API"):
			categories["Refactoring"] = append(categories["Refactoring"], tool)

		case strings.Contains(name, "federation") || strings.Contains(name, "Federation"):
			categories["Federation"] = append(categories["Federation"], tool)

		case strings.Contains(name, "Doc") || strings.Contains(name, "doc"):
			categories["Documentation"] = append(categories["Documentation"], tool)

		case name == "getStatus" || name == "doctor" || name == "reindex" ||
			name == "expandToolset" || strings.Contains(name, "daemon") ||
			strings.Contains(name, "Job") || strings.Contains(name, "Webhook"):
			categories["System"] = append(categories["System"], tool)

		default:
			categories["Other"] = append(categories["Other"], tool)
		}
	}

	// Return in display order
	order := []string{
		"Compound (v8.0)", "Discovery", "Navigation", "Architecture",
		"Impact & Risk", "Code Review", "Refactoring", "Federation",
		"Documentation", "System", "Other",
	}

	result := make([]toolCategory, 0)
	for _, name := range order {
		if tools, ok := categories[name]; ok && len(tools) > 0 {
			// Sort tools within category
			sort.Slice(tools, func(i, j int) bool {
				return tools[i].Name < tools[j].Name
			})
			result = append(result, toolCategory{Name: name, Tools: tools})
		}
	}

	return result
}

func showToolDetails(tool mcp.Tool) error {
	if toolsJSONFlag {
		data, err := json.MarshalIndent(tool, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Tool: %s\n", tool.Name)
	fmt.Println("────────────────────────────────────────────────────")
	fmt.Println()
	fmt.Printf("Description:\n  %s\n", tool.Description)
	fmt.Println()

	// Show input schema in a readable way
	if tool.InputSchema != nil {
		fmt.Println("Parameters:")
		props, ok := tool.InputSchema["properties"].(map[string]interface{})
		if ok && len(props) > 0 {
			required := make(map[string]bool)
			if reqList, ok := tool.InputSchema["required"].([]string); ok {
				for _, r := range reqList {
					required[r] = true
				}
			}
			if reqList, ok := tool.InputSchema["required"].([]interface{}); ok {
				for _, r := range reqList {
					if s, ok := r.(string); ok {
						required[s] = true
					}
				}
			}

			// Sort parameter names
			names := make([]string, 0, len(props))
			for name := range props {
				names = append(names, name)
			}
			sort.Strings(names)

			for _, name := range names {
				prop := props[name].(map[string]interface{})
				reqMarker := ""
				if required[name] {
					reqMarker = " (required)"
				}

				typ := "any"
				if t, ok := prop["type"].(string); ok {
					typ = t
				}

				desc := ""
				if d, ok := prop["description"].(string); ok {
					desc = d
				}

				fmt.Printf("  %s%s (%s)\n", name, reqMarker, typ)
				if desc != "" {
					fmt.Printf("    %s\n", desc)
				}

				// Show enum values if present
				if enum, ok := prop["enum"].([]interface{}); ok {
					values := make([]string, len(enum))
					for i, v := range enum {
						values[i] = fmt.Sprintf("%v", v)
					}
					fmt.Printf("    Values: %s\n", strings.Join(values, ", "))
				}
				if enum, ok := prop["enum"].([]string); ok {
					fmt.Printf("    Values: %s\n", strings.Join(enum, ", "))
				}
			}
		} else {
			fmt.Println("  (no parameters)")
		}
	}

	return nil
}
