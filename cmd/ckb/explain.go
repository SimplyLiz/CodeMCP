package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/explain"
)

var (
	explainIncludeUsage    bool
	explainIncludeCoChange bool
	explainHistoryLimit    int
	explainFormat          string
)

var explainCmd = &cobra.Command{
	Use:   "explain <symbol>",
	Short: "Explain why code exists",
	Long: `Explain why code exists with full context.

Provides:
  - Origin (first commit that introduced the code)
  - Evolution timeline (how the code changed over time)
  - Co-change patterns (files that typically change together)
  - Warnings (dead code, stale, complex, etc.)
  - References from commit messages (issues, PRs)

Examples:
  ckb explain src/auth/login.go
  ckb explain src/auth/login.go:42
  ckb explain --history-limit=20 src/handler.go`,
	Args: cobra.ExactArgs(1),
	Run:  runExplain,
}

func init() {
	explainCmd.Flags().BoolVar(&explainIncludeUsage, "include-usage", true, "Include telemetry usage data")
	explainCmd.Flags().BoolVar(&explainIncludeCoChange, "include-cochange", true, "Include co-change analysis")
	explainCmd.Flags().IntVar(&explainHistoryLimit, "history-limit", 10, "Number of timeline entries")
	explainCmd.Flags().StringVar(&explainFormat, "format", "json", "Output format (json, human)")
	rootCmd.AddCommand(explainCmd)
}

func runExplain(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(explainFormat)
	symbol := args[0]

	repoRoot := mustGetRepoRoot()

	explainer := explain.NewExplainer(repoRoot, logger)

	ctx := context.Background()
	result, err := explainer.Explain(ctx, explain.ExplainOptions{
		Symbol:          symbol,
		IncludeUsage:    explainIncludeUsage,
		IncludeCoChange: explainIncludeCoChange,
		HistoryLimit:    explainHistoryLimit,
		RepoRoot:        repoRoot,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error explaining symbol: %v\n", err)
		os.Exit(1)
	}

	// Format and output
	output, err := FormatResponse(result, OutputFormat(explainFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Explain completed",
		"symbol", symbol,
		"duration", time.Since(start).Milliseconds(),
	)
}
