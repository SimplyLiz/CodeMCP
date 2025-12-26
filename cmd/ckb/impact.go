package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/query"
)

var (
	impactDepth        int
	impactIncludeTests bool
	impactFormat       string
	// Diff subcommand flags
	impactDiffStaged    bool
	impactDiffBase      string
	impactDiffStrict    bool
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
  ckb impact diff --strict           # Fail if index is stale`,
	Run: runImpactDiff,
}

func init() {
	impactCmd.Flags().IntVar(&impactDepth, "depth", 2, "Maximum impact depth")
	impactCmd.Flags().BoolVar(&impactIncludeTests, "include-tests", false, "Include test dependencies")
	impactCmd.Flags().StringVar(&impactFormat, "format", "json", "Output format (json, human)")

	// Diff subcommand flags
	impactDiffCmd.Flags().BoolVar(&impactDiffStaged, "staged", false, "Analyze only staged changes (--cached)")
	impactDiffCmd.Flags().StringVar(&impactDiffBase, "base", "HEAD", "Base branch for comparison")
	impactDiffCmd.Flags().IntVar(&impactDepth, "depth", 2, "Maximum depth for transitive impact (1-4)")
	impactDiffCmd.Flags().BoolVar(&impactIncludeTests, "include-tests", false, "Include test files in analysis")
	impactDiffCmd.Flags().BoolVar(&impactDiffStrict, "strict", false, "Fail if SCIP index is stale")
	impactDiffCmd.Flags().StringVar(&impactFormat, "format", "json", "Output format (json, human)")

	impactCmd.AddCommand(impactDiffCmd)
	rootCmd.AddCommand(impactCmd)
}

func runImpact(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(impactFormat)
	symbolID := args[0]

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

	logger.Debug("Impact analysis completed", map[string]interface{}{
		"symbolId": symbolID,
		"direct":   len(response.DirectImpact),
		"duration": time.Since(start).Milliseconds(),
	})
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
	output, err := FormatResponse(cliResponse, OutputFormat(impactFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Change impact analysis completed", map[string]interface{}{
		"filesChanged":   response.Summary.FilesChanged,
		"symbolsChanged": response.Summary.SymbolsChanged,
		"riskLevel":      response.Summary.EstimatedRisk,
		"duration":       time.Since(start).Milliseconds(),
	})
}

// ChangeSetResponseCLI contains change set analysis results for CLI output
type ChangeSetResponseCLI struct {
	Summary          *ChangeSummaryCLI       `json:"summary"`
	ChangedSymbols   []ChangedSymbolCLI      `json:"changedSymbols"`
	AffectedSymbols  []ImpactItemCLI         `json:"affectedSymbols"`
	ModulesAffected  []ModuleImpactCLI       `json:"modulesAffected"`
	BlastRadius      *BlastRadiusCLI         `json:"blastRadius,omitempty"`
	RiskScore        *RiskScoreCLI           `json:"riskScore,omitempty"`
	Recommendations  []RecommendationCLI     `json:"recommendations,omitempty"`
	IndexStaleness   *IndexStalenessCLI      `json:"indexStaleness,omitempty"`
	Provenance       *ProvenanceCLI          `json:"provenance,omitempty"`
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
