package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	metricsFormat string
	metricsDays   int
	metricsTool   string
)

var metricsCmd = &cobra.Command{
	Use:   "metrics",
	Short: "Show wide-result tool metrics",
	Long: `Display aggregated metrics for MCP wide-result tools.

Tracks truncation rates, execution times, and result counts for tools like
findReferences, searchSymbols, analyzeImpact, getCallGraph, getHotspots,
and summarizePr.

This data helps inform whether Frontier mode is needed for specific tools.

Examples:
  ckb metrics                    # Last 7 days
  ckb metrics --days=30          # Last 30 days
  ckb metrics --tool=findReferences
  ckb metrics --format=human`,
	Run: runMetrics,
}

func init() {
	metricsCmd.Flags().StringVar(&metricsFormat, "format", "json", "Output format (json, human)")
	metricsCmd.Flags().IntVar(&metricsDays, "days", 7, "Number of days to include (1-90)")
	metricsCmd.Flags().StringVar(&metricsTool, "tool", "", "Filter to specific tool")
	rootCmd.AddCommand(metricsCmd)
}

func runMetrics(cmd *cobra.Command, args []string) {
	logger := newLogger(metricsFormat)
	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	db := engine.DB()
	if db == nil {
		fmt.Fprintln(os.Stderr, "Error: database not available")
		os.Exit(1)
	}

	// Clamp days to reasonable range
	if metricsDays < 1 {
		metricsDays = 1
	}
	if metricsDays > 90 {
		metricsDays = 90
	}

	since := time.Now().AddDate(0, 0, -metricsDays)

	aggregates, err := db.GetWideResultAggregates(since)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting metrics: %v\n", err)
		os.Exit(1)
	}

	// Get table stats
	totalRecords, oldest, newest, err := db.GetWideResultStats()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting stats: %v\n", err)
		os.Exit(1)
	}

	response := MetricsResponseCLI{
		Period:       fmt.Sprintf("last %d days", metricsDays),
		Since:        since.Format("2006-01-02"),
		TotalRecords: totalRecords,
		Tools:        make([]ToolMetricsCLI, 0),
	}

	if oldest != nil {
		response.OldestRecord = oldest.Format("2006-01-02 15:04:05")
	}
	if newest != nil {
		response.NewestRecord = newest.Format("2006-01-02 15:04:05")
	}

	for name, agg := range aggregates {
		// Apply tool filter if specified
		if metricsTool != "" && name != metricsTool {
			continue
		}

		tool := ToolMetricsCLI{
			Name:             name,
			QueryCount:       agg.QueryCount,
			TotalResults:     agg.TotalResults,
			TotalReturned:    agg.TotalReturned,
			TotalTruncated:   agg.TotalTruncated,
			TruncationRate:   agg.AvgTruncationRate(),
			TotalTokens:      agg.TotalTokens,
			AvgTokens:        agg.AvgTokens(),
			TotalMs:          agg.TotalMs,
			AvgLatencyMs:     agg.AvgLatencyMs(),
			NeedsFrontier:    agg.AvgTruncationRate() > 0.30,
		}
		response.Tools = append(response.Tools, tool)
	}

	output, err := FormatResponse(response, OutputFormat(metricsFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)
}

// MetricsResponseCLI contains metrics summary for CLI output
type MetricsResponseCLI struct {
	Period       string           `json:"period"`
	Since        string           `json:"since"`
	TotalRecords int64            `json:"totalRecords"`
	OldestRecord string           `json:"oldestRecord,omitempty"`
	NewestRecord string           `json:"newestRecord,omitempty"`
	Tools        []ToolMetricsCLI `json:"tools"`
}

// ToolMetricsCLI contains per-tool metrics
type ToolMetricsCLI struct {
	Name           string  `json:"name"`
	QueryCount     int64   `json:"queryCount"`
	TotalResults   int64   `json:"totalResults"`
	TotalReturned  int64   `json:"totalReturned"`
	TotalTruncated int64   `json:"totalTruncated"`
	TruncationRate float64 `json:"truncationRate"`
	TotalTokens    int64   `json:"totalTokens"`
	AvgTokens      float64 `json:"avgTokens"`
	TotalMs        int64   `json:"totalMs"`
	AvgLatencyMs   float64 `json:"avgLatencyMs"`
	NeedsFrontier  bool    `json:"needsFrontier"`
}
