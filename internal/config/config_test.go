package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Check version
	if cfg.Version != 5 {
		t.Errorf("Version = %d, want 5", cfg.Version)
	}

	// Check backends are enabled
	if !cfg.Backends.Scip.Enabled {
		t.Error("SCIP backend should be enabled by default")
	}
	if !cfg.Backends.Lsp.Enabled {
		t.Error("LSP backend should be enabled by default")
	}
	if !cfg.Backends.Git.Enabled {
		t.Error("Git backend should be enabled by default")
	}

	// Check index path
	if cfg.Backends.Scip.IndexPath != ".scip/index.scip" {
		t.Errorf("SCIP IndexPath = %q, want %q", cfg.Backends.Scip.IndexPath, ".scip/index.scip")
	}

	// Check LSP servers
	if _, ok := cfg.Backends.Lsp.Servers["go"]; !ok {
		t.Error("LSP servers should include 'go'")
	}
	if _, ok := cfg.Backends.Lsp.Servers["typescript"]; !ok {
		t.Error("LSP servers should include 'typescript'")
	}

	// Check query policy
	if len(cfg.QueryPolicy.BackendPreferenceOrder) == 0 {
		t.Error("BackendPreferenceOrder should not be empty")
	}
	if cfg.QueryPolicy.MergeMode != "prefer-first" {
		t.Errorf("MergeMode = %q, want %q", cfg.QueryPolicy.MergeMode, "prefer-first")
	}

	// Check cache settings
	if cfg.Cache.QueryTtlSeconds <= 0 {
		t.Error("QueryTtlSeconds should be positive")
	}

	// Check budget settings
	if cfg.Budget.MaxModules <= 0 {
		t.Error("MaxModules should be positive")
	}
	if cfg.Budget.EstimatedMaxTokens <= 0 {
		t.Error("EstimatedMaxTokens should be positive")
	}

	// Check daemon defaults
	if cfg.Daemon.Port != 9120 {
		t.Errorf("Daemon.Port = %d, want 9120", cfg.Daemon.Port)
	}
	if cfg.Daemon.Bind != "localhost" {
		t.Errorf("Daemon.Bind = %q, want %q", cfg.Daemon.Bind, "localhost")
	}

	// Check telemetry is disabled by default
	if cfg.Telemetry.Enabled {
		t.Error("Telemetry should be disabled by default")
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		version int
		wantErr bool
	}{
		{"version 5", 5, false},
		{"version 6", 6, false},
		{"version 1 unsupported", 1, true},
		{"version 7 unsupported", 7, true},
		{"version 0 unsupported", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Version = tt.version

			err := cfg.Validate()

			if tt.wantErr && err == nil {
				t.Error("Validate() should return error for unsupported version")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate() returned unexpected error: %v", err)
			}

			// Check error type
			if err != nil {
				if _, ok := err.(*ConfigError); !ok {
					t.Errorf("Validate() error type = %T, want *ConfigError", err)
				}
			}
		})
	}
}

