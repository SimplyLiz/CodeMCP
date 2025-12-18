package audit

import (
	"math"
	"testing"
)

func TestGetRiskLevel(t *testing.T) {
	tests := []struct {
		score     float64
		wantLevel string
	}{
		{90, RiskLevelCritical},
		{80, RiskLevelCritical},
		{79, RiskLevelHigh},
		{60, RiskLevelHigh},
		{59, RiskLevelMedium},
		{40, RiskLevelMedium},
		{39, RiskLevelLow},
		{0, RiskLevelLow},
	}

	for _, tt := range tests {
		t.Run(tt.wantLevel, func(t *testing.T) {
			got := GetRiskLevel(tt.score)
			if got != tt.wantLevel {
				t.Errorf("GetRiskLevel(%v) = %q, want %q", tt.score, got, tt.wantLevel)
			}
		})
	}
}

func TestRiskWeightsSum(t *testing.T) {
	// Risk weights should sum to 1.0 (with floating point tolerance)
	var total float64
	for _, weight := range RiskWeights {
		total += weight
	}

	if math.Abs(total-1.0) > 0.0001 {
		t.Errorf("RiskWeights sum = %v, want 1.0", total)
	}
}

func TestRiskWeightsComplete(t *testing.T) {
	// Ensure all factors have weights
	factors := []string{
		FactorComplexity,
		FactorTestCoverage,
		FactorBusFactor,
		FactorStaleness,
		FactorSecuritySensitive,
		FactorErrorRate,
		FactorCoChangeCoupling,
		FactorChurn,
	}

	for _, factor := range factors {
		if _, ok := RiskWeights[factor]; !ok {
			t.Errorf("Missing weight for factor: %s", factor)
		}
	}
}

func TestSecurityKeywords(t *testing.T) {
	// Ensure security keywords list is not empty
	if len(SecurityKeywords) == 0 {
		t.Error("SecurityKeywords should not be empty")
	}

	// Check for essential keywords
	essential := []string{"password", "secret", "token", "auth"}
	for _, kw := range essential {
		found := false
		for _, sk := range SecurityKeywords {
			if sk == kw {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Missing essential security keyword: %s", kw)
		}
	}
}

func TestIsSourceFile(t *testing.T) {
	tests := []struct {
		ext      string
		wantTrue bool
	}{
		{".go", true},
		{".ts", true},
		{".py", true},
		{".java", true},
		{".rs", true},
		{".txt", false},
		{".md", false},
		{".json", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := isSourceFile(tt.ext)
			if got != tt.wantTrue {
				t.Errorf("isSourceFile(%q) = %v, want %v", tt.ext, got, tt.wantTrue)
			}
		})
	}
}

func TestMinFunction(t *testing.T) {
	tests := []struct {
		a, b    float64
		wantMin float64
	}{
		{1.0, 2.0, 1.0},
		{5.0, 3.0, 3.0},
		{0.0, 1.0, 0.0},
		{-1.0, 1.0, -1.0},
		{1.5, 1.5, 1.5},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := min(tt.a, tt.b)
			if got != tt.wantMin {
				t.Errorf("min(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.wantMin)
			}
		})
	}
}

func TestRiskFactorStructure(t *testing.T) {
	factor := RiskFactor{
		Factor:       FactorComplexity,
		Value:        "42",
		Weight:       0.20,
		Contribution: 15.0,
	}

	if factor.Factor != FactorComplexity {
		t.Errorf("RiskFactor.Factor = %q, want %q", factor.Factor, FactorComplexity)
	}
	if factor.Weight != 0.20 {
		t.Errorf("RiskFactor.Weight = %v, want %v", factor.Weight, 0.20)
	}
}

func TestQuickWinStructure(t *testing.T) {
	win := QuickWin{
		Action: "Add tests",
		Target: "src/main.go",
		Effort: "medium",
		Impact: "high",
	}

	if win.Action != "Add tests" {
		t.Errorf("QuickWin.Action = %q, want %q", win.Action, "Add tests")
	}
	if win.Effort != "medium" {
		t.Errorf("QuickWin.Effort = %q, want %q", win.Effort, "medium")
	}
}

func TestRiskSummaryStructure(t *testing.T) {
	summary := RiskSummary{
		Critical: 5,
		High:     10,
		Medium:   20,
		Low:      15,
	}

	total := summary.Critical + summary.High + summary.Medium + summary.Low
	if total != 50 {
		t.Errorf("Total items = %d, want %d", total, 50)
	}
}

func TestRiskItemStructure(t *testing.T) {
	item := RiskItem{
		File:      "src/auth/login.go",
		Module:    "auth",
		RiskScore: 75.5,
		RiskLevel: RiskLevelHigh,
		Factors: []RiskFactor{
			{Factor: FactorComplexity, Value: "45", Contribution: 15.0},
		},
		Recommendation: "Consider refactoring to reduce complexity",
	}

	if item.File != "src/auth/login.go" {
		t.Errorf("RiskItem.File = %q, want %q", item.File, "src/auth/login.go")
	}
	if item.RiskLevel != RiskLevelHigh {
		t.Errorf("RiskItem.RiskLevel = %q, want %q", item.RiskLevel, RiskLevelHigh)
	}
}

func TestAuditOptionsStructure(t *testing.T) {
	opts := AuditOptions{
		RepoRoot:  "/path/to/repo",
		MinScore:  50.0,
		Limit:     25,
		Factor:    FactorComplexity,
		QuickWins: true,
	}

	if opts.MinScore != 50.0 {
		t.Errorf("AuditOptions.MinScore = %v, want %v", opts.MinScore, 50.0)
	}
	if opts.QuickWins != true {
		t.Error("AuditOptions.QuickWins should be true")
	}
}
