package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"ckb/internal/federation"
	"ckb/internal/logging"
)

var federationCmd = &cobra.Command{
	Use:   "federation",
	Short: "Manage repository federations",
	Long: `Manage federations of repositories for cross-repo queries.

A federation is a named collection of repositories that can be queried together.
Each repository in a federation has a unique ID (alias) and a stable UUID.`,
}

// Flags
var (
	fedDescription string
	fedRepoID      string
	fedRepoPath    string
	fedRepoTags    string
	fedForce       bool
	fedDryRun      bool
	fedJSONOutput  bool
)

var fedCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new federation",
	Args:  cobra.ExactArgs(1),
	RunE:  runFedCreate,
}

var fedDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a federation",
	Args:  cobra.ExactArgs(1),
	RunE:  runFedDelete,
}

var fedListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all federations",
	RunE:  runFedList,
}

var fedStatusCmd = &cobra.Command{
	Use:   "status <name>",
	Short: "Show federation status",
	Args:  cobra.ExactArgs(1),
	RunE:  runFedStatus,
}

var fedAddCmd = &cobra.Command{
	Use:   "add <federation>",
	Short: "Add a repository to a federation",
	Long: `Add a repository to a federation.

The repository is identified by a user-chosen ID (alias) and its filesystem path.
A UUID is automatically generated and will remain stable across renames.`,
	Args: cobra.ExactArgs(1),
	RunE: runFedAdd,
}

var fedRemoveCmd = &cobra.Command{
	Use:   "remove <federation> <repo-id>",
	Short: "Remove a repository from a federation",
	Args:  cobra.ExactArgs(2),
	RunE:  runFedRemove,
}

var fedRenameCmd = &cobra.Command{
	Use:   "rename <federation> <old-id> <new-id>",
	Short: "Rename a repository in a federation",
	Long:  `Rename a repository's ID (alias). The UUID remains unchanged.`,
	Args:  cobra.ExactArgs(3),
	RunE:  runFedRename,
}

var fedReposCmd = &cobra.Command{
	Use:   "repos <name>",
	Short: "List repositories in a federation",
	Args:  cobra.ExactArgs(1),
	RunE:  runFedRepos,
}

var fedSyncCmd = &cobra.Command{
	Use:   "sync <name>",
	Short: "Sync federation index from repository data",
	Long: `Sync the federation index with data from all repositories.

This reads modules, ownership, hotspots, and decisions from each repository's
CKB database and stores summaries in the federation index for cross-repo queries.`,
	Args: cobra.ExactArgs(1),
	RunE: runFedSync,
}

func init() {
	rootCmd.AddCommand(federationCmd)

	// Create command
	fedCreateCmd.Flags().StringVar(&fedDescription, "description", "", "Federation description")
	federationCmd.AddCommand(fedCreateCmd)

	// Delete command
	fedDeleteCmd.Flags().BoolVar(&fedForce, "force", false, "Delete without confirmation")
	federationCmd.AddCommand(fedDeleteCmd)

	// List command
	fedListCmd.Flags().BoolVar(&fedJSONOutput, "json", false, "Output as JSON")
	federationCmd.AddCommand(fedListCmd)

	// Status command
	fedStatusCmd.Flags().BoolVar(&fedJSONOutput, "json", false, "Output as JSON")
	federationCmd.AddCommand(fedStatusCmd)

	// Add command
	fedAddCmd.Flags().StringVar(&fedRepoID, "repo-id", "", "Repository ID (required)")
	fedAddCmd.Flags().StringVar(&fedRepoPath, "path", "", "Repository path (required)")
	fedAddCmd.Flags().StringVar(&fedRepoTags, "tags", "", "Comma-separated tags")
	fedAddCmd.MarkFlagRequired("repo-id")
	fedAddCmd.MarkFlagRequired("path")
	federationCmd.AddCommand(fedAddCmd)

	// Remove command
	federationCmd.AddCommand(fedRemoveCmd)

	// Rename command
	federationCmd.AddCommand(fedRenameCmd)

	// Repos command
	fedReposCmd.Flags().BoolVar(&fedJSONOutput, "json", false, "Output as JSON")
	federationCmd.AddCommand(fedReposCmd)

	// Sync command
	fedSyncCmd.Flags().BoolVar(&fedForce, "force", false, "Force sync even if data is fresh")
	fedSyncCmd.Flags().BoolVar(&fedDryRun, "dry-run", false, "Show what would be synced")
	fedSyncCmd.Flags().BoolVar(&fedJSONOutput, "json", false, "Output as JSON")
	federationCmd.AddCommand(fedSyncCmd)
}

func runFedCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Create(name, fedDescription, logger)
	if err != nil {
		return fmt.Errorf("failed to create federation: %w", err)
	}
	defer fed.Close()

	fmt.Printf("Created federation: %s\n", name)
	return nil
}

func runFedDelete(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Check if exists
	exists, err := federation.Exists(name)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("federation %q does not exist", name)
	}

	// Confirm unless --force
	if !fedForce {
		fmt.Printf("Are you sure you want to delete federation %q? [y/N] ", name)
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) != "y" {
			fmt.Println("Cancelled")
			return nil
		}
	}

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Open(name, logger)
	if err != nil {
		return err
	}

	if err := fed.Delete(); err != nil {
		return fmt.Errorf("failed to delete federation: %w", err)
	}

	fmt.Printf("Deleted federation: %s\n", name)
	return nil
}

func runFedList(cmd *cobra.Command, args []string) error {
	names, err := federation.List()
	if err != nil {
		return fmt.Errorf("failed to list federations: %w", err)
	}

	if fedJSONOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(names)
	}

	if len(names) == 0 {
		fmt.Println("No federations found")
		return nil
	}

	fmt.Println("Federations:")
	for _, name := range names {
		fmt.Printf("  %s\n", name)
	}
	return nil
}