func TestConfigError_Error(t *testing.T) {
	err := &ConfigError{
		Field:   "version",
		Message: "unsupported version 99",
	}

	got := err.Error()
	want := "config error in field 'version': unsupported version 99"

	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestLoadConfig_Default(t *testing.T) {
	// Create a temp directory without config
	tmpDir := t.TempDir()

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Should return default config when no config file exists
	if cfg.Version != 5 {
		t.Errorf("Version = %d, want 5 (default)", cfg.Version)
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	// Create a temp directory with config
	tmpDir := t.TempDir()
	ckbDir := filepath.Join(tmpDir, ".ckb")
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		t.Fatalf("Failed to create .ckb dir: %v", err)
	}

	configContent := `{
		"version": 5,
		"repoRoot": ".",
		"backends": {
			"scip": {"enabled": true, "indexPath": "custom/index.scip"},
			"lsp": {"enabled": false},
			"git": {"enabled": true}
		},
		"budget": {
			"maxModules": 20,
			"maxSymbolsPerModule": 10
		}
	}`

	configPath := filepath.Join(ckbDir, "config.json")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	// Check custom values were loaded
	if cfg.Backends.Scip.IndexPath != "custom/index.scip" {
		t.Errorf("Scip.IndexPath = %q, want %q", cfg.Backends.Scip.IndexPath, "custom/index.scip")
	}
	if cfg.Backends.Lsp.Enabled {
		t.Error("LSP should be disabled per config")
	}
	if cfg.Budget.MaxModules != 20 {
		t.Errorf("Budget.MaxModules = %d, want 20", cfg.Budget.MaxModules)
	}
}

func TestConfig_Save(t *testing.T) {
	// Create a temp directory
	tmpDir := t.TempDir()
	ckbDir := filepath.Join(tmpDir, ".ckb")
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		t.Fatalf("Failed to create .ckb dir: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Budget.MaxModules = 42

	err := cfg.Save(tmpDir)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file was created
	configPath := filepath.Join(ckbDir, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	// Load it back and verify
	loaded, err := LoadConfig(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfig() after save error = %v", err)
	}

	if loaded.Budget.MaxModules != 42 {
		t.Errorf("Loaded Budget.MaxModules = %d, want 42", loaded.Budget.MaxModules)
	}
}

func TestSupportedConfigVersions(t *testing.T) {
	if len(SupportedConfigVersions) == 0 {
		t.Error("SupportedConfigVersions should not be empty")
	}

	// Check that 5 and 6 are supported
	has5, has6 := false, false
	for _, v := range SupportedConfigVersions {
		if v == 5 {
			has5 = true
		}
		if v == 6 {
			has6 = true
		}
	}

	if !has5 {
		t.Error("SupportedConfigVersions should include 5")
	}
	if !has6 {
		t.Error("SupportedConfigVersions should include 6")
	}
}

func TestBackendsConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Test SCIP config
	if cfg.Backends.Scip.IndexPath == "" {
		t.Error("SCIP IndexPath should not be empty")
	}

	// Test LSP config
	if cfg.Backends.Lsp.WorkspaceStrategy == "" {
		t.Error("LSP WorkspaceStrategy should not be empty")
	}

	// Test LSP server configs
	for name, server := range cfg.Backends.Lsp.Servers {
		if server.Command == "" {
			t.Errorf("LSP server %q has empty command", name)
		}
	}
}

func TestQueryPolicyConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Check timeout settings
	if cfg.QueryPolicy.TimeoutMs == nil {
		t.Error("TimeoutMs should not be nil")
	}

	for backend, timeout := range cfg.QueryPolicy.TimeoutMs {
		if timeout <= 0 {
			t.Errorf("Timeout for %q = %d, should be positive", backend, timeout)
		}
	}

	// Check max in-flight settings
	if cfg.QueryPolicy.MaxInFlightPerBackend == nil {
		t.Error("MaxInFlightPerBackend should not be nil")
	}
}

func TestDaemonConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Check schedule settings
	if cfg.Daemon.Schedule.Refresh == "" {
		t.Error("Schedule.Refresh should not be empty")
	}
	if cfg.Daemon.Schedule.FederationSync == "" {
		t.Error("Schedule.FederationSync should not be empty")
	}

	// Check watch settings
	if cfg.Daemon.Watch.DebounceMs <= 0 {
		t.Error("Watch.DebounceMs should be positive")
	}
	if len(cfg.Daemon.Watch.IgnorePatterns) == 0 {
		t.Error("Watch.IgnorePatterns should have defaults")
	}
}

func TestTelemetryConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Telemetry should be opt-in
	if cfg.Telemetry.Enabled {
		t.Error("Telemetry should be disabled by default")
	}

	// Check aggregation defaults
	if cfg.Telemetry.Aggregation.BucketSize == "" {
		t.Error("Aggregation.BucketSize should have a default")
	}
	if cfg.Telemetry.Aggregation.RetentionDays <= 0 {
		t.Error("Aggregation.RetentionDays should be positive")
	}

	// Check dead code defaults
	if cfg.Telemetry.DeadCode.MinObservationDays <= 0 {
		t.Error("DeadCode.MinObservationDays should be positive")
	}
	if len(cfg.Telemetry.DeadCode.ExcludePatterns) == 0 {
		t.Error("DeadCode.ExcludePatterns should have defaults")
	}

	// Check attribute keys
	if len(cfg.Telemetry.Attributes.FunctionKeys) == 0 {
		t.Error("Attributes.FunctionKeys should have defaults")
	}
}

func TestModulesConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Modules.Detection != "auto" {
		t.Errorf("Modules.Detection = %q, want %q", cfg.Modules.Detection, "auto")
	}

	// Check default ignore patterns
	if len(cfg.Modules.Ignore) == 0 {
		t.Error("Modules.Ignore should have defaults")
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		validate func(t *testing.T, cfg *Config, overrides []EnvOverride)
	}{
		{
			name: "logging level override",
			envVars: map[string]string{
				"CKB_LOG_LEVEL": "debug",
			},
			validate: func(t *testing.T, cfg *Config, overrides []EnvOverride) {
				if cfg.Logging.Level != "debug" {
					t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "debug")
				}
				if len(overrides) != 1 {
					t.Errorf("len(overrides) = %d, want 1", len(overrides))
				}
			},
		},
		{
			name: "budget int override",
			envVars: map[string]string{
				"CKB_BUDGET_MAX_MODULES": "50",
			},
			validate: func(t *testing.T, cfg *Config, overrides []EnvOverride) {
				if cfg.Budget.MaxModules != 50 {
					t.Errorf("Budget.MaxModules = %d, want 50", cfg.Budget.MaxModules)
				}
			},
		},
		{
			name: "backend bool override",
			envVars: map[string]string{
				"CKB_BACKENDS_SCIP_ENABLED": "false",
			},
			validate: func(t *testing.T, cfg *Config, overrides []EnvOverride) {
				if cfg.Backends.Scip.Enabled {
					t.Error("Backends.Scip.Enabled should be false")
				}
			},
		},
		{
			name: "multiple overrides",
			envVars: map[string]string{
				"CKB_LOG_LEVEL":          "warn",
				"CKB_BUDGET_MAX_MODULES": "100",
				"CKB_TELEMETRY_ENABLED":  "true",
			},
			validate: func(t *testing.T, cfg *Config, overrides []EnvOverride) {
				if cfg.Logging.Level != "warn" {
					t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "warn")
				}
				if cfg.Budget.MaxModules != 100 {
					t.Errorf("Budget.MaxModules = %d, want 100", cfg.Budget.MaxModules)
				}
				if !cfg.Telemetry.Enabled {
					t.Error("Telemetry.Enabled should be true")
				}
				if len(overrides) != 3 {
					t.Errorf("len(overrides) = %d, want 3", len(overrides))
				}
			},
		},
		{
			name: "invalid int ignored",
			envVars: map[string]string{
				"CKB_BUDGET_MAX_MODULES": "not-a-number",
			},
			validate: func(t *testing.T, cfg *Config, overrides []EnvOverride) {
				// Should keep default value
				if cfg.Budget.MaxModules != 10 {
					t.Errorf("Budget.MaxModules = %d, want 10 (default)", cfg.Budget.MaxModules)
				}
				if len(overrides) != 0 {
					t.Errorf("len(overrides) = %d, want 0 (invalid value should be skipped)", len(overrides))
				}
			},
		},
		{
			name: "tier override",
			envVars: map[string]string{
				"CKB_TIER": "fast",
			},
			validate: func(t *testing.T, cfg *Config, overrides []EnvOverride) {
				if cfg.Tier != "fast" {
					t.Errorf("Tier = %q, want %q", cfg.Tier, "fast")
				}
			},
		},
		{
			name: "daemon port override",
			envVars: map[string]string{
				"CKB_DAEMON_PORT": "8080",
			},
			validate: func(t *testing.T, cfg *Config, overrides []EnvOverride) {
				if cfg.Daemon.Port != 8080 {
					t.Errorf("Daemon.Port = %d, want 8080", cfg.Daemon.Port)
				}
			},
		},
		{
			name: "cache ttl overrides",
			envVars: map[string]string{
				"CKB_CACHE_QUERY_TTL_SECONDS": "600",
				"CKB_CACHE_VIEW_TTL_SECONDS":  "7200",
			},
			validate: func(t *testing.T, cfg *Config, overrides []EnvOverride) {
				if cfg.Cache.QueryTtlSeconds != 600 {
					t.Errorf("Cache.QueryTtlSeconds = %d, want 600", cfg.Cache.QueryTtlSeconds)
				}
				if cfg.Cache.ViewTtlSeconds != 7200 {
					t.Errorf("Cache.ViewTtlSeconds = %d, want 7200", cfg.Cache.ViewTtlSeconds)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear any existing env vars
			for envVar := range envVarMappings {
				os.Unsetenv(envVar)
			}

			// Set test env vars
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}
			defer func() {
				for k := range tt.envVars {
					os.Unsetenv(k)
				}
			}()

			cfg := DefaultConfig()
			overrides := applyEnvOverrides(cfg)

			tt.validate(t, cfg, overrides)
		})
	}
}

