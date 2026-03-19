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
	reviewBlockBreaking    bool
	reviewBlockSecrets     bool
	reviewRequireTests  bool
	reviewMaxRisk       float64
	reviewMaxComplexity int
	reviewMaxFiles      int
	// Critical paths
	reviewCriticalPaths []string
	// Lint dedup
	reviewLintReport string
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
	reviewCmd.Flags().BoolVar(&reviewBlockBreaking, "block-breaking", true, "Fail on breaking changes")
	reviewCmd.Flags().BoolVar(&reviewBlockSecrets, "block-secrets", true, "Fail on detected secrets")
	reviewCmd.Flags().BoolVar(&reviewRequireTests, "require-tests", false, "Warn if no tests cover changes")
	reviewCmd.Flags().Float64Var(&reviewMaxRisk, "max-risk", 0.7, "Maximum risk score (0 = disabled)")
	reviewCmd.Flags().IntVar(&reviewMaxComplexity, "max-complexity", 0, "Maximum complexity delta (0 = disabled)")
	reviewCmd.Flags().IntVar(&reviewMaxFiles, "max-files", 0, "Maximum file count (0 = disabled)")
	reviewCmd.Flags().StringSliceVar(&reviewCriticalPaths, "critical-paths", nil, "Glob patterns for safety-critical paths")
	reviewCmd.Flags().StringVar(&reviewLintReport, "lint-report", "", "Path to existing SARIF lint report to deduplicate against")

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
	policy.BlockBreakingChanges = reviewBlockBreaking
	policy.BlockSecrets = reviewBlockSecrets
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

	// Deduplicate against external lint report
	if reviewLintReport != "" {
		suppressed, lintErr := deduplicateLintFindings(response, reviewLintReport)
		if lintErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not parse lint report: %v\n", lintErr)
		} else if suppressed > 0 {
			logger.Debug("Deduplicated findings against lint report",
				"suppressed", suppressed, "remaining", len(response.Findings))
		}
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

	// Narrative
	if resp.Narrative != "" {
		b.WriteString(resp.Narrative + "\n\n")
	}

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

	// Top Findings — only Tier 1+2 by default, capped at 10
	if len(resp.Findings) > 0 {
		actionable, tier3Count := filterActionableFindings(resp.Findings)
		if len(actionable) > 0 {
			b.WriteString("Top Findings:\n")
			limit := 10
			if len(actionable) < limit {
				limit = len(actionable)
			}
			for _, f := range actionable[:limit] {
				sevLabel := strings.ToUpper(f.Severity)
				loc := f.File
				if f.StartLine > 0 {
					loc = fmt.Sprintf("%s:%d", f.File, f.StartLine)
				}
				b.WriteString(fmt.Sprintf("  %-7s %-40s %s\n", sevLabel, loc, f.Message))
			}
			remaining := len(actionable) - limit
			if remaining > 0 || tier3Count > 0 {
				parts := []string{}
				if remaining > 0 {
					parts = append(parts, fmt.Sprintf("%d more findings", remaining))
				}
				if tier3Count > 0 {
					parts = append(parts, fmt.Sprintf("%d informational", tier3Count))
				}
				b.WriteString(fmt.Sprintf("  ... and %s\n", strings.Join(parts, ", ")))
			}
			b.WriteString("\n")
		}
	}

	// Review Effort
	if resp.ReviewEffort != nil {
		b.WriteString(fmt.Sprintf("Estimated Review: ~%dmin (%s)\n",
			resp.ReviewEffort.EstimatedMinutes, resp.ReviewEffort.Complexity))
		// Only show effort factors for small/medium PRs
		if resp.PRTier != "large" {
			for _, f := range resp.ReviewEffort.Factors {
				b.WriteString(fmt.Sprintf("  · %s\n", f))
			}
		}
		b.WriteString("\n")
	}

	// Change Breakdown — skip for large PRs (the checks table already covers this)
	if resp.PRTier != "large" && resp.ChangeBreakdown != nil && len(resp.ChangeBreakdown.Summary) > 0 {
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
		clusterLimit := 10
		clusters := resp.SplitSuggestion.Clusters
		if len(clusters) > clusterLimit {
			clusters = clusters[:clusterLimit]
		}
		for i, c := range clusters {
			b.WriteString(fmt.Sprintf("  Cluster %d: %q — %d files (+%d −%d)\n",
				i+1, c.Name, c.FileCount, c.Additions, c.Deletions))
		}
		if len(resp.SplitSuggestion.Clusters) > clusterLimit {
			b.WriteString(fmt.Sprintf("  ... and %d more clusters\n",
				len(resp.SplitSuggestion.Clusters)-clusterLimit))
		}
		b.WriteString("\n")
	}

	// Code Health — only show files with actual changes (skip unchanged and new files)
	if resp.HealthReport != nil && len(resp.HealthReport.Deltas) > 0 {
		b.WriteString("Code Health:\n")
		shown := 0
		for _, d := range resp.HealthReport.Deltas {
			if d.Delta == 0 && !d.NewFile {
				continue // skip unchanged
			}
			if shown >= 10 {
				continue // count remaining but don't print
			}
			arrow := "→"
			label := ""
			if d.NewFile {
				arrow = "★"
				label = " (new)"
			} else if d.Delta < 0 {
				arrow = "↓"
			} else if d.Delta > 0 {
				arrow = "↑"
			}
			b.WriteString(fmt.Sprintf("  %s %s %s (%d)%s\n",
				d.Grade, arrow, d.File, d.HealthAfter, label))
			shown++
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

	// Narrative
	if resp.Narrative != "" {
		b.WriteString("> " + resp.Narrative + "\n\n")
	}

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

	// Top Risks — the review narrative between checks and findings
	if len(resp.Summary.TopRisks) > 0 {
		b.WriteString("### Top Risks\n\n")
		for _, risk := range resp.Summary.TopRisks {
			b.WriteString(fmt.Sprintf("- %s\n", risk))
		}
		b.WriteString("\n")
	}

	// Findings — Tier 1+2 only, capped at 10
	if len(resp.Findings) > 0 {
		actionable, tier3Count := filterActionableFindings(resp.Findings)
		label := fmt.Sprintf("Findings (%d)", len(actionable))
		if tier3Count > 0 {
			label = fmt.Sprintf("Findings (%d actionable, %d informational)", len(actionable), tier3Count)
		}
		if len(actionable) > 0 {
			b.WriteString(fmt.Sprintf("<details><summary>%s</summary>\n\n", label))
			b.WriteString("| Severity | File | Finding |\n")
			b.WriteString("|----------|------|---------|\n")
			limit := 10
			if len(actionable) < limit {
				limit = len(actionable)
			}
			for _, f := range actionable[:limit] {
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
			if len(actionable) > limit {
				b.WriteString(fmt.Sprintf("\n... and %d more\n", len(actionable)-limit))
			}
			b.WriteString("\n</details>\n\n")
		}
	}

	// Change Breakdown — skip for large PRs
	if resp.PRTier != "large" && resp.ChangeBreakdown != nil && len(resp.ChangeBreakdown.Summary) > 0 {
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
		clusters := resp.SplitSuggestion.Clusters
		clusterLimit := 10
		b.WriteString(fmt.Sprintf("<details><summary>✂️ Suggested PR Split (%d clusters)</summary>\n\n",
			len(clusters)))
		b.WriteString("| Cluster | Files | Changes | Independent |\n")
		b.WriteString("|---------|-------|---------|-------------|\n")
		if len(clusters) > clusterLimit {
			clusters = clusters[:clusterLimit]
		}
		for _, c := range clusters {
			indep := "✅"
			if !c.Independent {
				indep = "❌"
			}
			b.WriteString(fmt.Sprintf("| %s | %d | +%d −%d | %s |\n",
				c.Name, c.FileCount, c.Additions, c.Deletions, indep))
		}
		if len(resp.SplitSuggestion.Clusters) > clusterLimit {
			b.WriteString(fmt.Sprintf("\n... and %d more clusters\n",
				len(resp.SplitSuggestion.Clusters)-clusterLimit))
		}
		b.WriteString("\n</details>\n\n")
	}

	// Code Health — show degraded files first, then new files; skip unchanged
	if resp.HealthReport != nil && len(resp.HealthReport.Deltas) > 0 {
		// Separate into degraded, improved, and new
		var degraded, improved, newFiles []query.CodeHealthDelta
		for _, d := range resp.HealthReport.Deltas {
			switch {
			case d.NewFile:
				newFiles = append(newFiles, d)
			case d.Delta < 0:
				degraded = append(degraded, d)
			case d.Delta > 0:
				improved = append(improved, d)
			}
		}

		healthTitle := "Code Health"
		if len(degraded) > 0 {
			healthTitle = fmt.Sprintf("Code Health — %d degraded", len(degraded))
		}
		b.WriteString(fmt.Sprintf("<details><summary>%s</summary>\n\n", healthTitle))

		if len(degraded) > 0 {
			b.WriteString("**Degraded:**\n\n")
			b.WriteString("| File | Before | After | Delta | Grade |\n")
			b.WriteString("|------|--------|-------|-------|-------|\n")
			limit := 10
			if len(degraded) < limit {
				limit = len(degraded)
			}
			for _, d := range degraded[:limit] {
				b.WriteString(fmt.Sprintf("| `%s` | %d | %d | %+d | %s→%s |\n",
					d.File, d.HealthBefore, d.HealthAfter, d.Delta, d.GradeBefore, d.Grade))
			}
			if len(degraded) > limit {
				b.WriteString(fmt.Sprintf("\n... and %d more degraded files\n", len(degraded)-limit))
			}
			b.WriteString("\n")
		}
		if len(improved) > 0 {
			b.WriteString(fmt.Sprintf("**Improved:** %d file(s)\n\n", len(improved)))
		}
		if len(newFiles) > 0 {
			b.WriteString(fmt.Sprintf("**New files:** %d (avg health: %d)\n\n",
				len(newFiles), avgHealth(newFiles)))
		}

		if resp.HealthReport.Degraded > 0 || resp.HealthReport.Improved > 0 {
			b.WriteString(fmt.Sprintf("%d degraded · %d improved · avg %+.1f\n",
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

// filterActionableFindings separates Tier 1+2 (actionable) from Tier 3 (informational).
func filterActionableFindings(findings []query.ReviewFinding) (actionable []query.ReviewFinding, tier3Count int) {
	for _, f := range findings {
		if f.Tier <= 2 {
			actionable = append(actionable, f)
		} else {
			tier3Count++
		}
	}
	return
}

func avgHealth(deltas []query.CodeHealthDelta) int {
	if len(deltas) == 0 {
		return 0
	}
	total := 0
	for _, d := range deltas {
		total += d.HealthAfter
	}
	return total / len(deltas)
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