func runFedStatus(cmd *cobra.Command, args []string) error {
	name := args[0]

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Open(name, logger)
	if err != nil {
		return err
	}
	defer fed.Close()

	config := fed.Config()
	repos := fed.ListRepos()

	// Get indexed repos from index
	indexedRepos, err := fed.Index().ListRepos()
	if err != nil {
		indexedRepos = nil
	}

	if fedJSONOutput {
		status := map[string]interface{}{
			"name":        config.Name,
			"description": config.Description,
			"createdAt":   config.CreatedAt,
			"updatedAt":   config.UpdatedAt,
			"repoCount":   len(repos),
			"repos":       repos,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	fmt.Printf("Federation: %s\n", config.Name)
	if config.Description != "" {
		fmt.Printf("Description: %s\n", config.Description)
	}
	fmt.Printf("Created: %s\n", config.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Repos: %d\n", len(repos))

	// Check compatibility
	checks, err := federation.CheckAllReposCompatibility(fed)
	if err == nil {
		compatible := 0
		for _, c := range checks {
			if c.Status == federation.CompatibilityOK {
				compatible++
			}
		}
		fmt.Printf("Compatible: %d/%d\n", compatible, len(checks))
	}

	// Show staleness
	if len(indexedRepos) > 0 {
		fmt.Println("\nPer-repo status:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  REPO ID\tPATH\tSTATUS\tLAST SYNC")
		for _, r := range indexedRepos {
			syncTime := "never"
			if r.LastSyncedAt != nil {
				syncTime = r.LastSyncedAt.Format("2006-01-02 15:04")
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", r.RepoID, r.Path, r.Status, syncTime)
		}
		w.Flush()
	}

	return nil
}

func runFedAdd(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Resolve path to absolute
	absPath, err := filepath.Abs(fedRepoPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Check path exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", absPath)
	}

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Open(name, logger)
	if err != nil {
		return err
	}
	defer fed.Close()

	// Parse tags
	var tags []string
	if fedRepoTags != "" {
		tags = strings.Split(fedRepoTags, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
	}

	repo, err := fed.AddRepo(fedRepoID, absPath, tags)
	if err != nil {
		return fmt.Errorf("failed to add repo: %w", err)
	}

	fmt.Printf("Added repository %s to federation %s\n", fedRepoID, name)
	fmt.Printf("  UUID: %s\n", repo.RepoUID)
	fmt.Printf("  Path: %s\n", repo.Path)
	if len(tags) > 0 {
		fmt.Printf("  Tags: %s\n", strings.Join(tags, ", "))
	}

	// Check compatibility
	check, err := federation.CheckSchemaCompatibility(fedRepoID, absPath)
	if err == nil && check.Status != federation.CompatibilityOK {
		fmt.Printf("\nWarning: %s\n", check.Message)
		if check.Action != "" {
			fmt.Printf("Action: %s\n", check.Action)
		}
	}

	return nil
}

func runFedRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	repoID := args[1]

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Open(name, logger)
	if err != nil {
		return err
	}
	defer fed.Close()

	if err := fed.RemoveRepo(repoID); err != nil {
		return fmt.Errorf("failed to remove repo: %w", err)
	}

	fmt.Printf("Removed repository %s from federation %s\n", repoID, name)
	return nil
}

func runFedRename(cmd *cobra.Command, args []string) error {
	name := args[0]
	oldID := args[1]
	newID := args[2]

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Open(name, logger)
	if err != nil {
		return err
	}
	defer fed.Close()

	if err := fed.RenameRepo(oldID, newID); err != nil {
		return fmt.Errorf("failed to rename repo: %w", err)
	}

	fmt.Printf("Renamed repository %s to %s in federation %s\n", oldID, newID, name)
	return nil
}

func runFedRepos(cmd *cobra.Command, args []string) error {
	name := args[0]

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Open(name, logger)
	if err != nil {
		return err
	}
	defer fed.Close()

	repos := fed.ListRepos()

	if fedJSONOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(repos)
	}

	if len(repos) == 0 {
		fmt.Println("No repositories in federation")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "REPO ID\tPATH\tTAGS\tADDED")
	for _, r := range repos {
		tags := strings.Join(r.Tags, ", ")
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.RepoID, r.Path, tags, r.AddedAt.Format("2006-01-02"))
	}
	w.Flush()

	return nil
}

func runFedSync(cmd *cobra.Command, args []string) error {
	name := args[0]

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Open(name, logger)
	if err != nil {
		return err
	}
	defer fed.Close()

	opts := federation.SyncOptions{
		Force:  fedForce,
		DryRun: fedDryRun,
	}

	results, err := fed.Sync(opts)
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	if fedJSONOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	// Print results
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "REPO ID\tSTATUS\tMODULES\tOWNERSHIP\tHOTSPOTS\tDECISIONS\tDURATION")
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\t%dms\n",
			r.RepoID, r.Status,
			r.ModulesSynced, r.OwnershipSynced, r.HotspotsSynced, r.DecisionsSynced,
			r.Duration.Milliseconds())
	}
	w.Flush()

	// Summary
	success := 0
	failed := 0
	for _, r := range results {
		if r.Status == "success" {
			success++
		} else if r.Status == "failed" {
			failed++
		}
	}

	fmt.Printf("\nSynced %d/%d repositories\n", success, len(results))
	if failed > 0 {
		fmt.Printf("Failed: %d (use --json for details)\n", failed)
	}

	return nil
}
