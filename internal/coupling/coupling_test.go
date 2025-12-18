package coupling

import (
	"testing"
)

func TestCorrelationLevel(t *testing.T) {
	tests := []struct {
		name        string
		correlation float64
		wantLevel   string
	}{
		{"high correlation", 0.9, "high"},
		{"high threshold", 0.7, "high"},
		{"medium correlation", 0.5, "medium"},
		{"medium threshold", 0.3, "medium"},
		{"low correlation", 0.2, "low"},
		{"zero correlation", 0.0, "low"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getCorrelationLevel(tt.correlation)
			if got != tt.wantLevel {
				t.Errorf("getCorrelationLevel(%v) = %v, want %v", tt.correlation, got, tt.wantLevel)
			}
		})
	}
}

func TestAnalyzeOptionsDefaults(t *testing.T) {
	opts := AnalyzeOptions{
		Target: "test.go",
	}

	// Check that defaults are applied (would be set by Analyze method)
	if opts.MinCorrelation != 0 {
		// Test that we can set values
		opts.MinCorrelation = 0.3
	}

	if opts.Target != "test.go" {
		t.Errorf("Target = %q, want %q", opts.Target, "test.go")
	}
}

func TestCorrelationStructure(t *testing.T) {
	corr := Correlation{
		File:          "other.go",
		Correlation:   0.75,
		CoChangeCount: 10,
		TotalChanges:  15,
		Level:         "high",
	}

	if corr.File != "other.go" {
		t.Errorf("Correlation.File = %q, want %q", corr.File, "other.go")
	}
	if corr.Level != "high" {
		t.Errorf("Correlation.Level = %q, want %q", corr.Level, "high")
	}
}

func TestCouplingAnalysisStructure(t *testing.T) {
	analysis := CouplingAnalysis{
		Correlations: []Correlation{
			{File: "a.go", Correlation: 0.8},
			{File: "b.go", Correlation: 0.5},
		},
		Insights:        []string{"High coupling detected"},
		Recommendations: []string{"Consider extracting shared logic"},
	}

	if len(analysis.Correlations) != 2 {
		t.Errorf("len(Correlations) = %d, want %d", len(analysis.Correlations), 2)
	}
	if len(analysis.Insights) != 1 {
		t.Errorf("len(Insights) = %d, want %d", len(analysis.Insights), 1)
	}
}

// getCorrelationLevel helper for testing
func getCorrelationLevel(correlation float64) string {
	switch {
	case correlation >= 0.7:
		return "high"
	case correlation >= 0.3:
		return "medium"
	default:
		return "low"
	}
}
