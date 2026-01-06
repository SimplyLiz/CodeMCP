package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/complexity"
)

var (
	complexityFormat           string
	complexityIncludeFunctions bool
	complexitySortBy           string
	complexityLimit            int
)

var complexityCmd = &cobra.Command{
	Use:   "complexity <file>",
	Short: "Get code complexity metrics for a source file",
	Long: `Get code complexity metrics using tree-sitter parsing.

Returns cyclomatic and cognitive complexity for each function, plus file-level
aggregates. Supports Go, JavaScript, TypeScript, Python, Rust, Java, and Kotlin.

Examples:
  ckb complexity internal/api/handler.go
  ckb complexity --include-functions=false src/main.ts
  ckb complexity --sort=cognitive --limit=10 pkg/service.go
  ckb complexity --format=human internal/query/engine.go`,
	Args: cobra.ExactArgs(1),
	Run:  runComplexity,
}

func init() {
	complexityCmd.Flags().StringVar(&complexityFormat, "format", "json", "Output format (json, human)")
	complexityCmd.Flags().BoolVar(&complexityIncludeFunctions, "include-functions", true, "Include per-function complexity")
	complexityCmd.Flags().StringVar(&complexitySortBy, "sort", "cyclomatic", "Sort by: cyclomatic, cognitive, or name")
	complexityCmd.Flags().IntVar(&complexityLimit, "limit", 0, "Limit number of functions shown (0 for all)")
	rootCmd.AddCommand(complexityCmd)
}

func runComplexity(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(complexityFormat)
	filePath := args[0]

	repoRoot := mustGetRepoRoot()

	// Resolve the file path
	absPath := filePath
	if !filepath.IsAbs(filePath) {
		absPath = filepath.Join(repoRoot, filePath)
	}

	// Check if file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: file not found: %s\n", absPath)
		os.Exit(1)
	}

	// Check if complexity analysis is available (requires CGO)
	if !complexity.IsAvailable() {
		fmt.Fprintf(os.Stderr, "Error: complexity analysis requires CGO (tree-sitter)\n")
		fmt.Fprintf(os.Stderr, "This binary was built without CGO support.\n")
		os.Exit(1)
	}

	// Create complexity analyzer
	analyzer := complexity.NewAnalyzer()
	ctx := context.Background()

	// Analyze the file
	fc, err := analyzer.AnalyzeFile(ctx, absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error analyzing file: %v\n", err)
		os.Exit(1)
	}

	if fc.Error != "" {
		fmt.Fprintf(os.Stderr, "Error: %s\n", fc.Error)
		os.Exit(1)
	}

	cliResponse := convertComplexityResponse(fc, filePath)

	output, err := FormatResponse(cliResponse, OutputFormat(complexityFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Complexity analysis completed",
		"file", filePath,
		"functionCount", fc.FunctionCount,
		"maxCyclomatic", fc.MaxCyclomatic,
		"maxCognitive", fc.MaxCognitive,
		"duration", time.Since(start).Milliseconds(),
	)
}

// ComplexityResponseCLI contains complexity results for CLI output
type ComplexityResponseCLI struct {
	File      string                  `json:"file"`
	Language  string                  `json:"language"`
	Summary   ComplexitySummaryCLI    `json:"summary"`
	Functions []FunctionComplexityCLI `json:"functions,omitempty"`
}

type ComplexitySummaryCLI struct {
	FunctionCount     int     `json:"functionCount"`
	TotalCyclomatic   int     `json:"totalCyclomatic"`
	TotalCognitive    int     `json:"totalCognitive"`
	MaxCyclomatic     int     `json:"maxCyclomatic"`
	MaxCognitive      int     `json:"maxCognitive"`
	AverageCyclomatic float64 `json:"averageCyclomatic"`
	AverageCognitive  float64 `json:"averageCognitive"`
}

type FunctionComplexityCLI struct {
	Name       string `json:"name"`
	StartLine  int    `json:"startLine"`
	EndLine    int    `json:"endLine"`
	Cyclomatic int    `json:"cyclomatic"`
	Cognitive  int    `json:"cognitive"`
	Risk       string `json:"risk"` // low, medium, high
}

func convertComplexityResponse(fc *complexity.FileComplexity, originalPath string) *ComplexityResponseCLI {
	result := &ComplexityResponseCLI{
		File:     originalPath,
		Language: string(fc.Language),
		Summary: ComplexitySummaryCLI{
			FunctionCount:   fc.FunctionCount,
			TotalCyclomatic: fc.TotalCyclomatic,
			TotalCognitive:  fc.TotalCognitive,
			MaxCyclomatic:   fc.MaxCyclomatic,
			MaxCognitive:    fc.MaxCognitive,
		},
	}

	if fc.FunctionCount > 0 {
		result.Summary.AverageCyclomatic = float64(fc.TotalCyclomatic) / float64(fc.FunctionCount)
		result.Summary.AverageCognitive = float64(fc.TotalCognitive) / float64(fc.FunctionCount)
	}

	if complexityIncludeFunctions && len(fc.Functions) > 0 {
		functions := make([]FunctionComplexityCLI, 0, len(fc.Functions))
		for _, f := range fc.Functions {
			risk := "low"
			if f.Cyclomatic > 10 || f.Cognitive > 15 {
				risk = "medium"
			}
			if f.Cyclomatic > 20 || f.Cognitive > 30 {
				risk = "high"
			}

			functions = append(functions, FunctionComplexityCLI{
				Name:       f.Name,
				StartLine:  f.StartLine,
				EndLine:    f.EndLine,
				Cyclomatic: f.Cyclomatic,
				Cognitive:  f.Cognitive,
				Risk:       risk,
			})
		}

		// Sort functions
		switch complexitySortBy {
		case "cognitive":
			sort.Slice(functions, func(i, j int) bool {
				return functions[i].Cognitive > functions[j].Cognitive
			})
		case "name":
			sort.Slice(functions, func(i, j int) bool {
				return functions[i].Name < functions[j].Name
			})
		default: // cyclomatic
			sort.Slice(functions, func(i, j int) bool {
				return functions[i].Cyclomatic > functions[j].Cyclomatic
			})
		}

		// Apply limit
		if complexityLimit > 0 && len(functions) > complexityLimit {
			functions = functions[:complexityLimit]
		}

		result.Functions = functions
	}

	return result
}
