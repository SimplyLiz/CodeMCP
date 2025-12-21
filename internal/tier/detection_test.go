package tier

import (
	"context"
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"version: 1.2.3", "1.2.3"},
		{"gopls 0.15.3", "0.15.3"},
		{"1.2", "1.2"},
		{"v1.2.3-beta.1", "1.2.3"}, // Regex matches simpler pattern first
		{"", ""},
		{"some random text", "some"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseVersion(tt.input)
			if got != tt.expected {
				t.Errorf("parseVersion(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		version    string
		minVersion string
		expected   bool
	}{
		{"1.2.3", "1.2.3", true},
		{"1.2.4", "1.2.3", true},
		{"1.3.0", "1.2.3", true},
		{"2.0.0", "1.2.3", true},
		{"1.2.2", "1.2.3", false},
		{"1.1.0", "1.2.3", false},
		{"0.15.3", "0.15.0", true},
		{"0.14.0", "0.15.0", false},
		{"", "1.0.0", true}, // Empty version treated as OK
		{"1.0.0", "", true}, // Empty min treated as OK
		{"v1.2.3", "v1.2.0", true},
		{"1.2.3-beta", "1.2.3", true},
	}

	for _, tt := range tests {
		t.Run(tt.version+">="+tt.minVersion, func(t *testing.T) {
			got := versionAtLeast(tt.version, tt.minVersion)
			if got != tt.expected {
				t.Errorf("versionAtLeast(%q, %q) = %v, want %v", tt.version, tt.minVersion, got, tt.expected)
			}
		})
	}
}

func TestToolDetector_CheckTool_Found(t *testing.T) {
	mock := NewMockRunner()
	mock.SetLookPath("scip-go", "/usr/local/bin/scip-go")
	mock.SetCommand("scip-go", "v1.2.3", "", nil)

	detector := NewToolDetector(mock, 5*time.Second)
	ctx := context.Background()

	req := IndexerRequirement{
		Name:         "scip-go",
		Binary:       "scip-go",
		VersionArgs:  []string{"--version"},
		Provider:     ProviderSCIP,
		Capabilities: []Capability{CapDefinitions, CapReferences},
	}

	status := detector.CheckTool(ctx, req)

	if !status.Found {
		t.Error("expected tool to be found")
	}
	if status.Path != "/usr/local/bin/scip-go" {
		t.Errorf("expected path /usr/local/bin/scip-go, got %s", status.Path)
	}
	if status.Version != "1.2.3" {
		t.Errorf("expected version 1.2.3, got %s", status.Version)
	}
	if !status.VersionOK {
		t.Error("expected version to be OK")
	}
}

func TestToolDetector_CheckTool_NotFound(t *testing.T) {
	mock := NewMockRunner()
	// Don't set any paths - tool is not found

	detector := NewToolDetector(mock, 5*time.Second)
	ctx := context.Background()

	req := IndexerRequirement{
		Name:   "missing-tool",
		Binary: "missing-tool",
	}

	status := detector.CheckTool(ctx, req)

	if status.Found {
		t.Error("expected tool not to be found")
	}
	if status.Error != "not found in PATH" {
		t.Errorf("expected error 'not found in PATH', got %s", status.Error)
	}
}

func TestToolDetector_CheckTool_VersionTooLow(t *testing.T) {
	mock := NewMockRunner()
	mock.SetLookPath("gopls", "/usr/local/bin/gopls")
	mock.SetCommand("gopls", "v0.14.0", "", nil)

	detector := NewToolDetector(mock, 5*time.Second)
	ctx := context.Background()

	req := IndexerRequirement{
		Name:        "gopls",
		Binary:      "gopls",
		VersionArgs: []string{"version"},
		MinVersion:  "0.15.0",
		Provider:    ProviderLSP,
	}

	status := detector.CheckTool(ctx, req)

	if !status.Found {
		t.Error("expected tool to be found")
	}
	if status.VersionOK {
		t.Error("expected version NOT to be OK")
	}
	if status.Error == "" {
		t.Error("expected error about version")
	}
}

func TestToolDetector_Caching(t *testing.T) {
	mock := NewMockRunner()
	mock.SetLookPath("scip-go", "/usr/local/bin/scip-go")
	mock.SetCommand("scip-go", "v1.0.0", "", nil)

	detector := NewToolDetector(mock, 5*time.Second)
	ctx := context.Background()

	req := IndexerRequirement{
		Name:        "scip-go",
		Binary:      "scip-go",
		VersionArgs: []string{"--version"},
	}

	// First call
	status1 := detector.CheckTool(ctx, req)

	// Change the mock response
	mock.SetCommand("scip-go", "v2.0.0", "", nil)

	// Second call should return cached result
	status2 := detector.CheckTool(ctx, req)

	if status1.Version != status2.Version {
		t.Errorf("expected cached result, got different versions: %s vs %s", status1.Version, status2.Version)
	}

	// Clear cache and check again
	detector.ClearCache()
	status3 := detector.CheckTool(ctx, req)

	if status3.Version != "2.0.0" {
		t.Errorf("expected fresh result after cache clear, got %s", status3.Version)
	}
}

func TestToolDetector_DetectLanguageTier(t *testing.T) {
	mock := NewMockRunner()
	mock.SetLookPath("scip-go", "/usr/local/bin/scip-go")
	mock.SetCommand("scip-go", "v1.0.0", "", nil)

	detector := NewToolDetector(mock, 5*time.Second)
	ctx := context.Background()

	status := detector.DetectLanguageTier(ctx, LangGo)

	if status.ToolTier != TierEnhanced {
		t.Errorf("expected TierEnhanced, got %v", status.ToolTier)
	}
	if !status.Capabilities[string(CapReferences)] {
		t.Error("expected CapReferences to be available")
	}
}

func TestToolDetector_DetectLanguageTier_BasicOnly(t *testing.T) {
	mock := NewMockRunner()
	// Don't set up any tools - should fall back to basic

	detector := NewToolDetector(mock, 5*time.Second)
	ctx := context.Background()

	status := detector.DetectLanguageTier(ctx, LangGo)

	if status.ToolTier != TierBasic {
		t.Errorf("expected TierBasic, got %v", status.ToolTier)
	}
	if len(status.Missing) == 0 {
		t.Error("expected missing tools to be reported")
	}
}

func TestToolDetector_DetectAllLanguages_Concurrent(t *testing.T) {
	mock := NewMockRunner()
	mock.SetLookPath("scip-go", "/usr/local/bin/scip-go")
	mock.SetCommand("scip-go", "v1.0.0", "", nil)

	detector := NewToolDetector(mock, 5*time.Second)
	ctx := context.Background()

	languages := []Language{LangGo, LangTypeScript, LangPython}
	results := detector.DetectAllLanguages(ctx, languages)

	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Go should be enhanced (scip-go is available)
	if results[LangGo].ToolTier != TierEnhanced {
		t.Errorf("expected Go to be TierEnhanced, got %v", results[LangGo].ToolTier)
	}

	// TypeScript should be basic (no tools configured)
	if results[LangTypeScript].ToolTier != TierBasic {
		t.Errorf("expected TypeScript to be TierBasic, got %v", results[LangTypeScript].ToolTier)
	}
}

func TestSortedLanguages(t *testing.T) {
	results := map[Language]LanguageToolStatus{
		LangPython:     {},
		LangGo:         {},
		LangTypeScript: {},
	}

	sorted := SortedLanguages(results)

	expected := []Language{LangGo, LangPython, LangTypeScript}
	for i, lang := range sorted {
		if lang != expected[i] {
			t.Errorf("expected %s at position %d, got %s", expected[i], i, lang)
		}
	}
}

func TestSortedTools(t *testing.T) {
	tools := []ToolStatus{
		{Name: "gopls"},
		{Name: "scip-go"},
		{Name: "delve"},
	}

	sorted := SortedTools(tools)

	expected := []string{"delve", "gopls", "scip-go"}
	for i, tool := range sorted {
		if tool.Name != expected[i] {
			t.Errorf("expected %s at position %d, got %s", expected[i], i, tool.Name)
		}
	}
}
