package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/project"
	"ckb/internal/query"
	"ckb/internal/tier"
)

var (
	doctorFix    bool
	doctorCheck  string
	doctorFormat string
	doctorTier   string
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose CKB issues",
	Long: `Diagnose CKB configuration and environment issues.

Use --tier to check requirements for a specific tier:
  ckb doctor --tier enhanced   # Check if enhanced tier tools are installed
  ckb doctor --tier full       # Check if full tier tools (including LSP) are installed

Use --fix to output a shell script with suggested fixes (does not auto-execute).`,
	Run: runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Output fix script (does not auto-execute)")
	doctorCmd.Flags().StringVar(&doctorCheck, "check", "", "Run specific check (git, scip, lsp, config, storage)")
	doctorCmd.Flags().StringVar(&doctorFormat, "format", "human", "Output format (json, human)")
	doctorCmd.Flags().StringVar(&doctorTier, "tier", "", "Check requirements for specific tier (basic, enhanced, full)")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) {
	start := time.Now()
	repoRoot := mustGetRepoRoot()

	// If --tier is specified, run tier-aware diagnostics
	if doctorTier != "" {
		runTierDoctor(repoRoot, doctorTier)
		return
	}

	// Otherwise run existing doctor diagnostics
	logger := newLogger(doctorFormat)
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	// Get doctor results from Query Engine
	response, err := engine.Doctor(ctx, doctorCheck)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running diagnostics: %v\n", err)
		os.Exit(1)
	}

	// If --fix requested, output fix script
	if doctorFix {
		script := engine.GenerateFixScript(response)
		fmt.Println(script)
		return
	}

	// Convert to CLI response format
	cliResponse := convertDoctorResponse(response)

	// Format and output
	output, err := FormatResponse(cliResponse, OutputFormat(doctorFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	duration := time.Since(start).Milliseconds()
	if doctorFormat == "human" {
		fmt.Printf("\n(Diagnostics took %dms)\n", duration)
	}

	// Exit with non-zero if unhealthy
	if !response.Healthy {
		os.Exit(1)
	}
}

// runTierDoctor runs tier-aware diagnostics.
func runTierDoctor(repoRoot, tierFlag string) {
	// Parse requested tier - accept both naming conventions
	var analysisTier tier.AnalysisTier
	switch tierFlag {
	case "basic", "fast":
		analysisTier = tier.TierBasic
	case "enhanced", "standard":
		analysisTier = tier.TierEnhanced
	case "full":
		analysisTier = tier.TierFull
	default:
		fmt.Fprintf(os.Stderr, "Error: invalid tier '%s': must be one of: basic, enhanced, full (or: fast, standard)\n", tierFlag)
		os.Exit(1)
	}

	// Detect languages in the workspace
	_, _, projectLangs := project.DetectAllLanguages(repoRoot)
	if len(projectLangs) == 0 {
		fmt.Fprintln(os.Stderr, "No supported languages detected in this project.")
		fmt.Fprintln(os.Stderr, "Run 'ckb doctor' for general diagnostics.")
		os.Exit(1)
	}

	// Convert project.Language to tier.Language
	var tierLangs []tier.Language
	for _, pl := range projectLangs {
		if tl, ok := tier.ParseLanguage(string(pl)); ok {
			tierLangs = append(tierLangs, tl)
		}
	}

	if len(tierLangs) == 0 {
		fmt.Fprintln(os.Stderr, "No tier-supported languages found.")
		os.Exit(1)
	}

	// Create detector and validator
	ctx := context.Background()
	runner := tier.NewCachingRunner(tier.NewRealRunner(5 * time.Second))
	detector := tier.NewToolDetector(runner, 5*time.Second)

	config := tier.ValidationConfig{
		RequestedTier:      analysisTier,
		AllowFallback:      true,
		CheckPrerequisites: true,
		WorkspaceRoot:      repoRoot,
	}
	validator := tier.NewValidator(detector, config)

	// Validate
	result := validator.Validate(ctx, tierLangs)

	// Output
	var format tier.OutputFormat
	if doctorFormat == "json" {
		format = tier.FormatJSON
	} else {
		format = tier.FormatHuman
	}

	if err := tier.DoctorOutput(os.Stdout, result, analysisTier, format); err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	// Determine exit code
	exitCode := tier.DetermineExitCode(result, true)
	if exitCode != tier.ExitSuccess && exitCode != tier.ExitDegraded {
		os.Exit(int(exitCode))
	}
}

// DoctorResponseCLI contains diagnostic results for CLI output
type DoctorResponseCLI struct {
	Healthy bool             `json:"healthy"`
	Checks  []DoctorCheckCLI `json:"checks"`
}

// DoctorCheckCLI represents a single diagnostic check
type DoctorCheckCLI struct {
	Name           string         `json:"name"`
	Status         string         `json:"status"` // "pass", "warn", "fail"
	Message        string         `json:"message"`
	SuggestedFixes []FixActionCLI `json:"suggestedFixes,omitempty"`
}

// FixActionCLI represents a suggested fix
type FixActionCLI struct {
	Type        string `json:"type"`
	Command     string `json:"command,omitempty"`
	Description string `json:"description"`
	Safe        bool   `json:"safe"`
}

func convertDoctorResponse(resp *query.DoctorResponse) *DoctorResponseCLI {
	checks := make([]DoctorCheckCLI, 0, len(resp.Checks))
	for _, c := range resp.Checks {
		fixes := make([]FixActionCLI, 0, len(c.SuggestedFixes))
		for _, f := range c.SuggestedFixes {
			fixes = append(fixes, FixActionCLI{
				Type:        f.Type,
				Command:     f.Command,
				Description: f.Description,
				Safe:        f.Safe,
			})
		}
		checks = append(checks, DoctorCheckCLI{
			Name:           c.Name,
			Status:         c.Status,
			Message:        c.Message,
			SuggestedFixes: fixes,
		})
	}

	return &DoctorResponseCLI{
		Healthy: resp.Healthy,
		Checks:  checks,
	}
}
