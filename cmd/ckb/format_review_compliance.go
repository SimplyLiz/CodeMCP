package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/query"
)

// formatReviewCompliance generates a compliance evidence report suitable for audit.
// Covers: traceability, reviewer independence, critical-path findings, health grades.
func formatReviewCompliance(resp *query.ReviewPRResponse) string {
	var b strings.Builder

	b.WriteString("=" + strings.Repeat("=", 69) + "\n")
	b.WriteString("  CKB COMPLIANCE EVIDENCE REPORT\n")
	b.WriteString("=" + strings.Repeat("=", 69) + "\n\n")

	b.WriteString(fmt.Sprintf("Generated:   %s\n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("CKB Version: %s\n", resp.CkbVersion))
	b.WriteString(fmt.Sprintf("Schema:      %s\n", resp.SchemaVersion))
	b.WriteString(fmt.Sprintf("Verdict:     %s (%d/100)\n\n", strings.ToUpper(resp.Verdict), resp.Score))

	// --- Section 1: Summary ---
	b.WriteString("1. CHANGE SUMMARY\n")
	b.WriteString(strings.Repeat("-", 40) + "\n")
	b.WriteString(fmt.Sprintf("  Total Files:      %d\n", resp.Summary.TotalFiles))
	b.WriteString(fmt.Sprintf("  Reviewable Files: %d\n", resp.Summary.ReviewableFiles))
	b.WriteString(fmt.Sprintf("  Generated Files:  %d (excluded)\n", resp.Summary.GeneratedFiles))
	b.WriteString(fmt.Sprintf("  Critical Files:   %d\n", resp.Summary.CriticalFiles))
	b.WriteString(fmt.Sprintf("  Total Changes:    %d\n", resp.Summary.TotalChanges))
	b.WriteString(fmt.Sprintf("  Modules Changed:  %d\n", resp.Summary.ModulesChanged))
	if len(resp.Summary.Languages) > 0 {
		b.WriteString(fmt.Sprintf("  Languages:        %s\n", strings.Join(resp.Summary.Languages, ", ")))
	}
	b.WriteString("\n")

	// --- Section 2: Quality Gate Results ---
	b.WriteString("2. QUALITY GATE RESULTS\n")
	b.WriteString(strings.Repeat("-", 40) + "\n")
	b.WriteString(fmt.Sprintf("  %-20s %-8s %s\n", "CHECK", "STATUS", "DETAIL"))
	b.WriteString(fmt.Sprintf("  %-20s %-8s %s\n", strings.Repeat("-", 20), strings.Repeat("-", 8), strings.Repeat("-", 30)))
	for _, c := range resp.Checks {
		b.WriteString(fmt.Sprintf("  %-20s %-8s %s\n", c.Name, strings.ToUpper(c.Status), c.Summary))
	}
	b.WriteString(fmt.Sprintf("\n  Passed: %d  Warned: %d  Failed: %d  Skipped: %d\n\n",
		resp.Summary.ChecksPassed, resp.Summary.ChecksWarned,
		resp.Summary.ChecksFailed, resp.Summary.ChecksSkipped))

	// --- Section 3: Traceability ---
	b.WriteString("3. TRACEABILITY\n")
	b.WriteString(strings.Repeat("-", 40) + "\n")
	traceFound := false
	for _, c := range resp.Checks {
		if c.Name == "traceability" {
			traceFound = true
			b.WriteString(fmt.Sprintf("  Status: %s\n", strings.ToUpper(c.Status)))
			b.WriteString(fmt.Sprintf("  Detail: %s\n", c.Summary))
			if result, ok := c.Details.(query.TraceabilityResult); ok {
				if len(result.TicketRefs) > 0 {
					b.WriteString("  References:\n")
					for _, ref := range result.TicketRefs {
						b.WriteString(fmt.Sprintf("    - %s (source: %s", ref.ID, ref.Source))
						if ref.Commit != "" {
							b.WriteString(fmt.Sprintf(", commit: %s", ref.Commit[:minInt(8, len(ref.Commit))]))
						}
						b.WriteString(")\n")
					}
				}
			}
		}
	}
	if !traceFound {
		b.WriteString("  Not configured (traceability patterns not set)\n")
	}
	b.WriteString("\n")

	// --- Section 4: Reviewer Independence ---
	b.WriteString("4. REVIEWER INDEPENDENCE\n")
	b.WriteString(strings.Repeat("-", 40) + "\n")
	indepFound := false
	for _, c := range resp.Checks {
		if c.Name == "independence" {
			indepFound = true
			b.WriteString(fmt.Sprintf("  Status: %s\n", strings.ToUpper(c.Status)))
			b.WriteString(fmt.Sprintf("  Detail: %s\n", c.Summary))
			if result, ok := c.Details.(query.IndependenceResult); ok {
				b.WriteString(fmt.Sprintf("  Authors:       %s\n", strings.Join(result.Authors, ", ")))
				b.WriteString(fmt.Sprintf("  Min Reviewers: %d\n", result.MinReviewers))
			}
		}
	}
	if !indepFound {
		b.WriteString("  Not configured (requireIndependentReview not set)\n")
	}
	b.WriteString("\n")

	// --- Section 5: Critical Path Findings ---
	b.WriteString("5. SAFETY-CRITICAL PATH FINDINGS\n")
	b.WriteString(strings.Repeat("-", 40) + "\n")
	critCount := 0
	for _, f := range resp.Findings {
		if f.Category == "critical" || f.RuleID == "ckb/traceability/critical-orphan" || f.RuleID == "ckb/independence/critical-path-review" {
			critCount++
			b.WriteString(fmt.Sprintf("  [%s] %s\n", strings.ToUpper(f.Severity), f.Message))
			if f.File != "" {
				b.WriteString(fmt.Sprintf("         File: %s\n", f.File))
			}
			if f.Suggestion != "" {
				b.WriteString(fmt.Sprintf("         Action: %s\n", f.Suggestion))
			}
		}
	}
	if critCount == 0 {
		b.WriteString("  No safety-critical findings.\n")
	}
	b.WriteString("\n")

	// --- Section 6: Code Health ---
	b.WriteString("6. CODE HEALTH\n")
	b.WriteString(strings.Repeat("-", 40) + "\n")
	if resp.HealthReport != nil && len(resp.HealthReport.Deltas) > 0 {
		b.WriteString(fmt.Sprintf("  %-40s %-8s %-8s %s\n", "FILE", "BEFORE", "AFTER", "DELTA"))
		b.WriteString(fmt.Sprintf("  %-40s %-8s %-8s %s\n", strings.Repeat("-", 40), strings.Repeat("-", 8), strings.Repeat("-", 8), strings.Repeat("-", 8)))
		for _, d := range resp.HealthReport.Deltas {
			b.WriteString(fmt.Sprintf("  %-40s %-8s %-8s %+d\n",
				truncatePath(d.File, 40),
				fmt.Sprintf("%s(%d)", d.GradeBefore, d.HealthBefore),
				fmt.Sprintf("%s(%d)", d.Grade, d.HealthAfter),
				d.Delta))
		}
		b.WriteString(fmt.Sprintf("\n  Degraded: %d  Improved: %d  Average Delta: %+.1f\n",
			resp.HealthReport.Degraded, resp.HealthReport.Improved, resp.HealthReport.AverageDelta))
	} else {
		b.WriteString("  No health data available.\n")
	}
	b.WriteString("\n")

	// --- Section 7: All Findings ---
	b.WriteString("7. COMPLETE FINDINGS\n")
	b.WriteString(strings.Repeat("-", 40) + "\n")
	if len(resp.Findings) > 0 {
		for i, f := range resp.Findings {
			b.WriteString(fmt.Sprintf("  %d. [%s] [%s] %s\n", i+1, strings.ToUpper(f.Severity), f.RuleID, f.Message))
			if f.File != "" {
				loc := f.File
				if f.StartLine > 0 {
					loc = fmt.Sprintf("%s:%d", f.File, f.StartLine)
				}
				b.WriteString(fmt.Sprintf("     File: %s\n", loc))
			}
		}
	} else {
		b.WriteString("  No findings.\n")
	}
	b.WriteString("\n")

	// --- Footer ---
	b.WriteString(strings.Repeat("=", 70) + "\n")
	b.WriteString("  END OF COMPLIANCE EVIDENCE REPORT\n")
	b.WriteString(strings.Repeat("=", 70) + "\n")

	return b.String()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}
