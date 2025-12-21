package tier

import (
	"testing"
)

func TestParseTierMode(t *testing.T) {
	tests := []struct {
		input    string
		expected TierMode
		wantErr  bool
	}{
		{"", TierModeAuto, false},
		{"auto", TierModeAuto, false},
		{"fast", TierModeFast, false},
		{"standard", TierModeStandard, false},
		{"full", TierModeFull, false},
		{"invalid", TierModeAuto, true},
		{"FAST", TierModeAuto, true}, // Case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseTierMode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTierMode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ParseTierMode(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestTierString(t *testing.T) {
	tests := []struct {
		tier     AnalysisTier
		expected string
	}{
		{TierBasic, "Fast"},
		{TierEnhanced, "Standard"},
		{TierFull, "Full"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.tier.String(); got != tt.expected {
				t.Errorf("tier.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestTierModeName(t *testing.T) {
	tests := []struct {
		tier     AnalysisTier
		expected string
	}{
		{TierBasic, "fast"},
		{TierEnhanced, "standard"},
		{TierFull, "full"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.tier.ModeName(); got != tt.expected {
				t.Errorf("tier.ModeName() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestResolveTier_Auto(t *testing.T) {
	d := NewDetector()
	d.SetRequestedMode(TierModeAuto)

	// No backends available
	tier, err := d.ResolveTier()
	if err != nil {
		t.Errorf("ResolveTier() unexpected error: %v", err)
	}
	if tier != TierBasic {
		t.Errorf("ResolveTier() = %v, want %v", tier, TierBasic)
	}

	// SCIP available
	d.SetScipAvailable(true)
	tier, err = d.ResolveTier()
	if err != nil {
		t.Errorf("ResolveTier() unexpected error: %v", err)
	}
	if tier != TierEnhanced {
		t.Errorf("ResolveTier() = %v, want %v", tier, TierEnhanced)
	}

	// Both SCIP and telemetry available
	d.SetTelemetryAvailable(true)
	tier, err = d.ResolveTier()
	if err != nil {
		t.Errorf("ResolveTier() unexpected error: %v", err)
	}
	if tier != TierFull {
		t.Errorf("ResolveTier() = %v, want %v", tier, TierFull)
	}
}

func TestResolveTier_Fast(t *testing.T) {
	d := NewDetector()
	d.SetScipAvailable(true)
	d.SetTelemetryAvailable(true)
	d.SetRequestedMode(TierModeFast)

	// Fast mode should always work, even with SCIP+telemetry available
	tier, err := d.ResolveTier()
	if err != nil {
		t.Errorf("ResolveTier() unexpected error: %v", err)
	}
	if tier != TierBasic {
		t.Errorf("ResolveTier() = %v, want %v (fast mode ignores backends)", tier, TierBasic)
	}
}

func TestResolveTier_Standard_RequiresSCIP(t *testing.T) {
	d := NewDetector()
	d.SetRequestedMode(TierModeStandard)

	// Standard without SCIP should error
	_, err := d.ResolveTier()
	if err == nil {
		t.Error("ResolveTier() expected error when requesting standard without SCIP")
	}

	// Standard with SCIP should work
	d.SetScipAvailable(true)
	tier, err := d.ResolveTier()
	if err != nil {
		t.Errorf("ResolveTier() unexpected error: %v", err)
	}
	if tier != TierEnhanced {
		t.Errorf("ResolveTier() = %v, want %v", tier, TierEnhanced)
	}
}

func TestResolveTier_Full_RequiresTelemetry(t *testing.T) {
	d := NewDetector()
	d.SetScipAvailable(true)
	d.SetRequestedMode(TierModeFull)

	// Full without telemetry should error
	_, err := d.ResolveTier()
	if err == nil {
		t.Error("ResolveTier() expected error when requesting full without telemetry")
	}

	// Full with telemetry should work
	d.SetTelemetryAvailable(true)
	tier, err := d.ResolveTier()
	if err != nil {
		t.Errorf("ResolveTier() unexpected error: %v", err)
	}
	if tier != TierFull {
		t.Errorf("ResolveTier() = %v, want %v", tier, TierFull)
	}
}

func TestIsExplicitMode(t *testing.T) {
	d := NewDetector()

	if d.IsExplicitMode() {
		t.Error("new detector should not be in explicit mode")
	}

	d.SetRequestedMode(TierModeFast)
	if !d.IsExplicitMode() {
		t.Error("detector should be in explicit mode after setting fast")
	}

	d.SetRequestedMode(TierModeAuto)
	if d.IsExplicitMode() {
		t.Error("detector should not be in explicit mode after setting auto")
	}
}

func TestGetTierInfo_ModeDisplay(t *testing.T) {
	d := NewDetector()
	d.SetScipAvailable(true)

	// Auto-detected mode
	info := d.GetTierInfo()
	if info.Mode != "auto-detected" {
		t.Errorf("TierInfo.Mode = %q, want 'auto-detected'", info.Mode)
	}
	if info.Explicit {
		t.Error("TierInfo.Explicit should be false for auto mode")
	}

	// Explicit fast mode
	d.SetRequestedMode(TierModeFast)
	info = d.GetTierInfo()
	if info.Mode != "fast" {
		t.Errorf("TierInfo.Mode = %q, want 'fast'", info.Mode)
	}
	if !info.Explicit {
		t.Error("TierInfo.Explicit should be true for explicit mode")
	}
	if info.CurrentName != "Fast" {
		t.Errorf("TierInfo.CurrentName = %q, want 'Fast'", info.CurrentName)
	}

	// Explicit standard mode
	d.SetRequestedMode(TierModeStandard)
	info = d.GetTierInfo()
	if info.Mode != "standard" {
		t.Errorf("TierInfo.Mode = %q, want 'standard'", info.Mode)
	}
	if info.CurrentName != "Standard" {
		t.Errorf("TierInfo.CurrentName = %q, want 'Standard'", info.CurrentName)
	}
}

func TestValidTierModes(t *testing.T) {
	modes := ValidTierModes()
	expected := []string{"auto", "fast", "standard", "full"}

	if len(modes) != len(expected) {
		t.Errorf("ValidTierModes() = %v, want %v", modes, expected)
	}

	for i, mode := range modes {
		if mode != expected[i] {
			t.Errorf("ValidTierModes()[%d] = %q, want %q", i, mode, expected[i])
		}
	}
}
