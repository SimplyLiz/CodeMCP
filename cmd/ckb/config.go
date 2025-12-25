package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"ckb/internal/config"
)

var (
	configFormat   string
	configShowDiff bool
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CKB configuration",
	Long:  "View and manage CKB configuration stored in .ckb/config.json",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long: `Display the current CKB configuration.

Examples:
  ckb config show              # Pretty-print current config
  ckb config show --json       # Raw JSON output
  ckb config show --diff       # Only show non-default values`,
	Run: runConfigShow,
}

var configEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "List supported environment variables",
	Long:  "Display all supported CKB environment variable overrides",
	Run:   runConfigEnv,
}

func init() {
	configShowCmd.Flags().StringVar(&configFormat, "format", "human", "Output format (json, human)")
	configShowCmd.Flags().BoolVar(&configShowDiff, "diff", false, "Only show non-default values")

	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configEnvCmd)
	rootCmd.AddCommand(configCmd)
}

// ConfigShowResponse is the response format for config show
type ConfigShowResponse struct {
	ConfigPath   string                 `json:"configPath,omitempty"`
	UsedDefaults bool                   `json:"usedDefaults"`
	EnvOverrides []config.EnvOverride   `json:"envOverrides,omitempty"`
	Config       map[string]interface{} `json:"config"`
}

func runConfigShow(cmd *cobra.Command, args []string) {
	repoRoot := mustGetRepoRoot()

	// Load config with details
	result, err := config.LoadConfigWithDetails(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if configFormat == "json" {
		outputConfigJSON(result, configShowDiff)
	} else {
		outputConfigHuman(result, configShowDiff)
	}
}

func outputConfigJSON(result *config.LoadResult, diffOnly bool) {
	// Convert config to map for JSON output
	configBytes, err := json.Marshal(result.Config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling config: %v\n", err)
		os.Exit(1)
	}

	var configMap map[string]interface{}
	if err := json.Unmarshal(configBytes, &configMap); err != nil {
		fmt.Fprintf(os.Stderr, "Error unmarshaling config: %v\n", err)
		os.Exit(1)
	}

	if diffOnly {
		// Get defaults and compute diff
		defaultBytes, _ := json.Marshal(config.DefaultConfig())
		var defaultMap map[string]interface{}
		json.Unmarshal(defaultBytes, &defaultMap)
		configMap = computeDiff(configMap, defaultMap)
	}

	response := ConfigShowResponse{
		ConfigPath:   result.ConfigPath,
		UsedDefaults: result.UsedDefaults,
		EnvOverrides: result.EnvOverrides,
		Config:       configMap,
	}

	output, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(output))
}

