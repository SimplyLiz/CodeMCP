package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

func formatComplianceHuman(report *compliance.ComplianceReport) string {
	var b strings.Builder

	b.WriteString("=" + strings.Repeat("=", 69) + "\n")
	b.WriteString("  CKB COMPLIANCE AUDIT REPORT\n")
	b.WriteString("=" + strings.Repeat("=", 69) + "\n\n")

	b.WriteString(fmt.Sprintf("  Repository:   %s\n", report.Repo))
	b.WriteString(fmt.Sprintf("  Generated:    %s\n", report.AnalyzedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("  Verdict:      %s\n", strings.ToUpper(report.Verdict)))
	b.WriteString(fmt.Sprintf("  Score:        %d/100\n", report.Score))
	b.WriteString(fmt.Sprintf("  Files:        %d scanned, %d with issues\n",
		report.Summary.FilesScanned, report.Summary.FilesWithIssues))
	b.WriteString(fmt.Sprintf("  Findings:     %d total", report.Summary.TotalFindings))
	if report.Summary.BySeverity["error"] > 0 {
		b.WriteString(fmt.Sprintf(" (%d errors", report.Summary.BySeverity["error"]))
		if report.Summary.BySeverity["warning"] > 0 {
			b.WriteString(fmt.Sprintf(", %d warnings", report.Summary.BySeverity["warning"]))
		}
		b.WriteString(")")
	}
	b.WriteString("\n\n")

	// Framework coverage
	b.WriteString("FRAMEWORK COVERAGE\n")
	b.WriteString(strings.Repeat("-", 70) + "\n")
	b.WriteString(fmt.Sprintf("  %-35s %6s %6s %6s %6s %5s\n",
		"FRAMEWORK", "CHECKS", "PASS", "WARN", "FAIL", "SCORE"))
	b.WriteString(fmt.Sprintf("  %-35s %6s %6s %6s %6s %5s\n",
		strings.Repeat("-", 35), "------", "------", "------", "------", "-----"))

	for _, cov := range report.Coverage {
		b.WriteString(fmt.Sprintf("  %-35s %6d %6d %6d %6d  %3d%%\n",
			cov.Name, cov.TotalChecks, cov.Passed, cov.Warned, cov.Failed, cov.Score))
	}
	b.WriteString("\n")

	// Check results
	b.WriteString("CHECK RESULTS\n")
	b.WriteString(strings.Repeat("-", 70) + "\n")
	b.WriteString(fmt.Sprintf("  %-40s %-8s %s\n", "CHECK", "STATUS", "SUMMARY"))
	b.WriteString(fmt.Sprintf("  %-40s %-8s %s\n",
		strings.Repeat("-", 40), strings.Repeat("-", 8), strings.Repeat("-", 20)))

	for _, c := range report.Checks {
		statusIcon := "  "
		switch c.Status {
		case "pass":
			statusIcon = "PASS"
		case "warn":
			statusIcon = "WARN"
		case "fail":
			statusIcon = "FAIL"
		case "skip":
			statusIcon = "SKIP"
		}
		b.WriteString(fmt.Sprintf("  %-40s %-8s %s\n", c.Name, statusIcon, c.Summary))
	}
	b.WriteString("\n")

	// Findings grouped by severity
	if len(report.Findings) > 0 {
		b.WriteString("FINDINGS\n")
		b.WriteString(strings.Repeat("-", 70) + "\n")

		for i, f := range report.Findings {
			b.WriteString(fmt.Sprintf("  %d. [%s] %s\n", i+1, strings.ToUpper(f.Severity), f.Message))
			if f.File != "" {
				loc := f.File
				if f.StartLine > 0 {
					loc = fmt.Sprintf("%s:%d", f.File, f.StartLine)
				}
				b.WriteString(fmt.Sprintf("     File:    %s\n", loc))
			}
			if f.Detail != "" {
				b.WriteString(fmt.Sprintf("     Article: %s\n", f.Detail))
			}
			if f.Suggestion != "" {
				b.WriteString(fmt.Sprintf("     Action:  %s\n", f.Suggestion))
			}
			if f.RuleID != "" {
				b.WriteString(fmt.Sprintf("     Rule:    %s\n", f.RuleID))
			}
		}
		b.WriteString("\n")
	}

	// Footer
	b.WriteString(strings.Repeat("=", 70) + "\n")
	b.WriteString("  END OF COMPLIANCE AUDIT REPORT\n")
	b.WriteString(strings.Repeat("=", 70) + "\n")

	return b.String()
}

func formatComplianceMarkdown(report *compliance.ComplianceReport) string {
	var b strings.Builder

	b.WriteString("# CKB Compliance Audit Report\n\n")
	b.WriteString(fmt.Sprintf("**Repository:** %s  \n", report.Repo))
	b.WriteString(fmt.Sprintf("**Date:** %s  \n", report.AnalyzedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("**Verdict:** %s | **Score:** %d/100  \n",
		strings.ToUpper(report.Verdict), report.Score))
	b.WriteString(fmt.Sprintf("**Files:** %d scanned, %d with issues  \n",
		report.Summary.FilesScanned, report.Summary.FilesWithIssues))
	b.WriteString(fmt.Sprintf("**Findings:** %d total\n\n", report.Summary.TotalFindings))

	// Framework coverage table
	b.WriteString("## Framework Coverage\n\n")
	b.WriteString("| Framework | Checks | Pass | Warn | Fail | Score |\n")
	b.WriteString("|-----------|--------|------|------|------|-------|\n")
	for _, cov := range report.Coverage {
		b.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d | %d%% |\n",
			cov.Name, cov.TotalChecks, cov.Passed, cov.Warned, cov.Failed, cov.Score))
	}
	b.WriteString("\n")

	// Findings
	if len(report.Findings) > 0 {
		b.WriteString("## Findings\n\n")

		// Group by severity
		for _, sev := range []string{"error", "warning", "info"} {
			var sevFindings []int
			for i, f := range report.Findings {
				if f.Severity == sev {
					sevFindings = append(sevFindings, i)
				}
			}
			if len(sevFindings) == 0 {
				continue
			}

			sevLabel := strings.ToUpper(sev[:1]) + sev[1:]
			b.WriteString(fmt.Sprintf("### %s (%d)\n\n", sevLabel, len(sevFindings)))

			for _, idx := range sevFindings {
				f := report.Findings[idx]
				loc := ""
				if f.File != "" {
					if f.StartLine > 0 {
						loc = fmt.Sprintf("`%s:%d`", f.File, f.StartLine)
					} else {
						loc = fmt.Sprintf("`%s`", f.File)
					}
				}
				b.WriteString(fmt.Sprintf("- **%s** %s", f.Message, loc))
				if f.Detail != "" {
					b.WriteString(fmt.Sprintf(" — %s", f.Detail))
				}
				b.WriteString("\n")
				if f.Suggestion != "" {
					b.WriteString(fmt.Sprintf("  - *Action:* %s\n", f.Suggestion))
				}
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}
