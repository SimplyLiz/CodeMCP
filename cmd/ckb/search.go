package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/query"
)

var (
	searchScope  string
	searchKinds  string
	searchLimit  int
	searchFormat string
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search for symbols",
	Long: `Search for symbols matching a query string.

Search semantics:
  - V1: Substring match, case-insensitive (no fuzzy matching)
  - Ranking: exact match bonus, visibility weight, kind priority

Examples:
  ckb search handleRequest
  ckb search handleRequest --scope=api-module
  ckb search handleRequest --kinds=function,method
  ckb search handleRequest --limit=10`,
	Args: cobra.ExactArgs(1),
	Run:  runSearch,
}

func init() {
	searchCmd.Flags().StringVar(&searchScope, "scope", "", "Limit search to module ID")
	searchCmd.Flags().StringVar(&searchKinds, "kinds", "", "Filter by kinds (comma-separated: class,function,method,etc)")
	searchCmd.Flags().IntVar(&searchLimit, "limit", 20, "Maximum number of results")
	searchCmd.Flags().StringVar(&searchFormat, "format", "json", "Output format (json, human)")
	rootCmd.AddCommand(searchCmd)
}

func runSearch(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(searchFormat)
	queryStr := args[0]

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	// Parse kinds filter
	var kindsFilter []string
	if searchKinds != "" {
		kindsFilter = strings.Split(searchKinds, ",")
	}

	// Search symbols using Query Engine
	opts := query.SearchSymbolsOptions{
		Query: queryStr,
		Scope: searchScope,
		Kinds: kindsFilter,
		Limit: searchLimit,
	}
	response, err := engine.SearchSymbols(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error searching symbols: %v\n", err)
		os.Exit(1)
	}

	// Convert to CLI response format
	cliResponse := convertSearchResponse(queryStr, response)

	// Format and output
	output, err := FormatResponse(cliResponse, OutputFormat(searchFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Search query completed",
		"query", queryStr,
		"results", len(response.Symbols),
		"duration", time.Since(start).Milliseconds(),
	)
}

// SearchResponseCLI contains search results for CLI output
type SearchResponseCLI struct {
	Query        string            `json:"query"`
	TotalMatches int               `json:"totalMatches"`
	Symbols      []SearchSymbolCLI `json:"symbols"`
	Provenance   *ProvenanceCLI    `json:"provenance,omitempty"`
}

// SearchSymbolCLI represents a symbol in search results
type SearchSymbolCLI struct {
	StableID             string       `json:"stableId"`
	Name                 string       `json:"name"`
	Kind                 string       `json:"kind"`
	SignatureNormalized  string       `json:"signatureNormalized,omitempty"`
	Visibility           string       `json:"visibility"`
	VisibilityConfidence float64      `json:"visibilityConfidence"`
	ContainerName        string       `json:"containerName,omitempty"`
	ModuleID             string       `json:"moduleId"`
	ModuleName           string       `json:"moduleName,omitempty"`
	Location             *LocationCLI `json:"location,omitempty"`
	RelevanceScore       float64      `json:"relevanceScore"`
}

func convertSearchResponse(queryStr string, resp *query.SearchSymbolsResponse) *SearchResponseCLI {
	symbols := make([]SearchSymbolCLI, 0, len(resp.Symbols))
	for _, s := range resp.Symbols {
		visibility := "unknown"
		visibilityConfidence := 0.0
		if s.Visibility != nil {
			visibility = s.Visibility.Visibility
			visibilityConfidence = s.Visibility.Confidence
		}

		sym := SearchSymbolCLI{
			StableID:             s.StableId,
			Name:                 s.Name,
			Kind:                 s.Kind,
			Visibility:           visibility,
			VisibilityConfidence: visibilityConfidence,
			ModuleID:             s.ModuleId,
			RelevanceScore:       s.Score,
		}

		if s.Location != nil {
			sym.Location = &LocationCLI{
				FileID:      s.Location.FileId,
				Path:        s.Location.FileId,
				StartLine:   s.Location.StartLine,
				StartColumn: s.Location.StartColumn,
			}
		}

		symbols = append(symbols, sym)
	}

	result := &SearchResponseCLI{
		Query:        queryStr,
		TotalMatches: resp.TotalCount,
		Symbols:      symbols,
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
