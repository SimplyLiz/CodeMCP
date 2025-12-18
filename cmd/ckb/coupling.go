package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/coupling"
)

var (
	couplingMinCorrelation float64
	couplingWindowDays     int
	couplingLimit          int
	couplingFormat         string
)

var couplingCmd = &cobra.Command{
	Use:   "coupling <file>",
	Short: "Find co-change patterns",
	Long: `Find files that historically change together.

Analyzes git commit history to find files that frequently change
in the same commits as the target file. This reveals hidden coupling
that static analysis misses.

Output includes:
  - Correlated files with correlation scores
  - Co-change counts
  - Insights about coupling patterns
  - Recommendations for improvement

Examples:
  ckb coupling src/auth/login.go
  ckb coupling --min-correlation=0.5 src/handler.go
  ckb coupling --window=180 src/api/routes.go`,
	Args: cobra.ExactArgs(1),
	Run:  runCoupling,
}

func init() {
	couplingCmd.Flags().Float64Var(&couplingMinCorrelation, "min-correlation", 0.3, "Minimum correlation threshold (0-1)")
	couplingCmd.Flags().IntVar(&couplingWindowDays, "window", 365, "Analysis window in days")
	couplingCmd.Flags().IntVar(&couplingLimit, "limit", 20, "Maximum results to return")
	couplingCmd.Flags().StringVar(&couplingFormat, "format", "json", "Output format (json, human)")
	rootCmd.AddCommand(couplingCmd)
}

func runCoupling(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(couplingFormat)
	target := args[0]

	repoRoot := mustGetRepoRoot()

	analyzer := coupling.NewAnalyzer(repoRoot, logger)

	ctx := context.Background()
	result, err := analyzer.Analyze(ctx, coupling.AnalyzeOptions{
		Target:         target,
		MinCorrelation: couplingMinCorrelation,
		WindowDays:     couplingWindowDays,
		Limit:          couplingLimit,
		RepoRoot:       repoRoot,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error analyzing coupling: %v\n", err)
		os.Exit(1)
	}

	// Format and output
	output, err := FormatResponse(result, OutputFormat(couplingFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Coupling analysis completed", map[string]interface{}{
		"target":       target,
		"correlations": len(result.Correlations),
		"duration":     time.Since(start).Milliseconds(),
	})
}
