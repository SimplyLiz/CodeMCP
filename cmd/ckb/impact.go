package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SimplyLiz/CodeMCP/internal/envelope"
	"github.com/SimplyLiz/CodeMCP/internal/query"
)

// CompactPrepareChange is a condensed view of prepareChange analysis,
// suitable for token-budget-constrained callers that do not need the full
// dependency listing.
type CompactPrepareChange struct {
	Target        string   `json:"target"`
	Risk          string   `json:"risk"`
	AffectedCount int      `json:"affected_count"`
	AffectedFiles []string `json:"affected_files"` // top 10
	TestsNeeded   []string `json:"tests_needed"`   // top 5
	OwnerSuggest  string   `json:"owner_suggest,omitempty"`
	Summary       string   `json:"summary"`
	Backend       string   `json:"backend"`
	Accuracy      string   `json:"accuracy"`
}

var (
	impactDepth        int
	impactIncludeTests bool
	impactFormat       string
	// Diff subcommand flags
	impactDiffStaged bool
	impactDiffBase   string
	impactDiffStrict bool
	// prepareChange subcommand flags
	prepareChangeFormat     string
	prepareChangeChangeType string
)

var impactCmd = &cobra.Command{
	Use:   "impact <symbolId>",
	Short: "Analyze change impact",
	Long: `Analyze the potential impact of changing a symbol.

Provides:
  - Direct dependents (symbols that reference this symbol)
  - Transitive impact (symbols affected through the dependency chain)
  - Impact by module
  - Risk assessment based on visibility and usage

Examples:
  ckb impact symbol-123
  ckb impact symbol-123 --depth=3
  ckb impact symbol-123 --include-tests`,
	Args: cobra.ExactArgs(1),
	Run:  runImpact,
}

var prepareChangeCmd = &cobra.Command{
	Use:   "prepare <target>",
	Short: "Pre-change analysis: blast radius, tests, coupling, and risk",
	Long: `Analyze what would break if you change a symbol or file.

Returns blast radius, affected tests, co-change coupling, and risk score.

Examples:
  ckb impact prepare symbol-123
  ckb impact prepare internal/foo/bar.go
  ckb impact prepare symbol-123 --format=compact`,
	Args: cobra.ExactArgs(1),
	Run:  runPrepareChange,
}

var impactDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Analyze impact of code changes",
	Long: `Analyze the impact of a set of code changes from git diff.

Answers three key questions:
  1. What downstream code might break?
  2. Which tests should I run?
  3. Who needs to review this?

Examples:
  ckb impact diff                    # Analyze current working tree changes
  ckb impact diff --staged           # Analyze only staged changes
  ckb impact diff --base=main        # Compare against main branch
  ckb impact diff --depth=3          # Deeper transitive analysis
  ckb impact diff --strict           # Fail if index is stale
  ckb impact diff --format=markdown  # Output as markdown for PR comments`,
	Run: runImpactDiff,
}

func init() {
	impactCmd.Flags().IntVar(&impactDepth, "depth", 2, "Maximum impact depth")
	impactCmd.Flags().BoolVar(&impactIncludeTests, "include-tests", false, "Include test dependencies")
	impactCmd.Flags().StringVar(&impactFormat, "format", "human", "Output format (human, json)")

	// Diff subcommand flags
	impactDiffCmd.Flags().BoolVar(&impactDiffStaged, "staged", false, "Analyze only staged changes (--cached)")
	impactDiffCmd.Flags().StringVar(&impactDiffBase, "base", "HEAD", "Base branch for comparison")
	impactDiffCmd.Flags().IntVar(&impactDepth, "depth", 2, "Maximum depth for transitive impact (1-4)")
	impactDiffCmd.Flags().BoolVar(&impactIncludeTests, "include-tests", false, "Include test files in analysis")
	impactDiffCmd.Flags().BoolVar(&impactDiffStrict, "strict", false, "Fail if SCIP index is stale")
	impactDiffCmd.Flags().StringVar(&impactFormat, "format", "human", "Output format (json, human, markdown)")

	// prepareChange subcommand flags
	prepareChangeCmd.Flags().StringVar(&prepareChangeFormat, "format", "full", "Output format (full, compact)")
	prepareChangeCmd.Flags().StringVar(&prepareChangeChangeType, "change-type", "modify", "Change type (modify, rename, delete, extract, move)")

	impactCmd.AddCommand(impactDiffCmd)
	impactCmd.AddCommand(prepareChangeCmd)
	rootCmd.AddCommand(impactCmd)
}

