package main

import (
	"ckb/internal/version"

	"github.com/spf13/cobra"
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
}
