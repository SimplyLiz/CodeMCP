package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SimplyLiz/CodeMCP/internal/perf"
)

var (
	perfMinCorrelation float64
	perfMinCoChanges   int
	perfWindowDays     int
	perfLimit          int
	perfScope          []string
	perfFormat         string
)

var perfCmd = &cobra.Command{
	Use:   "perf",
	Short: "Scan for structural performance problems",
	Long: `Scan the codebase for structural issues that indicate hidden complexity.

Currently detects:

  Hidden coupling — file pairs that co-change frequently in git history but
  have no static import edge between them. This is the most actionable signal:
  files that look unrelated in the dependency graph but are implicitly coupled
  through shared state, side effects, or a third party.

Unlike the coupling command (which analyzes a single target file), perf scans
the whole repo (or a scoped subset) and surfaces the highest-signal pairs.

Examples:
  ckb perf
  ckb perf --min-correlation=0.5
  ckb perf --scope=internal/auth,internal/sessions
  ckb perf --window=180 --format=json`,
	Run: runPerf,
}

func init() {
	perfCmd.Flags().Float64Var(&perfMinCorrelation, "min-correlation", 0.3, "Minimum co-change correlation threshold (0–1)")
	perfCmd.Flags().IntVar(&perfMinCoChanges, "min-co-changes", 3, "Minimum number of shared commits to consider a pair")
	perfCmd.Flags().IntVar(&perfWindowDays, "window", 365, "Git history window in days")
	perfCmd.Flags().IntVar(&perfLimit, "limit", 50, "Maximum hidden-coupling pairs to return")
	perfCmd.Flags().StringSliceVar(&perfScope, "scope", nil, "Limit scan to these paths (comma-separated or repeated flag)")
	perfCmd.Flags().StringVar(&perfFormat, "format", "human", "Output format: human or json")
	rootCmd.AddCommand(perfCmd)
}

func runPerf(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(perfFormat)
	repoRoot := mustGetRepoRoot()

	analyzer := perf.NewAnalyzer(repoRoot, logger)

	ctx := context.Background()
	result, err := analyzer.Scan(ctx, perf.ScanOptions{
		RepoRoot:       repoRoot,
		Scope:          perfScope,
		MinCorrelation: perfMinCorrelation,
		MinCoChanges:   perfMinCoChanges,
		WindowDays:     perfWindowDays,
		Limit:          perfLimit,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running perf scan: %v\n", err)
		os.Exit(1)
	}

	if OutputFormat(perfFormat) == FormatJSON {
		output, err := FormatResponse(result, FormatJSON)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(output)
		return
	}

	// Human-readable output.
	printPerfResult(result)

	logger.Debug("Perf scan completed",
		"hidden", len(result.HiddenCoupling),
		"pairsChecked", result.Summary.PairsChecked,
		"duration", time.Since(start).Milliseconds(),
	)
}

func printPerfResult(result *perf.PerfScanResult) {
	s := result.Summary
	fmt.Printf("Perf scan: %d files, %d pairs checked, %d hidden coupling pairs found\n",
		s.FilesObserved, s.PairsChecked, s.HiddenPairsFound)
	fmt.Printf("Window: %s – %s\n\n",
		s.AnalysisFrom.Format("2006-01-02"), s.AnalysisTo.Format("2006-01-02"))

	if len(result.HiddenCoupling) == 0 {
		fmt.Println("No hidden coupling detected.")
		return
	}

	fmt.Println("Hidden coupling (co-change without import edge):")
	fmt.Println(strings.Repeat("─", 70))

	for _, p := range result.HiddenCoupling {
		fmt.Printf("[%s] %.0f%%  %d commits\n", strings.ToUpper(p.Level), p.Correlation*100, p.CoChangeCount)
		fmt.Printf("  %s\n", p.FileA)
		fmt.Printf("  %s\n", p.FileB)
		fmt.Printf("  → %s\n\n", p.Explanation)
	}
}
