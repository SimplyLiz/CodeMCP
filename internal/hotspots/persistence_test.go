package hotspots

import (
	"testing"
	"time"
)

func TestHotspotSnapshotBasics(t *testing.T) {
	snapshot := HotspotSnapshot{
		TargetID:             "internal/api/handler.go",
		TargetType:           "file",
		SnapshotDate:         time.Now(),
		ChurnCommits30d:      15,
		ChurnCommits90d:      45,
		ChurnAuthors30d:      3,
		ComplexityCyclomatic: 12.5,
		ComplexityCognitive:  18.2,
		CouplingAfferent:     5,
		CouplingEfferent:     10,
		CouplingInstability:  0.667,
		Score:                0.75,
	}

	// Verify fields
	if snapshot.TargetID != "internal/api/handler.go" {
		t.Errorf("Expected TargetID 'internal/api/handler.go', got '%s'", snapshot.TargetID)
	}

	if snapshot.TargetType != "file" {
		t.Errorf("Expected TargetType 'file', got '%s'", snapshot.TargetType)
	}

	if snapshot.Score != 0.75 {
		t.Errorf("Expected Score 0.75, got %f", snapshot.Score)
	}
}

func TestCalculateTrend_Increasing(t *testing.T) {
	snapshots := []HotspotSnapshot{
		{SnapshotDate: time.Now().AddDate(0, 0, -30), Score: 0.2},
		{SnapshotDate: time.Now().AddDate(0, 0, -20), Score: 0.4},
		{SnapshotDate: time.Now().AddDate(0, 0, -10), Score: 0.6},
		{SnapshotDate: time.Now(), Score: 0.8},
	}

	trend := CalculateTrend(snapshots)

	if trend.Direction != "increasing" {
		t.Errorf("Expected direction 'increasing', got '%s'", trend.Direction)
	}

	if trend.Velocity <= 0 {
		t.Errorf("Expected positive velocity for increasing trend, got %f", trend.Velocity)
	}
}

func TestCalculateTrend_Decreasing(t *testing.T) {
	snapshots := []HotspotSnapshot{
		{SnapshotDate: time.Now().AddDate(0, 0, -30), Score: 0.8},
		{SnapshotDate: time.Now().AddDate(0, 0, -20), Score: 0.6},
		{SnapshotDate: time.Now().AddDate(0, 0, -10), Score: 0.4},
		{SnapshotDate: time.Now(), Score: 0.2},
	}

	trend := CalculateTrend(snapshots)

	if trend.Direction != "decreasing" {
		t.Errorf("Expected direction 'decreasing', got '%s'", trend.Direction)
	}

	if trend.Velocity >= 0 {
		t.Errorf("Expected negative velocity for decreasing trend, got %f", trend.Velocity)
	}
}

func TestCalculateTrend_Stable(t *testing.T) {
	snapshots := []HotspotSnapshot{
		{SnapshotDate: time.Now().AddDate(0, 0, -30), Score: 0.5},
		{SnapshotDate: time.Now().AddDate(0, 0, -20), Score: 0.51},
		{SnapshotDate: time.Now().AddDate(0, 0, -10), Score: 0.49},
		{SnapshotDate: time.Now(), Score: 0.5},
	}

	trend := CalculateTrend(snapshots)

	if trend.Direction != "stable" {
		t.Errorf("Expected direction 'stable', got '%s'", trend.Direction)
	}
}

func TestCalculateTrend_InsufficientData(t *testing.T) {
	// Only one snapshot - can't calculate trend
	snapshots := []HotspotSnapshot{
		{SnapshotDate: time.Now(), Score: 0.5},
	}

	trend := CalculateTrend(snapshots)

	// With < 2 snapshots, returns "stable" as direction
	if trend.Direction != "stable" {
		t.Errorf("Expected direction 'stable' for insufficient data, got '%s'", trend.Direction)
	}

	if trend.DataPoints != 1 {
		t.Errorf("Expected DataPoints 1, got %d", trend.DataPoints)
	}
}

func TestCalculateTrend_EmptySnapshots(t *testing.T) {
	trend := CalculateTrend(nil)

	if trend.Direction != "stable" {
		t.Errorf("Expected direction 'stable' for empty snapshots, got '%s'", trend.Direction)
	}

	if trend.DataPoints != 0 {
		t.Errorf("Expected DataPoints 0, got %d", trend.DataPoints)
	}
}

func TestCalculateInstability(t *testing.T) {
	tests := []struct {
		afferent int
		efferent int
		expected float64
	}{
		{5, 10, 0.667}, // Ce / (Ca + Ce) = 10 / (5 + 10)
		{0, 10, 1.0},   // All outgoing
		{10, 0, 0.0},   // All incoming
		{0, 0, 0.5},    // No dependencies - returns neutral 0.5
		{5, 5, 0.5},    // Balanced
	}

	for _, tt := range tests {
		result := CalculateInstability(tt.afferent, tt.efferent)
		// Use approximate comparison for floats
		diff := result - tt.expected
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.01 {
			t.Errorf("CalculateInstability(%d, %d) = %f, expected ~%f",
				tt.afferent, tt.efferent, result, tt.expected)
		}
	}
}

