package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/query"
)

var (
	recentFormat       string
	recentModuleFilter string
	recentLimit        int
	recentTimeStart    string
	recentTimeEnd      string
)

var recentCmd = &cobra.Command{
	Use:   "recent",
	Short: "Find recently relevant files and symbols",
	Long: `Find what matters now - files/symbols with recent activity that may need attention.

Default time window is 7 days.

Examples:
  ckb recent
  ckb recent --module=internal/api
  ckb recent --limit=50
  ckb recent --start=2024-01-01 --end=2024-01-31
  ckb recent --format=human`,
	Run: runRecent,
}

func init() {
	recentCmd.Flags().StringVar(&recentFormat, "format", "json", "Output format (json, human)")
	recentCmd.Flags().StringVar(&recentModuleFilter, "module", "", "Module path to focus on")
	recentCmd.Flags().IntVar(&recentLimit, "limit", 20, "Maximum results to return")
	recentCmd.Flags().StringVar(&recentTimeStart, "start", "", "Start date (ISO8601 or YYYY-MM-DD)")
	recentCmd.Flags().StringVar(&recentTimeEnd, "end", "", "End date (ISO8601 or YYYY-MM-DD)")
	rootCmd.AddCommand(recentCmd)
}

func runRecent(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(recentFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	opts := query.RecentlyRelevantOptions{
		ModuleFilter: recentModuleFilter,
		Limit:        recentLimit,
	}

	if recentTimeStart != "" || recentTimeEnd != "" {
		opts.TimeWindow = &query.TimeWindowSelector{
			Start: recentTimeStart,
			End:   recentTimeEnd,
		}
	}

	response, err := engine.RecentlyRelevant(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting recent items: %v\n", err)
		os.Exit(1)
	}

	cliResponse := convertRecentResponse(response)

	output, err := FormatResponse(cliResponse, OutputFormat(recentFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Recent query completed", map[string]interface{}{
		"count":    len(response.Items),
		"duration": time.Since(start).Milliseconds(),
	})
}

// RecentResponseCLI contains recent items for CLI output
type RecentResponseCLI struct {
	Items       []RecentItemCLI `json:"items"`
	TotalCount  int             `json:"totalCount"`
	TimeWindow  string          `json:"timeWindow"`
	Confidence  float64         `json:"confidence"`
	Limitations []string        `json:"limitations,omitempty"`
	Provenance  *ProvenanceCLI  `json:"provenance,omitempty"`
}

type RecentItemCLI struct {
	Type         string   `json:"type"`
	Path         string   `json:"path,omitempty"`
	SymbolId     string   `json:"symbolId,omitempty"`
	Name         string   `json:"name"`
	LastModified string   `json:"lastModified"`
	ChangeCount  int      `json:"changeCount"`
	Authors      []string `json:"authors,omitempty"`
	Score        float64  `json:"score"`
}

func convertRecentResponse(resp *query.RecentlyRelevantResponse) *RecentResponseCLI {
	items := make([]RecentItemCLI, 0, len(resp.Items))
	for _, i := range resp.Items {
		item := RecentItemCLI{
			Type:         i.Type,
			Path:         i.Path,
			SymbolId:     i.SymbolId,
			Name:         i.Name,
			LastModified: i.LastModified,
			ChangeCount:  i.ChangeCount,
			Authors:      i.Authors,
		}
		if i.Ranking != nil {
			item.Score = i.Ranking.Score
		}
		items = append(items, item)
	}

	result := &RecentResponseCLI{
		Items:       items,
		TotalCount:  resp.TotalCount,
		TimeWindow:  resp.TimeWindow,
		Confidence:  resp.Confidence,
		Limitations: resp.Limitations,
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
