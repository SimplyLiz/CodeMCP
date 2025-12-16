package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/query"
)

var (
	doctorFix    bool
	doctorCheck  string
	doctorFormat string
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose CKB issues",
	Long: `Diagnose CKB configuration and environment issues.
Use --fix to output a shell script with suggested fixes (does not auto-execute).`,
	Run: runDoctor,
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorFix, "fix", false, "Output fix script (does not auto-execute)")
	doctorCmd.Flags().StringVar(&doctorCheck, "check", "", "Run specific check (git, scip, lsp, config, storage)")
	doctorCmd.Flags().StringVar(&doctorFormat, "format", "human", "Output format (json, human)")
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(doctorFormat)

	repoRoot := mustGetRepoRoot()
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
