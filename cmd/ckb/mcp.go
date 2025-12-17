package main

import (
	"os"

	"ckb/internal/logging"
	"ckb/internal/mcp"
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
)

func init() {
	rootCmd.AddCommand(mcpCmd)
	mcpCmd.Flags().BoolVar(&mcpStdio, "stdio", true, "Use stdio for communication (default)")
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
