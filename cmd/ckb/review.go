package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SimplyLiz/CodeMCP/internal/query"
)

var (
	reviewFormat     string
	reviewBaseBranch string
	reviewHeadBranch string
	reviewChecks     []string
	reviewCI         bool
	reviewFailOn     string
	// Policy overrides
	reviewNoBreaking    bool
	reviewNoSecrets     bool
	reviewRequireTests  bool
	reviewMaxRisk       float64
	reviewMaxComplexity int
	reviewMaxFiles      int
	// Critical paths
	reviewCriticalPaths []string
)

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Comprehensive PR review with quality gates",
	Long: `Run a unified code review that orchestrates multiple checks in parallel:

- Breaking API changes (SCIP-based)
- Secret detection
- Affected tests
- Complexity delta (tree-sitter)
- Coupling gaps (git co-change analysis)
- Hotspot overlap
- Risk scoring
- Safety-critical path checks

Output formats: human (default), json, markdown, github-actions

Examples:
  ckb review                              # Review current branch vs main
  ckb review --base=develop               # Custom base branch
  ckb review --checks=breaking,secrets    # Only specific checks
  ckb review --ci                         # CI mode (exit codes: 0=pass, 1=fail, 2=warn)
  ckb review --format=markdown            # PR comment ready output
  ckb review --format=github-actions      # GitHub Actions annotations
  ckb review --critical-paths=drivers/**,protocol/**  # Safety-critical paths`,
	Run: runReview,
}

func init() {
	reviewCmd.Flags().StringVar(&reviewFormat, "format", "human", "Output format (human, json, markdown, github-actions)")
	reviewCmd.Flags().StringVar(&reviewBaseBranch, "base", "main", "Base branch to compare against")
	reviewCmd.Flags().StringVar(&reviewHeadBranch, "head", "", "Head branch (default: current branch)")
	reviewCmd.Flags().StringSliceVar(&reviewChecks, "checks", nil, "Comma-separated list of checks (breaking,secrets,tests,complexity,coupling,hotspots,risk,critical,generated)")
	reviewCmd.Flags().BoolVar(&reviewCI, "ci", false, "CI mode: exit 1 on fail, exit 2 on warn")
	reviewCmd.Flags().StringVar(&reviewFailOn, "fail-on", "", "Override fail level (error, warning, none)")

	// Policy overrides
	reviewCmd.Flags().BoolVar(&reviewNoBreaking, "no-breaking", true, "Fail on breaking changes")
	reviewCmd.Flags().BoolVar(&reviewNoSecrets, "no-secrets", true, "Fail on detected secrets")
	reviewCmd.Flags().BoolVar(&reviewRequireTests, "require-tests", false, "Warn if no tests cover changes")
	reviewCmd.Flags().Float64Var(&reviewMaxRisk, "max-risk", 0.7, "Maximum risk score (0 = disabled)")
	reviewCmd.Flags().IntVar(&reviewMaxComplexity, "max-complexity", 0, "Maximum complexity delta (0 = disabled)")
	reviewCmd.Flags().IntVar(&reviewMaxFiles, "max-files", 0, "Maximum file count (0 = disabled)")
	reviewCmd.Flags().StringSliceVar(&reviewCriticalPaths, "critical-paths", nil, "Glob patterns for safety-critical paths")

	rootCmd.AddCommand(reviewCmd)
}

