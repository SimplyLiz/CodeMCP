package main

import (
	"os"

	"ckb/internal/config"
	"ckb/internal/tier"
	"ckb/internal/version"

	"github.com/spf13/cobra"
)

var (
	// tierFlag is the CLI --tier flag value
	tierFlag string

	// verbosity is the count of -v flags (0=warn, 1=info, 2+=debug)
	verbosity int

	// quiet suppresses all log output
	quiet bool
)

var rootCmd = &cobra.Command{
	Use:   "ckb",
	Short: "CKB - Code Knowledge Backend",
	Long: `CKB (Code Knowledge Backend) is a language-agnostic codebase comprehension layer
that orchestrates existing code intelligence backends (SCIP, Glean, LSP, Git) and provides
semantically compressed, LLM-optimized views.`,
	Version: version.Version,
}

func init() {
	rootCmd.SetVersionTemplate("CKB version {{.Version}}\n")
	rootCmd.PersistentFlags().StringVar(&tierFlag, "tier", "",
		"Analysis tier: fast, standard, full, or auto (default: auto)")
	rootCmd.PersistentFlags().CountVarP(&verbosity, "verbose", "v",
		"Increase verbosity (-v=info, -vv=debug)")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false,
		"Suppress all log output")
}

// resolveTierMode determines the effective tier mode from CLI flag, env var, and config.
// Precedence: CLI flag > CKB_TIER env var > config.json tier > auto
func resolveTierMode(cfg *config.Config) (tier.TierMode, error) {
	// 1. CLI flag (highest priority)
	if tierFlag != "" {
		return tier.ParseTierMode(tierFlag)
	}

	// 2. Environment variable
	if env := os.Getenv("CKB_TIER"); env != "" {
		return tier.ParseTierMode(env)
	}

	// 3. Config file default
	if cfg != nil && cfg.Tier != "" {
		return tier.ParseTierMode(cfg.Tier)
	}

	// 4. Auto-detect (default)
	return tier.TierModeAuto, nil
}
