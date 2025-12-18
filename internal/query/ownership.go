package query

import (
	"context"
	"path/filepath"
	"time"

	"ckb/internal/errors"
	"ckb/internal/output"
	"ckb/internal/ownership"
)

// GetOwnershipOptions contains options for getOwnership.
type GetOwnershipOptions struct {
	Path           string
	IncludeBlame   bool
	IncludeHistory bool
}

// OwnerInfo represents an owner in the response.
type OwnerInfo struct {
	ID         string  `json:"id"`
	Type       string  `json:"type"`       // "user", "team", "email"
	Scope      string  `json:"scope"`      // "maintainer", "reviewer", "contributor"
	Source     string  `json:"source"`     // "codeowners", "git-blame", "declared"
	Confidence float64 `json:"confidence"`
}

// BlameContributor represents a contributor from git-blame.
type BlameContributor struct {
	Author     string  `json:"author"`
	Email      string  `json:"email"`
	LineCount  int     `json:"lineCount"`
	Percentage float64 `json:"percentage"`
}

// BlameOwnershipInfo represents git-blame ownership info.
type BlameOwnershipInfo struct {
	TotalLines   int                `json:"totalLines"`
	Contributors []BlameContributor `json:"contributors"`
}

// OwnershipHistoryEvent represents an ownership change event.
type OwnershipHistoryEvent struct {
	Pattern    string `json:"pattern"`
	OwnerID    string `json:"ownerId"`
	Event      string `json:"event"` // "added", "removed", "promoted", "demoted"
	Reason     string `json:"reason,omitempty"`
	RecordedAt string `json:"recordedAt"`
}

// GetOwnershipResponse is the response for getOwnership.
type GetOwnershipResponse struct {
	CkbVersion      string                   `json:"ckbVersion"`
	SchemaVersion   string                   `json:"schemaVersion"`
	Tool            string                   `json:"tool"`
	Path            string                   `json:"path"`
	Owners          []OwnerInfo              `json:"owners"`
	BlameOwnership  *BlameOwnershipInfo      `json:"blameOwnership,omitempty"`
	History         []OwnershipHistoryEvent  `json:"history,omitempty"`
	Confidence      float64                  `json:"confidence"`
	ConfidenceBasis []ConfidenceBasisItem    `json:"confidenceBasis"`
	Limitations     []string                 `json:"limitations,omitempty"`
	Provenance      *Provenance              `json:"provenance,omitempty"`
	Drilldowns      []output.Drilldown       `json:"drilldowns,omitempty"`
}

