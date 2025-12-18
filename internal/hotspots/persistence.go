package hotspots

import (
	"time"
)

// HotspotSnapshot represents a point-in-time snapshot of hotspot metrics
type HotspotSnapshot struct {
	ID                   int64     `json:"id,omitempty"`
	TargetID             string    `json:"targetId"`             // file path, module ID, or symbol ID
	TargetType           string    `json:"targetType"`           // "file" | "module" | "symbol"
	SnapshotDate         time.Time `json:"snapshotDate"`
	ChurnCommits30d      int       `json:"churnCommits30d"`
	ChurnCommits90d      int       `json:"churnCommits90d"`
	ChurnAuthors30d      int       `json:"churnAuthors30d"`
	ComplexityCyclomatic float64   `json:"complexityCyclomatic,omitempty"`
	ComplexityCognitive  float64   `json:"complexityCognitive,omitempty"`
	CouplingAfferent     int       `json:"couplingAfferent"`     // incoming dependencies
	CouplingEfferent     int       `json:"couplingEfferent"`     // outgoing dependencies
	CouplingInstability  float64   `json:"couplingInstability"`  // efferent / (afferent + efferent)
	Score                float64   `json:"score"`                // composite hotspot score
}

// HotspotTrend represents the trend analysis for a hotspot
type HotspotTrend struct {
	Direction     string  `json:"direction"`     // "increasing" | "stable" | "decreasing"
	Velocity      float64 `json:"velocity"`      // rate of change per day
	Projection30d float64 `json:"projection30d"` // predicted score in 30 days
	DataPoints    int     `json:"dataPoints"`    // number of snapshots used
}

// CalculateTrend computes a trend from a series of snapshots
func CalculateTrend(snapshots []HotspotSnapshot) *HotspotTrend {
	if len(snapshots) < 2 {
		return &HotspotTrend{
			Direction:     "stable",
			Velocity:      0,
			Projection30d: 0,
			DataPoints:    len(snapshots),
		}
	}

	// Sort by date (oldest first)
	// Assume snapshots are already sorted

	// Calculate linear regression for velocity
	var sumX, sumY, sumXY, sumX2 float64
	n := float64(len(snapshots))

	baseDate := snapshots[0].SnapshotDate
	for i, s := range snapshots {
		x := float64(s.SnapshotDate.Sub(baseDate).Hours() / 24) // days since first snapshot
		y := s.Score
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
		_ = i
	}

	// Linear regression: y = mx + b
	// m = (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	denominator := n*sumX2 - sumX*sumX
	var velocity float64
	if denominator != 0 {
		velocity = (n*sumXY - sumX*sumY) / denominator
	}

	// Determine direction
	direction := "stable"
	if velocity > 0.01 {
		direction = "increasing"
	} else if velocity < -0.01 {
		direction = "decreasing"
	}

	// Project 30 days forward from latest snapshot
	latestScore := snapshots[len(snapshots)-1].Score
	projection30d := latestScore + velocity*30

	// Don't project negative
	if projection30d < 0 {
		projection30d = 0
	}

	return &HotspotTrend{
		Direction:     direction,
		Velocity:      velocity,
		Projection30d: projection30d,
		DataPoints:    len(snapshots),
	}
}

// CalculateInstability computes Martin's instability metric
// Instability = Ce / (Ca + Ce) where Ce = efferent, Ca = afferent
func CalculateInstability(afferent, efferent int) float64 {
	total := afferent + efferent
	if total == 0 {
		return 0.5 // Neutral if no couplings
	}
	return float64(efferent) / float64(total)
}

// ComputeCompositeScore calculates the overall hotspot score
// Weights: churn 40%, coupling 30%, complexity 30%
func ComputeCompositeScore(churnScore, couplingScore, complexityScore float64) float64 {
	return churnScore*0.4 + couplingScore*0.3 + complexityScore*0.3
}

// NormalizeChurnScore converts raw churn metrics to 0-1 score
func NormalizeChurnScore(commits30d, commits90d, authors30d int) float64 {
	// Higher commits and authors = higher score
	// Use logarithmic scaling to prevent extreme outliers
	commitScore := logNormalize(float64(commits30d), 10)      // 10 commits = 0.5
	commitScore90 := logNormalize(float64(commits90d), 30)    // 30 commits = 0.5
	authorScore := logNormalize(float64(authors30d), 3)       // 3 authors = 0.5

	// Weight recent commits more heavily
	return commitScore*0.5 + commitScore90*0.2 + authorScore*0.3
}

// logNormalize normalizes a value using logarithmic scaling
// Returns 0.5 when value equals midpoint
func logNormalize(value, midpoint float64) float64 {
	if value <= 0 {
		return 0
	}
	// Use sigmoid-like curve: 1 / (1 + e^(-k*(x-m)))
	// Simplified: value / (value + midpoint)
	return value / (value + midpoint)
}

// NormalizeCouplingScore converts coupling metrics to 0-1 score
func NormalizeCouplingScore(afferent, efferent int) float64 {
	total := afferent + efferent
	if total == 0 {
		return 0
	}
	// High coupling = high score
	return logNormalize(float64(total), 10) // 10 total dependencies = 0.5
}

// NormalizeComplexityScore converts complexity metrics to 0-1 score
func NormalizeComplexityScore(cyclomatic, cognitive float64) float64 {
	cyclomaticScore := logNormalize(cyclomatic, 10)  // 10 cyclomatic = 0.5
	cognitiveScore := logNormalize(cognitive, 15)    // 15 cognitive = 0.5
	return (cyclomaticScore + cognitiveScore) / 2
}
