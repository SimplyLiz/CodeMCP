package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/query"
)

var (
	prSummaryFormat           string
	prSummaryBaseBranch       string
	prSummaryHeadBranch       string
	prSummaryIncludeOwnership bool
)

var prSummaryCmd = &cobra.Command{
	Use:   "pr-summary",
	Short: "Analyze PR changes with risk assessment and suggested reviewers",
	Long: `Analyze changes between branches and provide a PR summary.

Includes:
  - Changed files with hotspot analysis
  - Affected modules with risk levels
  - Overall risk assessment with factors
  - Suggested reviewers based on ownership

Examples:
  ckb pr-summary
  ckb pr-summary --base=main --head=feature/my-branch
  ckb pr-summary --no-ownership
  ckb pr-summary --format=human`,
	Run: runPrSummary,
}

func init() {
	prSummaryCmd.Flags().StringVar(&prSummaryFormat, "format", "json", "Output format (json, human)")
	prSummaryCmd.Flags().StringVar(&prSummaryBaseBranch, "base", "main", "Base branch to compare against")
	prSummaryCmd.Flags().StringVar(&prSummaryHeadBranch, "head", "", "Head branch (default: current branch)")
	prSummaryCmd.Flags().BoolVar(&prSummaryIncludeOwnership, "ownership", true, "Include ownership analysis for reviewer suggestions")
	prSummaryCmd.Flags().BoolVar(&prSummaryIncludeOwnership, "no-ownership", false, "Disable ownership analysis")
	rootCmd.AddCommand(prSummaryCmd)
}

func runPrSummary(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(prSummaryFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)
	ctx := newContext()

	// Handle --no-ownership flag
	includeOwnership := prSummaryIncludeOwnership
	if cmd.Flags().Changed("no-ownership") {
		includeOwnership = false
	}

	opts := query.SummarizePROptions{
		BaseBranch:       prSummaryBaseBranch,
		HeadBranch:       prSummaryHeadBranch,
		IncludeOwnership: includeOwnership,
	}
	response, err := engine.SummarizePR(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error summarizing PR: %v\n", err)
		os.Exit(1)
	}

	cliResponse := convertPRSummaryResponse(response)

	output, err := FormatResponse(cliResponse, OutputFormat(prSummaryFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("PR summary completed", map[string]interface{}{
		"baseBranch": prSummaryBaseBranch,
		"headBranch": prSummaryHeadBranch,
		"files":      response.Summary.TotalFiles,
		"riskLevel":  response.RiskAssessment.Level,
		"duration":   time.Since(start).Milliseconds(),
	})
}

// PRSummaryResponseCLI contains PR summary for CLI output
type PRSummaryResponseCLI struct {
	Summary         PRSummaryCLI          `json:"summary"`
	ChangedFiles    []PRFileChangeCLI     `json:"changedFiles"`
	ModulesAffected []PRModuleImpactCLI   `json:"modulesAffected"`
	RiskAssessment  PRRiskAssessmentCLI   `json:"riskAssessment"`
	Reviewers       []SuggestedReviewCLI  `json:"suggestedReviewers,omitempty"`
	Provenance      *ProvenanceCLI        `json:"provenance,omitempty"`
}

type PRSummaryCLI struct {
	TotalFiles        int      `json:"totalFiles"`
	TotalAdditions    int      `json:"totalAdditions"`
	TotalDeletions    int      `json:"totalDeletions"`
	TotalModules      int      `json:"totalModules"`
	HotspotsTouched   int      `json:"hotspotsTouched"`
	OwnershipCoverage float64  `json:"ownershipCoverage"`
	Languages         []string `json:"languages"`
}

type PRFileChangeCLI struct {
	Path         string  `json:"path"`
	Status       string  `json:"status"`
	Additions    int     `json:"additions"`
	Deletions    int     `json:"deletions"`
	Module       string  `json:"module,omitempty"`
	IsHotspot    bool    `json:"isHotspot,omitempty"`
	HotspotScore float64 `json:"hotspotScore,omitempty"`
	Language     string  `json:"language,omitempty"`
}

type PRModuleImpactCLI struct {
	ModuleId     string   `json:"moduleId"`
	Name         string   `json:"name"`
	FilesChanged int      `json:"filesChanged"`
	RiskLevel    string   `json:"riskLevel"`
	Reasons      []string `json:"reasons,omitempty"`
}

type PRRiskAssessmentCLI struct {
	Level       string   `json:"level"`
	Score       float64  `json:"score"`
	Factors     []string `json:"factors"`
	Suggestions []string `json:"suggestions,omitempty"`
}

type SuggestedReviewCLI struct {
	Owner      string  `json:"owner"`
	Reason     string  `json:"reason"`
	Coverage   float64 `json:"coverage"`
	Confidence float64 `json:"confidence"`
}

func convertPRSummaryResponse(resp *query.SummarizePRResponse) *PRSummaryResponseCLI {
	changedFiles := make([]PRFileChangeCLI, 0, len(resp.ChangedFiles))
	for _, f := range resp.ChangedFiles {
		changedFiles = append(changedFiles, PRFileChangeCLI{
			Path:         f.Path,
			Status:       f.Status,
			Additions:    f.Additions,
			Deletions:    f.Deletions,
			Module:       f.Module,
			IsHotspot:    f.IsHotspot,
			HotspotScore: f.HotspotScore,
			Language:     f.Language,
		})
	}

	modulesAffected := make([]PRModuleImpactCLI, 0, len(resp.ModulesAffected))
	for _, m := range resp.ModulesAffected {
		modulesAffected = append(modulesAffected, PRModuleImpactCLI{
			ModuleId:     m.ModuleId,
			Name:         m.Name,
			FilesChanged: m.FilesChanged,
			RiskLevel:    m.RiskLevel,
			Reasons:      m.Reasons,
		})
	}

	reviewers := make([]SuggestedReviewCLI, 0, len(resp.Reviewers))
	for _, r := range resp.Reviewers {
		reviewers = append(reviewers, SuggestedReviewCLI{
			Owner:      r.Owner,
			Reason:     r.Reason,
			Coverage:   r.Coverage,
			Confidence: r.Confidence,
		})
	}

	result := &PRSummaryResponseCLI{
		Summary: PRSummaryCLI{
			TotalFiles:        resp.Summary.TotalFiles,
			TotalAdditions:    resp.Summary.TotalAdditions,
			TotalDeletions:    resp.Summary.TotalDeletions,
			TotalModules:      resp.Summary.TotalModules,
			HotspotsTouched:   resp.Summary.HotspotsTouched,
			OwnershipCoverage: resp.Summary.OwnershipCoverage,
			Languages:         resp.Summary.Languages,
		},
		ChangedFiles:    changedFiles,
		ModulesAffected: modulesAffected,
		RiskAssessment: PRRiskAssessmentCLI{
			Level:       resp.RiskAssessment.Level,
			Score:       resp.RiskAssessment.Score,
			Factors:     resp.RiskAssessment.Factors,
			Suggestions: resp.RiskAssessment.Suggestions,
		},
		Reviewers: reviewers,
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
