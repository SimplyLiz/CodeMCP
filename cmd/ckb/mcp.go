package main

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ckb/internal/config"
	"ckb/internal/index"
	"ckb/internal/mcp"
	"ckb/internal/project"
	"ckb/internal/repos"
	"ckb/internal/repostate"
	"ckb/internal/slogutil"
	"ckb/internal/version"

	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start MCP server for Claude Code integration",
	Long: `Start the Model Context Protocol (MCP) server.

The MCP server enables Claude Code and other MCP clients to query CKB
for codebase comprehension information. It communicates via stdio using
JSON-RPC 2.0 protocol.

The server exposes the following tools:
  - getStatus: Get CKB system status
  - doctor: Diagnose configuration issues
  - getSymbol: Get symbol metadata and location
  - searchSymbols: Search for symbols by name
  - findReferences: Find all references to a symbol
  - getArchitecture: Get codebase architecture
  - analyzeImpact: Analyze the impact of changing a symbol

Example usage:
  ckb mcp --stdio

This command is typically invoked by MCP clients (like Claude Code) and
not directly by users.`,
	RunE: runMCP,
}

var (
	mcpStdio         bool
	mcpWatch         bool
	mcpWatchInterval time.Duration
	mcpRepo          string
	mcpPreset        string
	mcpListPresets   bool
)

const defaultWatchInterval = 10 * time.Second

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.Flags().BoolVar(&mcpStdio, "stdio", true, "Use stdio for communication (default)")
	mcpCmd.Flags().BoolVar(&mcpWatch, "watch", false, "Watch for changes and auto-reindex")
	mcpCmd.Flags().DurationVar(&mcpWatchInterval, "watch-interval", defaultWatchInterval,
		"Watch mode polling interval (min 5s, max 5m)")
	mcpCmd.Flags().StringVar(&mcpRepo, "repo", "", "Repository path or registry name (auto-detected)")
	mcpCmd.Flags().StringVar(&mcpPreset, "preset", mcp.DefaultPreset,
		"Tool preset: core, review, refactor, federation, docs, ops, full")
	mcpCmd.Flags().BoolVar(&mcpListPresets, "list-presets", false,
		"List available presets with tool counts and token estimates")
}

