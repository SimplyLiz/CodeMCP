package telemetry

import (
	"testing"
)

func TestCanUseUsageDisplay(t *testing.T) {
	tests := []struct {
		name     string
		level    CoverageLevel
		expected bool
	}{
		{"high level allows", CoverageHigh, true},
		{"medium level allows", CoverageMedium, true},
		{"low level allows", CoverageLow, true},
		{"insufficient blocks", CoverageInsufficient, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := TelemetryCoverage{
				Overall: OverallCoverage{Level: tt.level},
			}
			if result := c.CanUseUsageDisplay(); result != tt.expected {
				t.Errorf("CanUseUsageDisplay() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCanUseImpactEnrichment(t *testing.T) {
	tests := []struct {
		name          string
		level         CoverageLevel
		effectiveRate float64
		expected      bool
	}{
		{"high level + good rate", CoverageHigh, 0.6, true},
		{"medium level + good rate", CoverageMedium, 0.5, true},
		{"high level + low rate", CoverageHigh, 0.3, false},
		{"low level + good rate", CoverageLow, 0.8, false},
		{"insufficient level", CoverageInsufficient, 0.9, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := TelemetryCoverage{
				Overall:       OverallCoverage{Level: tt.level},
				MatchCoverage: MatchCoverage{EffectiveRate: tt.effectiveRate},
			}
			if result := c.CanUseImpactEnrichment(); result != tt.expected {
				t.Errorf("CanUseImpactEnrichment() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCanUseHotspotWeighting(t *testing.T) {
	tests := []struct {
		name          string
		level         CoverageLevel
		effectiveRate float64
		expected      bool
	}{
		{"high level + good rate", CoverageHigh, 0.5, true},
		{"medium level + good rate", CoverageMedium, 0.4, true},
		{"low level + good rate", CoverageLow, 0.4, true},
		{"low level + low rate", CoverageLow, 0.3, false},
		{"insufficient level", CoverageInsufficient, 0.9, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := TelemetryCoverage{
				Overall:       OverallCoverage{Level: tt.level},
				MatchCoverage: MatchCoverage{EffectiveRate: tt.effectiveRate},
			}
			if result := c.CanUseHotspotWeighting(); result != tt.expected {
				t.Errorf("CanUseHotspotWeighting() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDefaultCoverageRequirements(t *testing.T) {
	reqs := DefaultCoverageRequirements()

	if len(reqs) != 4 {
		t.Errorf("expected 4 requirements, got %d", len(reqs))
	}

	// Check expected features are present
	features := make(map[string]bool)
	for _, req := range reqs {
		features[req.Feature] = true
	}

	expected := []string{"dead_code_candidates", "usage_display", "impact_enrichment", "hotspot_weighting"}
	for _, f := range expected {
		if !features[f] {
			t.Errorf("expected feature %q not found", f)
		}
	}

	// Check dead_code_candidates has appropriate requirements
	for _, req := range reqs {
		if req.Feature == "dead_code_candidates" {
			if req.MinCoverageLevel != CoverageMedium {
				t.Errorf("dead_code MinCoverageLevel = %v, want %v", req.MinCoverageLevel, CoverageMedium)
			}
			if req.MinEffectiveRate < 0.5 {
				t.Errorf("dead_code MinEffectiveRate = %v, want >= 0.5", req.MinEffectiveRate)
			}
		}
	}
}

func TestCheckRequirement(t *testing.T) {
	t.Run("meets all requirements", func(t *testing.T) {
		c := TelemetryCoverage{
			Overall:       OverallCoverage{Level: CoverageHigh},
			MatchCoverage: MatchCoverage{EffectiveRate: 0.8},
		}
		req := CoverageRequirement{
			Feature:          "test",
			MinCoverageLevel: CoverageMedium,
			MinEffectiveRate: 0.5,
		}

		met, reason := c.CheckRequirement(req)
		if !met {
			t.Errorf("should meet requirement, got reason: %s", reason)
		}
		if reason != "" {
			t.Errorf("expected empty reason when met, got %q", reason)
		}
	})

	t.Run("fails level requirement", func(t *testing.T) {
		c := TelemetryCoverage{
			Overall:       OverallCoverage{Level: CoverageLow},
			MatchCoverage: MatchCoverage{EffectiveRate: 0.8},
		}
		req := CoverageRequirement{
			Feature:          "test",
			MinCoverageLevel: CoverageHigh,
			MinEffectiveRate: 0.5,
		}

		met, reason := c.CheckRequirement(req)
		if met {
			t.Error("should fail level requirement")
		}
		if reason == "" {
			t.Error("expected reason when not met")
		}
	})

	t.Run("fails effective rate requirement", func(t *testing.T) {
		c := TelemetryCoverage{
			Overall:       OverallCoverage{Level: CoverageHigh},
			MatchCoverage: MatchCoverage{EffectiveRate: 0.3},
		}
		req := CoverageRequirement{
			Feature:          "test",
			MinCoverageLevel: CoverageMedium,
			MinEffectiveRate: 0.5,
		}

		met, reason := c.CheckRequirement(req)
		if met {
			t.Error("should fail rate requirement")
		}
		if reason == "" {
			t.Error("expected reason when not met")
		}
	})

	t.Run("medium requirement accepts high", func(t *testing.T) {
		c := TelemetryCoverage{
			Overall:       OverallCoverage{Level: CoverageHigh},
			MatchCoverage: MatchCoverage{EffectiveRate: 0.8},
		}
		req := CoverageRequirement{
			Feature:          "test",
			MinCoverageLevel: CoverageMedium,
			MinEffectiveRate: 0.5,
		}

		met, _ := c.CheckRequirement(req)
		if !met {
			t.Error("high should satisfy medium requirement")
		}
	})

	t.Run("low requirement accepts medium", func(t *testing.T) {
		c := TelemetryCoverage{
			Overall:       OverallCoverage{Level: CoverageMedium},
			MatchCoverage: MatchCoverage{EffectiveRate: 0.5},
		}
		req := CoverageRequirement{
			Feature:          "test",
			MinCoverageLevel: CoverageLow,
			MinEffectiveRate: 0.0,
		}

		met, _ := c.CheckRequirement(req)
		if !met {
			t.Error("medium should satisfy low requirement")
		}
	})

	t.Run("insufficient requirement always met", func(t *testing.T) {
		c := TelemetryCoverage{
			Overall:       OverallCoverage{Level: CoverageInsufficient},
			MatchCoverage: MatchCoverage{EffectiveRate: 0.0},
		}
		req := CoverageRequirement{
			Feature:          "test",
			MinCoverageLevel: CoverageInsufficient,
			MinEffectiveRate: 0.0,
		}

		met, _ := c.CheckRequirement(req)
		if !met {
			t.Error("insufficient requirement should always be met")
		}
	})
}

func TestMinHelper(t *testing.T) {
	tests := []struct {
		a, b     float64
		expected float64
	}{
		{1.0, 2.0, 1.0},
		{2.0, 1.0, 1.0},
		{0.0, 1.0, 0.0},
		{-1.0, 1.0, -1.0},
		{0.5, 0.5, 0.5},
	}

	for _, tt := range tests {
		result := min(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("min(%v, %v) = %v, want %v", tt.a, tt.b, result, tt.expected)
		}
	}
}
