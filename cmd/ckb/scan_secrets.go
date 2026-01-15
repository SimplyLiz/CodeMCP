package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/secrets"
	"ckb/internal/version"
)

var (
	secretsScope        string
	secretsPaths        []string
	secretsExclude      []string
	secretsMinSeverity  string
	secretsSinceCommit  string
	secretsMaxCommits   int
	secretsUseGitleaks  bool
	secretsUseTrufflehog bool
	secretsFormat       string
	secretsNoAllowlist  bool
)

var scanSecretsCmd = &cobra.Command{
	Use:   "scan-secrets",
	Short: "Scan for exposed secrets",
	Long: `Scan for exposed secrets (API keys, tokens, passwords) in the codebase.

Uses builtin pattern matching with entropy analysis, and optionally integrates
with external tools (gitleaks, trufflehog) for comprehensive coverage.

Scopes:
  workdir   - Scan current working directory files (default)
  staged    - Scan only git staged files (for pre-commit hooks)
  history   - Scan git commit history (slower, more thorough)

Examples:
  # Scan current directory
  ckb scan-secrets

  # Scan only high/critical severity
  ckb scan-secrets --min-severity=high

  # Scan staged files (useful for pre-commit hooks)
  ckb scan-secrets --scope=staged

  # Scan git history
  ckb scan-secrets --scope=history --max-commits=100

  # Scan specific paths
  ckb scan-secrets --paths="src/**/*.ts" --paths="config/*.json"

  # Use external tools for more patterns
  ckb scan-secrets --use-gitleaks --use-trufflehog`,
	Run: runScanSecrets,
}

func init() {
	scanSecretsCmd.Flags().StringVar(&secretsScope, "scope", "workdir", "Scan scope: workdir, staged, history")
	scanSecretsCmd.Flags().StringArrayVar(&secretsPaths, "paths", nil, "Limit scan to these paths (glob patterns)")
	scanSecretsCmd.Flags().StringArrayVar(&secretsExclude, "exclude", nil, "Exclude these paths from scan")
	scanSecretsCmd.Flags().StringVar(&secretsMinSeverity, "min-severity", "", "Minimum severity: critical, high, medium, low")
	scanSecretsCmd.Flags().StringVar(&secretsSinceCommit, "since", "", "For history scope: scan commits since this ref")
	scanSecretsCmd.Flags().IntVar(&secretsMaxCommits, "max-commits", 100, "For history scope: maximum commits to scan")
	scanSecretsCmd.Flags().BoolVar(&secretsUseGitleaks, "use-gitleaks", false, "Use gitleaks if available")
	scanSecretsCmd.Flags().BoolVar(&secretsUseTrufflehog, "use-trufflehog", false, "Use trufflehog if available")
	scanSecretsCmd.Flags().StringVarP(&secretsFormat, "output", "o", "json", "Output format: json, human, sarif")
	scanSecretsCmd.Flags().BoolVar(&secretsNoAllowlist, "no-allowlist", false, "Disable allowlist suppression")

	rootCmd.AddCommand(scanSecretsCmd)
}