func outputConfigHuman(result *config.LoadResult, diffOnly bool) {
	// Header
	fmt.Println("CKB Configuration")
	fmt.Println(strings.Repeat("─", 50))

	// Source info
	if result.UsedDefaults {
		fmt.Println("Source: defaults (no config file found)")
	} else if result.ConfigPath != "" {
		fmt.Printf("Source: %s\n", result.ConfigPath)
	}

	// Env overrides
	if len(result.EnvOverrides) > 0 {
		fmt.Println("\nEnvironment Overrides:")
		for _, ov := range result.EnvOverrides {
			fmt.Printf("  %s=%s → %s\n", ov.EnvVar, ov.FromValue, ov.Path)
		}
	}

	fmt.Println()

	// Config content
	cfg := result.Config
	defaults := config.DefaultConfig()

	if diffOnly {
		fmt.Println("Modified Settings (differs from defaults):")
		fmt.Println()
		printConfigDiff(cfg, defaults)
	} else {
		printConfigSection("version", cfg.Version, defaults.Version)
		printConfigSection("tier", valueOrDefault(cfg.Tier, "auto"), "auto")

		fmt.Println("\nbackends:")
		printConfigSection("  scip.enabled", cfg.Backends.Scip.Enabled, defaults.Backends.Scip.Enabled)
		printConfigSection("  scip.indexPath", cfg.Backends.Scip.IndexPath, defaults.Backends.Scip.IndexPath)
		printConfigSection("  lsp.enabled", cfg.Backends.Lsp.Enabled, defaults.Backends.Lsp.Enabled)
		printConfigSection("  git.enabled", cfg.Backends.Git.Enabled, defaults.Backends.Git.Enabled)

		fmt.Println("\ncache:")
		printConfigSection("  queryTtlSeconds", cfg.Cache.QueryTtlSeconds, defaults.Cache.QueryTtlSeconds)
		printConfigSection("  viewTtlSeconds", cfg.Cache.ViewTtlSeconds, defaults.Cache.ViewTtlSeconds)
		printConfigSection("  negativeTtlSeconds", cfg.Cache.NegativeTtlSeconds, defaults.Cache.NegativeTtlSeconds)

		fmt.Println("\nbudget:")
		printConfigSection("  maxModules", cfg.Budget.MaxModules, defaults.Budget.MaxModules)
		printConfigSection("  maxSymbolsPerModule", cfg.Budget.MaxSymbolsPerModule, defaults.Budget.MaxSymbolsPerModule)
		printConfigSection("  maxImpactItems", cfg.Budget.MaxImpactItems, defaults.Budget.MaxImpactItems)
		printConfigSection("  maxDrilldowns", cfg.Budget.MaxDrilldowns, defaults.Budget.MaxDrilldowns)
		printConfigSection("  estimatedMaxTokens", cfg.Budget.EstimatedMaxTokens, defaults.Budget.EstimatedMaxTokens)

		fmt.Println("\nlogging:")
		printConfigSection("  level", cfg.Logging.Level, defaults.Logging.Level)
		printConfigSection("  format", cfg.Logging.Format, defaults.Logging.Format)

		fmt.Println("\nprivacy:")
		printConfigSection("  mode", cfg.Privacy.Mode, defaults.Privacy.Mode)

		fmt.Println("\ntelemetry:")
		printConfigSection("  enabled", cfg.Telemetry.Enabled, defaults.Telemetry.Enabled)

		fmt.Println("\ndaemon:")
		printConfigSection("  port", cfg.Daemon.Port, defaults.Daemon.Port)
		printConfigSection("  bind", cfg.Daemon.Bind, defaults.Daemon.Bind)
	}

	fmt.Println()
	fmt.Println("Use 'ckb config show --json' for full configuration")
	fmt.Println("Use 'ckb config env' to see supported environment variables")
}

func printConfigSection(name string, value, defaultValue interface{}) {
	modified := ""
	if !isEqual(value, defaultValue) {
		modified = fmt.Sprintf(" (default: %v)", defaultValue)
	}
	fmt.Printf("%s: %v%s\n", name, value, modified)
}

