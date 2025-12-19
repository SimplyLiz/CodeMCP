package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/telemetry"
)

// Telemetry command flags
var (
	telemetryFormat       string
	usagePeriod           string
	usageIncludeCallers   bool
	deadCodeMinConfidence float64
	deadCodeLimit         int
)

var telemetryCmd = &cobra.Command{
	Use:   "telemetry",
	Short: "Telemetry and observed usage commands",
	Long: `Commands for working with runtime telemetry data.

These commands help you understand how your code is actually used at runtime
based on OpenTelemetry data ingested into CKB.`,
}

var telemetryStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show telemetry system status",
	Long: `Display the current status of the telemetry system.

Shows:
  - Whether telemetry is enabled
  - Last sync time and event counts
  - Coverage metrics
  - Unmapped services
  - Recommendations for improving coverage`,
	Run: runTelemetryStatus,
}

var telemetryUsageCmd = &cobra.Command{
	Use:   "usage <symbolId>",
	Short: "Get observed usage for a symbol",
	Long: `Retrieve runtime usage data for a specific symbol.

Returns:
  - Total and period call counts
  - First/last observed timestamps
  - Match quality and confidence
  - Calling services (if enabled)
  - Blended confidence (static + observed)

Examples:
  ckb telemetry usage symbol-123
  ckb telemetry usage symbol-123 --period=30d
  ckb telemetry usage symbol-123 --include-callers`,
	Args: cobra.ExactArgs(1),
	Run:  runTelemetryUsage,
}

var telemetryUnmappedCmd = &cobra.Command{
	Use:   "unmapped",
	Short: "List unmapped services",
	Long: `Show services that are sending telemetry but are not mapped to a repository.

These services need to be added to telemetry.serviceMap in your CKB config
for their events to be properly matched to symbols.`,
	Run: runTelemetryUnmapped,
}

var telemetryTestMapCmd = &cobra.Command{
	Use:   "test-map <serviceName>",
	Short: "Test service name mapping",
	Long: `Test how a service name would be resolved to a repository.

Use this to verify your service map configuration before deploying.

Examples:
  ckb telemetry test-map api-gateway
  ckb telemetry test-map payment-service`,
	Args: cobra.ExactArgs(1),
	Run:  runTelemetryTestMap,
}

var deadCodeCmd = &cobra.Command{
	Use:   "dead-code",
	Short: "Find dead code candidates",
	Long: `Find potential dead code based on telemetry analysis.

Identifies symbols that:
  - Have no observed runtime calls in the telemetry window
  - Meet coverage requirements (medium or higher)
  - Are not excluded by configured patterns

Important: This feature requires sufficient telemetry coverage.
Results include confidence scores and should be manually reviewed.

Examples:
  ckb dead-code
  ckb dead-code --min-confidence=0.8
  ckb dead-code --limit=50`,
	Run: runDeadCode,
}

func init() {
	// Telemetry status
	telemetryStatusCmd.Flags().StringVar(&telemetryFormat, "format", "json", "Output format (json, human)")

	// Telemetry usage
	telemetryUsageCmd.Flags().StringVar(&telemetryFormat, "format", "json", "Output format (json, human)")
	telemetryUsageCmd.Flags().StringVar(&usagePeriod, "period", "90d", "Time period (7d, 30d, 90d, all)")
	telemetryUsageCmd.Flags().BoolVar(&usageIncludeCallers, "include-callers", false, "Include caller breakdown")

	// Telemetry unmapped
	telemetryUnmappedCmd.Flags().StringVar(&telemetryFormat, "format", "json", "Output format (json, human)")

	// Telemetry test-map
	telemetryTestMapCmd.Flags().StringVar(&telemetryFormat, "format", "json", "Output format (json, human)")

	// Dead code
	deadCodeCmd.Flags().StringVar(&telemetryFormat, "format", "json", "Output format (json, human)")
	deadCodeCmd.Flags().Float64Var(&deadCodeMinConfidence, "min-confidence", 0.7, "Minimum confidence threshold")
	deadCodeCmd.Flags().IntVar(&deadCodeLimit, "limit", 100, "Maximum candidates to return")

	// Add subcommands to telemetry
	telemetryCmd.AddCommand(telemetryStatusCmd)
	telemetryCmd.AddCommand(telemetryUsageCmd)
	telemetryCmd.AddCommand(telemetryUnmappedCmd)
	telemetryCmd.AddCommand(telemetryTestMapCmd)

	// Add commands to root
	rootCmd.AddCommand(telemetryCmd)
	rootCmd.AddCommand(deadCodeCmd)
}

