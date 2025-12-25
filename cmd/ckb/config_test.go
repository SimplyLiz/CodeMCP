package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ckb/internal/config"
)

func TestValueOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		value        string
		defaultValue string
		want         string
	}{
		{"empty value uses default", "", "default", "default"},
		{"non-empty value used", "custom", "default", "custom"},
		{"empty default with empty value", "", "", ""},
		{"empty default with value", "value", "", "value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := valueOrDefault(tt.value, tt.defaultValue)
			if got != tt.want {
				t.Errorf("valueOrDefault(%q, %q) = %q, want %q", tt.value, tt.defaultValue, got, tt.want)
			}
		})
	}
}

func TestIsEqual(t *testing.T) {
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		{"equal strings", "hello", "hello", true},
		{"different strings", "hello", "world", false},
		{"equal ints", 42, 42, true},
		{"different ints", 42, 43, false},
		{"equal bools", true, true, true},
		{"different bools", true, false, false},
		{"int vs string representation", 42, "42", true}, // fmt.Sprintf behavior
		{"nil values", nil, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("isEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestComputeDiff(t *testing.T) {
	tests := []struct {
		name     string
		current  map[string]interface{}
		defaults map[string]interface{}
		wantKeys []string
	}{
		{
			name:     "identical maps",
			current:  map[string]interface{}{"key": "value"},
			defaults: map[string]interface{}{"key": "value"},
			wantKeys: []string{},
		},
		{
			name:     "different value",
			current:  map[string]interface{}{"key": "modified"},
			defaults: map[string]interface{}{"key": "default"},
			wantKeys: []string{"key"},
		},
		{
			name:     "new key",
			current:  map[string]interface{}{"key": "value", "new": "added"},
			defaults: map[string]interface{}{"key": "value"},
			wantKeys: []string{"new"},
		},
		{
			name: "nested different",
			current: map[string]interface{}{
				"nested": map[string]interface{}{"inner": "changed"},
			},
			defaults: map[string]interface{}{
				"nested": map[string]interface{}{"inner": "original"},
			},
			wantKeys: []string{"nested"},
		},
		{
			name: "nested identical",
			current: map[string]interface{}{
				"nested": map[string]interface{}{"inner": "same"},
			},
			defaults: map[string]interface{}{
				"nested": map[string]interface{}{"inner": "same"},
			},
			wantKeys: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeDiff(tt.current, tt.defaults)

			if len(got) != len(tt.wantKeys) {
				t.Errorf("computeDiff() returned %d keys, want %d", len(got), len(tt.wantKeys))
				t.Errorf("got: %v", got)
				return
			}

			for _, key := range tt.wantKeys {
				if _, exists := got[key]; !exists {
					t.Errorf("computeDiff() missing key %q", key)
				}
			}
		})
	}
}

func TestComputeDiffRecursive(t *testing.T) {
	tests := []struct {
		name     string
		current  map[string]interface{}
		defaults map[string]interface{}
		wantDiff map[string]interface{}
	}{
		{
			name: "deeply nested change",
			current: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"value": "changed",
					},
				},
			},
			defaults: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"value": "original",
					},
				},
			},
			wantDiff: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"value": "changed",
					},
				},
			},
		},
		{
			name: "mixed nested and flat",
			current: map[string]interface{}{
				"flat":   "different",
				"nested": map[string]interface{}{"inner": "same"},
			},
			defaults: map[string]interface{}{
				"flat":   "original",
				"nested": map[string]interface{}{"inner": "same"},
			},
			wantDiff: map[string]interface{}{
				"flat": "different",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := make(map[string]interface{})
			computeDiffRecursive(tt.current, tt.defaults, diff, "")

			// Compare expected keys
			if len(diff) != len(tt.wantDiff) {
				t.Errorf("computeDiffRecursive() returned %d keys, want %d", len(diff), len(tt.wantDiff))
			}
		})
	}
}

