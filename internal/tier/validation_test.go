package tier

import (
	"context"
	"testing"
	"time"
)

func TestValidator_Validate_AllSatisfied(t *testing.T) {
	mock := NewMockRunner()
	mock.SetLookPath("scip-go", "/usr/local/bin/scip-go")
	mock.SetCommand("scip-go", "v1.0.0", "", nil)

	detector := NewToolDetector(mock, 5*time.Second)
	config := ValidationConfig{
		RequestedTier: TierEnhanced,
		AllowFallback: false,
	}
	validator := NewValidator(detector, config)

	ctx := context.Background()
	result := validator.Validate(ctx, []Language{LangGo})

	if !result.AllSatisfied {
		t.Error("expected AllSatisfied to be true")
	}
	if result.Degraded {
		t.Error("expected Degraded to be false")
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors, got: %v", result.Errors)
	}
}

func TestValidator_Validate_NotSatisfied_NoFallback(t *testing.T) {
	mock := NewMockRunner()
	// No tools available

	detector := NewToolDetector(mock, 5*time.Second)
	config := ValidationConfig{
		RequestedTier: TierEnhanced,
		AllowFallback: false,
	}
	validator := NewValidator(detector, config)

	ctx := context.Background()
	result := validator.Validate(ctx, []Language{LangGo})

	if result.AllSatisfied {
		t.Error("expected AllSatisfied to be false")
	}
	if !result.Degraded {
		t.Error("expected Degraded to be true")
	}
	if len(result.Errors) == 0 {
		t.Error("expected errors when AllowFallback is false")
	}
}

func TestValidator_Validate_NotSatisfied_WithFallback(t *testing.T) {
	mock := NewMockRunner()
	// No tools available

	detector := NewToolDetector(mock, 5*time.Second)
	config := ValidationConfig{
		RequestedTier: TierEnhanced,
		AllowFallback: true,
	}
	validator := NewValidator(detector, config)

	ctx := context.Background()
	result := validator.Validate(ctx, []Language{LangGo})

	if result.AllSatisfied {
		t.Error("expected AllSatisfied to be false")
	}
	if !result.Degraded {
		t.Error("expected Degraded to be true")
	}
	if len(result.Errors) > 0 {
		t.Errorf("expected no errors when AllowFallback is true, got: %v", result.Errors)
	}
}

func TestValidator_Validate_PerLanguageTier(t *testing.T) {
	mock := NewMockRunner()
	mock.SetLookPath("scip-go", "/usr/local/bin/scip-go")
	mock.SetCommand("scip-go", "v1.0.0", "", nil)

	detector := NewToolDetector(mock, 5*time.Second)
	config := ValidationConfig{
		RequestedTier: TierFull, // Global default
		AllowFallback: false,
		PerLanguageTier: map[Language]AnalysisTier{
			LangGo: TierEnhanced, // Override for Go
		},
	}
	validator := NewValidator(detector, config)

	ctx := context.Background()
	result := validator.Validate(ctx, []Language{LangGo})

	// Go should satisfy its per-language tier (Enhanced)
	if !result.AllSatisfied {
		t.Error("expected AllSatisfied to be true with per-language override")
	}

	goValidation := result.Languages[LangGo]
	if goValidation.RequestedTier != TierEnhanced {
		t.Errorf("expected requested tier to be Enhanced, got %v", goValidation.RequestedTier)
	}
}

func TestValidator_ValidateSingle(t *testing.T) {
	mock := NewMockRunner()
	mock.SetLookPath("scip-go", "/usr/local/bin/scip-go")
	mock.SetCommand("scip-go", "v1.0.0", "", nil)

	detector := NewToolDetector(mock, 5*time.Second)
	config := ValidationConfig{
		RequestedTier: TierEnhanced,
	}
	validator := NewValidator(detector, config)

	ctx := context.Background()
	validation := validator.ValidateSingle(ctx, LangGo)

	if !validation.Satisfied {
		t.Error("expected validation to be satisfied")
	}
	if validation.ToolTier != TierEnhanced {
		t.Errorf("expected ToolTier to be Enhanced, got %v", validation.ToolTier)
	}
}

func TestGetEffectiveTier(t *testing.T) {
	tests := []struct {
		name        string
		toolTier    AnalysisTier
		runtimeTier AnalysisTier
		expected    AnalysisTier
	}{
		{
			name:        "same tier",
			toolTier:    TierEnhanced,
			runtimeTier: TierEnhanced,
			expected:    TierEnhanced,
		},
		{
			name:        "runtime lower",
			toolTier:    TierFull,
			runtimeTier: TierEnhanced,
			expected:    TierEnhanced,
		},
		{
			name:        "tool lower",
			toolTier:    TierBasic,
			runtimeTier: TierEnhanced,
			expected:    TierBasic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validation := LanguageValidation{
				ToolTier:    tt.toolTier,
				RuntimeTier: tt.runtimeTier,
			}
			got := GetEffectiveTier(validation)
			if got != tt.expected {
				t.Errorf("GetEffectiveTier() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetCapabilitySummary(t *testing.T) {
	validation := LanguageValidation{
		Capabilities: map[string]bool{
			string(CapDefinitions): true,
			string(CapReferences):  true,
			string(CapCallGraph):   true,
		},
		CapabilityProviders: map[string]string{
			string(CapDefinitions): "scip-go",
			string(CapReferences):  "scip-go",
		},
	}

	summary := GetCapabilitySummary(validation)

	if len(summary.Available) != 3 {
		t.Errorf("expected 3 available capabilities, got %d", len(summary.Available))
	}
	if len(summary.Unavailable) == 0 {
		t.Error("expected some unavailable capabilities")
	}
	if summary.Providers[CapDefinitions] != "scip-go" {
		t.Errorf("expected provider scip-go, got %s", summary.Providers[CapDefinitions])
	}
}

func TestSortedLanguageValidations(t *testing.T) {
	result := ValidationResult{
		Languages: map[Language]LanguageValidation{
			LangPython:     {Language: LangPython, DisplayName: "Python"},
			LangGo:         {Language: LangGo, DisplayName: "Go"},
			LangTypeScript: {Language: LangTypeScript, DisplayName: "TypeScript"},
		},
	}

	sorted := SortedLanguageValidations(result)

	expected := []Language{LangGo, LangPython, LangTypeScript}
	for i, v := range sorted {
		if v.Language != expected[i] {
			t.Errorf("expected %s at position %d, got %s", expected[i], i, v.Language)
		}
	}
}
