package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/query"
)

var (
	ownershipIncludeBlame   bool
	ownershipIncludeHistory bool
	ownershipFormat         string
	// Drift subcommand flags
	driftScope     string
	driftThreshold float64
	driftLimit     int
	driftFormat    string
)

var ownershipCmd = &cobra.Command{
	Use:   "ownership <path>",
	Short: "Get ownership information for a file or path",
	Long: `Query ownership information for a file or directory path.

Returns owners from CODEOWNERS rules and git-blame analysis with
confidence scores and time-weighted contributor rankings.

Examples:
  ckb ownership internal/api/handler.go
  ckb ownership internal/api/ --include-blame
  ckb ownership src/components --include-history
  ckb ownership . --format=json`,
	Args: cobra.ExactArgs(1),
	Run:  runOwnership,
}

var ownershipDriftCmd = &cobra.Command{
	Use:   "drift [scope]",
	Short: "Detect ownership drift between CODEOWNERS and git-blame",
	Long: `Detect ownership drift by comparing CODEOWNERS declarations against actual
git-blame ownership. Returns files where declared owners differ significantly
from who actually writes the code.

Examples:
  ckb ownership drift
  ckb ownership drift internal/api
  ckb ownership drift --threshold=0.5 --limit=50
  ckb ownership drift --format=json`,
	Args: cobra.MaximumNArgs(1),
	Run:  runOwnershipDrift,
}

func init() {
	ownershipCmd.Flags().BoolVar(&ownershipIncludeBlame, "include-blame", true, "Include git-blame ownership analysis")
	ownershipCmd.Flags().BoolVar(&ownershipIncludeHistory, "include-history", false, "Include ownership change history")
	ownershipCmd.Flags().StringVar(&ownershipFormat, "format", "human", "Output format (json, human)")

	// Drift subcommand flags
	ownershipDriftCmd.Flags().StringVar(&driftFormat, "format", "json", "Output format (json, human)")
	ownershipDriftCmd.Flags().Float64Var(&driftThreshold, "threshold", 0.3, "Drift score threshold to report (0-1)")
	ownershipDriftCmd.Flags().IntVar(&driftLimit, "limit", 20, "Maximum files to return")

	ownershipCmd.AddCommand(ownershipDriftCmd)
	rootCmd.AddCommand(ownershipCmd)
}

func runOwnership(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(ownershipFormat)

	path := args[0]

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := context.Background()

	opts := query.GetOwnershipOptions{
		Path:           path,
		IncludeBlame:   ownershipIncludeBlame,
		IncludeHistory: ownershipIncludeHistory,
	}

	response, err := engine.GetOwnership(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting ownership: %v\n", err)
		os.Exit(1)
	}

	// Format and output
	output, err := FormatResponse(response, OutputFormat(ownershipFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Ownership query completed", map[string]interface{}{
		"path":     path,
		"owners":   len(response.Owners),
		"duration": time.Since(start).Milliseconds(),
	})
}

func runOwnershipDrift(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(driftFormat)

	scope := ""
	if len(args) > 0 {
		scope = args[0]
	}

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := context.Background()

	opts := query.OwnershipDriftOptions{
		Scope:     scope,
		Threshold: driftThreshold,
		Limit:     driftLimit,
	}

	response, err := engine.GetOwnershipDrift(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting ownership drift: %v\n", err)
		os.Exit(1)
	}

	cliResponse := convertOwnershipDriftResponse(response)

	output, err := FormatResponse(cliResponse, OutputFormat(driftFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Ownership drift query completed", map[string]interface{}{
		"scope":           scope,
		"filesWithDrift":  response.Summary.FilesWithDrift,
		"duration":        time.Since(start).Milliseconds(),
	})
}

// OwnershipDriftResponseCLI contains ownership drift results for CLI output
type OwnershipDriftResponseCLI struct {
	Summary      DriftSummaryCLI  `json:"summary"`
	DriftedFiles []DriftedFileCLI `json:"driftedFiles"`
	Limitations  []string         `json:"limitations,omitempty"`
	Provenance   *ProvenanceCLI   `json:"provenance,omitempty"`
}

type DriftSummaryCLI struct {
	TotalFilesAnalyzed int     `json:"totalFilesAnalyzed"`
	FilesWithDrift     int     `json:"filesWithDrift"`
	AverageDriftScore  float64 `json:"averageDriftScore"`
	MostDriftedModule  string  `json:"mostDriftedModule,omitempty"`
}

type DriftedFileCLI struct {
	Path           string           `json:"path"`
	DriftScore     float64          `json:"driftScore"`
	DeclaredOwners []string         `json:"declaredOwners"`
	ActualOwners   []ActualOwnerCLI `json:"actualOwners"`
	Reason         string           `json:"reason"`
	Recommendation string           `json:"recommendation"`
}

type ActualOwnerCLI struct {
	ID         string  `json:"id"`
	Percentage float64 `json:"percentage"`
}

func convertOwnershipDriftResponse(resp *query.OwnershipDriftResponse) *OwnershipDriftResponseCLI {
	driftedFiles := make([]DriftedFileCLI, 0, len(resp.DriftedFiles))
	for _, f := range resp.DriftedFiles {
		actualOwners := make([]ActualOwnerCLI, 0, len(f.ActualOwners))
		for _, o := range f.ActualOwners {
			actualOwners = append(actualOwners, ActualOwnerCLI{
				ID:         o.ID,
				Percentage: o.Percentage,
			})
		}
		driftedFiles = append(driftedFiles, DriftedFileCLI{
			Path:           f.Path,
			DriftScore:     f.DriftScore,
			DeclaredOwners: f.DeclaredOwners,
			ActualOwners:   actualOwners,
			Reason:         f.Reason,
			Recommendation: f.Recommendation,
		})
	}

	result := &OwnershipDriftResponseCLI{
		Summary: DriftSummaryCLI{
			TotalFilesAnalyzed: resp.Summary.TotalFilesAnalyzed,
			FilesWithDrift:     resp.Summary.FilesWithDrift,
			AverageDriftScore:  resp.Summary.AverageDriftScore,
			MostDriftedModule:  resp.Summary.MostDriftedModule,
		},
		DriftedFiles: driftedFiles,
		Limitations:  resp.Limitations,
	}

	if resp.Provenance != nil {
		result.Provenance = &ProvenanceCLI{
			RepoStateId:     resp.Provenance.RepoStateId,
			RepoStateDirty:  resp.Provenance.RepoStateDirty,
			QueryDurationMs: resp.Provenance.QueryDurationMs,
		}
	}

	return result
}
