package query

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"ckb/internal/version"
)

// SummarizePROptions contains options for the summarize-pr tool.
type SummarizePROptions struct {
	BaseBranch       string `json:"baseBranch"`       // Base branch to compare against (default: "main")
	HeadBranch       string `json:"headBranch"`       // Head branch (default: current branch)
	IncludeOwnership bool   `json:"includeOwnership"` // Include ownership analysis (default: true)
}

// SummarizePRResponse is the response for summarize-pr.
type SummarizePRResponse struct {
	CkbVersion      string            `json:"ckbVersion"`
	SchemaVersion   string            `json:"schemaVersion"`
	Tool            string            `json:"tool"`
	Summary         PRSummary         `json:"summary"`
	ChangedFiles    []PRFileChange    `json:"changedFiles"`
	ModulesAffected []PRModuleImpact  `json:"modulesAffected"`
	RiskAssessment  PRRiskAssessment  `json:"riskAssessment"`
	Reviewers       []SuggestedReview `json:"suggestedReviewers,omitempty"`
	Provenance      *Provenance       `json:"provenance,omitempty"`
}

// PRSummary provides a high-level overview of the PR.
type PRSummary struct {
	TotalFiles         int      `json:"totalFiles"`
	TotalAdditions     int      `json:"totalAdditions"`
	TotalDeletions     int      `json:"totalDeletions"`
	TotalModules       int      `json:"totalModules"`
	HotspotsTouched    int      `json:"hotspotsTouched"`
	OwnershipCoverage  float64  `json:"ownershipCoverage"` // % of files where author is owner
	Languages          []string `json:"languages"`
}

// PRFileChange represents a changed file in the PR.
type PRFileChange struct {
	Path       string  `json:"path"`
	Status     string  `json:"status"` // added, modified, deleted, renamed
	Additions  int     `json:"additions"`
	Deletions  int     `json:"deletions"`
	Module     string  `json:"module,omitempty"`
	IsHotspot  bool    `json:"isHotspot,omitempty"`
	HotspotScore float64 `json:"hotspotScore,omitempty"`
	Language   string  `json:"language,omitempty"`
}

// PRModuleImpact describes the impact on a module in a PR context.
type PRModuleImpact struct {
	ModuleId     string   `json:"moduleId"`
	Name         string   `json:"name"`
	FilesChanged int      `json:"filesChanged"`
	RiskLevel    string   `json:"riskLevel"` // low, medium, high
	Reasons      []string `json:"reasons,omitempty"`
}

// PRRiskAssessment provides an overall risk assessment.
type PRRiskAssessment struct {
	Level       string   `json:"level"`   // low, medium, high
	Score       float64  `json:"score"`   // 0-1
	Factors     []string `json:"factors"` // Reasons for the risk level
	Suggestions []string `json:"suggestions,omitempty"`
}

// SuggestedReview represents a suggested reviewer.
type SuggestedReview struct {
	Owner      string  `json:"owner"`
	Reason     string  `json:"reason"`
	Coverage   float64 `json:"coverage"` // % of changed files they own
	Confidence float64 `json:"confidence"`
}