func printConfigDiff(cfg, defaults *config.Config) {
	diffs := []string{}

	if cfg.Version != defaults.Version {
		diffs = append(diffs, fmt.Sprintf("version: %d (default: %d)", cfg.Version, defaults.Version))
	}
	if cfg.Tier != defaults.Tier && cfg.Tier != "" {
		diffs = append(diffs, fmt.Sprintf("tier: %s (default: auto)", cfg.Tier))
	}

	// Backends
	if cfg.Backends.Scip.Enabled != defaults.Backends.Scip.Enabled {
		diffs = append(diffs, fmt.Sprintf("backends.scip.enabled: %v (default: %v)", cfg.Backends.Scip.Enabled, defaults.Backends.Scip.Enabled))
	}
	if cfg.Backends.Scip.IndexPath != defaults.Backends.Scip.IndexPath {
		diffs = append(diffs, fmt.Sprintf("backends.scip.indexPath: %s (default: %s)", cfg.Backends.Scip.IndexPath, defaults.Backends.Scip.IndexPath))
	}
	if cfg.Backends.Lsp.Enabled != defaults.Backends.Lsp.Enabled {
		diffs = append(diffs, fmt.Sprintf("backends.lsp.enabled: %v (default: %v)", cfg.Backends.Lsp.Enabled, defaults.Backends.Lsp.Enabled))
	}
	if cfg.Backends.Git.Enabled != defaults.Backends.Git.Enabled {
		diffs = append(diffs, fmt.Sprintf("backends.git.enabled: %v (default: %v)", cfg.Backends.Git.Enabled, defaults.Backends.Git.Enabled))
	}

	// Cache
	if cfg.Cache.QueryTtlSeconds != defaults.Cache.QueryTtlSeconds {
		diffs = append(diffs, fmt.Sprintf("cache.queryTtlSeconds: %d (default: %d)", cfg.Cache.QueryTtlSeconds, defaults.Cache.QueryTtlSeconds))
	}
	if cfg.Cache.ViewTtlSeconds != defaults.Cache.ViewTtlSeconds {
		diffs = append(diffs, fmt.Sprintf("cache.viewTtlSeconds: %d (default: %d)", cfg.Cache.ViewTtlSeconds, defaults.Cache.ViewTtlSeconds))
	}
	if cfg.Cache.NegativeTtlSeconds != defaults.Cache.NegativeTtlSeconds {
		diffs = append(diffs, fmt.Sprintf("cache.negativeTtlSeconds: %d (default: %d)", cfg.Cache.NegativeTtlSeconds, defaults.Cache.NegativeTtlSeconds))
	}

	// Budget
	if cfg.Budget.MaxModules != defaults.Budget.MaxModules {
		diffs = append(diffs, fmt.Sprintf("budget.maxModules: %d (default: %d)", cfg.Budget.MaxModules, defaults.Budget.MaxModules))
	}
	if cfg.Budget.MaxSymbolsPerModule != defaults.Budget.MaxSymbolsPerModule {
		diffs = append(diffs, fmt.Sprintf("budget.maxSymbolsPerModule: %d (default: %d)", cfg.Budget.MaxSymbolsPerModule, defaults.Budget.MaxSymbolsPerModule))
	}
	if cfg.Budget.MaxImpactItems != defaults.Budget.MaxImpactItems {
		diffs = append(diffs, fmt.Sprintf("budget.maxImpactItems: %d (default: %d)", cfg.Budget.MaxImpactItems, defaults.Budget.MaxImpactItems))
	}
	if cfg.Budget.EstimatedMaxTokens != defaults.Budget.EstimatedMaxTokens {
		diffs = append(diffs, fmt.Sprintf("budget.estimatedMaxTokens: %d (default: %d)", cfg.Budget.EstimatedMaxTokens, defaults.Budget.EstimatedMaxTokens))
	}

	// Logging
	if cfg.Logging.Level != defaults.Logging.Level {
		diffs = append(diffs, fmt.Sprintf("logging.level: %s (default: %s)", cfg.Logging.Level, defaults.Logging.Level))
	}
	if cfg.Logging.Format != defaults.Logging.Format {
		diffs = append(diffs, fmt.Sprintf("logging.format: %s (default: %s)", cfg.Logging.Format, defaults.Logging.Format))
	}

	// Telemetry
	if cfg.Telemetry.Enabled != defaults.Telemetry.Enabled {
		diffs = append(diffs, fmt.Sprintf("telemetry.enabled: %v (default: %v)", cfg.Telemetry.Enabled, defaults.Telemetry.Enabled))
	}

	// Daemon
	if cfg.Daemon.Port != defaults.Daemon.Port {
		diffs = append(diffs, fmt.Sprintf("daemon.port: %d (default: %d)", cfg.Daemon.Port, defaults.Daemon.Port))
	}
	if cfg.Daemon.Bind != defaults.Daemon.Bind {
		diffs = append(diffs, fmt.Sprintf("daemon.bind: %s (default: %s)", cfg.Daemon.Bind, defaults.Daemon.Bind))
	}

	if len(diffs) == 0 {
		fmt.Println("  (no modifications - using all defaults)")
	} else {
		for _, d := range diffs {
			fmt.Printf("  %s\n", d)
		}
	}
}

