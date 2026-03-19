package main

import (
	"fmt"
	"os"
	"sort"
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
	// Traceability
	reviewTracePatterns []string
	reviewRequireTrace  bool
	// Independence
	reviewRequireIndependent bool
	reviewMinReviewers       int
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
- Code health scoring (8-factor weighted score)
- Finding baseline management

Output formats: human (default), json, markdown, github-actions

Examples:
  ckb review                              # Review current branch vs main
  ckb review --base=develop               # Custom base branch
  ckb review --checks=breaking,secrets    # Only specific checks
  ckb review --checks=health              # Only code health check
  ckb review --ci                         # CI mode (exit codes: 0=pass, 1=fail, 2=warn)
  ckb review --format=markdown            # PR comment ready output
  ckb review --format=github-actions      # GitHub Actions annotations
  ckb review --critical-paths=drivers/**,protocol/**  # Safety-critical paths
  ckb review baseline save --tag=v1.0     # Save finding baseline
  ckb review baseline diff                # Compare against baseline`,
	Run: runReview,
}

func init() {
	reviewCmd.Flags().StringVar(&reviewFormat, "format", "human", "Output format (human, json, markdown, github-actions, sarif, codeclimate, compliance)")
	reviewCmd.Flags().StringVar(&reviewBaseBranch, "base", "main", "Base branch to compare against")
	reviewCmd.Flags().StringVar(&reviewHeadBranch, "head", "", "Head branch (default: current branch)")
	reviewCmd.Flags().StringSliceVar(&reviewChecks, "checks", nil, "Comma-separated list of checks (breaking,secrets,tests,complexity,coupling,hotspots,risk,critical,generated,classify,split,health,traceability,independence)")
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

	// Traceability
	reviewCmd.Flags().StringSliceVar(&reviewTracePatterns, "trace-patterns", nil, "Regex patterns for ticket IDs (e.g., JIRA-\\d+)")
	reviewCmd.Flags().BoolVar(&reviewRequireTrace, "require-trace", false, "Require ticket references in commits")

	// Independence
	reviewCmd.Flags().BoolVar(&reviewRequireIndependent, "require-independent", false, "Require independent reviewer (author != reviewer)")
	reviewCmd.Flags().IntVar(&reviewMinReviewers, "min-reviewers", 0, "Minimum number of independent reviewers")

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
	if len(reviewTracePatterns) > 0 {
		policy.TraceabilityPatterns = reviewTracePatterns
		policy.RequireTraceability = true
	}
	if reviewRequireTrace {
		policy.RequireTraceability = true
	}
	if reviewRequireIndependent {
		policy.RequireIndependentReview = true
	}
	if reviewMinReviewers > 0 {
		policy.MinReviewers = reviewMinReviewers
	}

	// Validate inputs
	if reviewMaxRisk < 0 {
		fmt.Fprintf(os.Stderr, "Error: --max-risk must be >= 0 (got %.2f)\n", reviewMaxRisk)
		os.Exit(1)
	}
	if reviewFailOn != "" {
		validLevels := map[string]bool{"error": true, "warning": true, "none": true}
		if !validLevels[reviewFailOn] {
			fmt.Fprintf(os.Stderr, "Error: --fail-on must be one of: error, warning, none (got %q)\n", reviewFailOn)
			os.Exit(1)
		}
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
	case "compliance":
		output = formatReviewCompliance(response)
	case "sarif":
		var fmtErr error
		output, fmtErr = formatReviewSARIF(response)
		if fmtErr != nil {
			fmt.Fprintf(os.Stderr, "Error formatting SARIF: %v\n", fmtErr)
			os.Exit(1)
		}
	case "codeclimate":
		var fmtErr error
		output, fmtErr = formatReviewCodeClimate(response)
		if fmtErr != nil {
			fmt.Fprintf(os.Stderr, "Error formatting CodeClimate: %v\n", fmtErr)
			os.Exit(1)
		}
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

	// Review Effort
	if resp.ReviewEffort != nil {
		b.WriteString(fmt.Sprintf("Estimated Review: ~%dmin (%s)\n",
			resp.ReviewEffort.EstimatedMinutes, resp.ReviewEffort.Complexity))
		for _, f := range resp.ReviewEffort.Factors {
			b.WriteString(fmt.Sprintf("  · %s\n", f))
		}
		b.WriteString("\n")
	}

	// Change Breakdown
	if resp.ChangeBreakdown != nil && len(resp.ChangeBreakdown.Summary) > 0 {
		b.WriteString("Change Breakdown:\n")
		cats := sortedMapKeys(resp.ChangeBreakdown.Summary)
		for _, cat := range cats {
			b.WriteString(fmt.Sprintf("  %-12s %d files\n", cat, resp.ChangeBreakdown.Summary[cat]))
		}
		b.WriteString("\n")
	}

	// PR Split Suggestion
	if resp.SplitSuggestion != nil && resp.SplitSuggestion.ShouldSplit {
		b.WriteString(fmt.Sprintf("PR Split: %s\n", resp.SplitSuggestion.Reason))
		for i, c := range resp.SplitSuggestion.Clusters {
			b.WriteString(fmt.Sprintf("  Cluster %d: %q — %d files (+%d −%d)\n",
				i+1, c.Name, c.FileCount, c.Additions, c.Deletions))
		}
		b.WriteString("\n")
	}

	// Code Health
	if resp.HealthReport != nil && len(resp.HealthReport.Deltas) > 0 {
		b.WriteString("Code Health:\n")
		for _, d := range resp.HealthReport.Deltas {
			arrow := "→"
			if d.Delta < 0 {
				arrow = "↓"
			} else if d.Delta > 0 {
				arrow = "↑"
			}
			b.WriteString(fmt.Sprintf("  %s %s %s%s (%d%s%d)\n",
				d.Grade, arrow, d.GradeBefore, d.File, d.HealthBefore, arrow, d.HealthAfter))
		}
		if resp.HealthReport.Degraded > 0 || resp.HealthReport.Improved > 0 {
			b.WriteString(fmt.Sprintf("  %d degraded · %d improved · avg %+.1f\n",
				resp.HealthReport.Degraded, resp.HealthReport.Improved, resp.HealthReport.AverageDelta))
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
		b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", c.Name, statusEmoji, escapeMdTable(c.Summary)))
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
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", sevEmoji, loc, escapeMdTable(f.Message)))
		}
		b.WriteString("\n</details>\n\n")
	}

	// Change Breakdown
	if resp.ChangeBreakdown != nil && len(resp.ChangeBreakdown.Summary) > 0 {
		b.WriteString("<details><summary>Change Breakdown</summary>\n\n")
		b.WriteString("| Category | Files | Review Priority |\n")
		b.WriteString("|----------|-------|-----------------|\n")
		priorityEmoji := map[string]string{
			"new": "🔴 Full review", "churn": "🔴 Stability concern",
			"refactoring": "🟡 Verify correctness", "modified": "🟡 Standard review",
			"test": "🟡 Verify coverage", "moved": "🟢 Quick check",
			"config": "🟢 Quick check", "generated": "⚪ Skip (review source)",
		}
		cats := sortedMapKeys(resp.ChangeBreakdown.Summary)
		for _, cat := range cats {
			count := resp.ChangeBreakdown.Summary[cat]
			priority := priorityEmoji[cat]
			if priority == "" {
				priority = "🟡 Review"
			}
			b.WriteString(fmt.Sprintf("| %s | %d | %s |\n", cat, count, priority))
		}
		b.WriteString("\n</details>\n\n")
	}

	// PR Split Suggestion
	if resp.SplitSuggestion != nil && resp.SplitSuggestion.ShouldSplit {
		b.WriteString(fmt.Sprintf("<details><summary>✂️ Suggested PR Split (%d clusters)</summary>\n\n",
			len(resp.SplitSuggestion.Clusters)))
		b.WriteString("| Cluster | Files | Changes | Independent |\n")
		b.WriteString("|---------|-------|---------|-------------|\n")
		for _, c := range resp.SplitSuggestion.Clusters {
			indep := "✅"
			if !c.Independent {
				indep = "❌"
			}
			b.WriteString(fmt.Sprintf("| %s | %d | +%d −%d | %s |\n",
				c.Name, c.FileCount, c.Additions, c.Deletions, indep))
		}
		b.WriteString("\n</details>\n\n")
	}

	// Code Health
	if resp.HealthReport != nil && len(resp.HealthReport.Deltas) > 0 {
		b.WriteString("<details><summary>Code Health</summary>\n\n")
		b.WriteString("| File | Before | After | Delta | Grade |\n")
		b.WriteString("|------|--------|-------|-------|-------|\n")
		for _, d := range resp.HealthReport.Deltas {
			b.WriteString(fmt.Sprintf("| `%s` | %d | %d | %+d | %s→%s |\n",
				d.File, d.HealthBefore, d.HealthAfter, d.Delta, d.GradeBefore, d.Grade))
		}
		if resp.HealthReport.Degraded > 0 || resp.HealthReport.Improved > 0 {
			b.WriteString(fmt.Sprintf("\n%d degraded · %d improved · avg %+.1f\n",
				resp.HealthReport.Degraded, resp.HealthReport.Improved, resp.HealthReport.AverageDelta))
		}
		b.WriteString("\n</details>\n\n")
	}

	// Review Effort
	if resp.ReviewEffort != nil {
		b.WriteString(fmt.Sprintf("**Estimated review:** ~%dmin (%s)\n\n",
			resp.ReviewEffort.EstimatedMinutes, resp.ReviewEffort.Complexity))
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

// escapeMdTable escapes pipe characters that would break markdown table formatting.
func escapeMdTable(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

func sortedMapKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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

		msg := escapeGHA(f.Message)
		ruleID := escapeGHA(f.RuleID)

		if f.File != "" {
			if f.StartLine > 0 {
				b.WriteString(fmt.Sprintf("::%s file=%s,line=%d::%s [%s]\n",
					level, f.File, f.StartLine, msg, ruleID))
			} else {
				b.WriteString(fmt.Sprintf("::%s file=%s::%s [%s]\n",
					level, f.File, msg, ruleID))
			}
		} else {
			b.WriteString(fmt.Sprintf("::%s::%s [%s]\n", level, msg, ruleID))
		}
	}

	return b.String()
}

// escapeGHA escapes special characters for GitHub Actions workflow commands.
// See: https://github.com/actions/toolkit/blob/main/packages/core/src/command.ts
func escapeGHA(s string) string {
	s = strings.ReplaceAll(s, "%", "%25")
	s = strings.ReplaceAll(s, "\r", "%0D")
	s = strings.ReplaceAll(s, "\n", "%0A")
	return s
}
