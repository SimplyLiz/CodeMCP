package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SimplyLiz/CodeMCP/internal/query"
)

var (
	baselineTag        string
	baselineBaseBranch string
	baselineHeadBranch string
)

var baselineCmd = &cobra.Command{
	Use:   "baseline",
	Short: "Manage review finding baselines",
	Long: `Save, list, and compare review finding baselines.

Baselines let you snapshot current findings so future reviews
can distinguish new issues from pre-existing ones.

Examples:
  ckb review baseline save                     # Save with auto-generated tag
  ckb review baseline save --tag=v1.0          # Save with named tag
  ckb review baseline list                     # List saved baselines
  ckb review baseline diff --tag=latest        # Compare current findings against baseline`,
}

var baselineSaveCmd = &cobra.Command{
	Use:   "save",
	Short: "Save current findings as a baseline",
	Run:   runBaselineSave,
}

var baselineListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved baselines",
	Run:   runBaselineList,
}

var baselineDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Compare current findings against a baseline",
	Run:   runBaselineDiff,
}

func init() {
	baselineSaveCmd.Flags().StringVar(&baselineTag, "tag", "", "Baseline tag (default: timestamp)")
	baselineSaveCmd.Flags().StringVar(&baselineBaseBranch, "base", "main", "Base branch")
	baselineSaveCmd.Flags().StringVar(&baselineHeadBranch, "head", "", "Head branch")

	baselineDiffCmd.Flags().StringVar(&baselineTag, "tag", "latest", "Baseline tag to compare against")
	baselineDiffCmd.Flags().StringVar(&baselineBaseBranch, "base", "main", "Base branch")
	baselineDiffCmd.Flags().StringVar(&baselineHeadBranch, "head", "", "Head branch")

	baselineCmd.AddCommand(baselineSaveCmd)
	baselineCmd.AddCommand(baselineListCmd)
	baselineCmd.AddCommand(baselineDiffCmd)
	reviewCmd.AddCommand(baselineCmd)
}

func runBaselineSave(cmd *cobra.Command, args []string) {
	logger := newLogger("human")
	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	// Run review to get current findings
	opts := query.ReviewPROptions{
		BaseBranch: baselineBaseBranch,
		HeadBranch: baselineHeadBranch,
	}

	resp, err := engine.ReviewPR(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running review: %v\n", err)
		os.Exit(1)
	}

	if err := engine.SaveBaseline(resp.Findings, baselineTag, baselineBaseBranch, baselineHeadBranch); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving baseline: %v\n", err)
		os.Exit(1)
	}

	tag := baselineTag
	if tag == "" {
		tag = "(auto-generated)"
	}
	fmt.Printf("Baseline saved: %s (%d findings)\n", tag, len(resp.Findings))
}

func runBaselineList(cmd *cobra.Command, args []string) {
	logger := newLogger("human")
	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	baselines, err := engine.ListBaselines()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing baselines: %v\n", err)
		os.Exit(1)
	}

	if len(baselines) == 0 {
		fmt.Println("No baselines saved yet. Use 'ckb review baseline save' to create one.")
		return
	}

	fmt.Printf("%-20s %-20s %s\n", "TAG", "CREATED", "FINDINGS")
	fmt.Println(strings.Repeat("-", 50))
	for _, b := range baselines {
		fmt.Printf("%-20s %-20s %d\n", b.Tag, b.CreatedAt.Format("2006-01-02 15:04"), b.FindingCount)
	}
}

func runBaselineDiff(cmd *cobra.Command, args []string) {
	logger := newLogger("human")
	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	// Load baseline
	baseline, err := engine.LoadBaseline(baselineTag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading baseline %q: %v\n", baselineTag, err)
		os.Exit(1)
	}

	// Run current review
	opts := query.ReviewPROptions{
		BaseBranch: baselineBaseBranch,
		HeadBranch: baselineHeadBranch,
	}

	resp, err := engine.ReviewPR(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running review: %v\n", err)
		os.Exit(1)
	}

	// Compare
	newFindings, unchanged, resolved := query.CompareWithBaseline(resp.Findings, baseline)

	fmt.Printf("Baseline: %s (%s)\n", baseline.Tag, baseline.CreatedAt.Format("2006-01-02 15:04"))
	fmt.Printf("Compared: %d current vs %d baseline findings\n\n", len(resp.Findings), baseline.FindingCount)

	if len(newFindings) > 0 {
		fmt.Printf("NEW (%d):\n", len(newFindings))
		for _, f := range newFindings {
			loc := f.File
			if f.StartLine > 0 {
				loc = fmt.Sprintf("%s:%d", f.File, f.StartLine)
			}
			fmt.Printf("  + %-7s %-40s %s\n", strings.ToUpper(f.Severity), loc, f.Message)
		}
		fmt.Println()
	}

	if len(resolved) > 0 {
		fmt.Printf("RESOLVED (%d):\n", len(resolved))
		for _, f := range resolved {
			fmt.Printf("  - %-7s %-40s %s\n", strings.ToUpper(f.Severity), f.File, f.Message)
		}
		fmt.Println()
	}

	fmt.Printf("UNCHANGED: %d\n", len(unchanged))

	if len(newFindings) == 0 && len(resolved) > 0 {
		fmt.Println("\nProgress: findings are being resolved!")
	} else if len(newFindings) > 0 {
		fmt.Printf("\nRegression: %d new finding(s) introduced\n", len(newFindings))
	}
}
