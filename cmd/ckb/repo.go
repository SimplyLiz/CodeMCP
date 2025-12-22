package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"ckb/internal/repos"

	"github.com/spf13/cobra"
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage repository registry",
	Long: `Manage the global repository registry for multi-repo support.

The registry stores named shortcuts to repositories, enabling quick
context switching in MCP sessions and CLI commands.

Registry location: ~/.ckb/repos.json`,
}

var repoAddCmd = &cobra.Command{
	Use:   "add [name] [path]",
	Short: "Register a repository",
	Long: `Register a repository in the global registry.

If path is omitted, uses the current working directory.
If name is omitted, uses the directory name.`,
	Args: cobra.MaximumNArgs(2),
	RunE: runRepoAdd,
}

var repoListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered repositories",
	RunE:  runRepoList,
}

var repoRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Unregister a repository",
	Args:  cobra.ExactArgs(1),
	RunE:  runRepoRemove,
}

var repoRenameCmd = &cobra.Command{
	Use:   "rename <old> <new>",
	Short: "Rename a repository alias",
	Args:  cobra.ExactArgs(2),
	RunE:  runRepoRename,
}

var repoDefaultCmd = &cobra.Command{
	Use:   "default [name]",
	Short: "Get or set the default repository",
	Long: `Get or set the default repository.

Without arguments, prints the current default.
With a name, sets it as the new default.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRepoDefault,
}

var repoInfoCmd = &cobra.Command{
	Use:   "info [name]",
	Short: "Show detailed repository info",
	Long: `Show detailed information about a repository.

If name is omitted, shows info for the default repository.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRepoInfo,
}

var repoWhichCmd = &cobra.Command{
	Use:   "which",
	Short: "Print current repository (for scripts)",
	Long: `Print the current repository name.

Exits with code 1 if no repository is active.
Useful for scripts and shell prompts.`,
	RunE: runRepoWhich,
}

var repoCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate all registered repositories",
	RunE:  runRepoCheck,
}

var repoUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Alias for 'default'",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepoDefault(cmd, args)
	},
}

var (
	repoListJSON bool
	repoInfoJSON bool
)

func init() {
	rootCmd.AddCommand(repoCmd)

	repoCmd.AddCommand(repoAddCmd)
	repoCmd.AddCommand(repoListCmd)
	repoCmd.AddCommand(repoRemoveCmd)
	repoCmd.AddCommand(repoRenameCmd)
	repoCmd.AddCommand(repoDefaultCmd)
	repoCmd.AddCommand(repoInfoCmd)
	repoCmd.AddCommand(repoWhichCmd)
	repoCmd.AddCommand(repoCheckCmd)
	repoCmd.AddCommand(repoUseCmd)

	repoListCmd.Flags().BoolVar(&repoListJSON, "json", false, "Output as JSON")
	repoInfoCmd.Flags().BoolVar(&repoInfoJSON, "json", false, "Output as JSON")
}

func runRepoAdd(cmd *cobra.Command, args []string) error {
	var name, path string

	switch len(args) {
	case 0:
		// Use cwd for both name and path
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		path = cwd
		name = filepath.Base(cwd)
	case 1:
		// Name provided, use cwd for path
		name = args[0]
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		path = cwd
	case 2:
		name = args[0]
		path = args[1]
	}

	registry, err := repos.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	if err := registry.Add(name, path); err != nil {
		return err
	}

	// Check state and show appropriate message
	state := registry.ValidateState(name)
	entry, _, _ := registry.Get(name)

	fmt.Printf("Added %s\n", name)
	fmt.Printf("  Path: %s\n", entry.Path)

	if state == repos.RepoStateUninitialized {
		fmt.Printf("  Status: uninitialized\n")
		fmt.Printf("  Next: cd %s && ckb init\n", entry.Path)
	} else {
		fmt.Printf("  Status: %s\n", state)
	}

	return nil
}