func runPrepareChange(cmd *cobra.Command, args []string) {
	logger := newLogger(prepareChangeFormat)
	target := args[0]

	repoRoot := mustGetRepoRoot()
	eng := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	changeType := query.ChangeModify
	switch prepareChangeChangeType {
	case "rename":
		changeType = query.ChangeRename
	case "delete":
		changeType = query.ChangeDelete
	case "extract":
		changeType = query.ChangeExtract
	case "move":
		changeType = query.ChangeMove
	}

	result, err := eng.PrepareChange(ctx, query.PrepareChangeOptions{
		Target:     target,
		ChangeType: changeType,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if prepareChangeFormat == "compact" {
		activeBackend := eng.ActiveBackendName()
		compact := buildCompactPrepareChange(target, result, activeBackend)
		var out []byte
		out, err = json.MarshalIndent(compact, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error serializing output: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(out))
		return
	}

	// full format — reuse standard JSON/human output
	out, err := FormatResponse(result, OutputFormat(impactFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(out)
}

// buildCompactPrepareChange builds a CompactPrepareChange from a PrepareChangeResponse.
func buildCompactPrepareChange(target string, r *query.PrepareChangeResponse, activeBackend string) CompactPrepareChange {
	risk := "unknown"
	if r.RiskAssessment != nil {
		risk = r.RiskAssessment.Level
	}

	// Collect unique affected files from direct dependents (top 10)
	seen := make(map[string]bool)
	var affectedFiles []string
	for _, dep := range r.DirectDependents {
		if dep.File != "" && !seen[dep.File] {
			seen[dep.File] = true
			affectedFiles = append(affectedFiles, dep.File)
		}
		if len(affectedFiles) >= 10 {
			break
		}
	}

	affectedCount := len(r.DirectDependents)
	if r.TransitiveImpact != nil {
		affectedCount += r.TransitiveImpact.TotalCallers
	}

	// Top 5 tests
	var testsNeeded []string
	for i, t := range r.RelatedTests {
		if i >= 5 {
			break
		}
		name := t.File
		if t.Name != "" {
			name = t.Name
		}
		testsNeeded = append(testsNeeded, name)
	}

	summary := fmt.Sprintf("Changing %s affects %d files with %s risk.", target, len(affectedFiles), risk)

	return CompactPrepareChange{
		Target:        target,
		Risk:          risk,
		AffectedCount: affectedCount,
		AffectedFiles: affectedFiles,
		TestsNeeded:   testsNeeded,
		Summary:       summary,
		Backend:       activeBackend,
		Accuracy:      envelope.AccuracyForBackend(activeBackend),
	}
}

// formatImpactSubcommandError returns an error message when user provides
// a subcommand name instead of a symbol ID.
func formatImpactSubcommandError(arg string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Error: '%s' is not a valid symbol ID.\n\n", arg))
	if arg == "diff" {
		b.WriteString("Did you mean: ckb impact diff\n")
		b.WriteString("  Analyzes impact of code changes from git diff\n")
	}
	b.WriteString("\nTo analyze a specific symbol, provide its ID:\n")
	b.WriteString("  ckb impact <symbolId>\n")
	b.WriteString("\nTo find symbol IDs, use:\n")
	b.WriteString("  ckb search <name>\n")
	return b.String()
}

// formatSymbolNotFoundError returns an error message when a symbol is not found.
func formatSymbolNotFoundError(symbolID string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Error: Symbol not found: %s\n\n", symbolID))
	b.WriteString("To find valid symbol IDs, use:\n")
	b.WriteString("  ckb search <name>\n")
	return b.String()
}

func runImpact(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(impactFormat)
	symbolID := args[0]

	// Check if user might have meant a subcommand
	if symbolID == "diff" || symbolID == "help" {
		fmt.Fprint(os.Stderr, formatImpactSubcommandError(symbolID))
		os.Exit(1)
	}

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	// Analyze impact using Query Engine
	opts := query.AnalyzeImpactOptions{
		SymbolId:     symbolID,
		Depth:        impactDepth,
		IncludeTests: impactIncludeTests,
	}
	response, err := engine.AnalyzeImpact(ctx, opts)
	if err != nil {
		// Provide helpful error for symbol not found
		if strings.Contains(err.Error(), "not found") {
			fmt.Fprint(os.Stderr, formatSymbolNotFoundError(symbolID))
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error analyzing impact: %v\n", err)
		os.Exit(1)
	}

	// Convert to CLI response format
	cliResponse := convertImpactResponse(symbolID, response)

	// Format and output
	output, err := FormatResponse(cliResponse, OutputFormat(impactFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Impact analysis completed",
		"symbolId", symbolID,
		"direct", len(response.DirectImpact),
		"duration", time.Since(start).Milliseconds(),
	)
}

// ImpactResponseCLI contains impact analysis results for CLI output
type ImpactResponseCLI struct {
	SymbolID         string            `json:"symbolId"`
	Symbol           *SymbolInfoCLI    `json:"symbol,omitempty"`
	RiskScore        *RiskScoreCLI     `json:"riskScore,omitempty"`
	DirectImpact     []ImpactItemCLI   `json:"directImpact"`
	TransitiveImpact []ImpactItemCLI   `json:"transitiveImpact,omitempty"`
	ModulesAffected  []ModuleImpactCLI `json:"modulesAffected"`
	Provenance       *ProvenanceCLI    `json:"provenance,omitempty"`
}

// RiskScoreCLI describes risk assessment
type RiskScoreCLI struct {
	Level       string          `json:"level"`
	Score       float64         `json:"score"`
	Explanation string          `json:"explanation"`
	Factors     []RiskFactorCLI `json:"factors,omitempty"`
}

// RiskFactorCLI describes a risk factor
type RiskFactorCLI struct {
	Name   string  `json:"name"`
	Value  float64 `json:"value"`
	Weight float64 `json:"weight"`
}

// ImpactItemCLI represents an affected symbol
type ImpactItemCLI struct {
	StableID   string       `json:"stableId"`
	Name       string       `json:"name,omitempty"`
	Kind       string       `json:"kind"`
	Distance   int          `json:"distance"`
	ModuleID   string       `json:"moduleId"`
	Location   *LocationCLI `json:"location,omitempty"`
	Confidence float64      `json:"confidence"`
}

// ModuleImpactCLI shows impact on a specific module
type ModuleImpactCLI struct {
	ModuleID    string `json:"moduleId"`
	ModuleName  string `json:"moduleName,omitempty"`
	ImpactCount int    `json:"impactCount"`
	DirectCount int    `json:"directCount,omitempty"`
}

func convertImpactResponse(symbolID string, resp *query.AnalyzeImpactResponse) *ImpactResponseCLI {
	directImpact := make([]ImpactItemCLI, 0, len(resp.DirectImpact))
	for _, item := range resp.DirectImpact {
		impactItem := ImpactItemCLI{
			StableID:   item.StableId,
			Name:       item.Name,
			Kind:       item.Kind,
			Distance:   item.Distance,
			ModuleID:   item.ModuleId,
			Confidence: item.Confidence,
		}
		if item.Location != nil {
			impactItem.Location = &LocationCLI{
				FileID:      item.Location.FileId,
				Path:        item.Location.FileId,
				StartLine:   item.Location.StartLine,
				StartColumn: item.Location.StartColumn,
			}
		}
		directImpact = append(directImpact, impactItem)
	}

	transitiveImpact := make([]ImpactItemCLI, 0, len(resp.TransitiveImpact))
	for _, item := range resp.TransitiveImpact {
		impactItem := ImpactItemCLI{
			StableID:   item.StableId,
			Name:       item.Name,
			Kind:       item.Kind,
			Distance:   item.Distance,
			ModuleID:   item.ModuleId,
			Confidence: item.Confidence,
		}
		if item.Location != nil {
			impactItem.Location = &LocationCLI{
				FileID:      item.Location.FileId,
				Path:        item.Location.FileId,
				StartLine:   item.Location.StartLine,
				StartColumn: item.Location.StartColumn,
			}
		}
		transitiveImpact = append(transitiveImpact, impactItem)
	}

	modulesAffected := make([]ModuleImpactCLI, 0, len(resp.ModulesAffected))
	for _, m := range resp.ModulesAffected {
		modulesAffected = append(modulesAffected, ModuleImpactCLI{
			ModuleID:    m.ModuleId,
			ModuleName:  m.Name,
			ImpactCount: m.ImpactCount,
			DirectCount: m.DirectCount,
		})
	}

	result := &ImpactResponseCLI{
		SymbolID:         symbolID,
		DirectImpact:     directImpact,
		TransitiveImpact: transitiveImpact,
		ModulesAffected:  modulesAffected,
	}

	if resp.Symbol != nil {
		visibility := "unknown"
		visibilityConfidence := 0.0
		if resp.Symbol.Visibility != nil {
			visibility = resp.Symbol.Visibility.Visibility
			visibilityConfidence = resp.Symbol.Visibility.Confidence
		}
		result.Symbol = &SymbolInfoCLI{
			StableID:             resp.Symbol.StableId,
			Name:                 resp.Symbol.Name,
			Kind:                 resp.Symbol.Kind,
			Visibility:           visibility,
			VisibilityConfidence: visibilityConfidence,
		}
	}

	if resp.RiskScore != nil {
		factors := make([]RiskFactorCLI, 0, len(resp.RiskScore.Factors))
		for _, f := range resp.RiskScore.Factors {
			factors = append(factors, RiskFactorCLI{
				Name:   f.Name,
				Value:  f.Value,
				Weight: f.Weight,
			})
		}
		result.RiskScore = &RiskScoreCLI{
			Level:       resp.RiskScore.Level,
			Score:       resp.RiskScore.Score,
			Explanation: resp.RiskScore.Explanation,
			Factors:     factors,
		}
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

func runImpactDiff(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(impactFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	// Analyze change set using Query Engine
	opts := query.AnalyzeChangeSetOptions{
		Staged:          impactDiffStaged,
		BaseBranch:      impactDiffBase,
		TransitiveDepth: impactDepth,
		IncludeTests:    impactIncludeTests,
		Strict:          impactDiffStrict,
	}
	response, err := engine.AnalyzeChangeSet(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error analyzing change impact: %v\n", err)
		os.Exit(1)
	}

	// Convert to CLI response format
	cliResponse := convertChangeSetResponse(response)

	// Format and output
	var output string
	if impactFormat == "markdown" {
		output = formatImpactMarkdown(cliResponse)
	} else {
		output, err = FormatResponse(cliResponse, OutputFormat(impactFormat))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Println(output)

	logger.Debug("Change impact analysis completed",
		"filesChanged", response.Summary.FilesChanged,
		"symbolsChanged", response.Summary.SymbolsChanged,
		"riskLevel", response.Summary.EstimatedRisk,
		"duration", time.Since(start).Milliseconds(),
	)
}

// ChangeSetResponseCLI contains change set analysis results for CLI output
type ChangeSetResponseCLI struct {
	Summary         *ChangeSummaryCLI   `json:"summary"`
	ChangedSymbols  []ChangedSymbolCLI  `json:"changedSymbols"`
	AffectedSymbols []ImpactItemCLI     `json:"affectedSymbols"`
	ModulesAffected []ModuleImpactCLI   `json:"modulesAffected"`
	BlastRadius     *BlastRadiusCLI     `json:"blastRadius,omitempty"`
	RiskScore       *RiskScoreCLI       `json:"riskScore,omitempty"`
	Recommendations []RecommendationCLI `json:"recommendations,omitempty"`
	IndexStaleness  *IndexStalenessCLI  `json:"indexStaleness,omitempty"`
	Provenance      *ProvenanceCLI      `json:"provenance,omitempty"`
}

// ChangeSummaryCLI provides a high-level overview of changes
type ChangeSummaryCLI struct {
	FilesChanged         int    `json:"filesChanged"`
	SymbolsChanged       int    `json:"symbolsChanged"`
	DirectlyAffected     int    `json:"directlyAffected"`
	TransitivelyAffected int    `json:"transitivelyAffected"`
	EstimatedRisk        string `json:"estimatedRisk"`
}

// ChangedSymbolCLI represents a symbol that was changed
type ChangedSymbolCLI struct {
	SymbolID   string  `json:"symbolId"`
	Name       string  `json:"name"`
	File       string  `json:"file"`
	ChangeType string  `json:"changeType"`
	Lines      []int   `json:"lines,omitempty"`
	Confidence float64 `json:"confidence"`
}

// BlastRadiusCLI summarizes the impact spread
type BlastRadiusCLI struct {
	ModuleCount       int    `json:"moduleCount"`
	FileCount         int    `json:"fileCount"`
	UniqueCallerCount int    `json:"uniqueCallerCount"`
	RiskLevel         string `json:"riskLevel"`
}

// RecommendationCLI represents a suggested action
type RecommendationCLI struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Action   string `json:"action,omitempty"`
}

// IndexStalenessCLI provides index freshness information
type IndexStalenessCLI struct {
	IsStale          bool   `json:"isStale"`
	CommitsBehind    int    `json:"commitsBehind,omitempty"`
	IndexedCommit    string `json:"indexedCommit,omitempty"`
	HeadCommit       string `json:"headCommit,omitempty"`
	StalenessMessage string `json:"stalenessMessage,omitempty"`
}

func convertChangeSetResponse(resp *query.AnalyzeChangeSetResponse) *ChangeSetResponseCLI {
	// Convert changed symbols
	changedSymbols := make([]ChangedSymbolCLI, 0, len(resp.ChangedSymbols))
	for _, sym := range resp.ChangedSymbols {
		changedSymbols = append(changedSymbols, ChangedSymbolCLI{
			SymbolID:   sym.SymbolID,
			Name:       sym.Name,
			File:       sym.File,
			ChangeType: sym.ChangeType,
			Lines:      sym.Lines,
			Confidence: sym.Confidence,
		})
	}

	// Convert affected symbols
	affectedSymbols := make([]ImpactItemCLI, 0, len(resp.AffectedSymbols))
	for _, item := range resp.AffectedSymbols {
		impactItem := ImpactItemCLI{
			StableID:   item.StableId,
			Name:       item.Name,
			Kind:       item.Kind,
			Distance:   item.Distance,
			ModuleID:   item.ModuleId,
			Confidence: item.Confidence,
		}
		if item.Location != nil {
			impactItem.Location = &LocationCLI{
				FileID:      item.Location.FileId,
				Path:        item.Location.FileId,
				StartLine:   item.Location.StartLine,
				StartColumn: item.Location.StartColumn,
			}
		}
		affectedSymbols = append(affectedSymbols, impactItem)
	}

	// Convert modules affected
	modulesAffected := make([]ModuleImpactCLI, 0, len(resp.ModulesAffected))
	for _, m := range resp.ModulesAffected {
		modulesAffected = append(modulesAffected, ModuleImpactCLI{
			ModuleID:    m.ModuleId,
			ModuleName:  m.Name,
			ImpactCount: m.ImpactCount,
			DirectCount: m.DirectCount,
		})
	}

	// Convert recommendations
	recommendations := make([]RecommendationCLI, 0, len(resp.Recommendations))
	for _, rec := range resp.Recommendations {
		recommendations = append(recommendations, RecommendationCLI{
			Type:     rec.Type,
			Severity: rec.Severity,
			Message:  rec.Message,
			Action:   rec.Action,
		})
	}

	result := &ChangeSetResponseCLI{
		ChangedSymbols:  changedSymbols,
		AffectedSymbols: affectedSymbols,
		ModulesAffected: modulesAffected,
		Recommendations: recommendations,
	}

	// Convert summary
	if resp.Summary != nil {
		result.Summary = &ChangeSummaryCLI{
			FilesChanged:         resp.Summary.FilesChanged,
			SymbolsChanged:       resp.Summary.SymbolsChanged,
			DirectlyAffected:     resp.Summary.DirectlyAffected,
			TransitivelyAffected: resp.Summary.TransitivelyAffected,
			EstimatedRisk:        resp.Summary.EstimatedRisk,
		}
	}

	// Convert blast radius
	if resp.BlastRadius != nil {
		result.BlastRadius = &BlastRadiusCLI{
			ModuleCount:       resp.BlastRadius.ModuleCount,
			FileCount:         resp.BlastRadius.FileCount,
			UniqueCallerCount: resp.BlastRadius.UniqueCallerCount,
			RiskLevel:         resp.BlastRadius.RiskLevel,
		}
	}

	// Convert risk score
	if resp.RiskScore != nil {
		factors := make([]RiskFactorCLI, 0, len(resp.RiskScore.Factors))
		for _, f := range resp.RiskScore.Factors {
			factors = append(factors, RiskFactorCLI{
				Name:   f.Name,
				Value:  f.Value,
				Weight: f.Weight,
			})
		}
		result.RiskScore = &RiskScoreCLI{
			Level:       resp.RiskScore.Level,
			Score:       resp.RiskScore.Score,
			Explanation: resp.RiskScore.Explanation,
			Factors:     factors,
		}
	}

	// Convert index staleness
	if resp.IndexStaleness != nil {
		result.IndexStaleness = &IndexStalenessCLI{
			IsStale:          resp.IndexStaleness.IsStale,
			CommitsBehind:    resp.IndexStaleness.CommitsBehind,
			IndexedCommit:    resp.IndexStaleness.IndexedCommit,
			HeadCommit:       resp.IndexStaleness.HeadCommit,
			StalenessMessage: resp.IndexStaleness.StalenessMessage,
		}
	}

	// Convert provenance
	if resp.Provenance != nil {
		result.Provenance = &ProvenanceCLI{
			RepoStateId:     resp.Provenance.RepoStateId,
			RepoStateDirty:  resp.Provenance.RepoStateDirty,
			QueryDurationMs: resp.Provenance.QueryDurationMs,
		}
	}

	return result
}

// formatImpactMarkdown generates a markdown report for PR comments
func formatImpactMarkdown(resp *ChangeSetResponseCLI) string {
	var b strings.Builder

	// Header with risk badge
	riskEmoji := map[string]string{
		"critical": "🔴",
		"high":     "🟠",
		"medium":   "🟡",
		"low":      "🟢",
	}
	risk := "unknown"
	emoji := "⚪"
	if resp.Summary != nil {
		risk = resp.Summary.EstimatedRisk
		if e, ok := riskEmoji[risk]; ok {
			emoji = e
		}
	}

	b.WriteString(fmt.Sprintf("## %s Change Impact Analysis\n\n", emoji))

	// Summary table
	if resp.Summary != nil {
		s := resp.Summary
		b.WriteString("| Metric | Value |\n")
		b.WriteString("|:-------|------:|\n")
		b.WriteString(fmt.Sprintf("| **Risk Level** | **%s** %s |\n", strings.ToUpper(risk), emoji))
		b.WriteString(fmt.Sprintf("| Files Changed | %d |\n", s.FilesChanged))
		b.WriteString(fmt.Sprintf("| Symbols Changed | %d |\n", s.SymbolsChanged))
		b.WriteString(fmt.Sprintf("| Directly Affected | %d |\n", s.DirectlyAffected))
		b.WriteString(fmt.Sprintf("| Transitively Affected | %d |\n", s.TransitivelyAffected))
		b.WriteString("\n")
	}

	// Blast radius
	if resp.BlastRadius != nil {
		br := resp.BlastRadius
		b.WriteString(fmt.Sprintf("**Blast Radius:** %d modules, %d files, %d unique callers\n\n",
			br.ModuleCount, br.FileCount, br.UniqueCallerCount))
	}

	// Changed symbols
	if len(resp.ChangedSymbols) > 0 {
		b.WriteString("<details>\n")
		b.WriteString(fmt.Sprintf("<summary>📝 Changed Symbols (%d)</summary>\n\n", len(resp.ChangedSymbols)))
		b.WriteString("| Symbol | File | Type | Confidence |\n")
		b.WriteString("|:-------|:-----|:-----|----------:|\n")
		for i, sym := range resp.ChangedSymbols {
			if i >= 15 {
				b.WriteString(fmt.Sprintf("| … | +%d more | | |\n", len(resp.ChangedSymbols)-15))
				break
			}
			b.WriteString(fmt.Sprintf("| `%s` | `%s` | %s | %.0f%% |\n",
				sym.Name, sym.File, sym.ChangeType, sym.Confidence*100))
		}
		b.WriteString("\n</details>\n\n")
	}

	// Affected symbols
	if len(resp.AffectedSymbols) > 0 {
		b.WriteString("<details>\n")
		b.WriteString(fmt.Sprintf("<summary>🎯 Affected Downstream (%d)</summary>\n\n", len(resp.AffectedSymbols)))
		b.WriteString("| Symbol | Module | Distance | Kind |\n")
		b.WriteString("|:-------|:-------|:--------:|:-----|\n")
		for i, sym := range resp.AffectedSymbols {
			if i >= 20 {
				b.WriteString(fmt.Sprintf("| … | +%d more | | |\n", len(resp.AffectedSymbols)-20))
				break
			}
			b.WriteString(fmt.Sprintf("| `%s` | `%s` | %d | %s |\n",
				sym.Name, sym.ModuleID, sym.Distance, sym.Kind))
		}
		b.WriteString("\n</details>\n\n")
	}

	// Modules affected
	if len(resp.ModulesAffected) > 0 {
		b.WriteString("<details>\n")
		b.WriteString(fmt.Sprintf("<summary>📦 Modules Affected (%d)</summary>\n\n", len(resp.ModulesAffected)))
		b.WriteString("| Module | Impact Count | Direct |\n")
		b.WriteString("|:-------|-------------:|-------:|\n")
		for _, mod := range resp.ModulesAffected {
			b.WriteString(fmt.Sprintf("| `%s` | %d | %d |\n",
				mod.ModuleID, mod.ImpactCount, mod.DirectCount))
		}
		b.WriteString("\n</details>\n\n")
	}

	// Recommendations
	if len(resp.Recommendations) > 0 {
		b.WriteString("### Recommendations\n\n")
		for _, rec := range resp.Recommendations {
			icon := "ℹ️"
			if rec.Severity == "warning" {
				icon = "⚠️"
			} else if rec.Severity == "error" {
				icon = "🔴"
			}
			b.WriteString(fmt.Sprintf("- %s **%s**: %s\n", icon, rec.Type, rec.Message))
			if rec.Action != "" {
				b.WriteString(fmt.Sprintf("  - *Action:* %s\n", rec.Action))
			}
		}
		b.WriteString("\n")
	}

	// Index staleness warning
	if resp.IndexStaleness != nil && resp.IndexStaleness.IsStale {
		b.WriteString(fmt.Sprintf("> ⚠️ **Index is %d commit(s) behind HEAD.** Results may be incomplete.\n\n",
			resp.IndexStaleness.CommitsBehind))
	}

	// Footer
	b.WriteString("---\n")
	b.WriteString("<sub>Generated by <a href=\"https://github.com/SimplyLiz/CodeMCP\">CKB</a></sub>\n")

	return b.String()
}