// SummarizePR generates a summary of changes between branches.
func (e *Engine) SummarizePR(ctx context.Context, opts SummarizePROptions) (*SummarizePRResponse, error) {
	startTime := time.Now()

	// Default values
	if opts.BaseBranch == "" {
		opts.BaseBranch = "main"
	}

	// Get changed files from git
	if e.gitAdapter == nil {
		return nil, fmt.Errorf("git adapter not available")
	}

	// Get diff stats between branches
	// If no head branch specified, compare against working tree
	headRef := opts.HeadBranch
	if headRef == "" {
		headRef = "HEAD"
	}
	diffStats, err := e.gitAdapter.GetCommitRangeDiff(opts.BaseBranch, headRef)
	if err != nil {
		return nil, fmt.Errorf("failed to get diff: %w", err)
	}

	// Get repo state
	repoState, err := e.GetRepoState(ctx, "head")
	if err != nil {
		repoState = &RepoState{RepoStateId: "unknown"}
	}

	// Analyze changed files
	changedFiles := make([]PRFileChange, 0, len(diffStats))
	languages := make(map[string]bool)
	moduleFiles := make(map[string]int)
	totalAdditions := 0
	totalDeletions := 0
	hotspotCount := 0

	for _, df := range diffStats {
		// Determine status from DiffStats flags
		status := "modified"
		if df.IsNew {
			status = "added"
		} else if df.IsDeleted {
			status = "deleted"
		} else if df.IsRenamed {
			status = "renamed"
		}

		change := PRFileChange{
			Path:      df.FilePath,
			Status:    status,
			Additions: df.Additions,
			Deletions: df.Deletions,
			Language:  detectLanguage(df.FilePath),
		}

		totalAdditions += df.Additions
		totalDeletions += df.Deletions

		if change.Language != "" {
			languages[change.Language] = true
		}

		// Try to map file to module
		module := e.resolveFileModule(df.FilePath)
		if module != "" {
			change.Module = module
			moduleFiles[module]++
		}

		// Check if file is a hotspot
		hotspotScore := e.getFileHotspotScore(ctx, df.FilePath)
		if hotspotScore > 0.5 {
			change.IsHotspot = true
			change.HotspotScore = hotspotScore
			hotspotCount++
		}

		changedFiles = append(changedFiles, change)
	}

	// Build module impacts
	modulesAffected := make([]PRModuleImpact, 0, len(moduleFiles))
	for moduleId, fileCount := range moduleFiles {
		risk := "low"
		var reasons []string

		if fileCount > 5 {
			risk = "medium"
			reasons = append(reasons, fmt.Sprintf("Many files changed (%d)", fileCount))
		}
		if fileCount > 10 {
			risk = "high"
		}

		modulesAffected = append(modulesAffected, PRModuleImpact{
			ModuleId:     moduleId,
			Name:         moduleId,
			FilesChanged: fileCount,
			RiskLevel:    risk,
			Reasons:      reasons,
		})
	}

	// Sort modules by files changed
	sort.Slice(modulesAffected, func(i, j int) bool {
		return modulesAffected[i].FilesChanged > modulesAffected[j].FilesChanged
	})

	// Build language list
	langList := make([]string, 0, len(languages))
	for lang := range languages {
		langList = append(langList, lang)
	}
	sort.Strings(langList)

	// Calculate risk assessment
	risk := calculatePRRisk(len(changedFiles), totalAdditions+totalDeletions, hotspotCount, len(modulesAffected))

	// Get suggested reviewers
	var reviewers []SuggestedReview
	if opts.IncludeOwnership {
		reviewers = e.getSuggestedReviewers(ctx, changedFiles)
	}

	// Calculate ownership coverage
	ownershipCoverage := 0.0
	if len(reviewers) > 0 {
		ownershipCoverage = reviewers[0].Coverage
	}

	summary := PRSummary{
		TotalFiles:        len(changedFiles),
		TotalAdditions:    totalAdditions,
		TotalDeletions:    totalDeletions,
		TotalModules:      len(modulesAffected),
		HotspotsTouched:   hotspotCount,
		OwnershipCoverage: ownershipCoverage,
		Languages:         langList,
	}

	return &SummarizePRResponse{
		CkbVersion:      version.Version,
		SchemaVersion:   "6.1",
		Tool:            "summarizePr",
		Summary:         summary,
		ChangedFiles:    changedFiles,
		ModulesAffected: modulesAffected,
		RiskAssessment:  risk,
		Reviewers:       reviewers,
		Provenance: &Provenance{
			RepoStateId:     repoState.RepoStateId,
			RepoStateDirty:  repoState.Dirty,
			QueryDurationMs: time.Since(startTime).Milliseconds(),
		},
	}, nil
}

