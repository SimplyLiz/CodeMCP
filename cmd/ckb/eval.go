package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"ckb/internal/eval"
)

var (
	evalFixtures string
	evalFormat   string
	evalVerbose  bool
)

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Evaluate retrieval quality",
	Long: `Run retrieval quality benchmarks against the current index.

This command measures:
  - Recall@K: Percentage of tests where expected symbol was in top-K results
  - MRR: Mean Reciprocal Rank of correct results
  - Latency: Average query execution time

Test types:
  - needle: Find at least one expected symbol in results
  - ranking: Verify expected symbol ranks in top positions
  - expansion: Check graph connectivity to related symbols

Examples:
  ckb eval                           # Run built-in fixtures
  ckb eval --fixtures=./tests.json   # Run custom fixtures
  ckb eval --format=json             # Output as JSON`,
	Run: runEval,
}

func init() {
	evalCmd.Flags().StringVar(&evalFixtures, "fixtures", "", "Path to fixtures file or directory (default: built-in)")
	evalCmd.Flags().StringVar(&evalFormat, "format", "human", "Output format (human, json)")
	evalCmd.Flags().BoolVarP(&evalVerbose, "verbose", "v", false, "Show all test results, not just failures")
	rootCmd.AddCommand(evalCmd)
}

func runEval(cmd *cobra.Command, args []string) {
	logger := newLogger(evalFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	// Create eval suite
	suite := eval.NewSuite(engine, logger)

	// Load fixtures
	if evalFixtures != "" {
		// Custom fixtures
		info, err := os.Stat(evalFixtures)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error accessing fixtures: %v\n", err)
			os.Exit(1)
		}

		if info.IsDir() {
			if err := suite.LoadFixturesDir(evalFixtures); err != nil {
				fmt.Fprintf(os.Stderr, "Error loading fixtures directory: %v\n", err)
				os.Exit(1)
			}
		} else {
			if err := suite.LoadFixtures(evalFixtures); err != nil {
				fmt.Fprintf(os.Stderr, "Error loading fixtures: %v\n", err)
				os.Exit(1)
			}
		}
	} else {
		// Try to load built-in fixtures from repo
		fixturesDir := filepath.Join(repoRoot, "internal", "eval", "fixtures")
		if _, err := os.Stat(fixturesDir); err == nil {
			if err := suite.LoadFixturesDir(fixturesDir); err != nil {
				fmt.Fprintf(os.Stderr, "Error loading built-in fixtures: %v\n", err)
				os.Exit(1)
			}
		} else {
			// Add minimal built-in tests if no fixtures found
			suite.AddFixture(eval.TestCase{
				ID:              "builtin-search-basic",
				Type:            "needle",
				Description:     "Basic symbol search works",
				Query:           "Engine",
				ExpectedSymbols: []string{"Engine"},
				TopK:            10,
			})
		}
	}

	// Run evaluation
	result, err := suite.Run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running evaluation: %v\n", err)
		os.Exit(1)
	}

	// Output results
	if evalFormat == "json" {
		jsonBytes, err := result.JSON()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting JSON: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Println(result.FormatReport())

		// Print pass/fail summary
		if result.PassedTests == result.TotalTests {
			fmt.Printf("\n✓ All %d tests passed\n", result.TotalTests)
		} else {
			fmt.Printf("\n✗ %d of %d tests failed\n", result.FailedTests, result.TotalTests)
		}

		// Print success criteria status
		fmt.Println()
		if result.RecallAtK >= 75 {
			fmt.Println("✓ Recall@K target met (≥75%)")
		} else {
			fmt.Println("✗ Recall@K target NOT met (<75%)")
		}

		if result.AvgLatency < 100 {
			fmt.Println("✓ Latency target met (<100ms)")
		} else {
			fmt.Println("✗ Latency target NOT met (≥100ms)")
		}
	}

	// Exit with failure if tests failed
	if result.FailedTests > 0 {
		os.Exit(1)
	}
}
