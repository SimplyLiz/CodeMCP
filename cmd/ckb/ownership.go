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

func init() {
	ownershipCmd.Flags().BoolVar(&ownershipIncludeBlame, "include-blame", true, "Include git-blame ownership analysis")
	ownershipCmd.Flags().BoolVar(&ownershipIncludeHistory, "include-history", false, "Include ownership change history")
	ownershipCmd.Flags().StringVar(&ownershipFormat, "format", "human", "Output format (json, human)")
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
