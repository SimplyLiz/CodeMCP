package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
	// Register all framework check packages
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/euaiact"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/gdpr"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/iec61508"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/iso27001"
	_ "github.com/SimplyLiz/CodeMCP/internal/compliance/iso27701"
)

var (
	complianceFrameworks string
	complianceFormat     string
	complianceScope      string
	complianceCI         bool
	complianceFailOn     string
	complianceMinConf    float64
	complianceSILLevel   int
	complianceChecks     string
)

var auditComplianceCmd = &cobra.Command{
	Use:   "compliance",
	Short: "Regulatory compliance audit",
	Long: `Audit codebase against regulatory compliance frameworks.

Frameworks:
  gdpr        GDPR/DSGVO (Regulation (EU) 2016/679)
  eu-ai-act   EU AI Act (Regulation (EU) 2024/1689)
  iso27001    ISO 27001:2022 (Annex A Technology Controls)
  iso27701    ISO 27701 (Privacy Extension)
  iec61508    IEC 61508 / SIL (Safety Integrity)
  all         Run all frameworks

Each finding maps to a specific regulation article/clause with severity,
confidence score, and CWE reference where applicable.

Examples:
  ckb audit compliance --framework=gdpr
  ckb audit compliance --framework=gdpr,iso27001
  ckb audit compliance --framework=all --min-confidence=0.7
  ckb audit compliance --framework=iso27001 --format=sarif
  ckb audit compliance --framework=iec61508 --sil-level=3
  ckb audit compliance --framework=gdpr --ci --fail-on=error`,
	Run: runAuditCompliance,
}

func init() {
	auditComplianceCmd.Flags().StringVar(&complianceFrameworks, "framework", "", "Frameworks to audit (comma-separated or 'all')")
	auditComplianceCmd.Flags().StringVar(&complianceFormat, "format", "human", "Output format (human, json, markdown, sarif)")
	auditComplianceCmd.Flags().StringVar(&complianceScope, "scope", "", "Path prefix filter")
	auditComplianceCmd.Flags().BoolVar(&complianceCI, "ci", false, "CI mode: exit code 1 on failure")
	auditComplianceCmd.Flags().StringVar(&complianceFailOn, "fail-on", "error", "Severity threshold for failure (error, warning, none)")
	auditComplianceCmd.Flags().Float64Var(&complianceMinConf, "min-confidence", 0.5, "Minimum confidence to include findings (0.0-1.0)")
	auditComplianceCmd.Flags().IntVar(&complianceSILLevel, "sil-level", 2, "SIL level for IEC 61508 (1-4)")
	auditComplianceCmd.Flags().StringVar(&complianceChecks, "checks", "", "Filter to specific check IDs (comma-separated)")

	_ = auditComplianceCmd.MarkFlagRequired("framework")
	auditCmd.AddCommand(auditComplianceCmd)
}

func runAuditCompliance(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(complianceFormat)

	repoRoot := mustGetRepoRoot()

	// Parse frameworks
	var frameworks []compliance.FrameworkID
	for _, f := range strings.Split(complianceFrameworks, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if f == "all" {
			frameworks = []compliance.FrameworkID{compliance.FrameworkID("all")}
			break
		}
		frameworks = append(frameworks, compliance.FrameworkID(f))
	}

	// Parse checks filter
	var checks []string
	if complianceChecks != "" {
		for _, c := range strings.Split(complianceChecks, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				checks = append(checks, c)
			}
		}
	}

	opts := compliance.AuditOptions{
		RepoRoot:      repoRoot,
		Frameworks:    frameworks,
		Scope:         complianceScope,
		MinConfidence: complianceMinConf,
		SILLevel:      complianceSILLevel,
		Checks:        checks,
		FailOn:        complianceFailOn,
	}

	ctx := context.Background()
	report, err := compliance.RunAudit(ctx, opts, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Format output
	var output string
	switch OutputFormat(complianceFormat) {
	case FormatJSON:
		output, err = FormatResponse(report, FormatJSON)
	case FormatMarkdown:
		output = formatComplianceMarkdown(report)
	default:
		output = formatComplianceHuman(report)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Compliance audit completed",
		"frameworks", len(report.Frameworks),
		"findings", report.Summary.TotalFindings,
		"verdict", report.Verdict,
		"score", report.Score,
		"duration", time.Since(start).Milliseconds(),
	)

	// CI exit code
	if complianceCI {
		switch complianceFailOn {
		case "error":
			if report.Summary.BySeverity["error"] > 0 {
				os.Exit(1)
			}
		case "warning":
			if report.Summary.BySeverity["error"] > 0 || report.Summary.BySeverity["warning"] > 0 {
				os.Exit(1)
			}
		}
	}
}
