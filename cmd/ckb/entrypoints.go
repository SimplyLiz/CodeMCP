package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/query"
)

var (
	entrypointsFormat       string
	entrypointsModuleFilter string
	entrypointsLimit        int
)

var entrypointsCmd = &cobra.Command{
	Use:   "entrypoints",
	Short: "List system entrypoints (API handlers, CLI mains, jobs)",
	Long: `List system entrypoints with ranking signals.

Entrypoints include:
  - API handlers (HTTP routes, GraphQL resolvers)
  - CLI main functions
  - Background job handlers
  - Event handlers

Examples:
  ckb entrypoints
  ckb entrypoints --module=internal/api
  ckb entrypoints --limit=50
  ckb entrypoints --format=human`,
	Run: runEntrypoints,
}

func init() {
	entrypointsCmd.Flags().StringVar(&entrypointsFormat, "format", "json", "Output format (json, human)")
	entrypointsCmd.Flags().StringVar(&entrypointsModuleFilter, "module", "", "Filter to specific module")
	entrypointsCmd.Flags().IntVar(&entrypointsLimit, "limit", 30, "Maximum entrypoints to return")
	rootCmd.AddCommand(entrypointsCmd)
}

func runEntrypoints(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(entrypointsFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	opts := query.ListEntrypointsOptions{
		ModuleFilter: entrypointsModuleFilter,
		Limit:        entrypointsLimit,
	}
	response, err := engine.ListEntrypoints(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing entrypoints: %v\n", err)
		os.Exit(1)
	}

	cliResponse := convertEntrypointsResponse(response)

	output, err := FormatResponse(cliResponse, OutputFormat(entrypointsFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Entrypoints listing completed",
		"count", len(response.Entrypoints),
		"duration", time.Since(start).Milliseconds(),
	)
}

// EntrypointsResponseCLI contains entrypoints list for CLI output
type EntrypointsResponseCLI struct {
	Entrypoints []EntrypointDetailCLI `json:"entrypoints"`
	TotalCount  int                   `json:"totalCount"`
	Confidence  float64               `json:"confidence"`
	Warnings    []string              `json:"warnings,omitempty"`
	Provenance  *ProvenanceCLI        `json:"provenance,omitempty"`
}

type EntrypointDetailCLI struct {
	SymbolId       string       `json:"symbolId"`
	Name           string       `json:"name"`
	Type           string       `json:"type"`
	Location       *LocationCLI `json:"location,omitempty"`
	DetectionBasis string       `json:"detectionBasis"`
	FanOut         int          `json:"fanOut"`
	Score          float64      `json:"score"`
}

func convertEntrypointsResponse(resp *query.ListEntrypointsResponse) *EntrypointsResponseCLI {
	entrypoints := make([]EntrypointDetailCLI, 0, len(resp.Entrypoints))
	for _, e := range resp.Entrypoints {
		entry := EntrypointDetailCLI{
			SymbolId:       e.SymbolId,
			Name:           e.Name,
			Type:           e.Type,
			DetectionBasis: e.DetectionBasis,
			FanOut:         e.FanOut,
		}
		if e.Location != nil {
			entry.Location = &LocationCLI{
				FileID:      e.Location.FileId,
				Path:        e.Location.FileId,
				StartLine:   e.Location.StartLine,
				StartColumn: e.Location.StartColumn,
			}
		}
		if e.Ranking != nil {
			entry.Score = e.Ranking.Score
		}
		entrypoints = append(entrypoints, entry)
	}

	result := &EntrypointsResponseCLI{
		Entrypoints: entrypoints,
		TotalCount:  resp.TotalCount,
		Confidence:  resp.Confidence,
		Warnings:    resp.Warnings,
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
