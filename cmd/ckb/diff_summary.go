package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/query"
)

var (
	diffSummaryFormat    string
	diffSummaryCommit    string
	diffSummaryBase      string
	diffSummaryHead      string
	diffSummaryTimeStart string
	diffSummaryTimeEnd   string
)

var diffSummaryCmd = &cobra.Command{
	Use:   "diff-summary",
	Short: "Summarize what changed and what might break",
	Long: `Compress diffs into 'what changed, what might break'.

Supports commit ranges, single commits, or time windows.
Default: last 30 days.

Examples:
  ckb diff-summary
  ckb diff-summary --commit=abc1234
  ckb diff-summary --base=main --head=feature/my-branch
  ckb diff-summary --start=2024-01-01 --end=2024-06-30
  ckb diff-summary --format=human`,
	Run: runDiffSummary,
}

func init() {
	diffSummaryCmd.Flags().StringVar(&diffSummaryFormat, "format", "json", "Output format (json, human)")
	diffSummaryCmd.Flags().StringVar(&diffSummaryCommit, "commit", "", "Single commit hash to analyze")
	diffSummaryCmd.Flags().StringVar(&diffSummaryBase, "base", "", "Base commit/ref for range (use with --head)")
	diffSummaryCmd.Flags().StringVar(&diffSummaryHead, "head", "", "Head commit/ref for range (use with --base)")
	diffSummaryCmd.Flags().StringVar(&diffSummaryTimeStart, "start", "", "Start date for time window (ISO8601 or YYYY-MM-DD)")
	diffSummaryCmd.Flags().StringVar(&diffSummaryTimeEnd, "end", "", "End date for time window (ISO8601 or YYYY-MM-DD)")
	rootCmd.AddCommand(diffSummaryCmd)
}

func runDiffSummary(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(diffSummaryFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	opts := query.SummarizeDiffOptions{}

	// Determine which selector to use
	if diffSummaryCommit != "" {
		opts.Commit = diffSummaryCommit
	} else if diffSummaryBase != "" && diffSummaryHead != "" {
		opts.CommitRange = &query.CommitRangeSelector{
			Base: diffSummaryBase,
			Head: diffSummaryHead,
		}
	} else if diffSummaryTimeStart != "" || diffSummaryTimeEnd != "" {
		opts.TimeWindow = &query.TimeWindowSelector{
			Start: diffSummaryTimeStart,
			End:   diffSummaryTimeEnd,
		}
	}
	// If none specified, the engine will use default (last 30 days)

	response, err := engine.SummarizeDiff(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error summarizing diff: %v\n", err)
		os.Exit(1)
	}

	cliResponse := convertDiffSummaryResponse(response)

	output, err := FormatResponse(cliResponse, OutputFormat(diffSummaryFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Diff summary completed", map[string]interface{}{
		"files":    len(response.ChangedFiles),
		"symbols":  len(response.SymbolsAffected),
		"duration": time.Since(start).Milliseconds(),
	})
}

// DiffSummaryResponseCLI contains diff summary for CLI output
type DiffSummaryResponseCLI struct {
	Selector        DiffSelectorCLI         `json:"selector"`
	ChangedFiles    []DiffFileChangeCLI     `json:"changedFiles"`
	SymbolsAffected []DiffSymbolAffectedCLI `json:"symbolsAffected"`
	RiskSignals     []DiffRiskSignalCLI     `json:"riskSignals"`
	Summary         DiffSummaryTextCLI      `json:"summary"`
	Confidence      float64                 `json:"confidence"`
	Limitations     []string                `json:"limitations,omitempty"`
	Provenance      *ProvenanceCLI          `json:"provenance,omitempty"`
}

type DiffSelectorCLI struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type DiffFileChangeCLI struct {
	FilePath   string `json:"filePath"`
	ChangeType string `json:"changeType"`
	Additions  int    `json:"additions"`
	Deletions  int    `json:"deletions"`
	OldPath    string `json:"oldPath,omitempty"`
	Language   string `json:"language,omitempty"`
	Role       string `json:"role,omitempty"`
	RiskLevel  string `json:"riskLevel"`
}

type DiffSymbolAffectedCLI struct {
	SymbolId     string `json:"symbolId,omitempty"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	FilePath     string `json:"filePath"`
	ChangeType   string `json:"changeType"`
	IsPublicAPI  bool   `json:"isPublicApi"`
	IsEntrypoint bool   `json:"isEntrypoint"`
}

type DiffRiskSignalCLI struct {
	Type        string  `json:"type"`
	Severity    string  `json:"severity"`
	FilePath    string  `json:"filePath"`
	Description string  `json:"description"`
	Confidence  float64 `json:"confidence"`
}

type DiffSummaryTextCLI struct {
	OneLiner     string   `json:"oneLiner"`
	KeyChanges   []string `json:"keyChanges"`
	RiskOverview string   `json:"riskOverview,omitempty"`
}

func convertDiffSummaryResponse(resp *query.SummarizeDiffResponse) *DiffSummaryResponseCLI {
	changedFiles := make([]DiffFileChangeCLI, 0, len(resp.ChangedFiles))
	for _, f := range resp.ChangedFiles {
		changedFiles = append(changedFiles, DiffFileChangeCLI{
			FilePath:   f.FilePath,
			ChangeType: f.ChangeType,
			Additions:  f.Additions,
			Deletions:  f.Deletions,
			OldPath:    f.OldPath,
			Language:   f.Language,
			Role:       f.Role,
			RiskLevel:  f.RiskLevel,
		})
	}

	symbolsAffected := make([]DiffSymbolAffectedCLI, 0, len(resp.SymbolsAffected))
	for _, s := range resp.SymbolsAffected {
		symbolsAffected = append(symbolsAffected, DiffSymbolAffectedCLI{
			SymbolId:     s.SymbolId,
			Name:         s.Name,
			Kind:         s.Kind,
			FilePath:     s.FilePath,
			ChangeType:   s.ChangeType,
			IsPublicAPI:  s.IsPublicAPI,
			IsEntrypoint: s.IsEntrypoint,
		})
	}

	riskSignals := make([]DiffRiskSignalCLI, 0, len(resp.RiskSignals))
	for _, r := range resp.RiskSignals {
		riskSignals = append(riskSignals, DiffRiskSignalCLI{
			Type:        r.Type,
			Severity:    r.Severity,
			FilePath:    r.FilePath,
			Description: r.Description,
			Confidence:  r.Confidence,
		})
	}

	result := &DiffSummaryResponseCLI{
		Selector: DiffSelectorCLI{
			Type:  resp.Selector.Type,
			Value: resp.Selector.Value,
		},
		ChangedFiles:    changedFiles,
		SymbolsAffected: symbolsAffected,
		RiskSignals:     riskSignals,
		Summary: DiffSummaryTextCLI{
			OneLiner:     resp.Summary.OneLiner,
			KeyChanges:   resp.Summary.KeyChanges,
			RiskOverview: resp.Summary.RiskOverview,
		},
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
