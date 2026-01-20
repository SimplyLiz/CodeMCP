package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/export"
)

var (
	exportIncludeUsage      bool
	exportIncludeOwnership  bool
	exportIncludeContracts  bool
	exportIncludeComplexity bool
	exportMinComplexity     int
	exportMinCalls          int
	exportMaxSymbols        int
	exportFormat            string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export codebase for LLM consumption",
	Long: `Export codebase structure in LLM-friendly format.

Generates a token-efficient representation of the codebase
optimized for consumption by LLMs. Includes:
  - Module structure
  - Symbol definitions (classes, functions, interfaces)
  - Complexity scores
  - Usage metrics (if telemetry enabled)
  - Ownership information
  - Contract indicators

Examples:
  ckb export
  ckb export --min-complexity=5
  ckb export --max-symbols=1000
  ckb export --no-usage --no-ownership`,
	Run: runExport,
}

func init() {
	exportCmd.Flags().BoolVar(&exportIncludeUsage, "include-usage", true, "Include telemetry usage data")
	exportCmd.Flags().BoolVar(&exportIncludeOwnership, "include-ownership", true, "Include owner annotations")
	exportCmd.Flags().BoolVar(&exportIncludeContracts, "include-contracts", true, "Include contract indicators")
	exportCmd.Flags().BoolVar(&exportIncludeComplexity, "include-complexity", true, "Include complexity scores")
	exportCmd.Flags().IntVar(&exportMinComplexity, "min-complexity", 0, "Only include symbols with complexity >= N")
	exportCmd.Flags().IntVar(&exportMinCalls, "min-calls", 0, "Only include symbols with calls/day >= N")
	exportCmd.Flags().IntVar(&exportMaxSymbols, "max-symbols", 0, "Limit total symbols (0 = unlimited)")
	exportCmd.Flags().StringVar(&exportFormat, "format", "text", "Output format (text, json)")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger("json") // Use JSON logger for consistency

	repoRoot := mustGetRepoRoot()

	exporter := export.NewExporter(repoRoot, logger)

	ctx := context.Background()
	result, err := exporter.Export(ctx, export.ExportOptions{
		RepoRoot:          repoRoot,
		IncludeUsage:      exportIncludeUsage,
		IncludeOwnership:  exportIncludeOwnership,
		IncludeContracts:  exportIncludeContracts,
		IncludeComplexity: exportIncludeComplexity,
		MinComplexity:     exportMinComplexity,
		MinCalls:          exportMinCalls,
		MaxSymbols:        exportMaxSymbols,
		Format:            exportFormat,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error exporting: %v\n", err)
		os.Exit(1)
	}

	// Output based on format
	if exportFormat == "text" {
		formatted := exporter.FormatText(result, export.ExportOptions{
			IncludeComplexity: exportIncludeComplexity,
			IncludeUsage:      exportIncludeUsage,
			IncludeContracts:  exportIncludeContracts,
		})
		fmt.Println(formatted)
	} else {
		output, err := FormatResponse(result, OutputFormat(exportFormat))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(output)
	}

	logger.Debug("Export completed",
		"symbols", result.Metadata.SymbolCount,
		"files", result.Metadata.FileCount,
		"modules", result.Metadata.ModuleCount,
		"duration", time.Since(start).Milliseconds(),
	)
}
