package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/audit"
)

var (
	auditMinScore  float64
	auditLimit     int
	auditFactor    string
	auditQuickWins bool
	auditFormat    string
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Find risky code",
	Long: `Find risky code based on multiple signals.

Analyzes the codebase for risk using weighted factors:
  - Complexity (20%): Cyclomatic/cognitive complexity
  - Test Coverage (20%): Presence of test files
  - Bus Factor (15%): Number of active contributors
  - Security Sensitivity (15%): Security-related keywords
  - Staleness (10%): Time since last modification
  - Error Rate (10%): Runtime error frequency (requires telemetry)
  - Co-Change Coupling (5%): Hidden dependencies
  - Churn (5%): Recent modification frequency

Examples:
  ckb audit
  ckb audit --min-score=60
  ckb audit --factor=bus_factor
  ckb audit --quick-wins`,
	Run: runAudit,
}

func init() {
	auditCmd.Flags().Float64Var(&auditMinScore, "min-score", 40, "Minimum risk score to include (0-100)")
	auditCmd.Flags().IntVar(&auditLimit, "limit", 50, "Maximum items to return")
	auditCmd.Flags().StringVar(&auditFactor, "factor", "", "Filter by specific risk factor")
	auditCmd.Flags().BoolVar(&auditQuickWins, "quick-wins", false, "Only show quick wins (low effort, high impact)")
	auditCmd.Flags().StringVar(&auditFormat, "format", "json", "Output format (json, human)")
	rootCmd.AddCommand(auditCmd)
}

func runAudit(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(auditFormat)

	repoRoot := mustGetRepoRoot()

	analyzer := audit.NewAnalyzer(repoRoot, logger)

	ctx := context.Background()
	result, err := analyzer.Analyze(ctx, audit.AuditOptions{
		RepoRoot:  repoRoot,
		MinScore:  auditMinScore,
		Limit:     auditLimit,
		Factor:    auditFactor,
		QuickWins: auditQuickWins,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error auditing: %v\n", err)
		os.Exit(1)
	}

	// If quick-wins mode, output only quick wins
	var outputData interface{}
	if auditQuickWins {
		outputData = map[string]interface{}{
			"quickWins": result.QuickWins,
			"summary":   result.Summary,
		}
	} else {
		outputData = result
	}

	// Format and output
	output, err := FormatResponse(outputData, OutputFormat(auditFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Audit completed", map[string]interface{}{
		"items":    len(result.Items),
		"critical": result.Summary.Critical,
		"high":     result.Summary.High,
		"duration": time.Since(start).Milliseconds(),
	})
}