func TestLoadConfigWithDetails(t *testing.T) {
	// Create a temp directory without config
	tmpDir := t.TempDir()

	// Clear env vars
	os.Unsetenv("CKB_CONFIG_PATH")
	os.Unsetenv("CKB_LOG_LEVEL")

	result, err := LoadConfigWithDetails(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfigWithDetails() error = %v", err)
	}

	if !result.UsedDefaults {
		t.Error("UsedDefaults should be true when no config file exists")
	}

	if result.ConfigPath != "" {
		t.Errorf("ConfigPath = %q, want empty string", result.ConfigPath)
	}
}

func TestLoadConfigWithDetails_EnvConfigPath(t *testing.T) {
	// Create a temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "custom-config.json")
	configContent := `{
		"version": 5,
		"budget": {"maxModules": 99}
	}`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Set CKB_CONFIG_PATH
	os.Setenv("CKB_CONFIG_PATH", configPath)
	defer os.Unsetenv("CKB_CONFIG_PATH")

	result, err := LoadConfigWithDetails(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfigWithDetails() error = %v", err)
	}

	if result.ConfigPath != configPath {
		t.Errorf("ConfigPath = %q, want %q", result.ConfigPath, configPath)
	}

	if result.Config.Budget.MaxModules != 99 {
		t.Errorf("Budget.MaxModules = %d, want 99", result.Config.Budget.MaxModules)
	}
}

func TestLoadConfigWithDetails_EnvOverridesApplied(t *testing.T) {
	tmpDir := t.TempDir()

	// Set env vars
	os.Setenv("CKB_BUDGET_MAX_MODULES", "42")
	os.Setenv("CKB_LOG_LEVEL", "error")
	defer func() {
		os.Unsetenv("CKB_BUDGET_MAX_MODULES")
		os.Unsetenv("CKB_LOG_LEVEL")
	}()

	result, err := LoadConfigWithDetails(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfigWithDetails() error = %v", err)
	}

	// Check overrides were applied
	if result.Config.Budget.MaxModules != 42 {
		t.Errorf("Budget.MaxModules = %d, want 42", result.Config.Budget.MaxModules)
	}
	if result.Config.Logging.Level != "error" {
		t.Errorf("Logging.Level = %q, want %q", result.Config.Logging.Level, "error")
	}

	// Check overrides are recorded
	if len(result.EnvOverrides) != 2 {
		t.Errorf("len(EnvOverrides) = %d, want 2", len(result.EnvOverrides))
	}
}

func TestGetSupportedEnvVars(t *testing.T) {
	vars := GetSupportedEnvVars()

	if len(vars) == 0 {
		t.Error("GetSupportedEnvVars() should return non-empty list")
	}

	// Check some expected vars are present
	hasLogLevel := false
	hasBudgetMaxModules := false
	for _, v := range vars {
		if v == "CKB_LOG_LEVEL" || v == "CKB_LOGGING_LEVEL" {
			hasLogLevel = true
		}
		if v == "CKB_BUDGET_MAX_MODULES" {
			hasBudgetMaxModules = true
		}
	}

	if !hasLogLevel {
		t.Error("GetSupportedEnvVars() should include CKB_LOG_LEVEL or CKB_LOGGING_LEVEL")
	}
	if !hasBudgetMaxModules {
		t.Error("GetSupportedEnvVars() should include CKB_BUDGET_MAX_MODULES")
	}
}