func runTelemetryStatus(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(telemetryFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	cfg := engine.GetConfig()
	if cfg == nil {
		fmt.Fprintln(os.Stderr, "Error: configuration not available")
		os.Exit(1)
	}

	status := TelemetryStatusCLI{
		Enabled: cfg.Telemetry.Enabled,
	}

	if !cfg.Telemetry.Enabled {
		status.Recommendations = []string{
			"Telemetry is disabled. Enable with: ckb config set telemetry.enabled true",
			"Configure service map to map service names to repository IDs",
		}
		output, _ := FormatResponse(status, OutputFormat(telemetryFormat))
		fmt.Println(output)
		return
	}

	// Get telemetry storage
	db := engine.GetDB()
	if db == nil {
		output, _ := FormatResponse(status, OutputFormat(telemetryFormat))
		fmt.Println(output)
		return
	}

	storage := telemetry.NewStorage(db.Conn())

	// Get last sync
	lastSync, err := storage.GetLastSyncLog()
	if err == nil && lastSync != nil {
		status.LastSync = &lastSync.StartedAt
	}

	// Get events
	status.EventsLast24h, _ = storage.GetEventsLast24h()
	activeSources, _ := storage.GetActiveSourcesLast24h()
	status.SourcesActive = len(activeSources)

	// Get match stats
	exact, strong, weak, unmatched, _ := storage.GetMatchStats()
	total := exact + strong + weak + unmatched
	if total > 0 {
		effectiveRate := float64(exact+strong) / float64(total)
		status.Coverage = &TelemetryCoverageCLI{
			ExactRate:     float64(exact) / float64(total),
			StrongRate:    float64(strong) / float64(total),
			WeakRate:      float64(weak) / float64(total),
			UnmatchedRate: float64(unmatched) / float64(total),
			EffectiveRate: effectiveRate,
			Level:         getCoverageLevel(effectiveRate),
		}
	}

	// Get unmapped services
	unmapped, _ := storage.GetUnmappedServices(10)
	status.ServiceMapMapped = len(cfg.Telemetry.ServiceMap)
	status.ServiceMapUnmapped = len(unmapped)
	if len(unmapped) > 0 {
		status.UnmappedServices = unmapped
	}

	// Build recommendations
	if status.Coverage != nil && status.Coverage.Level == "insufficient" {
		status.Recommendations = append(status.Recommendations,
			"Coverage is insufficient for dead code detection. Ensure telemetry events include file_path attribute.")
	}
	if status.ServiceMapUnmapped > 0 {
		status.Recommendations = append(status.Recommendations,
			fmt.Sprintf("%d services are unmapped. Add them to telemetry.serviceMap in config.", status.ServiceMapUnmapped))
	}
	if status.EventsLast24h == 0 {
		status.Recommendations = append(status.Recommendations,
			"No telemetry events received in last 24h. Check OTEL collector configuration.")
	}

	output, err := FormatResponse(status, OutputFormat(telemetryFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(output)

	logger.Debug("Telemetry status completed", map[string]interface{}{
		"duration": time.Since(start).Milliseconds(),
	})
}

func runTelemetryUsage(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(telemetryFormat)
	symbolID := args[0]

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	cfg := engine.GetConfig()
	if cfg == nil || !cfg.Telemetry.Enabled {
		fmt.Fprintln(os.Stderr, "Error: telemetry is not enabled")
		os.Exit(1)
	}

	db := engine.GetDB()
	if db == nil {
		fmt.Fprintln(os.Stderr, "Error: database not available")
		os.Exit(1)
	}

	storage := telemetry.NewStorage(db.Conn())

	// Compute period filter
	periodFilter := computePeriodFilterCLI(usagePeriod)

	// Get usage data
	usages, err := storage.GetObservedUsage(symbolID, periodFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting usage: %v\n", err)
		os.Exit(1)
	}

	response := TelemetryUsageCLI{
		SymbolID: symbolID,
	}

	if len(usages) > 0 {
		var totalCalls int64
		var firstObserved, lastObserved time.Time

		for i, u := range usages {
			totalCalls += u.CallCount
			if i == 0 {
				lastObserved = u.IngestedAt
			}
			firstObserved = u.IngestedAt
		}

		response.TotalCalls = totalCalls
		response.FirstObserved = &firstObserved
		response.LastObserved = &lastObserved
		response.MatchQuality = string(usages[0].MatchQuality)
	}

	// Get callers if requested
	if usageIncludeCallers && cfg.Telemetry.Aggregation.StoreCallers {
		callers, callersErr := storage.GetObservedCallers(symbolID, 10)
		if callersErr == nil {
			for _, c := range callers {
				response.Callers = append(response.Callers, CallerInfoCLI{
					Service:   c.CallerService,
					CallCount: c.CallCount,
				})
			}
		}
	}

	// Get static refs
	refs, _ := engine.GetReferenceCount(symbolID)
	response.StaticRefs = refs

	output, err := FormatResponse(response, OutputFormat(telemetryFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(output)

	logger.Debug("Telemetry usage completed", map[string]interface{}{
		"symbolId": symbolID,
		"duration": time.Since(start).Milliseconds(),
	})
}

func runTelemetryUnmapped(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(telemetryFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	cfg := engine.GetConfig()
	if cfg == nil || !cfg.Telemetry.Enabled {
		fmt.Fprintln(os.Stderr, "Error: telemetry is not enabled")
		os.Exit(1)
	}

	db := engine.GetDB()
	if db == nil {
		fmt.Fprintln(os.Stderr, "Error: database not available")
		os.Exit(1)
	}

	storage := telemetry.NewStorage(db.Conn())
	unmapped, err := storage.GetUnmappedServices(100)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting unmapped services: %v\n", err)
		os.Exit(1)
	}

	response := UnmappedServicesCLI{
		Services: unmapped,
		Total:    len(unmapped),
	}

	output, err := FormatResponse(response, OutputFormat(telemetryFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(output)

	logger.Debug("Telemetry unmapped completed", map[string]interface{}{
		"count":    len(unmapped),
		"duration": time.Since(start).Milliseconds(),
	})
}

func runTelemetryTestMap(cmd *cobra.Command, args []string) {
	logger := newLogger(telemetryFormat)
	serviceName := args[0]

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	cfg := engine.GetConfig()
	if cfg == nil {
		fmt.Fprintln(os.Stderr, "Error: configuration not available")
		os.Exit(1)
	}

	// Create service mapper
	mapper, err := telemetry.NewServiceMapper(cfg.Telemetry)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating service mapper: %v\n", err)
		os.Exit(1)
	}
	result := mapper.Resolve(serviceName)

	response := ServiceMapTestCLI{
		ServiceName: serviceName,
		Matched:     result.Matched,
		RepoID:      result.RepoID,
		MatchType:   result.MatchType,
	}

	output, err := FormatResponse(response, OutputFormat(telemetryFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(output)
}

func runDeadCode(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(telemetryFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	cfg := engine.GetConfig()
	if cfg == nil || !cfg.Telemetry.Enabled {
		fmt.Fprintln(os.Stderr, "Error: telemetry is not enabled")
		os.Exit(1)
	}

	db := engine.GetDB()
	if db == nil {
		fmt.Fprintln(os.Stderr, "Error: database not available")
		os.Exit(1)
	}

	storage := telemetry.NewStorage(db.Conn())

	// Get match stats for coverage
	exact, strong, weak, unmatched, _ := storage.GetMatchStats()
	total := exact + strong + weak + unmatched
	if total == 0 {
		response := DeadCodeCLI{
			Limitations: []string{"No telemetry data available"},
		}
		output, _ := FormatResponse(response, OutputFormat(telemetryFormat))
		fmt.Println(output)
		return
	}

	// Build coverage
	effectiveRate := float64(exact+strong) / float64(total)
	coverageLevel := getCoverageLevel(effectiveRate)

	coverage := telemetry.TelemetryCoverage{
		MatchCoverage: telemetry.MatchCoverage{
			Exact:         float64(exact) / float64(total),
			Strong:        float64(strong) / float64(total),
			Weak:          float64(weak) / float64(total),
			Unmatched:     float64(unmatched) / float64(total),
			EffectiveRate: effectiveRate,
		},
		Overall: telemetry.OverallCoverage{
			Score: effectiveRate,
			Level: telemetry.CoverageLevel(coverageLevel),
		},
	}

	// Check coverage requirement
	if !coverage.CanUseDeadCode() {
		response := DeadCodeCLI{
			CoverageLevel: coverageLevel,
			Limitations:   []string{fmt.Sprintf("Coverage level %s is below required medium", coverageLevel)},
		}
		output, _ := FormatResponse(response, OutputFormat(telemetryFormat))
		fmt.Println(output)
		return
	}

	// Get observation window
	observationDays, _ := storage.GetObservationWindowDays()
	if observationDays < cfg.Telemetry.DeadCode.MinObservationDays {
		response := DeadCodeCLI{
			CoverageLevel: coverageLevel,
			Limitations: []string{
				fmt.Sprintf("Only %d days of data (need %d)", observationDays, cfg.Telemetry.DeadCode.MinObservationDays),
			},
		}
		output, _ := FormatResponse(response, OutputFormat(telemetryFormat))
		fmt.Println(output)
		return
	}

	// Create detector
	options := telemetry.DefaultDeadCodeOptions(cfg.Telemetry.DeadCode)
	options.MinConfidence = deadCodeMinConfidence
	options.Limit = deadCodeLimit

	detector := telemetry.NewDeadCodeDetector(storage, coverage, options)

	// Get symbols from engine
	symbols, err := engine.GetAllSymbols()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting symbols: %v\n", err)
		os.Exit(1)
	}

	// Convert to SymbolInfo slice
	var symbolInfos []telemetry.SymbolInfo
	for _, sym := range symbols {
		refs, _ := engine.GetReferenceCount(sym.ID)
		symbolInfos = append(symbolInfos, telemetry.SymbolInfo{
			ID:         sym.ID,
			Name:       sym.Name,
			File:       sym.File,
			Kind:       sym.Kind,
			StaticRefs: refs,
		})
	}

	candidates := detector.FindCandidates(symbolInfos)

	// Build response
	response := DeadCodeCLI{
		CoverageLevel:   coverageLevel,
		TotalSymbols:    len(symbols),
		TotalCandidates: len(candidates),
	}

	for _, c := range candidates {
		response.Candidates = append(response.Candidates, DeadCodeCandidateCLI{
			SymbolID:          c.SymbolID,
			Name:              c.Name,
			File:              c.File,
			Confidence:        c.Confidence,
			StaticRefs:        c.StaticRefs,
			ObservationWindow: c.ObservationWindow,
			MatchQuality:      string(c.MatchQuality),
		})
	}

	output, err := FormatResponse(response, OutputFormat(telemetryFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(output)

	logger.Debug("Dead code analysis completed", map[string]interface{}{
		"totalSymbols":    len(symbols),
		"totalCandidates": len(candidates),
		"duration":        time.Since(start).Milliseconds(),
	})
}

// CLI response types

// TelemetryStatusCLI is the CLI response for telemetry status
type TelemetryStatusCLI struct {
	Enabled            bool                  `json:"enabled"`
	LastSync           *time.Time            `json:"lastSync,omitempty"`
	EventsLast24h      int64                 `json:"eventsLast24h,omitempty"`
	SourcesActive      int                   `json:"sourcesActive,omitempty"`
	Coverage           *TelemetryCoverageCLI `json:"coverage,omitempty"`
	ServiceMapMapped   int                   `json:"serviceMapMapped,omitempty"`
	ServiceMapUnmapped int                   `json:"serviceMapUnmapped,omitempty"`
	UnmappedServices   []string              `json:"unmappedServices,omitempty"`
	Recommendations    []string              `json:"recommendations,omitempty"`
}

// TelemetryCoverageCLI is coverage info for CLI
type TelemetryCoverageCLI struct {
	ExactRate     float64 `json:"exactRate"`
	StrongRate    float64 `json:"strongRate"`
	WeakRate      float64 `json:"weakRate"`
	UnmatchedRate float64 `json:"unmatchedRate"`
	EffectiveRate float64 `json:"effectiveRate"`
	Level         string  `json:"level"`
}

// TelemetryUsageCLI is the CLI response for usage
type TelemetryUsageCLI struct {
	SymbolID      string          `json:"symbolId"`
	TotalCalls    int64           `json:"totalCalls"`
	FirstObserved *time.Time      `json:"firstObserved,omitempty"`
	LastObserved  *time.Time      `json:"lastObserved,omitempty"`
	MatchQuality  string          `json:"matchQuality,omitempty"`
	StaticRefs    int             `json:"staticRefs"`
	Callers       []CallerInfoCLI `json:"callers,omitempty"`
}

// CallerInfoCLI is caller breakdown for CLI
type CallerInfoCLI struct {
	Service   string `json:"service"`
	CallCount int64  `json:"callCount"`
}

// UnmappedServicesCLI is the CLI response for unmapped services
type UnmappedServicesCLI struct {
	Services []string `json:"services"`
	Total    int      `json:"total"`
}

// ServiceMapTestCLI is the CLI response for test-map
type ServiceMapTestCLI struct {
	ServiceName string `json:"serviceName"`
	Matched     bool   `json:"matched"`
	RepoID      string `json:"repoId,omitempty"`
	MatchType   string `json:"matchType,omitempty"`
}

// DeadCodeCLI is the CLI response for dead-code
type DeadCodeCLI struct {
	CoverageLevel   string                 `json:"coverageLevel,omitempty"`
	TotalSymbols    int                    `json:"totalSymbols,omitempty"`
	TotalCandidates int                    `json:"totalCandidates,omitempty"`
	Candidates      []DeadCodeCandidateCLI `json:"candidates,omitempty"`
	Limitations     []string               `json:"limitations,omitempty"`
}

// DeadCodeCandidateCLI is a dead code candidate for CLI
type DeadCodeCandidateCLI struct {
	SymbolID          string  `json:"symbolId"`
	Name              string  `json:"name"`
	File              string  `json:"file"`
	Confidence        float64 `json:"confidence"`
	StaticRefs        int     `json:"staticRefs"`
	ObservationWindow int     `json:"observationWindow"`
	MatchQuality      string  `json:"matchQuality"`
}

// Helper functions

func getCoverageLevel(effectiveRate float64) string {
	if effectiveRate >= 0.8 {
		return "high"
	} else if effectiveRate >= 0.6 {
		return "medium"
	} else if effectiveRate >= 0.4 {
		return "low"
	}
	return "insufficient"
}

func computePeriodFilterCLI(period string) string {
	now := time.Now()
	switch period {
	case "7d":
		return now.AddDate(0, 0, -7).Format("2006-01-02")
	case "30d":
		return now.AddDate(0, 0, -30).Format("2006-01-02")
	case "90d":
		return now.AddDate(0, 0, -90).Format("2006-01-02")
	case "all":
		return ""
	default:
		return now.AddDate(0, 0, -90).Format("2006-01-02")
	}
}
