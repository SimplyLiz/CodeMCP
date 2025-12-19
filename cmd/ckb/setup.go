package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	setupGlobal bool
	setupNpx    bool
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure CKB for Claude Code",
	Long: `Sets up CKB as an MCP server for Claude Code.

By default, creates a .mcp.json file in the current directory.
Use --global to configure for all projects.

Examples:
  ckb setup              # Configure for current project
  ckb setup --global     # Configure globally for all projects
  ckb setup --npx        # Use npx for portable setup`,
	RunE: runSetup,
}

func init() {
	setupCmd.Flags().BoolVar(&setupGlobal, "global", false, "Configure globally for all projects")
	setupCmd.Flags().BoolVar(&setupNpx, "npx", false, "Use npx @tastehub/ckb for portable setup")
	rootCmd.AddCommand(setupCmd)
}

// mcpConfig represents the .mcp.json structure
type mcpConfig struct {
	McpServers map[string]mcpServer `json:"mcpServers"`
}

type mcpServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func runSetup(cmd *cobra.Command, args []string) error {
	// Determine the CKB command to use
	var ckbCommand string
	var ckbArgs []string

	if setupNpx {
		ckbCommand = "npx"
		ckbArgs = []string{"-y", "@tastehub/ckb", "mcp"}
	} else {
		// Find the current ckb binary
		ckbPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to find ckb binary: %w", err)
		}
		// Resolve symlinks
		ckbPath, err = filepath.EvalSymlinks(ckbPath)
		if err != nil {
			return fmt.Errorf("failed to resolve ckb path: %w", err)
		}
		ckbCommand = ckbPath
		ckbArgs = []string{"mcp"}
	}

	if setupGlobal {
		return setupGlobalConfig(ckbCommand, ckbArgs)
	}

	return setupProjectConfig(ckbCommand, ckbArgs)
}

func setupProjectConfig(ckbCommand string, ckbArgs []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	mcpPath := filepath.Join(cwd, ".mcp.json")

	// Read existing config or create new
	config := mcpConfig{
		McpServers: make(map[string]mcpServer),
	}

	if data, readErr := os.ReadFile(mcpPath); readErr == nil {
		if jsonErr := json.Unmarshal(data, &config); jsonErr != nil {
			fmt.Printf("Warning: existing .mcp.json is invalid, will overwrite\n")
			config.McpServers = make(map[string]mcpServer)
		}
	}

	// Add or update CKB entry
	config.McpServers["ckb"] = mcpServer{
		Command: ckbCommand,
		Args:    ckbArgs,
	}

	// Write config
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(mcpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write .mcp.json: %w", err)
	}

	fmt.Printf("Added CKB to .mcp.json\n")
	fmt.Printf("  Command: %s %v\n", ckbCommand, ckbArgs)
	fmt.Println("\nClaude Code will now have access to CKB tools.")
	fmt.Println("Restart Claude Code to load the new configuration.")

	return nil
}

func setupGlobalConfig(ckbCommand string, ckbArgs []string) error {
	// Use claude mcp add command
	if !isClaudeAvailable() {
		fmt.Println("Claude CLI not found.")
		fmt.Println("Install it from: https://claude.ai/code")
		fmt.Println("\nOr use project-level setup:")
		fmt.Println("  ckb setup")
		return nil
	}

	// Build the command
	cmdArgs := []string{"mcp", "add", "--transport", "stdio", "ckb", "--scope", "user", "--"}
	cmdArgs = append(cmdArgs, ckbCommand)
	cmdArgs = append(cmdArgs, ckbArgs...)

	fmt.Printf("Running: claude %s\n", formatArgs(cmdArgs))

	execCmd := exec.Command("claude", cmdArgs...)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("failed to add CKB to Claude: %w", err)
	}

	fmt.Println("\nCKB added to Claude Code globally.")
	fmt.Println("Restart Claude Code to load the new configuration.")

	return nil
}

func isClaudeAvailable() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

func formatArgs(args []string) string {
	result := ""
	for i, arg := range args {
		if i > 0 {
			result += " "
		}
		result += arg
	}
	return result
}
