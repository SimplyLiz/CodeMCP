package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/docs"
)

var (
	docsFormat      string
	docsForce       bool
	docsShowSymbols bool
	docsAll         bool
	docsExported    bool
	docsLimit       int
	docsFailUnder   float64
)

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Documentation ↔ symbol linking",
	Long: `Commands for linking documentation to code symbols.

CKB can automatically detect symbol references in markdown docs and link them
to the code. This enables "What docs mention this symbol?" and "Which docs
are stale?" queries.

Examples:
  ckb docs index              # Scan and index documentation
  ckb docs symbol Engine      # Find docs referencing Engine
  ckb docs file README.md     # Show symbols in a doc
  ckb docs stale --all        # Check all docs for stale refs`,
}

var docsIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Scan and index documentation",
	Long: `Scan markdown files for symbol references and index them.

By default, scans: docs/, README.md, ARCHITECTURE.md, DESIGN.md, CONTRIBUTING.md

Uses incremental indexing (skips unchanged files) unless --force is specified.

Examples:
  ckb docs index
  ckb docs index --force`,
	Run: runDocsIndex,
}

var docsSymbolCmd = &cobra.Command{
	Use:   "symbol <symbol>",
	Short: "Find docs referencing a symbol",
	Long: `Find all documentation that references a symbol.

The symbol can be a partial name (e.g., "UserService.Authenticate") or
a full SCIP symbol ID.

Examples:
  ckb docs symbol Engine.Start
  ckb docs symbol internal/query.Engine`,
	Args: cobra.ExactArgs(1),
	Run:  runDocsSymbol,
}

var docsFileCmd = &cobra.Command{
	Use:   "file <path>",
	Short: "Show info about a doc file",
	Long: `Show information about an indexed documentation file.

Use --symbols to list all symbol references in the doc.

Examples:
  ckb docs file README.md
  ckb docs file docs/architecture.md --symbols`,
	Args: cobra.ExactArgs(1),
	Run:  runDocsFile,
}

var docsModuleCmd = &cobra.Command{
	Use:   "module <module>",
	Short: "Find docs linked to a module",
	Long: `Find documentation explicitly linked to a module via directives.

Docs can be linked to modules using <!-- ckb:module path/to/module --> directives.

Examples:
  ckb docs module internal/query
  ckb docs module api`,
	Args: cobra.ExactArgs(1),
	Run:  runDocsModule,
}

var docsStaleCmd = &cobra.Command{
	Use:   "stale [path]",
	Short: "Check for stale symbol references",
	Long: `Check documentation for stale symbol references.

A reference is stale if:
- The symbol no longer exists in the index
- The reference is ambiguous (matches multiple symbols)
- The language is not indexed

Use --all to check all indexed docs, or specify a path.

Examples:
  ckb docs stale README.md
  ckb docs stale --all`,
	Run: runDocsStale,
}

var docsCoverageCmd = &cobra.Command{
	Use:   "coverage",
	Short: "Show documentation coverage",
	Long: `Show documentation coverage statistics.

Reports how many symbols are documented and which high-centrality
symbols are missing documentation.

Use --fail-under to enforce a minimum coverage threshold in CI.

Examples:
  ckb docs coverage
  ckb docs coverage --exported-only
  ckb docs coverage --fail-under=80  # Exit 1 if coverage < 80%`,
	Run: runDocsCoverage,
}

func init() {
	// Index flags
	docsIndexCmd.Flags().StringVar(&docsFormat, "format", "json", "Output format (json, human)")
	docsIndexCmd.Flags().BoolVar(&docsForce, "force", false, "Force re-index all docs")

	// Symbol flags
	docsSymbolCmd.Flags().StringVar(&docsFormat, "format", "json", "Output format (json, human)")
	docsSymbolCmd.Flags().IntVar(&docsLimit, "limit", 10, "Maximum results")

	// File flags
	docsFileCmd.Flags().StringVar(&docsFormat, "format", "json", "Output format (json, human)")
	docsFileCmd.Flags().BoolVar(&docsShowSymbols, "symbols", false, "List referenced symbols")

	// Module flags
	docsModuleCmd.Flags().StringVar(&docsFormat, "format", "json", "Output format (json, human)")

	// Stale flags
	docsStaleCmd.Flags().StringVar(&docsFormat, "format", "json", "Output format (json, human)")
	docsStaleCmd.Flags().BoolVar(&docsAll, "all", false, "Check all indexed docs")

	// Coverage flags
	docsCoverageCmd.Flags().StringVar(&docsFormat, "format", "json", "Output format (json, human)")
	docsCoverageCmd.Flags().BoolVar(&docsExported, "exported-only", false, "Only count exported symbols")
	docsCoverageCmd.Flags().IntVar(&docsLimit, "top", 10, "Number of top undocumented symbols")
	docsCoverageCmd.Flags().Float64Var(&docsFailUnder, "fail-under", 0, "Exit 1 if coverage below threshold (0-100)")

	docsCmd.AddCommand(docsIndexCmd)
	docsCmd.AddCommand(docsSymbolCmd)
	docsCmd.AddCommand(docsFileCmd)
	docsCmd.AddCommand(docsModuleCmd)
	docsCmd.AddCommand(docsStaleCmd)
	docsCmd.AddCommand(docsCoverageCmd)
	rootCmd.AddCommand(docsCmd)
}

