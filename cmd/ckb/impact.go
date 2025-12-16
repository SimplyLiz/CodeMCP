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

func init() {
	impactCmd.Flags().IntVar(&impactDepth, "depth", 2, "Maximum impact depth")
	impactCmd.Flags().BoolVar(&impactIncludeTests, "include-tests", false, "Include test dependencies")
	impactCmd.Flags().StringVar(&impactFormat, "format", "json", "Output format (json, human)")
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
