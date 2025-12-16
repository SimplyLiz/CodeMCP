package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config represents the complete CKB configuration (v5 schema)
type Config struct {
	Version  int    `json:"version" mapstructure:"version"`
	RepoRoot string `json:"repoRoot" mapstructure:"repoRoot"`

	Backends       BackendsConfig       `json:"backends" mapstructure:"backends"`
	QueryPolicy    QueryPolicyConfig    `json:"queryPolicy" mapstructure:"queryPolicy"`
	LspSupervisor  LspSupervisorConfig  `json:"lspSupervisor" mapstructure:"lspSupervisor"`
	Modules        ModulesConfig        `json:"modules" mapstructure:"modules"`
	ImportScan     ImportScanConfig     `json:"importScan" mapstructure:"importScan"`
	Cache          CacheConfig          `json:"cache" mapstructure:"cache"`
	Budget         BudgetConfig         `json:"budget" mapstructure:"budget"`
	BackendLimits  BackendLimitsConfig  `json:"backendLimits" mapstructure:"backendLimits"`
	Privacy        PrivacyConfig        `json:"privacy" mapstructure:"privacy"`
	Logging        LoggingConfig        `json:"logging" mapstructure:"logging"`
}

// BackendsConfig contains backend-specific configuration
type BackendsConfig struct {
	Scip ScipConfig `json:"scip" mapstructure:"scip"`
	Lsp  LspConfig  `json:"lsp" mapstructure:"lsp"`
	Git  GitConfig  `json:"git" mapstructure:"git"`
}

// ScipConfig contains SCIP backend configuration
type ScipConfig struct {
	Enabled   bool   `json:"enabled" mapstructure:"enabled"`
	IndexPath string `json:"indexPath" mapstructure:"indexPath"`
}

// LspConfig contains LSP backend configuration
type LspConfig struct {
	Enabled           bool                    `json:"enabled" mapstructure:"enabled"`
	WorkspaceStrategy string                  `json:"workspaceStrategy" mapstructure:"workspaceStrategy"`
	Servers           map[string]LspServerCfg `json:"servers" mapstructure:"servers"`
}

// LspServerCfg contains configuration for a single LSP server
type LspServerCfg struct {
	Command string   `json:"command" mapstructure:"command"`
	Args    []string `json:"args" mapstructure:"args"`
}

// GitConfig contains Git backend configuration
type GitConfig struct {
	Enabled bool `json:"enabled" mapstructure:"enabled"`
}

// QueryPolicyConfig contains query execution policy
type QueryPolicyConfig struct {
	BackendPreferenceOrder []string           `json:"backendPreferenceOrder" mapstructure:"backendPreferenceOrder"`
	AlwaysUse              []string           `json:"alwaysUse" mapstructure:"alwaysUse"`
	MaxInFlightPerBackend  map[string]int     `json:"maxInFlightPerBackend" mapstructure:"maxInFlightPerBackend"`
	CoalesceWindowMs       int                `json:"coalesceWindowMs" mapstructure:"coalesceWindowMs"`
	MergeMode              string             `json:"mergeMode" mapstructure:"mergeMode"`
	SupplementThreshold    float64            `json:"supplementThreshold" mapstructure:"supplementThreshold"`
	TimeoutMs              map[string]int     `json:"timeoutMs" mapstructure:"timeoutMs"`
}

// LspSupervisorConfig contains LSP supervisor configuration
type LspSupervisorConfig struct {
	MaxTotalProcesses    int `json:"maxTotalProcesses" mapstructure:"maxTotalProcesses"`
	QueueSizePerLanguage int `json:"queueSizePerLanguage" mapstructure:"queueSizePerLanguage"`
	MaxQueueWaitMs       int `json:"maxQueueWaitMs" mapstructure:"maxQueueWaitMs"`
}

// ModulesConfig contains module detection configuration
type ModulesConfig struct {
	Detection string   `json:"detection" mapstructure:"detection"`
	Roots     []string `json:"roots" mapstructure:"roots"`
	Ignore    []string `json:"ignore" mapstructure:"ignore"`
}

// ImportScanConfig contains import scanning configuration
type ImportScanConfig struct {
	Enabled          bool                       `json:"enabled" mapstructure:"enabled"`
	MaxFileSizeBytes int                        `json:"maxFileSizeBytes" mapstructure:"maxFileSizeBytes"`
	ScanTimeoutMs    int                        `json:"scanTimeoutMs" mapstructure:"scanTimeoutMs"`
	CustomPatterns   map[string]interface{}     `json:"customPatterns" mapstructure:"customPatterns"`
}

// CacheConfig contains cache configuration
type CacheConfig struct {
	QueryTtlSeconds    int `json:"queryTtlSeconds" mapstructure:"queryTtlSeconds"`
	ViewTtlSeconds     int `json:"viewTtlSeconds" mapstructure:"viewTtlSeconds"`
	NegativeTtlSeconds int `json:"negativeTtlSeconds" mapstructure:"negativeTtlSeconds"`
}