// resolveFileModule maps a file path to its module.
func (e *Engine) resolveFileModule(filePath string) string {
	// Simple heuristic: use first two path components
	parts := strings.Split(filePath, "/")
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return ""
}

// getFileHotspotScore returns the hotspot score for a file (0-1).
func (e *Engine) getFileHotspotScore(ctx context.Context, filePath string) float64 {
	// Try to get hotspot data from cache or compute
	opts := GetHotspotsOptions{Limit: 100}
	resp, err := e.GetHotspots(ctx, opts)
	if err != nil {
		return 0
	}

	for _, h := range resp.Hotspots {
		if h.FilePath == filePath && h.Ranking != nil {
			return h.Ranking.Score
		}
	}

	return 0
}

// getSuggestedReviewers identifies potential reviewers based on ownership.
func (e *Engine) getSuggestedReviewers(ctx context.Context, files []PRFileChange) []SuggestedReview {
	ownerCounts := make(map[string]int)
	totalFiles := len(files)

	for _, f := range files {
		opts := GetOwnershipOptions{Path: f.Path}
		resp, err := e.GetOwnership(ctx, opts)
		if err != nil || resp == nil {
			continue
		}

		for _, owner := range resp.Owners {
			ownerCounts[owner.ID]++
		}
	}

	// Convert to suggestions
	var suggestions []SuggestedReview
	for owner, count := range ownerCounts {
		coverage := float64(count) / float64(totalFiles)
		suggestions = append(suggestions, SuggestedReview{
			Owner:      owner,
			Reason:     fmt.Sprintf("Owns %d of %d changed files", count, totalFiles),
			Coverage:   coverage,
			Confidence: coverage,
		})
	}

	// Sort by coverage
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Coverage > suggestions[j].Coverage
	})

	// Limit to top 5
	if len(suggestions) > 5 {
		suggestions = suggestions[:5]
	}

	return suggestions
}

// calculatePRRisk computes the risk assessment for a PR.
func calculatePRRisk(fileCount, totalChanges, hotspotCount, moduleCount int) PRRiskAssessment {
	var factors []string
	var suggestions []string
	score := 0.0

	// File count factor
	if fileCount > 20 {
		score += 0.3
		factors = append(factors, fmt.Sprintf("Large PR with %d files", fileCount))
		suggestions = append(suggestions, "Consider splitting into smaller PRs")
	} else if fileCount > 10 {
		score += 0.15
		factors = append(factors, fmt.Sprintf("Medium-sized PR with %d files", fileCount))
	}

	// Change size factor
	if totalChanges > 1000 {
		score += 0.3
		factors = append(factors, fmt.Sprintf("High churn: %d lines changed", totalChanges))
	} else if totalChanges > 500 {
		score += 0.15
		factors = append(factors, fmt.Sprintf("Moderate churn: %d lines changed", totalChanges))
	}

	// Hotspot factor
	if hotspotCount > 0 {
		hotspotImpact := float64(hotspotCount) * 0.1
		if hotspotImpact > 0.3 {
			hotspotImpact = 0.3
		}
		score += hotspotImpact
		factors = append(factors, fmt.Sprintf("Touches %d hotspot(s)", hotspotCount))
		suggestions = append(suggestions, "Extra review recommended for hotspot files")
	}

	// Module spread factor
	if moduleCount > 5 {
		score += 0.2
		factors = append(factors, fmt.Sprintf("Spans %d modules", moduleCount))
		suggestions = append(suggestions, "Consider module-specific reviewers")
	}

	// Determine level
	level := "low"
	if score > 0.6 {
		level = "high"
	} else if score > 0.3 {
		level = "medium"
	}

	if len(factors) == 0 {
		factors = append(factors, "Small, focused change")
	}

	return PRRiskAssessment{
		Level:       level,
		Score:       score,
		Factors:     factors,
		Suggestions: suggestions,
	}
}

