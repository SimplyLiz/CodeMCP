package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/query"
)

var (
	statusFormat string
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show CKB system status",
	Long:  "Display the current status of CKB backends, cache, and repository state",
	Run:   runStatus,
}

func init() {
	statusCmd.Flags().StringVar(&statusFormat, "format", "human", "Output format (json, human)")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(statusFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	// Get status from Query Engine
	response, err := engine.GetStatus(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting status: %v\n", err)
		os.Exit(1)
	}

	// Convert to CLI response format
	cliResponse := convertStatusResponse(response)

	// Format and output
	output, err := FormatResponse(cliResponse, OutputFormat(statusFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	duration := time.Since(start).Milliseconds()
	if statusFormat == "human" {
		fmt.Printf("\n(Query took %dms)\n", duration)
	}
}

// StatusResponseCLI contains the complete system status for CLI output
type StatusResponseCLI struct {
	CkbVersion string              `json:"ckbVersion"`
	RepoState  *query.RepoState    `json:"repoState"`
	Backends   []BackendStatusCLI  `json:"backends"`
	Cache      CacheStatusCLI      `json:"cache"`
	Healthy    bool                `json:"healthy"`
}

// BackendStatusCLI describes the status of a backend
type BackendStatusCLI struct {
	ID           string   `json:"id"`
	Available    bool     `json:"available"`
	Healthy      bool     `json:"healthy"`
	Capabilities []string `json:"capabilities"`
	Details      string   `json:"details,omitempty"`
}

// CacheStatusCLI describes the cache state
type CacheStatusCLI struct {
	QueryCount int     `json:"queryCount"`
	ViewCount  int     `json:"viewCount"`
	HitRate    float64 `json:"hitRate"`
	SizeBytes  int64   `json:"sizeBytes"`
}

func convertStatusResponse(resp *query.StatusResponse) *StatusResponseCLI {
	backends := make([]BackendStatusCLI, 0, len(resp.Backends))
	for _, b := range resp.Backends {
		backends = append(backends, BackendStatusCLI{
			ID:           b.Id,
			Available:    b.Available,
			Healthy:      b.Healthy,
			Capabilities: b.Capabilities,
			Details:      b.Details,
		})
	}

	var cache CacheStatusCLI
	if resp.Cache != nil {
		cache = CacheStatusCLI{
			QueryCount: resp.Cache.QueriesCached,
			ViewCount:  resp.Cache.ViewsCached,
			HitRate:    resp.Cache.HitRate,
			SizeBytes:  resp.Cache.SizeBytes,
		}
	}

	return &StatusResponseCLI{
		CkbVersion: resp.CkbVersion,
		RepoState:  resp.RepoState,
		Backends:   backends,
		Cache:      cache,
		Healthy:    resp.Healthy,
	}
}