// GetOwnership returns ownership information for a file or path.
func (e *Engine) GetOwnership(ctx context.Context, opts GetOwnershipOptions) (*GetOwnershipResponse, error) {
	startTime := time.Now()

	var confidenceBasis []ConfidenceBasisItem
	var limitations []string
	var allOwners []OwnerInfo

	// Normalize path
	normalizedPath := opts.Path
	if !filepath.IsAbs(normalizedPath) {
		normalizedPath = filepath.Clean(normalizedPath)
	} else {
		// Make path relative to repo root
		relPath, err := filepath.Rel(e.repoRoot, normalizedPath)
		if err == nil {
			normalizedPath = relPath
		}
	}

	// Get repo state
	repoState, err := e.GetRepoState(ctx, "head")
	if err != nil {
		return nil, e.wrapError(err, errors.InternalError)
	}

	// Try to find and parse CODEOWNERS
	codeownersPath := ownership.FindCodeownersFile(e.repoRoot)
	var codeownersFile *ownership.CodeownersFile
	if codeownersPath != "" {
		codeownersFile, err = ownership.ParseCodeownersFile(codeownersPath)
		if err != nil {
			limitations = append(limitations, "Failed to parse CODEOWNERS: "+err.Error())
		} else {
			// Get CODEOWNERS owners for this path
			codeownersOwners := codeownersFile.GetOwnersForPath(normalizedPath)
			if len(codeownersOwners) > 0 {
				owners := ownership.CodeownersToOwners(codeownersOwners)
				for _, o := range owners {
					allOwners = append(allOwners, OwnerInfo{
						ID:         o.ID,
						Type:       o.Type,
						Scope:      o.Scope,
						Source:     o.Source,
						Confidence: o.Confidence,
					})
				}
				confidenceBasis = append(confidenceBasis, ConfidenceBasisItem{
					Backend: "codeowners",
					Status:  "available",
				})
			}
		}
	} else {
		limitations = append(limitations, "No CODEOWNERS file found")
	}

	// Get git-blame ownership if requested
	var blameOwnership *BlameOwnershipInfo
	if opts.IncludeBlame {
		blameConfig := ownership.DefaultBlameConfig()
		blameResult, blameErr := ownership.RunGitBlame(e.repoRoot, normalizedPath)
		if blameErr != nil {
			limitations = append(limitations, "Git blame failed: "+blameErr.Error())
		} else {
			blameOwn := ownership.ComputeBlameOwnership(blameResult, blameConfig)

			// Add blame owners to the list
			blameOwners := ownership.BlameOwnershipToOwners(blameOwn)
			for _, o := range blameOwners {
				allOwners = append(allOwners, OwnerInfo{
					ID:         o.ID,
					Type:       o.Type,
					Scope:      o.Scope,
					Source:     o.Source,
					Confidence: o.Confidence,
				})
			}

			// Build blame ownership info
			contributors := make([]BlameContributor, 0, len(blameOwn.Contributors))
			for _, c := range blameOwn.Contributors {
				contributors = append(contributors, BlameContributor{
					Author:     c.Author,
					Email:      c.Email,
					LineCount:  c.LineCount,
					Percentage: c.Percentage,
				})
			}

			blameOwnership = &BlameOwnershipInfo{
				TotalLines:   blameOwn.TotalLines,
				Contributors: contributors,
			}

			confidenceBasis = append(confidenceBasis, ConfidenceBasisItem{
				Backend: "git-blame",
				Status:  "available",
			})
		}
	}

	// Get history if requested (placeholder - would query storage)
	var history []OwnershipHistoryEvent
	if opts.IncludeHistory {
		// TODO: Query ownership_history table from storage
		limitations = append(limitations, "Ownership history not yet implemented")
	}

	// Compute overall confidence
	confidence := 0.5 // Base confidence
	if len(allOwners) > 0 {
		// Higher confidence if we have CODEOWNERS
		hasCODEOWNERS := false
		for _, o := range allOwners {
			if o.Source == "codeowners" {
				hasCODEOWNERS = true
				break
			}
		}
		if hasCODEOWNERS {
			confidence = 1.0
		} else if blameOwnership != nil {
			confidence = 0.79
		}
	}

	// Build completeness
	completeness := CompletenessInfo{
		Score:  1.0,
		Reason: "ownership-computed",
	}
	if len(limitations) > 0 {
		completeness.Score = 0.7
		completeness.Reason = "partial-ownership"
	}

	// Build provenance
	provenance := e.buildProvenance(repoState, "head", startTime, nil, completeness)

	// Generate drilldowns
	var drilldowns []output.Drilldown
	if len(allOwners) > 0 {
		// Suggest exploring the module
		drilldowns = append(drilldowns, output.Drilldown{
			Label:          "Explore module",
			Query:          "getModuleOverview for " + filepath.Dir(normalizedPath),
			RelevanceScore: 0.8,
		})
	}

	return &GetOwnershipResponse{
		CkbVersion:      "6.0",
		SchemaVersion:   "6.0",
		Tool:            "getOwnership",
		Path:            normalizedPath,
		Owners:          allOwners,
		BlameOwnership:  blameOwnership,
		History:         history,
		Confidence:      confidence,
		ConfidenceBasis: confidenceBasis,
		Limitations:     limitations,
		Provenance:      provenance,
		Drilldowns:      drilldowns,
	}, nil
}