func runReview(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(reviewFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	policy := query.DefaultReviewPolicy()
	policy.NoBreakingChanges = reviewNoBreaking
	policy.NoSecrets = reviewNoSecrets
	policy.RequireTests = reviewRequireTests
	policy.MaxRiskScore = reviewMaxRisk
	policy.MaxComplexityDelta = reviewMaxComplexity
	policy.MaxFiles = reviewMaxFiles
	if reviewFailOn != "" {
		policy.FailOnLevel = reviewFailOn
	}
	if len(reviewCriticalPaths) > 0 {
		policy.CriticalPaths = reviewCriticalPaths
	}

	opts := query.ReviewPROptions{
		BaseBranch: reviewBaseBranch,
		HeadBranch: reviewHeadBranch,
		Policy:     policy,
		Checks:     reviewChecks,
	}

	response, err := engine.ReviewPR(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running review: %v\n", err)
		os.Exit(1)
	}

	// Format output
	var output string
	switch OutputFormat(reviewFormat) {
	case "markdown":
		output = formatReviewMarkdown(response)
	case "github-actions":
		output = formatReviewGitHubActions(response)
	case FormatJSON:
		var fmtErr error
		output, fmtErr = formatJSON(response)
		if fmtErr != nil {
			fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", fmtErr)
			os.Exit(1)
		}
	default:
		output = formatReviewHuman(response)
	}

	fmt.Println(output)

	logger.Debug("Review completed",
		"baseBranch", reviewBaseBranch,
		"headBranch", reviewHeadBranch,
		"verdict", response.Verdict,
		"score", response.Score,
		"checks", len(response.Checks),
		"findings", len(response.Findings),
		"duration", time.Since(start).Milliseconds(),
	)

	// CI mode exit codes
	if reviewCI {
		switch response.Verdict {
		case "fail":
			os.Exit(1)
		case "warn":
			os.Exit(2)
		}
	}
}

// --- Output Formatters ---

func formatReviewHuman(resp *query.ReviewPRResponse) string {
	var b strings.Builder

	// Header box
	verdictIcon := "✓"
	verdictLabel := "PASS"
	switch resp.Verdict {
	case "fail":
		verdictIcon = "✗"
		verdictLabel = "FAIL"
	case "warn":
		verdictIcon = "⚠"
		verdictLabel = "WARN"
	}

	b.WriteString(fmt.Sprintf("CKB Review: %s %s — %d/100\n", verdictIcon, verdictLabel, resp.Score))
	b.WriteString(strings.Repeat("=", 60) + "\n")
	b.WriteString(fmt.Sprintf("%d files · +%d changes · %d modules\n",
		resp.Summary.TotalFiles, resp.Summary.TotalChanges, resp.Summary.ModulesChanged))

	if resp.Summary.GeneratedFiles > 0 {
		b.WriteString(fmt.Sprintf("%d generated (excluded) · %d reviewable",
			resp.Summary.GeneratedFiles, resp.Summary.ReviewableFiles))
		if resp.Summary.CriticalFiles > 0 {
			b.WriteString(fmt.Sprintf(" · %d critical", resp.Summary.CriticalFiles))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Checks table
	b.WriteString("Checks:\n")
	for _, c := range resp.Checks {
		icon := "✓"
		switch c.Status {
		case "fail":
			icon = "✗"
		case "warn":
			icon = "⚠"
		case "skip":
			icon = "○"
		case "info":
			icon = "○"
		}
		status := strings.ToUpper(c.Status)
		b.WriteString(fmt.Sprintf("  %s %-5s %-20s %s\n", icon, status, c.Name, c.Summary))
	}
	b.WriteString("\n")

	// Top Findings
	if len(resp.Findings) > 0 {
		b.WriteString("Top Findings:\n")
		limit := 10
		if len(resp.Findings) < limit {
			limit = len(resp.Findings)
		}
		for _, f := range resp.Findings[:limit] {
			sevLabel := strings.ToUpper(f.Severity)
			loc := f.File
			if f.StartLine > 0 {
				loc = fmt.Sprintf("%s:%d", f.File, f.StartLine)
			}
			b.WriteString(fmt.Sprintf("  %-7s %-40s %s\n", sevLabel, loc, f.Message))
		}
		if len(resp.Findings) > limit {
			b.WriteString(fmt.Sprintf("  ... and %d more findings\n", len(resp.Findings)-limit))
		}
		b.WriteString("\n")
	}

	// Reviewers
	if len(resp.Reviewers) > 0 {
		b.WriteString("Suggested Reviewers:\n  ")
		var parts []string
		for _, r := range resp.Reviewers {
			parts = append(parts, fmt.Sprintf("@%s (%.0f%%)", r.Owner, r.Coverage*100))
		}
		b.WriteString(strings.Join(parts, " · "))
		b.WriteString("\n")
	}

	return b.String()
}

func formatReviewMarkdown(resp *query.ReviewPRResponse) string {
	var b strings.Builder

	// Header
	verdictEmoji := "✅"
	switch resp.Verdict {
	case "fail":
		verdictEmoji = "🔴"
	case "warn":
		verdictEmoji = "🟡"
	}

	b.WriteString(fmt.Sprintf("## CKB Review: %s %s — %d/100\n\n",
		verdictEmoji, strings.ToUpper(resp.Verdict), resp.Score))

	b.WriteString(fmt.Sprintf("**%d files** (+%d changes) · **%d modules**",
		resp.Summary.TotalFiles, resp.Summary.TotalChanges, resp.Summary.ModulesChanged))
	if len(resp.Summary.Languages) > 0 {
		b.WriteString(" · `" + strings.Join(resp.Summary.Languages, "` `") + "`")
	}
	b.WriteString("\n")

	if resp.Summary.GeneratedFiles > 0 || resp.Summary.CriticalFiles > 0 {
		b.WriteString(fmt.Sprintf("**%d reviewable**", resp.Summary.ReviewableFiles))
		if resp.Summary.GeneratedFiles > 0 {
			b.WriteString(fmt.Sprintf(" · %d generated (excluded)", resp.Summary.GeneratedFiles))
		}
		if resp.Summary.CriticalFiles > 0 {
			b.WriteString(fmt.Sprintf(" · **%d safety-critical**", resp.Summary.CriticalFiles))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Checks table
	b.WriteString("| Check | Status | Detail |\n")
	b.WriteString("|-------|--------|--------|\n")
	for _, c := range resp.Checks {
		statusEmoji := "✅ PASS"
		switch c.Status {
		case "fail":
			statusEmoji = "🔴 FAIL"
		case "warn":
			statusEmoji = "🟡 WARN"
		case "skip":
			statusEmoji = "⚪ SKIP"
		case "info":
			statusEmoji = "ℹ️ INFO"
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", c.Name, statusEmoji, c.Summary))
	}
	b.WriteString("\n")

	// Findings in collapsible section
	if len(resp.Findings) > 0 {
		b.WriteString(fmt.Sprintf("<details><summary>Findings (%d)</summary>\n\n", len(resp.Findings)))
		b.WriteString("| Severity | File | Finding |\n")
		b.WriteString("|----------|------|---------|\n")
		for _, f := range resp.Findings {
			sevEmoji := "ℹ️"
			switch f.Severity {
			case "error":
				sevEmoji = "🔴"
			case "warning":
				sevEmoji = "🟡"
			}
			loc := f.File
			if f.StartLine > 0 {
				loc = fmt.Sprintf("`%s:%d`", f.File, f.StartLine)
			} else if f.File != "" {
				loc = fmt.Sprintf("`%s`", f.File)
			}
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", sevEmoji, loc, f.Message))
		}
		b.WriteString("\n</details>\n\n")
	}

	// Reviewers
	if len(resp.Reviewers) > 0 {
		var parts []string
		for _, r := range resp.Reviewers {
			parts = append(parts, fmt.Sprintf("@%s (%.0f%%)", r.Owner, r.Coverage*100))
		}
		b.WriteString("**Reviewers:** " + strings.Join(parts, " · ") + "\n\n")
	}

	// Marker for update-in-place
	b.WriteString("<!-- ckb-review-marker -->\n")

	return b.String()
}

func formatReviewGitHubActions(resp *query.ReviewPRResponse) string {
	var b strings.Builder

	for _, f := range resp.Findings {
		level := "notice"
		switch f.Severity {
		case "error":
			level = "error"
		case "warning":
			level = "warning"
		}

		if f.File != "" {
			if f.StartLine > 0 {
				b.WriteString(fmt.Sprintf("::%s file=%s,line=%d::%s [%s]\n",
					level, f.File, f.StartLine, f.Message, f.RuleID))
			} else {
				b.WriteString(fmt.Sprintf("::%s file=%s::%s [%s]\n",
					level, f.File, f.Message, f.RuleID))
			}
		} else {
			b.WriteString(fmt.Sprintf("::%s::%s [%s]\n", level, f.Message, f.RuleID))
		}
	}

	return b.String()
}