func runDocsIndex(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(docsFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	stats, err := engine.IndexDocs(docsForce)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error indexing docs: %v\n", err)
		os.Exit(1)
	}

	response := &DocsIndexResponseCLI{
		DocsIndexed:     stats.DocsIndexed,
		DocsSkipped:     stats.DocsSkipped,
		ReferencesFound: stats.ReferencesFound,
		Resolved:        stats.Resolved,
		Ambiguous:       stats.Ambiguous,
		Missing:         stats.Missing,
		DurationMs:      time.Since(start).Milliseconds(),
	}

	output, err := FormatResponse(response, OutputFormat(docsFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)
}

func runDocsSymbol(cmd *cobra.Command, args []string) {
	logger := newLogger(docsFormat)
	symbol := args[0]

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	refs, err := engine.GetDocsForSymbol(symbol, docsLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding docs: %v\n", err)
		os.Exit(1)
	}

	response := &DocsSymbolResponseCLI{
		Symbol:     symbol,
		References: convertDocRefs(refs),
		Count:      len(refs),
	}

	output, err := FormatResponse(response, OutputFormat(docsFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)
}

func runDocsFile(cmd *cobra.Command, args []string) {
	logger := newLogger(docsFormat)
	path := args[0]

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	doc, err := engine.GetDocumentInfo(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting doc info: %v\n", err)
		os.Exit(1)
	}

	if doc == nil {
		fmt.Fprintf(os.Stderr, "Document not found: %s\n", path)
		fmt.Fprintf(os.Stderr, "Run 'ckb docs index' first.\n")
		os.Exit(1)
	}

	response := &DocsFileResponseCLI{
		Path:           doc.Path,
		Type:           string(doc.Type),
		Title:          doc.Title,
		ReferenceCount: len(doc.References),
		ModuleCount:    len(doc.Modules),
		LastIndexed:    doc.LastIndexed.Format(time.RFC3339),
	}

	if docsShowSymbols {
		for _, ref := range doc.References {
			response.Symbols = append(response.Symbols, DocRefCLI{
				RawText:    ref.RawText,
				Line:       ref.Line,
				Resolution: string(ref.Resolution),
				SymbolID:   ref.SymbolID,
			})
		}
	}

	output, err := FormatResponse(response, OutputFormat(docsFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)
}

func runDocsModule(cmd *cobra.Command, args []string) {
	logger := newLogger(docsFormat)
	moduleID := args[0]

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	docsList, err := engine.GetDocsForModule(moduleID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding docs: %v\n", err)
		os.Exit(1)
	}

	response := &DocsModuleResponseCLI{
		ModuleID: moduleID,
		Count:    len(docsList),
	}

	for _, doc := range docsList {
		response.Docs = append(response.Docs, DocSummaryCLI{
			Path:  doc.Path,
			Type:  string(doc.Type),
			Title: doc.Title,
		})
	}

	output, err := FormatResponse(response, OutputFormat(docsFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)
}

func runDocsStale(cmd *cobra.Command, args []string) {
	logger := newLogger(docsFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	var reports []docs.StalenessReport
	var err error

	if len(args) > 0 {
		report, e := engine.CheckDocStaleness(args[0])
		if e != nil {
			fmt.Fprintf(os.Stderr, "Error checking staleness: %v\n", e)
			os.Exit(1)
		}
		if report != nil {
			reports = []docs.StalenessReport{*report}
		}
	} else if docsAll {
		reports, err = engine.CheckAllDocsStaleness()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking staleness: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Specify a path or use --all\n")
		os.Exit(1)
	}

	response := &DocsStaleResponseCLI{}
	for _, r := range reports {
		if len(r.Stale) > 0 {
			response.Reports = append(response.Reports, convertStaleReport(r))
			response.TotalStale += len(r.Stale)
		}
	}

	output, err := FormatResponse(response, OutputFormat(docsFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)
}

func runDocsCoverage(cmd *cobra.Command, args []string) {
	logger := newLogger(docsFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	report, err := engine.GetDocCoverage(docsExported, docsLimit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting coverage: %v\n", err)
		os.Exit(1)
	}

	response := &DocsCoverageResponseCLI{
		TotalSymbols:    report.TotalSymbols,
		Documented:      report.Documented,
		Undocumented:    report.Undocumented,
		CoveragePercent: report.CoveragePercent,
	}

	for _, u := range report.TopUndocumented {
		response.TopUndocumented = append(response.TopUndocumented, UndocSymbolCLI{
			SymbolID:   u.SymbolID,
			Name:       u.Name,
			Centrality: u.Centrality,
		})
	}

	output, err := FormatResponse(response, OutputFormat(docsFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	// Check threshold after output (so CI tools can parse results even on failure)
	if docsFailUnder > 0 && report.CoveragePercent < docsFailUnder {
		fmt.Fprintf(os.Stderr, "\nCoverage %.1f%% is below threshold %.1f%%\n",
			report.CoveragePercent, docsFailUnder)
		os.Exit(1)
	}
}

// CLI Response types

type DocsIndexResponseCLI struct {
	DocsIndexed     int   `json:"docsIndexed"`
	DocsSkipped     int   `json:"docsSkipped"`
	ReferencesFound int   `json:"referencesFound"`
	Resolved        int   `json:"resolved"`
	Ambiguous       int   `json:"ambiguous"`
	Missing         int   `json:"missing"`
	DurationMs      int64 `json:"durationMs"`
}

type DocsSymbolResponseCLI struct {
	Symbol     string      `json:"symbol"`
	References []DocRefCLI `json:"references"`
	Count      int         `json:"count"`
}

type DocRefCLI struct {
	DocPath    string  `json:"docPath"`
	RawText    string  `json:"rawText"`
	Line       int     `json:"line"`
	Context    string  `json:"context,omitempty"`
	Resolution string  `json:"resolution"`
	SymbolID   *string `json:"symbolId,omitempty"`
}

type DocsFileResponseCLI struct {
	Path           string      `json:"path"`
	Type           string      `json:"type"`
	Title          string      `json:"title"`
	ReferenceCount int         `json:"referenceCount"`
	ModuleCount    int         `json:"moduleCount"`
	LastIndexed    string      `json:"lastIndexed"`
	Symbols        []DocRefCLI `json:"symbols,omitempty"`
}

type DocsModuleResponseCLI struct {
	ModuleID string          `json:"moduleId"`
	Docs     []DocSummaryCLI `json:"docs"`
	Count    int             `json:"count"`
}

type DocSummaryCLI struct {
	Path  string `json:"path"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

type DocsStaleResponseCLI struct {
	Reports    []StaleReportCLI `json:"reports"`
	TotalStale int              `json:"totalStale"`
}

type StaleReportCLI struct {
	DocPath         string        `json:"docPath"`
	TotalReferences int           `json:"totalReferences"`
	Valid           int           `json:"valid"`
	Stale           []StaleRefCLI `json:"stale"`
}

type StaleRefCLI struct {
	RawText     string   `json:"rawText"`
	Line        int      `json:"line"`
	Reason      string   `json:"reason"`
	Message     string   `json:"message"`
	Suggestions []string `json:"suggestions,omitempty"`
}

type DocsCoverageResponseCLI struct {
	TotalSymbols    int              `json:"totalSymbols"`
	Documented      int              `json:"documented"`
	Undocumented    int              `json:"undocumented"`
	CoveragePercent float64          `json:"coveragePercent"`
	TopUndocumented []UndocSymbolCLI `json:"topUndocumented,omitempty"`
}

type UndocSymbolCLI struct {
	SymbolID   string  `json:"symbolId"`
	Name       string  `json:"name"`
	Centrality float64 `json:"centrality"`
}

// Conversion helpers

func convertDocRefs(refs []docs.DocReference) []DocRefCLI {
	result := make([]DocRefCLI, 0, len(refs))
	for _, ref := range refs {
		result = append(result, DocRefCLI{
			DocPath:    ref.DocPath,
			RawText:    ref.RawText,
			Line:       ref.Line,
			Context:    truncateString(ref.Context, 100),
			Resolution: string(ref.Resolution),
			SymbolID:   ref.SymbolID,
		})
	}
	return result
}

func convertStaleReport(r docs.StalenessReport) StaleReportCLI {
	result := StaleReportCLI{
		DocPath:         r.DocPath,
		TotalReferences: r.TotalReferences,
		Valid:           r.Valid,
	}
	for _, s := range r.Stale {
		result.Stale = append(result.Stale, StaleRefCLI{
			RawText:     s.RawText,
			Line:        s.Line,
			Reason:      string(s.Reason),
			Message:     s.Message,
			Suggestions: limitStrings(s.Suggestions, 5),
		})
	}
	return result
}

func limitStrings(s []string, max int) []string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
