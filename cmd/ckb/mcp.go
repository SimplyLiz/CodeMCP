package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ckb/internal/index"
	"ckb/internal/logging"
	"ckb/internal/mcp"
	"ckb/internal/project"
	"ckb/internal/repostate"
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
	mcpStdio bool
	mcpWatch bool
)

const watchPollInterval = 30 * time.Second

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.Flags().BoolVar(&mcpStdio, "stdio", true, "Use stdio for communication (default)")
	mcpCmd.Flags().BoolVar(&mcpWatch, "watch", false, "Watch for changes and auto-reindex")
}

func runMCP(cmd *cobra.Command, args []string) error {
	// Create logger for MCP server
	// Use stderr for logs since stdout is used for MCP protocol
	logger := logging.NewLogger(logging.Config{
		Format: logging.JSONFormat,
		Level:  logging.InfoLevel,
		Output: os.Stderr,
	})

	logger.Info("Starting MCP server", map[string]interface{}{
		"version": version.Version,
	})

	// Get repo root and create Query Engine
	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	// Start watch mode if enabled
	if mcpWatch {
		go runWatchLoop(repoRoot, logger)
		logger.Info("Watch mode enabled", map[string]interface{}{
			"pollInterval": watchPollInterval.String(),
		})
	}

	// Create and start MCP server
	server := mcp.NewMCPServer(version.Version, engine, logger)

	if err := server.Start(); err != nil {
		logger.Error("MCP server error", map[string]interface{}{
			"error": err.Error(),
		})
		return err
	}

	return nil
}

// runWatchLoop periodically checks index freshness and reindexes if stale
func runWatchLoop(repoRoot string, logger *logging.Logger) {
	ckbDir := filepath.Join(repoRoot, ".ckb")
	ticker := time.NewTicker(watchPollInterval)
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

		logger.Info("Index stale, triggering reindex", map[string]interface{}{
			"reason": freshness.Reason,
		})

		if err := triggerReindex(repoRoot, ckbDir, logger); err != nil {
			logger.Error("Reindex failed", map[string]interface{}{
				"error": err.Error(),
			})
		}
	}
}

// triggerReindex runs the SCIP indexer and updates metadata
func triggerReindex(repoRoot, ckbDir string, logger *logging.Logger) error {
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
		logger.Debug("Skipping reindex, locked by another process", nil)
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

	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		logger.Error("Indexer failed", map[string]interface{}{
			"error":  err.Error(),
			"stderr": stderr.String(),
		})
		return err
	}

	duration := time.Since(start)

	// Update metadata
	newMeta := &index.IndexMeta{
		CreatedAt:   time.Now(),
		FileCount:   countSourceFiles(repoRoot, config.Language),
		Duration:    duration.Round(time.Millisecond * 100).String(),
		Indexer:     indexer.CheckCommand,
		IndexerArgs: parts,
	}

	if rs, err := repostate.ComputeRepoState(repoRoot); err == nil {
		newMeta.CommitHash = rs.HeadCommit
		newMeta.RepoStateID = rs.RepoStateID
	}

	if err := newMeta.Save(ckbDir); err != nil {
		logger.Error("Failed to save index metadata", map[string]interface{}{
			"error": err.Error(),
		})
	}

	logger.Info("Reindex complete", map[string]interface{}{
		"duration": duration.String(),
		"files":    newMeta.FileCount,
	})

	return nil
}
