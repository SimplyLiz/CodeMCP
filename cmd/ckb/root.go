package main

import (
	"github.com/spf13/cobra"
)

const (
	// CKBVersion is the current version of CKB
	CKBVersion = "0.1.0"
)

var rootCmd = &cobra.Command{
	Use:   "ckb",
	Short: "CKB - Code Knowledge Backend",
	Long: `CKB (Code Knowledge Backend) is a language-agnostic codebase comprehension layer
that orchestrates existing code intelligence backends (SCIP, Glean, LSP, Git) and provides
semantically compressed, LLM-optimized views.`,
	Version: CKBVersion,
}

func init() {
	rootCmd.SetVersionTemplate("CKB version {{.Version}}\n")
}