func TestComputeCompositeScore(t *testing.T) {
	// Weights: churn 40%, coupling 30%, complexity 30%
	tests := []struct {
		churn      float64
		coupling   float64
		complexity float64
		expected   float64
	}{
		{1.0, 1.0, 1.0, 1.0}, // All maxed out
		{0.0, 0.0, 0.0, 0.0}, // All zero
		{0.5, 0.5, 0.5, 0.5}, // All half
		{1.0, 0.0, 0.0, 0.4}, // Only churn
		{0.0, 1.0, 0.0, 0.3}, // Only coupling
		{0.0, 0.0, 1.0, 0.3}, // Only complexity
	}

	for _, tt := range tests {
		result := ComputeCompositeScore(tt.churn, tt.coupling, tt.complexity)
		diff := result - tt.expected
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.01 {
			t.Errorf("ComputeCompositeScore(%f, %f, %f) = %f, expected ~%f",
				tt.churn, tt.coupling, tt.complexity, result, tt.expected)
		}
	}
}

func TestNormalizeChurnScore(t *testing.T) {
	// Test that higher churn produces higher scores
	lowChurn := NormalizeChurnScore(2, 5, 1)
	highChurn := NormalizeChurnScore(20, 50, 5)

	if highChurn <= lowChurn {
		t.Errorf("Expected high churn (%f) > low churn (%f)", highChurn, lowChurn)
	}

	// Test boundary: zero churn
	zeroChurn := NormalizeChurnScore(0, 0, 0)
	if zeroChurn != 0 {
		t.Errorf("Expected zero churn to return 0, got %f", zeroChurn)
	}

	// Scores should be between 0 and 1
	if lowChurn < 0 || lowChurn > 1 {
		t.Errorf("Low churn score out of bounds: %f", lowChurn)
	}
	if highChurn < 0 || highChurn > 1 {
		t.Errorf("High churn score out of bounds: %f", highChurn)
	}
}

func TestNormalizeCouplingScore(t *testing.T) {
	// Test that higher coupling produces higher scores
	lowCoupling := NormalizeCouplingScore(1, 1)
	highCoupling := NormalizeCouplingScore(10, 15)

	if highCoupling <= lowCoupling {
		t.Errorf("Expected high coupling (%f) > low coupling (%f)", highCoupling, lowCoupling)
	}

	// Test boundary: no coupling
	zeroCoupling := NormalizeCouplingScore(0, 0)
	if zeroCoupling != 0 {
		t.Errorf("Expected zero coupling to return 0, got %f", zeroCoupling)
	}

	// Scores should be between 0 and 1
	if lowCoupling < 0 || lowCoupling > 1 {
		t.Errorf("Low coupling score out of bounds: %f", lowCoupling)
	}
	if highCoupling < 0 || highCoupling > 1 {
		t.Errorf("High coupling score out of bounds: %f", highCoupling)
	}
}

func TestNormalizeComplexityScore(t *testing.T) {
	// Test that higher complexity produces higher scores
	lowComplexity := NormalizeComplexityScore(3, 5)
	highComplexity := NormalizeComplexityScore(20, 30)

	if highComplexity <= lowComplexity {
		t.Errorf("Expected high complexity (%f) > low complexity (%f)", highComplexity, lowComplexity)
	}

	// Test boundary: zero complexity
	zeroComplexity := NormalizeComplexityScore(0, 0)
	if zeroComplexity != 0 {
		t.Errorf("Expected zero complexity to return 0, got %f", zeroComplexity)
	}

	// Scores should be between 0 and 1
	if lowComplexity < 0 || lowComplexity > 1 {
		t.Errorf("Low complexity score out of bounds: %f", lowComplexity)
	}
	if highComplexity < 0 || highComplexity > 1 {
		t.Errorf("High complexity score out of bounds: %f", highComplexity)
	}
}

func TestTrendProjection(t *testing.T) {
	// Test that projection is calculated correctly
	snapshots := []HotspotSnapshot{
		{SnapshotDate: time.Now().AddDate(0, 0, -30), Score: 0.5},
		{SnapshotDate: time.Now(), Score: 0.8},
	}

	trend := CalculateTrend(snapshots)

	// Projection should be higher than current for increasing trend
	if trend.Projection30d <= 0.8 {
		t.Errorf("Expected projection > 0.8 for increasing trend, got %f", trend.Projection30d)
	}

	// Verify data points
	if trend.DataPoints != 2 {
		t.Errorf("Expected DataPoints 2, got %d", trend.DataPoints)
	}
}