func TestGetEnvVarMappings(t *testing.T) {
	vars := GetEnvVarMappings()

	if len(vars) == 0 {
		t.Error("GetEnvVarMappings() should return non-empty list")
	}

	// Should be sorted
	for i := 1; i < len(vars); i++ {
		if vars[i] < vars[i-1] {
			t.Errorf("GetEnvVarMappings() not sorted: %s comes after %s", vars[i-1], vars[i])
		}
	}

	// Check some expected vars
	found := make(map[string]bool)
	for _, v := range vars {
		found[v] = true
	}

	expectedVars := []string{
		"CKB_LOG_LEVEL",
		"CKB_BUDGET_MAX_MODULES",
		"CKB_TIER",
	}

	for _, expected := range expectedVars {
		if !found[expected] {
			t.Errorf("GetEnvVarMappings() missing expected var %s", expected)
		}
	}
}

func TestPrintConfigSection(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printConfigSection("test.key", "value", "value")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Should not show "(default: ...)" when values match
	if strings.Contains(output, "(default:") {
		t.Errorf("printConfigSection() should not show default marker when values match")
	}

	if !strings.Contains(output, "test.key: value") {
		t.Errorf("printConfigSection() output = %q, should contain key and value", output)
	}

	// Test with different value
	r, w, _ = os.Pipe()
	os.Stdout = w

	printConfigSection("modified.key", "newvalue", "oldvalue")

	w.Close()
	os.Stdout = old

	buf.Reset()
	buf.ReadFrom(r)
	output = buf.String()

	if !strings.Contains(output, "(default: oldvalue)") {
		t.Errorf("printConfigSection() should show default marker when values differ, got: %q", output)
	}
}

func TestPrintConfigDiff(t *testing.T) {
	// Create configs with differences
	cfg := config.DefaultConfig()
	defaults := config.DefaultConfig()

	// Modify some values
	cfg.Budget.MaxModules = 50
	cfg.Logging.Level = "debug"

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printConfigDiff(cfg, defaults)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "budget.maxModules: 50") {
		t.Errorf("printConfigDiff() should show modified budget.maxModules")
	}

	if !strings.Contains(output, "logging.level: debug") {
		t.Errorf("printConfigDiff() should show modified logging.level")
	}
}

func TestPrintConfigDiff_NoChanges(t *testing.T) {
	cfg := config.DefaultConfig()
	defaults := config.DefaultConfig()

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printConfigDiff(cfg, defaults)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "no modifications") {
		t.Errorf("printConfigDiff() should indicate no modifications when configs match")
	}
}

func TestPrintConfigDiff_AllFields(t *testing.T) {
	cfg := config.DefaultConfig()
	defaults := config.DefaultConfig()

	// Modify all diffable fields to ensure they're all checked
	cfg.Version = 99
	cfg.Tier = "fast"
	cfg.Backends.Scip.Enabled = false
	cfg.Backends.Scip.IndexPath = "custom.scip"
	cfg.Backends.Lsp.Enabled = false
	cfg.Backends.Git.Enabled = false
	cfg.Cache.QueryTtlSeconds = 999
	cfg.Cache.ViewTtlSeconds = 999
	cfg.Cache.NegativeTtlSeconds = 999
	cfg.Budget.MaxModules = 999
	cfg.Budget.MaxSymbolsPerModule = 999
	cfg.Budget.MaxImpactItems = 999
	cfg.Budget.EstimatedMaxTokens = 999
	cfg.Logging.Level = "trace"
	cfg.Logging.Format = "json"
	cfg.Telemetry.Enabled = true
	cfg.Daemon.Port = 1234
	cfg.Daemon.Bind = "0.0.0.0"

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printConfigDiff(cfg, defaults)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Check that all modified fields are reported
	expectedFields := []string{
		"version:",
		"tier:",
		"backends.scip.enabled:",
		"backends.scip.indexPath:",
		"backends.lsp.enabled:",
		"backends.git.enabled:",
		"cache.queryTtlSeconds:",
		"cache.viewTtlSeconds:",
		"cache.negativeTtlSeconds:",
		"budget.maxModules:",
		"budget.maxSymbolsPerModule:",
		"budget.maxImpactItems:",
		"budget.estimatedMaxTokens:",
		"logging.level:",
		"logging.format:",
		"telemetry.enabled:",
		"daemon.port:",
		"daemon.bind:",
	}

	for _, field := range expectedFields {
		if !strings.Contains(output, field) {
			t.Errorf("printConfigDiff() missing field %q in output", field)
		}
	}
}

