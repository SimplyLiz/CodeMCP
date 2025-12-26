package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/query"
)

var (
	affectedTestsStaged      bool
	affectedTestsBase        string
	affectedTestsDepth       int
	affectedTestsFormat      string
	affectedTestsUseCoverage bool
)

var affectedTestsCmd = &cobra.Command{
	Use:   "affected-tests",
	Short: "Find tests affected by current changes",
	Long: `Analyze code changes to determine which tests should be run.

Uses SCIP symbol analysis to trace from changed code to test files,
and applies language-specific heuristics to find corresponding test files.

Examples:
  ckb affected-tests                    # Analyze current working tree changes
  ckb affected-tests --staged           # Analyze only staged changes
  ckb affected-tests --base=main        # Compare against main branch
  ckb affected-tests --format=list      # Output just file paths (for CI)
  ckb affected-tests --depth=2          # Deeper transitive analysis`,
	Run: runAffectedTests,
}

func init() {
	affectedTestsCmd.Flags().BoolVar(&affectedTestsStaged, "staged", false, "Analyze only staged changes")
	affectedTestsCmd.Flags().StringVar(&affectedTestsBase, "base", "HEAD", "Base branch for comparison")
	affectedTestsCmd.Flags().IntVar(&affectedTestsDepth, "depth", 1, "Maximum depth for transitive impact (1-3)")
	affectedTestsCmd.Flags().StringVar(&affectedTestsFormat, "format", "human", "Output format (json, human, list)")
	affectedTestsCmd.Flags().BoolVar(&affectedTestsUseCoverage, "coverage", false, "Use coverage data if available")

	rootCmd.AddCommand(affectedTestsCmd)
}

func runAffectedTests(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(affectedTestsFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	// Get affected tests
	opts := query.GetAffectedTestsOptions{
		Staged:          affectedTestsStaged,
		BaseBranch:      affectedTestsBase,
		TransitiveDepth: affectedTestsDepth,
		UseCoverage:     affectedTestsUseCoverage,
	}

	response, err := engine.GetAffectedTests(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding affected tests: %v\n", err)
		os.Exit(1)
	}

	// Convert to CLI format
	cliResponse := convertAffectedTestsResponse(response)

	// Format output
	switch affectedTestsFormat {
	case "list":
		// Simple list of file paths for CI
		for _, test := range response.Tests {
			fmt.Println(test.FilePath)
		}
	case "json":
		output, err := FormatResponse(cliResponse, FormatJSON)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(output)
	default:
		// Human-readable format
		fmt.Println(formatAffectedTestsHuman(response))
	}

	logger.Debug("Affected tests analysis completed", map[string]interface{}{
		"testFiles": len(response.Tests),
		"duration":  time.Since(start).Milliseconds(),
	})
}

// AffectedTestsResponseCLI is the CLI response format for affected tests.
type AffectedTestsResponseCLI struct {
	Tests        []AffectedTestCLI `json:"tests"`
	Summary      *TestSummaryCLI   `json:"summary"`
	CoverageUsed bool              `json:"coverageUsed"`
	Confidence   float64           `json:"confidence"`
	RunCommand   string            `json:"runCommand,omitempty"`
	Provenance   *ProvenanceCLI    `json:"provenance,omitempty"`
}

// AffectedTestCLI describes an affected test file.
type AffectedTestCLI struct {
	FilePath   string   `json:"filePath"`
	TestNames  []string `json:"testNames,omitempty"`
	Reason     string   `json:"reason"`
	AffectedBy []string `json:"affectedBy,omitempty"`
	Confidence float64  `json:"confidence"`
}

// TestSummaryCLI provides a summary of affected tests.
type TestSummaryCLI struct {
	TotalFiles       int    `json:"totalFiles"`
	DirectFiles      int    `json:"directFiles"`
	TransitiveFiles  int    `json:"transitiveFiles"`
	CoverageFiles    int    `json:"coverageFiles"`
	EstimatedRuntime string `json:"estimatedRuntime,omitempty"`
}

func convertAffectedTestsResponse(resp *query.AffectedTestsResponse) *AffectedTestsResponseCLI {
	tests := make([]AffectedTestCLI, len(resp.Tests))
	for i, t := range resp.Tests {
		tests[i] = AffectedTestCLI{
			FilePath:   t.FilePath,
			TestNames:  t.TestNames,
			Reason:     t.Reason,
			AffectedBy: t.AffectedBy,
			Confidence: t.Confidence,
		}
	}

	var summary *TestSummaryCLI
	if resp.Summary != nil {
		summary = &TestSummaryCLI{
			TotalFiles:       resp.Summary.TotalFiles,
			DirectFiles:      resp.Summary.DirectFiles,
			TransitiveFiles:  resp.Summary.TransitiveFiles,
			CoverageFiles:    resp.Summary.CoverageFiles,
			EstimatedRuntime: resp.Summary.EstimatedRuntime,
		}
	}

	var provenance *ProvenanceCLI
	if resp.Provenance != nil {
		provenance = &ProvenanceCLI{
			RepoStateId:     resp.Provenance.RepoStateId,
			RepoStateDirty:  resp.Provenance.RepoStateDirty,
			QueryDurationMs: resp.Provenance.QueryDurationMs,
		}
	}

	return &AffectedTestsResponseCLI{
		Tests:        tests,
		Summary:      summary,
		CoverageUsed: resp.CoverageUsed,
		Confidence:   resp.Confidence,
		RunCommand:   resp.RunCommand,
		Provenance:   provenance,
	}
}

func formatAffectedTestsHuman(resp *query.AffectedTestsResponse) string {
	var b strings.Builder

	b.WriteString("Affected Tests\n")
	b.WriteString("──────────────────────────────────────────────────────────\n\n")

	if len(resp.Tests) == 0 {
		b.WriteString("No affected tests found.\n")
		return b.String()
	}

	// Summary
	if resp.Summary != nil {
		b.WriteString(fmt.Sprintf("Found %d test files:\n", resp.Summary.TotalFiles))
		if resp.Summary.DirectFiles > 0 {
			b.WriteString(fmt.Sprintf("  • %d direct (test references changed code)\n", resp.Summary.DirectFiles))
		}
		if resp.Summary.TransitiveFiles > 0 {
			b.WriteString(fmt.Sprintf("  • %d transitive (test uses affected code)\n", resp.Summary.TransitiveFiles))
		}
		if resp.Summary.CoverageFiles > 0 {
			b.WriteString(fmt.Sprintf("  • %d from coverage data\n", resp.Summary.CoverageFiles))
		}
		b.WriteString("\n")
	}

	// Test files
	b.WriteString("Test files:\n")
	for i, test := range resp.Tests {
		if i >= 20 {
			b.WriteString(fmt.Sprintf("  ... and %d more\n", len(resp.Tests)-20))
			break
		}

		icon := "○"
		if test.Reason == "direct" {
			icon = "●"
		}
		confidence := fmt.Sprintf("%.0f%%", test.Confidence*100)
		b.WriteString(fmt.Sprintf("  %s %s (%s, %s)\n", icon, test.FilePath, test.Reason, confidence))
	}
	b.WriteString("\n")

	// Run command
	if resp.RunCommand != "" {
		b.WriteString("Run command:\n")
		b.WriteString(fmt.Sprintf("  %s\n", resp.RunCommand))
		b.WriteString("\n")
	}

	// Confidence
	b.WriteString(fmt.Sprintf("Overall confidence: %.0f%%\n", resp.Confidence*100))

	return b.String()
}