func runScanSecrets(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(secretsFormat)

	repoRoot := mustGetRepoRoot()

	// Parse scope
	var scope secrets.ScanScope
	switch strings.ToLower(secretsScope) {
	case "workdir", "":
		scope = secrets.ScopeWorkdir
	case "staged":
		scope = secrets.ScopeStaged
	case "history":
		scope = secrets.ScopeHistory
	default:
		fmt.Fprintf(os.Stderr, "Invalid scope: %s (use: workdir, staged, history)\n", secretsScope)
		os.Exit(1)
	}

	// Parse severity
	var minSeverity secrets.Severity
	switch strings.ToLower(secretsMinSeverity) {
	case "critical":
		minSeverity = secrets.SeverityCritical
	case "high":
		minSeverity = secrets.SeverityHigh
	case "medium":
		minSeverity = secrets.SeverityMedium
	case "low":
		minSeverity = secrets.SeverityLow
	case "":
		// No filter
	default:
		fmt.Fprintf(os.Stderr, "Invalid severity: %s (use: critical, high, medium, low)\n", secretsMinSeverity)
		os.Exit(1)
	}

	// Build scan options
	opts := secrets.ScanOptions{
		RepoRoot:       repoRoot,
		Scope:          scope,
		Paths:          secretsPaths,
		ExcludePaths:   secretsExclude,
		MinSeverity:    minSeverity,
		SinceCommit:    secretsSinceCommit,
		MaxCommits:     secretsMaxCommits,
		UseGitleaks:    secretsUseGitleaks,
		UseTrufflehog:  secretsUseTrufflehog,
		ApplyAllowlist: !secretsNoAllowlist,
	}

	// Create scanner and run
	scanner := secrets.NewScanner(repoRoot, logger)
	ctx := context.Background()

	result, err := scanner.Scan(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning: %v\n", err)
		os.Exit(1)
	}

	// Output result
	switch secretsFormat {
	case "human":
		printHumanSecretResults(result)
	case "sarif":
		output, err := FormatSecretsAsSARIF(result, version.Version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting SARIF output: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(output)
	default: // json
		output, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(output))
	}

	logger.Debug("Secret scan completed",
		"findings", result.Summary.TotalFindings,
		"files", result.Summary.FilesWithSecrets,
		"duration", time.Since(start).Milliseconds(),
	)

	// Exit with error code if critical findings
	if result.Summary.BySeverity[secrets.SeverityCritical] > 0 {
		os.Exit(2)
	}
}

func printHumanSecretResults(result *secrets.ScanResult) {
	// Print header
	fmt.Printf("Secret Scan Results\n")
	fmt.Printf("==================\n\n")

	// Print summary
	fmt.Printf("Scanned:    %s (%s scope)\n", result.RepoRoot, result.Scope)
	fmt.Printf("Duration:   %s\n", result.Duration)
	fmt.Printf("Findings:   %d\n", result.Summary.TotalFindings)
	fmt.Printf("Files:      %d\n", result.Summary.FilesWithSecrets)

	if result.Suppressed > 0 {
		fmt.Printf("Suppressed: %d (via allowlist)\n", result.Suppressed)
	}
	fmt.Println()

	// Print severity breakdown
	if result.Summary.TotalFindings > 0 {
		fmt.Printf("By Severity:\n")
		if c := result.Summary.BySeverity[secrets.SeverityCritical]; c > 0 {
			fmt.Printf("  🔴 Critical: %d\n", c)
		}
		if h := result.Summary.BySeverity[secrets.SeverityHigh]; h > 0 {
			fmt.Printf("  🟠 High:     %d\n", h)
		}
		if m := result.Summary.BySeverity[secrets.SeverityMedium]; m > 0 {
			fmt.Printf("  🟡 Medium:   %d\n", m)
		}
		if l := result.Summary.BySeverity[secrets.SeverityLow]; l > 0 {
			fmt.Printf("  ⚪ Low:      %d\n", l)
		}
		fmt.Println()

		// Print findings
		fmt.Printf("Findings:\n")
		fmt.Printf("---------\n")
		for _, f := range result.Findings {
			icon := severityIcon(f.Severity)
			fmt.Printf("%s %s:%d\n", icon, f.File, f.Line)
			fmt.Printf("   Type:       %s\n", f.Type)
			fmt.Printf("   Severity:   %s\n", f.Severity)
			fmt.Printf("   Match:      %s\n", f.Match)
			fmt.Printf("   Rule:       %s\n", f.Rule)
			fmt.Printf("   Confidence: %.0f%%\n", f.Confidence*100)
			if f.Commit != "" {
				fmt.Printf("   Commit:     %s\n", f.Commit)
			}
			fmt.Println()
		}
	} else {
		fmt.Printf("✅ No secrets detected\n")
	}
}

func severityIcon(s secrets.Severity) string {
	switch s {
	case secrets.SeverityCritical:
		return "🔴"
	case secrets.SeverityHigh:
		return "🟠"
	case secrets.SeverityMedium:
		return "🟡"
	default:
		return "⚪"
	}
}