func runRepoList(cmd *cobra.Command, args []string) error {
	registry, err := repos.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	entries := registry.List()
	if len(entries) == 0 {
		fmt.Println("No repositories registered.")
		fmt.Println("Use 'ckb repo add <name>' to register the current directory.")
		return nil
	}

	// Sort by name
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	if repoListJSON {
		type repoInfo struct {
			Name      string `json:"name"`
			Path      string `json:"path"`
			State     string `json:"state"`
			IsDefault bool   `json:"is_default"`
		}
		var infos []repoInfo
		for _, e := range entries {
			infos = append(infos, repoInfo{
				Name:      e.Name,
				Path:      e.Path,
				State:     string(registry.ValidateState(e.Name)),
				IsDefault: e.Name == registry.Default,
			})
		}
		data, _ := json.MarshalIndent(infos, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Group by state
	var valid, uninitialized, missing []repos.RepoEntry
	for _, e := range entries {
		switch registry.ValidateState(e.Name) {
		case repos.RepoStateValid:
			valid = append(valid, e)
		case repos.RepoStateUninitialized:
			uninitialized = append(uninitialized, e)
		case repos.RepoStateMissing:
			missing = append(missing, e)
		}
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if len(valid) > 0 {
		fmt.Fprintln(w, "Valid:")
		for _, e := range valid {
			def := ""
			if e.Name == registry.Default {
				def = " (default)"
			}
			fmt.Fprintf(w, "  %s\t%s%s\n", e.Name, e.Path, def)
		}
	}

	if len(uninitialized) > 0 {
		if len(valid) > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, "Needs initialization:")
		for _, e := range uninitialized {
			fmt.Fprintf(w, "  %s\t%s\n", e.Name, e.Path)
			fmt.Fprintf(w, "  \t→ cd %s && ckb init\n", e.Path)
		}
	}

	if len(missing) > 0 {
		if len(valid) > 0 || len(uninitialized) > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, "Missing:")
		for _, e := range missing {
			fmt.Fprintf(w, "  %s\t%s\n", e.Name, e.Path)
			fmt.Fprintf(w, "  \t→ ckb repo remove %s\n", e.Name)
		}
	}

	w.Flush()
	return nil
}

func runRepoRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	registry, err := repos.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	if err := registry.Remove(name); err != nil {
		return err
	}

	fmt.Printf("Removed %s\n", name)
	return nil
}

func runRepoRename(cmd *cobra.Command, args []string) error {
	oldName, newName := args[0], args[1]

	registry, err := repos.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	if err := registry.Rename(oldName, newName); err != nil {
		return err
	}

	fmt.Printf("Renamed %s → %s\n", oldName, newName)
	return nil
}

func runRepoDefault(cmd *cobra.Command, args []string) error {
	registry, err := repos.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	if len(args) == 0 {
		// Get default
		if registry.Default == "" {
			fmt.Println("No default repository set.")
			fmt.Println("Use 'ckb repo default <name>' to set one.")
			return nil
		}
		fmt.Println(registry.Default)
		return nil
	}

	// Set default
	name := args[0]
	if err := registry.SetDefault(name); err != nil {
		return err
	}

	fmt.Printf("Default repository set to: %s\n", name)
	return nil
}

func runRepoInfo(cmd *cobra.Command, args []string) error {
	registry, err := repos.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	var name string
	if len(args) == 0 {
		name = registry.Default
		if name == "" {
			return fmt.Errorf("no repository specified and no default set")
		}
	} else {
		name = args[0]
	}

	entry, state, err := registry.Get(name)
	if err != nil {
		return err
	}

	if repoInfoJSON {
		info := map[string]interface{}{
			"name":       entry.Name,
			"path":       entry.Path,
			"state":      string(state),
			"is_default": entry.Name == registry.Default,
			"added_at":   entry.AddedAt,
		}
		if !entry.LastUsedAt.IsZero() {
			info["last_used_at"] = entry.LastUsedAt
		}
		data, _ := json.MarshalIndent(info, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Name: %s\n", entry.Name)
	fmt.Printf("Path: %s\n", entry.Path)
	fmt.Printf("State: %s\n", state)
	fmt.Printf("Default: %v\n", entry.Name == registry.Default)
	fmt.Printf("Added: %s\n", entry.AddedAt.Format("2006-01-02 15:04:05"))
	if !entry.LastUsedAt.IsZero() {
		fmt.Printf("Last Used: %s\n", entry.LastUsedAt.Format("2006-01-02 15:04:05"))
	}

	return nil
}

func runRepoWhich(cmd *cobra.Command, args []string) error {
	registry, err := repos.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	if registry.Default == "" {
		os.Exit(1)
	}

	fmt.Println(registry.Default)
	return nil
}

func runRepoCheck(cmd *cobra.Command, args []string) error {
	registry, err := repos.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	entries := registry.List()
	if len(entries) == 0 {
		fmt.Println("No repositories registered.")
		return nil
	}

	var hasIssues bool
	for _, e := range entries {
		state := registry.ValidateState(e.Name)
		if state != repos.RepoStateValid {
			hasIssues = true
			fmt.Printf("%s: %s\n", e.Name, state)
			switch state {
			case repos.RepoStateUninitialized:
				fmt.Printf("  → cd %s && ckb init\n", e.Path)
			case repos.RepoStateMissing:
				fmt.Printf("  → ckb repo remove %s\n", e.Name)
			}
		}
	}

	if !hasIssues {
		fmt.Printf("All %d repositories are valid.\n", len(entries))
	}

	return nil
}