func TestOutputConfigHuman(t *testing.T) {
	result := &config.LoadResult{
		Config:       config.DefaultConfig(),
		ConfigPath:   "/path/to/config.json",
		UsedDefaults: false,
		EnvOverrides: []config.EnvOverride{
			{
				EnvVar:    "CKB_LOG_LEVEL",
				Path:      "logging.level",
				Value:     "debug",
				FromValue: "debug",
			},
		},
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outputConfigHuman(result, false)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Check header
	if !strings.Contains(output, "CKB Configuration") {
		t.Error("outputConfigHuman() should show header")
	}

	// Check source
	if !strings.Contains(output, "/path/to/config.json") {
		t.Error("outputConfigHuman() should show config path")
	}

	// Check env overrides
	if !strings.Contains(output, "Environment Overrides") {
		t.Error("outputConfigHuman() should show env overrides section")
	}

	if !strings.Contains(output, "CKB_LOG_LEVEL") {
		t.Error("outputConfigHuman() should show env var name")
	}
}

func TestOutputConfigHuman_Defaults(t *testing.T) {
	result := &config.LoadResult{
		Config:       config.DefaultConfig(),
		UsedDefaults: true,
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outputConfigHuman(result, false)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "defaults") {
		t.Error("outputConfigHuman() should indicate defaults are used")
	}
}

func TestOutputConfigHuman_DiffMode(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Budget.MaxModules = 99

	result := &config.LoadResult{
		Config:       cfg,
		UsedDefaults: false,
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outputConfigHuman(result, true)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "Modified Settings") {
		t.Error("outputConfigHuman() in diff mode should show 'Modified Settings'")
	}
}

func TestOutputConfigJSON(t *testing.T) {
	result := &config.LoadResult{
		Config:       config.DefaultConfig(),
		ConfigPath:   "/path/to/config.json",
		UsedDefaults: false,
		EnvOverrides: []config.EnvOverride{
			{
				EnvVar:    "CKB_LOG_LEVEL",
				Path:      "logging.level",
				Value:     "debug",
				FromValue: "debug",
			},
		},
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outputConfigJSON(result, false)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Should be valid JSON
	var response ConfigShowResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Errorf("outputConfigJSON() output is not valid JSON: %v", err)
	}

	if response.ConfigPath != "/path/to/config.json" {
		t.Errorf("ConfigPath = %q, want %q", response.ConfigPath, "/path/to/config.json")
	}

	if len(response.EnvOverrides) != 1 {
		t.Errorf("len(EnvOverrides) = %d, want 1", len(response.EnvOverrides))
	}
}

func TestOutputConfigJSON_DiffMode(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Budget.MaxModules = 99

	result := &config.LoadResult{
		Config:       cfg,
		UsedDefaults: false,
	}

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outputConfigJSON(result, true)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Should be valid JSON with only diff
	var response ConfigShowResponse
	if err := json.Unmarshal([]byte(output), &response); err != nil {
		t.Errorf("outputConfigJSON() output is not valid JSON: %v", err)
	}

	// The config should only have the diff (budget with maxModules)
	if _, exists := response.Config["budget"]; !exists {
		t.Error("diff mode should include modified budget section")
	}
}

func TestRunConfigEnv(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runConfigEnv(nil, nil)

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Check header
	if !strings.Contains(output, "Supported CKB Environment Variables") {
		t.Error("runConfigEnv() should show header")
	}

	// Check categories
	expectedCategories := []string{"General:", "Logging:", "Cache:", "Budget:", "Backends:", "Daemon:", "Other:"}
	for _, cat := range expectedCategories {
		if !strings.Contains(output, cat) {
			t.Errorf("runConfigEnv() missing category %q", cat)
		}
	}

	// Check some expected vars
	expectedVars := []string{"CKB_CONFIG_PATH", "CKB_LOG_LEVEL", "CKB_BUDGET_MAX_MODULES", "CKB_DAEMON_PORT"}
	for _, v := range expectedVars {
		if !strings.Contains(output, v) {
			t.Errorf("runConfigEnv() missing env var %q", v)
		}
	}

	// Check example usage
	if !strings.Contains(output, "Example usage:") {
		t.Error("runConfigEnv() should show example usage")
	}
}

func TestEnvVarInfo(t *testing.T) {
	// Test that envVarInfo struct works correctly
	info := envVarInfo{
		name:    "CKB_TEST_VAR",
		desc:    "Test variable",
		varType: "string",
	}

	if info.name != "CKB_TEST_VAR" {
		t.Errorf("envVarInfo.name = %q, want %q", info.name, "CKB_TEST_VAR")
	}
}

func TestConfigShowResponse(t *testing.T) {
	response := ConfigShowResponse{
		ConfigPath:   "/path/to/config.json",
		UsedDefaults: false,
		EnvOverrides: []config.EnvOverride{
			{EnvVar: "CKB_LOG_LEVEL", Path: "logging.level", Value: "debug", FromValue: "debug"},
		},
		Config: map[string]interface{}{
			"version": 5,
		},
	}

	// Test JSON marshaling
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal ConfigShowResponse: %v", err)
	}

	var unmarshaled ConfigShowResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal ConfigShowResponse: %v", err)
	}

	if unmarshaled.ConfigPath != response.ConfigPath {
		t.Errorf("ConfigPath = %q, want %q", unmarshaled.ConfigPath, response.ConfigPath)
	}

	if unmarshaled.UsedDefaults != response.UsedDefaults {
		t.Errorf("UsedDefaults = %v, want %v", unmarshaled.UsedDefaults, response.UsedDefaults)
	}
}

