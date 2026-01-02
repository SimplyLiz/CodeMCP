package main

import (
	"github.com/spf13/cobra"
)

var reposCmd = &cobra.Command{
	Use:   "repos",
	Short: "List registered repositories (alias for 'repo list')",
	Long: `List all registered CKB repositories.

This is a convenience alias for 'ckb repo list'.

Examples:
  ckb repos           # List all repositories
  ckb repos --json    # Output as JSON`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepoList(cmd, args)
	},
}

func init() {
	reposCmd.Flags().BoolVar(&repoListJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(reposCmd)
}