func runMCP(cmd *cobra.Command, args []string) error {
	// Handle --list-presets flag
	if mcpListPresets {
		return listPresets()
	}

	// Create logger for MCP server
	// Writes to file (.ckb/logs/mcp.log) and stderr for errors
	cliLevel := slogutil.LevelFromVerbosity(verbosity, quiet)
	logger := slogutil.NewLogger(os.Stderr, cliLevel) // Default to stderr
	var factory *slogutil.LoggerFactory

	// Validate preset
	if !mcp.IsValidPreset(mcpPreset) {
		return fmt.Errorf("invalid preset: %s (valid: %v)", mcpPreset, mcp.ValidPresets())
	}

	// Determine mode and repo
	var server *mcp.MCPServer
	var repoRoot string
	var repoName string

	// Smart --repo detection: path vs registry name
	if mcpRepo != "" {
		if isRepoPath(mcpRepo) {
			// It's a path - use legacy single-engine mode
			repoRoot = mcpRepo
			fmt.Fprintf(os.Stderr, "Repository: %s (path)\n", repoRoot)
		} else {
			// It's a registry name - use multi-repo mode
			registry, err := repos.LoadRegistry()
			if err != nil {
				return fmt.Errorf("failed to load registry: %w", err)
			}
			entry, state, err := registry.Get(mcpRepo)
			if err != nil {
				return fmt.Errorf("repository '%s' not found in registry", mcpRepo)
			}
			if state != repos.RepoStateValid {
				return fmt.Errorf("repository '%s' is %s", mcpRepo, state)
			}
			repoRoot = entry.Path
			repoName = mcpRepo
			fmt.Fprintf(os.Stderr, "Repository: %s (%s) [%s]\n", repoName, repoRoot, state)

			// Use multi-repo mode
			server = mcp.NewMCPServerWithRegistry(version.Version, registry, logger)
			engine := mustGetEngine(repoRoot, logger)
			server.SetActiveRepo(repoName, repoRoot, engine)
		}
	} else {
		// No --repo flag - use smart resolution
		resolved, err := repos.ResolveActiveRepo("")
		if err != nil {
			return fmt.Errorf("failed to resolve repository: %w", err)
		}

		if resolved.Entry != nil {
			repoRoot = resolved.Entry.Path
			repoName = resolved.Entry.Name

			// Format status message based on resolution source
			switch resolved.Source {
			case repos.ResolvedFromEnv:
				fmt.Fprintf(os.Stderr, "Repository: %s (%s) [from CKB_REPO]\n", repoName, repoRoot)
			case repos.ResolvedFromCWD:
				fmt.Fprintf(os.Stderr, "Repository: %s (%s) [cwd match]\n", repoName, repoRoot)
			case repos.ResolvedFromCWDGit:
				// Auto-detected unregistered git repo
				if resolved.State == repos.RepoStateUninitialized {
					fmt.Fprintf(os.Stderr, "Repository: %s (%s) [auto-detected, uninitialized]\n", repoName, repoRoot)
					fmt.Fprintf(os.Stderr, "  ⚠️  Run 'ckb init && ckb repo add %s .' to fully set up\n", repoName)
				} else {
					fmt.Fprintf(os.Stderr, "Repository: %s (%s) [auto-detected]\n", repoName, repoRoot)
					fmt.Fprintf(os.Stderr, "  ℹ️  Run 'ckb repo add %s .' to register permanently\n", repoName)
				}
				if resolved.SkippedDefault != "" {
					fmt.Fprintf(os.Stderr, "  Note: Default '%s' skipped (different git repo)\n", resolved.SkippedDefault)
				}
			case repos.ResolvedFromDefault:
				fmt.Fprintf(os.Stderr, "Repository: %s (%s) [default]\n", repoName, repoRoot)
				// Warn if we're in a different git repo
				if resolved.DetectedGitRoot != "" && resolved.DetectedGitRoot != repoRoot {
					fmt.Fprintf(os.Stderr, "  ⚠️  CWD is in '%s' but using default repo\n", filepath.Base(resolved.DetectedGitRoot))
				}
			}

			// Use multi-repo mode if registry is available
			registry, err := repos.LoadRegistry()
			if err == nil && resolved.Source != repos.ResolvedFromCWDGit {
				server = mcp.NewMCPServerWithRegistry(version.Version, registry, logger)
				engine := mustGetEngine(repoRoot, logger)
				server.SetActiveRepo(repoName, repoRoot, engine)
			}
		} else {
			// No repo found - fall back to current directory
			repoRoot = mustGetRepoRoot()
			fmt.Fprintf(os.Stderr, "Repository: %s (current directory)\n", repoRoot)
		}
	}

	// Change to repo directory so relative paths work
	if repoRoot != "" && repoRoot != "." {
		if err := os.Chdir(repoRoot); err != nil {
			logger.Error("Failed to change to repo directory",
				"path", repoRoot,
				"error", err.Error(),
			)
			return err
		}
	}

	// Set up file logging with LoggerFactory
	cfg, _ := config.LoadConfig(repoRoot)
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	factory = slogutil.NewLoggerFactory(repoRoot, cfg, cliLevel)
	defer factory.Close()

	// Create tee logger: file + stderr (errors only to stderr)
	if fileLogger, err := factory.MCPLogger(); err == nil {
		stderrHandler := slogutil.NewCKBHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})
		logger = slogutil.NewTeeLogger(fileLogger.Handler(), stderrHandler)
	}

	// Create server if not already created (legacy single-engine mode)
	if server == nil {
		engine := mustGetEngine(repoRoot, logger)
		server = mcp.NewMCPServer(version.Version, engine, logger)
	}

	// Apply preset configuration
	if err := server.SetPreset(mcpPreset); err != nil {
		return fmt.Errorf("failed to set preset: %w", err)
	}

	// Log startup banner with token efficiency info
	preset, exposedCount, totalCount := server.GetPresetStats()
	activeTokens := server.EstimateActiveTokens()
	percentage := (exposedCount * 100) / totalCount

	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "CKB MCP Server v%s\n", version.Version)
	fmt.Fprintf(os.Stderr, "  Active tools: %d / %d (%d%%)\n", exposedCount, totalCount, percentage)
	fmt.Fprintf(os.Stderr, "  Estimated context: %s\n", mcp.FormatTokens(activeTokens))
	fmt.Fprintf(os.Stderr, "  Preset: %s\n", preset)
	fmt.Fprintln(os.Stderr)

	// Start watch mode if enabled
	if mcpWatch {
		// Validate and clamp watch interval
		watchInterval := mcpWatchInterval
		if watchInterval < 5*time.Second {
			watchInterval = 5 * time.Second
		}
		if watchInterval > 5*time.Minute {
			watchInterval = 5 * time.Minute
		}

		go runWatchLoop(repoRoot, watchInterval, logger)
		logger.Info("Watch mode enabled", "pollInterval", watchInterval.String())
	}

	if err := server.Start(); err != nil {
		logger.Error("MCP server error", "error", err.Error())
		return err
	}

	return nil
}