func runConfigEnv(cmd *cobra.Command, args []string) {
	fmt.Println("Supported CKB Environment Variables")
	fmt.Println(strings.Repeat("─", 50))
	fmt.Println()

	// Group by category
	categories := map[string][]envVarInfo{
		"General": {
			{"CKB_CONFIG_PATH", "Path to config file", "string"},
			{"CKB_TIER", "Analysis tier (fast, standard, full, auto)", "string"},
			{"CKB_REPO", "Repository path (for MCP without project context)", "string"},
		},
		"Logging": {
			{"CKB_LOG_LEVEL", "Log level (debug, info, warn, error)", "string"},
			{"CKB_LOG_FORMAT", "Log format (human, json)", "string"},
		},
		"Cache": {
			{"CKB_CACHE_QUERY_TTL_SECONDS", "Query cache TTL", "int"},
			{"CKB_CACHE_VIEW_TTL_SECONDS", "View cache TTL", "int"},
			{"CKB_CACHE_NEGATIVE_TTL_SECONDS", "Negative cache TTL", "int"},
		},
		"Budget": {
			{"CKB_BUDGET_MAX_MODULES", "Max modules in response", "int"},
			{"CKB_BUDGET_MAX_SYMBOLS_PER_MODULE", "Max symbols per module", "int"},
			{"CKB_BUDGET_MAX_IMPACT_ITEMS", "Max impact items", "int"},
			{"CKB_BUDGET_ESTIMATED_MAX_TOKENS", "Target token budget", "int"},
		},
		"Backends": {
			{"CKB_BACKENDS_SCIP_ENABLED", "Enable SCIP backend", "bool"},
			{"CKB_BACKENDS_LSP_ENABLED", "Enable LSP backend", "bool"},
			{"CKB_BACKENDS_GIT_ENABLED", "Enable Git backend", "bool"},
		},
		"Daemon": {
			{"CKB_DAEMON_PORT", "Daemon HTTP port", "int"},
			{"CKB_DAEMON_BIND", "Daemon bind address", "string"},
		},
		"Other": {
			{"CKB_TELEMETRY_ENABLED", "Enable telemetry features", "bool"},
			{"CKB_PRIVACY_MODE", "Privacy mode (normal, redacted)", "string"},
			{"CKB_NO_UPDATE_CHECK", "Disable update notifications", "bool"},
			{"CKB_AUTH_TOKEN", "Auth token for serve command", "string"},
		},
	}

	order := []string{"General", "Logging", "Cache", "Budget", "Backends", "Daemon", "Other"}
	for _, cat := range order {
		vars := categories[cat]
		fmt.Printf("%s:\n", cat)
		for _, v := range vars {
			fmt.Printf("  %-38s %s (%s)\n", v.name, v.desc, v.varType)
		}
		fmt.Println()
	}

	fmt.Println("Example usage:")
	fmt.Println("  CKB_BUDGET_MAX_MODULES=50 ckb arch")
	fmt.Println("  CKB_LOG_LEVEL=debug ckb serve")
	fmt.Println("  CKB_CONFIG_PATH=/etc/ckb/config.json ckb mcp")
}

type envVarInfo struct {
	name    string
	desc    string
	varType string
}

func valueOrDefault(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func isEqual(a, b interface{}) bool {
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func computeDiff(current, defaults map[string]interface{}) map[string]interface{} {
	diff := make(map[string]interface{})
	computeDiffRecursive(current, defaults, diff, "")
	return diff
}

func computeDiffRecursive(current, defaults map[string]interface{}, diff map[string]interface{}, prefix string) {
	for key, currentVal := range current {
		defaultVal, exists := defaults[key]

		// If key doesn't exist in defaults or values differ
		if !exists {
			diff[key] = currentVal
			continue
		}

		// Check if both are maps (nested objects)
		currentMap, currentIsMap := currentVal.(map[string]interface{})
		defaultMap, defaultIsMap := defaultVal.(map[string]interface{})

		if currentIsMap && defaultIsMap {
			nestedDiff := make(map[string]interface{})
			computeDiffRecursive(currentMap, defaultMap, nestedDiff, prefix+key+".")
			if len(nestedDiff) > 0 {
				diff[key] = nestedDiff
			}
		} else if fmt.Sprintf("%v", currentVal) != fmt.Sprintf("%v", defaultVal) {
			diff[key] = currentVal
		}
	}
}

// GetEnvVarMappings returns the list of supported env vars for documentation
func GetEnvVarMappings() []string {
	vars := config.GetSupportedEnvVars()
	sort.Strings(vars)
	return vars
}