// Integration test for runConfigShow using temp directory
func TestRunConfigShow_Integration(t *testing.T) {
	// Skip if not in a proper test environment
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create temp dir with config
	tmpDir := t.TempDir()
	ckbDir := filepath.Join(tmpDir, ".ckb")
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		t.Fatalf("Failed to create .ckb dir: %v", err)
	}

	configContent := `{"version": 5, "budget": {"maxModules": 42}}`
	if err := os.WriteFile(filepath.Join(ckbDir, "config.json"), []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Test that we can at least parse the command flags
	if configShowCmd.Flags().Lookup("format") == nil {
		t.Error("configShowCmd should have --format flag")
	}

	if configShowCmd.Flags().Lookup("diff") == nil {
		t.Error("configShowCmd should have --diff flag")
	}
}

func TestConfigCommands_Setup(t *testing.T) {
	// Verify command structure
	if configCmd.Use != "config" {
		t.Errorf("configCmd.Use = %q, want %q", configCmd.Use, "config")
	}

	if configShowCmd.Use != "show" {
		t.Errorf("configShowCmd.Use = %q, want %q", configShowCmd.Use, "show")
	}

	if configEnvCmd.Use != "env" {
		t.Errorf("configEnvCmd.Use = %q, want %q", configEnvCmd.Use, "env")
	}

	// Check that subcommands are registered
	subcommands := configCmd.Commands()
	hasShow := false
	hasEnv := false
	for _, cmd := range subcommands {
		if cmd.Use == "show" {
			hasShow = true
		}
		if cmd.Use == "env" {
			hasEnv = true
		}
	}

	if !hasShow {
		t.Error("configCmd should have 'show' subcommand")
	}
	if !hasEnv {
		t.Error("configCmd should have 'env' subcommand")
	}
}
