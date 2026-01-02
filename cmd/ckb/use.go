package main

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"ckb/internal/repos"

	"github.com/spf13/cobra"
)

var (
	useListFlag bool
)

var useCmd = &cobra.Command{
	Use:   "use [name]",
	Short: "Switch active repository",
	Long: `Switch the active repository for CKB commands.

Without arguments, shows the currently active repository.
With a name, switches to that repository.

The active repository is used by default for all CKB commands when
you're not in a registered repository's directory.

Examples:
  ckb use                  # Show current active repo
  ckb use myproject        # Switch to myproject
  ckb use --list           # List available repos`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUse,
}

func init() {
	useCmd.Flags().BoolVarP(&useListFlag, "list", "l", false, "List available repositories")
	rootCmd.AddCommand(useCmd)
}

func runUse(cmd *cobra.Command, args []string) error {
	registry, err := repos.LoadRegistry()
	if err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	// List mode
	if useListFlag {
		return listReposForUse(registry)
	}

	// No args: show current
	if len(args) == 0 {
		if registry.Default == "" {
			fmt.Println("No active repository.")
			fmt.Println()
			fmt.Println("Use 'ckb use <name>' to activate a repository.")
			fmt.Println("Use 'ckb use --list' to see available repositories.")
			return nil
		}

		entry, state, err := registry.Get(registry.Default)
		if err != nil {
			return err
		}

		fmt.Printf("Active: %s\n", entry.Name)
		fmt.Printf("Path:   %s\n", entry.Path)
		fmt.Printf("State:  %s\n", state)
		return nil
	}

	// Switch to specified repo
	name := args[0]

	// Validate repo exists
	entry, state, err := registry.Get(name)
	if err != nil {
		return err
	}

	// Warn if not in valid state but still allow switching
	switch state {
	case repos.RepoStateMissing:
		fmt.Fprintf(os.Stderr, "Warning: repository path no longer exists: %s\n", entry.Path)
		fmt.Fprintf(os.Stderr, "Consider running: ckb repo remove %s\n\n", name)
	case repos.RepoStateUninitialized:
		fmt.Fprintf(os.Stderr, "Warning: repository not initialized\n")
		fmt.Fprintf(os.Stderr, "Run: cd %s && ckb init\n\n", entry.Path)
	}

	// Set as default
	if err := registry.SetDefault(name); err != nil {
		return err
	}

	// Update last used timestamp
	if err := registry.TouchLastUsed(name); err != nil {
		// Non-fatal, just log
		fmt.Fprintf(os.Stderr, "Warning: failed to update last used time: %v\n", err)
	}

	fmt.Printf("Switched to: %s (%s)\n", name, entry.Path)
	return nil
}

func listReposForUse(registry *repos.Registry) error {
	entries := registry.List()
	if len(entries) == 0 {
		fmt.Println("No repositories registered.")
		fmt.Println()
		fmt.Println("Register a repository:")
		fmt.Println("  cd /path/to/project && ckb init")
		fmt.Println("  ckb repo add myproject /path/to/project")
		return nil
	}

	// Sort by last used (most recent first), then by name
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].LastUsedAt.IsZero() && entries[j].LastUsedAt.IsZero() {
			return entries[i].Name < entries[j].Name
		}
		if entries[i].LastUsedAt.IsZero() {
			return false
		}
		if entries[j].LastUsedAt.IsZero() {
			return true
		}
		return entries[i].LastUsedAt.After(entries[j].LastUsedAt)
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPATH\tLAST USED")

	for _, e := range entries {
		marker := " "
		if e.Name == registry.Default {
			marker = "*"
		}

		lastUsed := "never"
		if !e.LastUsedAt.IsZero() {
			lastUsed = formatRelativeTime(e.LastUsedAt)
		}

		fmt.Fprintf(w, "%s %s\t%s\t%s\n", marker, e.Name, e.Path, lastUsed)
	}

	w.Flush()

	if registry.Default != "" {
		fmt.Println()
		fmt.Printf("* = active repository\n")
	}

	return nil
}

// formatRelativeTime formats a time as a relative duration (e.g., "2h ago", "3d ago")
func formatRelativeTime(t time.Time) string {
	d := time.Since(t)

	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	}
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1d ago"
	}
	if days < 30 {
		return fmt.Sprintf("%dd ago", days)
	}
	months := days / 30
	if months == 1 {
		return "1mo ago"
	}
	return fmt.Sprintf("%dmo ago", months)
}
