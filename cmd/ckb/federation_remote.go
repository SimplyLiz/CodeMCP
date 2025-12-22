package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/federation"
	"ckb/internal/logging"
)

// Remote server flags
var (
	fedRemoteURL      string
	fedRemoteToken    string
	fedRemoteCacheTTL string
	fedRemoteTimeout  string
)

var fedAddRemoteCmd = &cobra.Command{
	Use:   "add-remote <federation> <name>",
	Short: "Add a remote CKB index server to a federation",
	Long: `Add a remote CKB index server to a federation.

The remote server will be queried alongside local repositories for hybrid queries.
Token can use environment variable expansion with ${VAR_NAME} syntax.

Examples:
  ckb federation add-remote my-org prod --url=https://ckb.example.com --token=$CKB_TOKEN
  ckb federation add-remote my-org staging --url=https://staging-ckb.example.com
  ckb federation add-remote my-org internal --url=http://localhost:8080 --cache-ttl=15m`,
	Args: cobra.ExactArgs(2),
	RunE: runFedAddRemote,
}

var fedRemoveRemoteCmd = &cobra.Command{
	Use:   "remove-remote <federation> <name>",
	Short: "Remove a remote server from a federation",
	Args:  cobra.ExactArgs(2),
	RunE:  runFedRemoveRemote,
}

var fedListRemoteCmd = &cobra.Command{
	Use:   "list-remote <federation>",
	Short: "List remote servers in a federation",
	Args:  cobra.ExactArgs(1),
	RunE:  runFedListRemote,
}

var fedSyncRemoteCmd = &cobra.Command{
	Use:   "sync-remote <federation> [name]",
	Short: "Sync metadata from remote server(s)",
	Long: `Sync repository metadata from remote index servers.

If a server name is provided, only that server is synced.
Otherwise, all enabled remote servers are synced.

Examples:
  ckb federation sync-remote my-org prod
  ckb federation sync-remote my-org`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runFedSyncRemote,
}

var fedStatusRemoteCmd = &cobra.Command{
	Use:   "status-remote <federation> <name>",
	Short: "Check remote server connectivity and status",
	Args:  cobra.ExactArgs(2),
	RunE:  runFedStatusRemote,
}

var fedEnableRemoteCmd = &cobra.Command{
	Use:   "enable-remote <federation> <name>",
	Short: "Enable a remote server",
	Args:  cobra.ExactArgs(2),
	RunE:  runFedEnableRemote,
}

var fedDisableRemoteCmd = &cobra.Command{
	Use:   "disable-remote <federation> <name>",
	Short: "Disable a remote server",
	Args:  cobra.ExactArgs(2),
	RunE:  runFedDisableRemote,
}

func init() {
	// Add remote command
	fedAddRemoteCmd.Flags().StringVar(&fedRemoteURL, "url", "", "Remote server URL (required)")
	fedAddRemoteCmd.Flags().StringVar(&fedRemoteToken, "token", "", "Auth token (supports ${ENV_VAR})")
	fedAddRemoteCmd.Flags().StringVar(&fedRemoteCacheTTL, "cache-ttl", "1h", "Cache TTL (e.g., 15m, 1h)")
	fedAddRemoteCmd.Flags().StringVar(&fedRemoteTimeout, "timeout", "30s", "Request timeout")
	fedAddRemoteCmd.MarkFlagRequired("url")
	federationCmd.AddCommand(fedAddRemoteCmd)

	// Remove remote command
	federationCmd.AddCommand(fedRemoveRemoteCmd)

	// List remote command
	fedListRemoteCmd.Flags().BoolVar(&fedJSONOutput, "json", false, "Output as JSON")
	federationCmd.AddCommand(fedListRemoteCmd)

	// Sync remote command
	fedSyncRemoteCmd.Flags().BoolVar(&fedJSONOutput, "json", false, "Output as JSON")
	federationCmd.AddCommand(fedSyncRemoteCmd)

	// Status remote command
	fedStatusRemoteCmd.Flags().BoolVar(&fedJSONOutput, "json", false, "Output as JSON")
	federationCmd.AddCommand(fedStatusRemoteCmd)

	// Enable/disable remote commands
	federationCmd.AddCommand(fedEnableRemoteCmd)
	federationCmd.AddCommand(fedDisableRemoteCmd)
}

func runFedAddRemote(cmd *cobra.Command, args []string) error {
	fedName := args[0]
	serverName := args[1]

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	// Parse durations
	cacheTTL, err := time.ParseDuration(fedRemoteCacheTTL)
	if err != nil {
		return fmt.Errorf("invalid cache-ttl: %w", err)
	}

	timeout, err := time.ParseDuration(fedRemoteTimeout)
	if err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}

	server := federation.RemoteServer{
		Name:     serverName,
		URL:      fedRemoteURL,
		Token:    fedRemoteToken,
		CacheTTL: federation.Duration{Duration: cacheTTL},
		Timeout:  federation.Duration{Duration: timeout},
		Enabled:  true,
	}

	if err := fed.AddRemoteServer(server); err != nil {
		return fmt.Errorf("failed to add remote server: %w", err)
	}

	fmt.Printf("Added remote server %q to federation %q\n", serverName, fedName)
	fmt.Printf("  URL: %s\n", fedRemoteURL)
	fmt.Printf("  Cache TTL: %s\n", cacheTTL)
	fmt.Printf("  Timeout: %s\n", timeout)

	if fedRemoteToken != "" {
		if strings.Contains(fedRemoteToken, "${") {
			fmt.Println("  Token: (using environment variable)")
		} else {
			fmt.Println("  Token: (configured)")
		}
	}

	fmt.Println("\nRun 'ckb federation sync-remote' to sync repository metadata.")

	return nil
}

