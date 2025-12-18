package federation

import (
	"time"
)

// StalenessLevel represents how stale data is
type StalenessLevel string

const (
	// StalenessLevelFresh indicates data is fresh (< 7 days, < 50 commits)
	StalenessLevelFresh StalenessLevel = "fresh"

	// StalenessLevelAging indicates data is aging (7-30 days or 50-200 commits)
	StalenessLevelAging StalenessLevel = "aging"

	// StalenessLevelStale indicates data is stale (30-90 days or 200-500 commits)
	StalenessLevelStale StalenessLevel = "stale"

	// StalenessLevelObsolete indicates data is obsolete (> 90 days or > 500 commits)
	StalenessLevelObsolete StalenessLevel = "obsolete"
)

// StalenessInfo contains staleness information for data
type StalenessInfo struct {
	// DataAge is time since last update
	DataAge time.Duration `json:"dataAge"`

	// CodeChanges is the number of commits since last update
	CodeChanges int `json:"codeChanges"`

	// Staleness is the staleness level
	Staleness StalenessLevel `json:"staleness"`

	// RefreshRecommended indicates if a refresh is recommended
	RefreshRecommended bool `json:"refreshRecommended"`

	// LastSyncedAt is when the data was last synced
	LastSyncedAt *time.Time `json:"lastSyncedAt,omitempty"`
}

// ComputeStaleness computes staleness from age and commit count
func ComputeStaleness(lastSync *time.Time, commitsSinceSync int) StalenessInfo {
	info := StalenessInfo{
		CodeChanges:  commitsSinceSync,
		LastSyncedAt: lastSync,
	}

	if lastSync == nil {
		info.Staleness = StalenessLevelObsolete
		info.RefreshRecommended = true
		return info
	}

	info.DataAge = time.Since(*lastSync)
	ageDays := int(info.DataAge.Hours() / 24)

	// Determine staleness level based on both age and commits
	switch {
	case ageDays > 90 || commitsSinceSync > 500:
		info.Staleness = StalenessLevelObsolete
		info.RefreshRecommended = true
	case ageDays > 30 || commitsSinceSync > 200:
		info.Staleness = StalenessLevelStale
		info.RefreshRecommended = true
	case ageDays > 7 || commitsSinceSync > 50:
		info.Staleness = StalenessLevelAging
		info.RefreshRecommended = false
	default:
		info.Staleness = StalenessLevelFresh
		info.RefreshRecommended = false
	}

	return info
}

// stalenessRank returns a numeric rank for staleness levels (higher = worse)
func stalenessRank(level StalenessLevel) int {
	switch level {
	case StalenessLevelFresh:
		return 0
	case StalenessLevelAging:
		return 1
	case StalenessLevelStale:
		return 2
	case StalenessLevelObsolete:
		return 3
	default:
		return 4
	}
}

// WorseOf returns the worse of two staleness levels
func WorseOf(a, b StalenessLevel) StalenessLevel {
	if stalenessRank(a) > stalenessRank(b) {
		return a
	}
	return b
}

// RepoStaleness contains staleness info for a repository
type RepoStaleness struct {
	RepoID    string        `json:"repoId"`
	Staleness StalenessInfo `json:"staleness"`
}

// FederationStaleness contains overall federation staleness
type FederationStaleness struct {
	// OverallStaleness is the worst staleness across all repos
	OverallStaleness StalenessLevel `json:"overallStaleness"`

	// RefreshRecommended is true if any repo needs refresh
	RefreshRecommended bool `json:"refreshRecommended"`

	// ReposChecked is the number of repos checked
	ReposChecked int `json:"reposChecked"`

	// PerRepo contains staleness info per repository
	PerRepo []RepoStaleness `json:"perRepo,omitempty"`
}

// ComputeFederationStaleness aggregates staleness across repos
// Uses "weakest link" - federation is as stale as its stalest repo
func ComputeFederationStaleness(repos []RepoStaleness) FederationStaleness {
	result := FederationStaleness{
		OverallStaleness:   StalenessLevelFresh,
		RefreshRecommended: false,
		ReposChecked:       len(repos),
		PerRepo:            repos,
	}

	for _, repo := range repos {
		result.OverallStaleness = WorseOf(result.OverallStaleness, repo.Staleness.Staleness)
		if repo.Staleness.RefreshRecommended {
			result.RefreshRecommended = true
		}
	}

	return result
}