// BudgetConfig contains response budget configuration
type BudgetConfig struct {
	MaxModules           int `json:"maxModules" mapstructure:"maxModules"`
	MaxSymbolsPerModule  int `json:"maxSymbolsPerModule" mapstructure:"maxSymbolsPerModule"`
	MaxImpactItems       int `json:"maxImpactItems" mapstructure:"maxImpactItems"`
	MaxDrilldowns        int `json:"maxDrilldowns" mapstructure:"maxDrilldowns"`
	EstimatedMaxTokens   int `json:"estimatedMaxTokens" mapstructure:"estimatedMaxTokens"`
}

// BackendLimitsConfig contains backend limits
type BackendLimitsConfig struct {
	MaxRefsPerQuery      int `json:"maxRefsPerQuery" mapstructure:"maxRefsPerQuery"`
	MaxFilesScanned      int `json:"maxFilesScanned" mapstructure:"maxFilesScanned"`
	MaxUnionModeTimeMs   int `json:"maxUnionModeTimeMs" mapstructure:"maxUnionModeTimeMs"`
}

// PrivacyConfig contains privacy settings
type PrivacyConfig struct {
	Mode string `json:"mode" mapstructure:"mode"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Format string `json:"format" mapstructure:"format"`
	Level  string `json:"level" mapstructure:"level"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Version:  5,
		RepoRoot: ".",
		Backends: BackendsConfig{
			Scip: ScipConfig{
				Enabled:   true,
				IndexPath: ".scip/index.scip",
			},
			Lsp: LspConfig{
				Enabled:           true,
				WorkspaceStrategy: "repo-root",
				Servers: map[string]LspServerCfg{
					"typescript": {
						Command: "typescript-language-server",
						Args:    []string{"--stdio"},
					},
					"dart": {
						Command: "dart",
						Args:    []string{"language-server"},
					},
				},
			},
			Git: GitConfig{
				Enabled: true,
			},
		},
		QueryPolicy: QueryPolicyConfig{
			BackendPreferenceOrder: []string{"scip", "glean", "lsp"},
			AlwaysUse:              []string{"git"},
			MaxInFlightPerBackend: map[string]int{
				"scip": 10,
				"lsp":  3,
				"git":  5,
			},
			CoalesceWindowMs:    50,
			MergeMode:           "prefer-first",
			SupplementThreshold: 0.8,
			TimeoutMs: map[string]int{
				"scip": 5000,
				"lsp":  15000,
				"git":  5000,
			},
		},
		LspSupervisor: LspSupervisorConfig{
			MaxTotalProcesses:    4,
			QueueSizePerLanguage: 10,
			MaxQueueWaitMs:       200,
		},
		Modules: ModulesConfig{
			Detection: "auto",
			Roots:     []string{},
			Ignore:    []string{"node_modules", "build", ".dart_tool", "vendor"},
		},
		ImportScan: ImportScanConfig{
			Enabled:          true,
			MaxFileSizeBytes: 1000000,
			ScanTimeoutMs:    30000,
			CustomPatterns:   map[string]interface{}{},
		},
		Cache: CacheConfig{
			QueryTtlSeconds:    300,
			ViewTtlSeconds:     3600,
			NegativeTtlSeconds: 60,
		},
		Budget: BudgetConfig{
			MaxModules:          10,
			MaxSymbolsPerModule: 5,
			MaxImpactItems:      20,
			MaxDrilldowns:       5,
			EstimatedMaxTokens:  4000,
		},
		BackendLimits: BackendLimitsConfig{
			MaxRefsPerQuery:    10000,
			MaxFilesScanned:    5000,
			MaxUnionModeTimeMs: 60000,
		},
		Privacy: PrivacyConfig{
			Mode: "normal",
		},
		Logging: LoggingConfig{
			Format: "human",
			Level:  "info",
		},
	}
}

// LoadConfig loads configuration from .ckb/config.json
func LoadConfig(repoRoot string) (*Config, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("version", 5)
	v.SetDefault("repoRoot", ".")

	// Configure viper
	v.SetConfigName("config")
	v.SetConfigType("json")
	v.AddConfigPath(filepath.Join(repoRoot, ".ckb"))

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		// If config doesn't exist, return default config
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	// Unmarshal into config struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Save writes the configuration to .ckb/config.json
func (c *Config) Save(repoRoot string) error {
	configPath := filepath.Join(repoRoot, ".ckb", "config.json")

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	// Write to file
	return os.WriteFile(configPath, data, 0644)
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Basic validation
	if c.Version != 5 {
		return &ConfigError{Field: "version", Message: "unsupported config version"}
	}

	// Add more validation as needed
	return nil
}

// ConfigError represents a configuration error
type ConfigError struct {
	Field   string
	Message string
}

func (e *ConfigError) Error() string {
	return "config error in field '" + e.Field + "': " + e.Message
}