func runFedRemoveRemote(cmd *cobra.Command, args []string) error {
	fedName := args[0]
	serverName := args[1]

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	if err := fed.RemoveRemoteServer(serverName); err != nil {
		return fmt.Errorf("failed to remove remote server: %w", err)
	}

	fmt.Printf("Removed remote server %q from federation %q\n", serverName, fedName)
	return nil
}

func runFedListRemote(cmd *cobra.Command, args []string) error {
	fedName := args[0]

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.WarnLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	servers := fed.ListRemoteServers()

	if fedJSONOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(servers)
	}

	if len(servers) == 0 {
		fmt.Println("No remote servers configured")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tURL\tENABLED\tLAST SYNC\tERROR")
	for _, s := range servers {
		enabled := "yes"
		if !s.Enabled {
			enabled = "no"
		}
		lastSync := "never"
		if s.LastSyncedAt != nil {
			lastSync = s.LastSyncedAt.Format("2006-01-02 15:04")
		}
		lastError := "-"
		if s.LastError != "" {
			if len(s.LastError) > 30 {
				lastError = s.LastError[:27] + "..."
			} else {
				lastError = s.LastError
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Name, s.URL, enabled, lastSync, lastError)
	}
	w.Flush()

	return nil
}

func runFedSyncRemote(cmd *cobra.Command, args []string) error {
	fedName := args[0]
	var serverName string
	if len(args) > 1 {
		serverName = args[1]
	}

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	// Create hybrid engine
	engine := federation.NewHybridEngine(fed, logger)
	if err := engine.InitRemoteClients(); err != nil {
		return fmt.Errorf("failed to initialize remote clients: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if serverName != "" {
		// Sync specific server
		if err := engine.SyncRemote(ctx, serverName); err != nil {
			return fmt.Errorf("failed to sync server %q: %w", serverName, err)
		}

		// Get cached repos for this server
		repos, _ := fed.Index().GetRemoteRepos(serverName)
		fmt.Printf("Synced %d repositories from %q\n", len(repos), serverName)
	} else {
		// Sync all servers
		errors := engine.SyncAllRemotes(ctx)

		servers := fed.GetEnabledRemoteServers()
		success := len(servers) - len(errors)

		if fedJSONOutput {
			result := map[string]interface{}{
				"total":   len(servers),
				"success": success,
				"errors":  errors,
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}

		fmt.Printf("Synced %d/%d remote servers\n", success, len(servers))
		if len(errors) > 0 {
			fmt.Println("\nErrors:")
			for _, e := range errors {
				fmt.Printf("  %s: %s\n", e.Source, e.Message)
			}
		}
	}

	return nil
}

func runFedStatusRemote(cmd *cobra.Command, args []string) error {
	fedName := args[0]
	serverName := args[1]

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.WarnLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	// Create hybrid engine
	engine := federation.NewHybridEngine(fed, logger)
	if err := engine.InitRemoteClients(); err != nil {
		return fmt.Errorf("failed to initialize remote clients: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, err := engine.GetRemoteStatus(ctx, serverName)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	if fedJSONOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	fmt.Printf("Server: %s\n", status.Name)
	fmt.Printf("URL: %s\n", status.URL)
	fmt.Printf("Enabled: %v\n", status.Enabled)
	fmt.Printf("Online: %v\n", status.Online)

	if status.Online {
		fmt.Printf("Latency: %s\n", status.Latency)
	} else if status.PingError != "" {
		fmt.Printf("Ping Error: %s\n", status.PingError)
	}

	if status.LastSyncedAt != nil {
		fmt.Printf("Last Synced: %s\n", status.LastSyncedAt.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Println("Last Synced: never")
	}

	if status.LastError != "" {
		fmt.Printf("Last Error: %s\n", status.LastError)
	}

	fmt.Printf("Cached Repos: %d\n", status.CachedRepoCount)

	return nil
}

func runFedEnableRemote(cmd *cobra.Command, args []string) error {
	fedName := args[0]
	serverName := args[1]

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	enabled := true
	updates := federation.RemoteServerUpdate{
		Enabled: &enabled,
	}

	if err := fed.UpdateRemoteServer(serverName, updates); err != nil {
		return fmt.Errorf("failed to enable server: %w", err)
	}

	fmt.Printf("Enabled remote server %q in federation %q\n", serverName, fedName)
	return nil
}

func runFedDisableRemote(cmd *cobra.Command, args []string) error {
	fedName := args[0]
	serverName := args[1]

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	enabled := false
	updates := federation.RemoteServerUpdate{
		Enabled: &enabled,
	}

	if err := fed.UpdateRemoteServer(serverName, updates); err != nil {
		return fmt.Errorf("failed to disable server: %w", err)
	}

	fmt.Printf("Disabled remote server %q in federation %q\n", serverName, fedName)
	return nil
}