// isRepoPath checks if a string looks like a filesystem path vs a registry name
func isRepoPath(s string) bool {
	// Contains path separator
	if strings.Contains(s, "/") || strings.Contains(s, "\\") {
		return true
	}
	// Starts with . (relative path)
	if strings.HasPrefix(s, ".") {
		return true
	}
	// Exists as a directory
	info, err := os.Stat(s)
	if err == nil && info.IsDir() {
		return true
	}
	return false
}

// runWatchLoop periodically checks index freshness and reindexes if stale
func runWatchLoop(repoRoot string, interval time.Duration, logger *slog.Logger) {
	ckbDir := filepath.Join(repoRoot, ".ckb")
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		meta, err := index.LoadMeta(ckbDir)
		if err != nil || meta == nil {
			// No metadata yet, skip
			continue
		}

		freshness := meta.CheckFreshness(repoRoot)
		if freshness.Fresh {
			continue
		}

		// Determine trigger type based on freshness reason
		trigger := index.TriggerStale
		triggerInfo := freshness.Reason
		if freshness.CommitsBehind > 0 {
			trigger = index.TriggerHEAD
			// Try to get branch info for triggerInfo
			if meta.CommitHash != "" && freshness.CurrentCommit != "" {
				triggerInfo = fmt.Sprintf("%d commit(s) behind", freshness.CommitsBehind)
			}
		}

		logger.Info("Index stale, triggering reindex",
			"trigger", string(trigger),
			"reason", freshness.Reason,
		)

		if err := triggerReindex(repoRoot, ckbDir, trigger, triggerInfo, logger); err != nil {
			logger.Error("Reindex failed", "error", err.Error())
		}
	}
}

// triggerReindex runs the SCIP indexer and updates metadata
func triggerReindex(repoRoot, ckbDir string, trigger index.RefreshTrigger, triggerInfo string, logger *slog.Logger) error {
	// Load project config to get language and indexer
	config, err := project.LoadConfig(repoRoot)
	if err != nil {
		return err
	}

	// Get indexer command
	indexer := project.GetIndexerInfo(config.Language)
	if indexer == nil {
		return nil // No indexer for this language
	}

	// Acquire lock
	lock, err := index.AcquireLock(ckbDir)
	if err != nil {
		// Another process is indexing, skip
		logger.Debug("Skipping reindex, locked by another process")
		return nil
	}
	defer lock.Release()

	// Run indexer
	start := time.Now()
	command := indexer.Command
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil
	}

	cmd := exec.Command(parts[0], parts[1:]...) // #nosec G204 //nolint:gosec // command from trusted indexer config
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		logger.Error("Indexer failed",
			"error", err.Error(),
			"stderr", stderr.String(),
		)
		return err
	}

	duration := time.Since(start)

	// Update metadata with refresh trigger info
	newMeta := &index.IndexMeta{
		CreatedAt:   time.Now(),
		FileCount:   countSourceFiles(repoRoot, config.Language),
		Duration:    duration.Round(time.Millisecond * 100).String(),
		Indexer:     indexer.CheckCommand,
		IndexerArgs: parts,
		LastRefresh: &index.LastRefresh{
			At:          time.Now(),
			Trigger:     trigger,
			TriggerInfo: triggerInfo,
			DurationMs:  duration.Milliseconds(),
		},
	}

	if rs, err := repostate.ComputeRepoState(repoRoot); err == nil {
		newMeta.CommitHash = rs.HeadCommit
		newMeta.RepoStateID = rs.RepoStateID
	}

	if err := newMeta.Save(ckbDir); err != nil {
		logger.Error("Failed to save index metadata", "error", err.Error())
	}

	logger.Info("Reindex complete",
		"trigger", string(trigger),
		"duration", duration.String(),
		"files", newMeta.FileCount,
	)

	return nil
}

// listPresets prints available presets with tool counts and token estimates
func listPresets() error {
	// Create a minimal logger for server initialization (silent)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create server to get tool definitions
	server := mcp.NewMCPServer(version.Version, nil, logger)
	allTools := server.GetToolDefinitions()
	presets := mcp.GetAllPresetInfo(allTools)

	fmt.Println()
	fmt.Println("Available presets:")
	fmt.Println()

	// Print table header
	fmt.Printf("  %-12s %6s %14s  %s\n", "PRESET", "TOOLS", "TOKENS", "DESCRIPTION")
	fmt.Printf("  %-12s %6s %14s  %s\n", "------", "-----", "------", "-----------")

	for _, p := range presets {
		suffix := ""
		if p.IsDefault {
			suffix = " (default)"
		}
		fmt.Printf("  %-12s %6d %14s  %s%s\n",
			p.Name,
			p.ToolCount,
			mcp.FormatTokens(p.TokenCount),
			p.Description,
			suffix,
		)
	}

	fmt.Println()
	fmt.Printf("Use: ckb mcp --preset=<name>\n")
	fmt.Println()

	return nil
}
