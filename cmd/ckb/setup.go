package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var (
	setupGlobal bool
	setupNpx    bool
	setupTool   string
)

// aiTool represents an AI coding tool that supports MCP
type aiTool struct {
	ID              string
	Name            string
	SupportsGlobal  bool
	SupportsProject bool
	GlobalUsesCmd   bool   // true = use CLI command, false = write file
	Format          string // "mcpServers" | "servers" | "mcp"
}

var aiTools = []aiTool{
	{ID: "claude-code", Name: "Claude Code", SupportsGlobal: true, SupportsProject: true, GlobalUsesCmd: true, Format: "mcpServers"},
	{ID: "cursor", Name: "Cursor", SupportsGlobal: true, SupportsProject: true, GlobalUsesCmd: false, Format: "mcpServers"},
	{ID: "windsurf", Name: "Windsurf", SupportsGlobal: true, SupportsProject: false, GlobalUsesCmd: false, Format: "mcpServers"},
	{ID: "vscode", Name: "VS Code", SupportsGlobal: true, SupportsProject: true, GlobalUsesCmd: true, Format: "servers"},
	{ID: "opencode", Name: "OpenCode", SupportsGlobal: true, SupportsProject: true, GlobalUsesCmd: false, Format: "mcp"},
	{ID: "claude-desktop", Name: "Claude Desktop", SupportsGlobal: true, SupportsProject: false, GlobalUsesCmd: false, Format: "mcpServers"},
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure CKB for AI coding tools",
	Long: `Sets up CKB as an MCP server for AI coding tools.

Supports: Claude Code, Cursor, Windsurf, VS Code, OpenCode, Claude Desktop

Examples:
  ckb setup                    # Interactive setup
  ckb setup --tool=cursor      # Configure for Cursor
  ckb setup --tool=vscode --global  # Configure VS Code globally
  ckb setup --npx              # Use npx for portable setup`,
	RunE: runSetup,
}

func init() {
	setupCmd.Flags().BoolVar(&setupGlobal, "global", false, "Configure globally for all projects")
	setupCmd.Flags().BoolVar(&setupNpx, "npx", false, "Use npx @tastehub/ckb for portable setup")
	setupCmd.Flags().StringVar(&setupTool, "tool", "", "AI tool to configure (claude-code, cursor, windsurf, vscode, opencode, claude-desktop)")
	rootCmd.AddCommand(setupCmd)
}

// Config types for different formats

// mcpServersConfig is used by Claude Code, Cursor, Windsurf, Claude Desktop
type mcpServersConfig struct {
	McpServers map[string]mcpServer `json:"mcpServers"`
}

type mcpServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// vsCodeConfig is used by VS Code (.vscode/mcp.json)
type vsCodeConfig struct {
	Servers map[string]vsCodeServer `json:"servers"`
}

type vsCodeServer struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// openCodeConfig is used by OpenCode
type openCodeConfig struct {
	Mcp map[string]openCodeMcpEntry `json:"mcp"`
}

type openCodeMcpEntry struct {
	Type    string   `json:"type"`
	Command []string `json:"command"`
	Enabled bool     `json:"enabled"`
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

	// Select tool
	var selectedTool *aiTool
	if setupTool != "" {
		// Find tool by ID
		for i := range aiTools {
			if aiTools[i].ID == setupTool {
				selectedTool = &aiTools[i]
				break
			}
		}
		if selectedTool == nil {
			return fmt.Errorf("unknown tool: %s. Valid options: claude-code, cursor, windsurf, vscode, opencode, claude-desktop", setupTool)
		}
	} else {
		// Interactive tool selection
		tool, err := selectTool()
		if err != nil {
			return err
		}
		selectedTool = tool
	}

	// Determine scope
	global := setupGlobal
	if !setupGlobal && setupTool == "" {
		// Ask for scope if tool supports both and not specified via flag
		if selectedTool.SupportsGlobal && selectedTool.SupportsProject {
			scope, err := selectScope(selectedTool)
			if err != nil {
				return err
			}
			global = scope
		} else if selectedTool.SupportsGlobal && !selectedTool.SupportsProject {
			global = true
		} else {
			global = false
		}
	}

	// Validate scope
	if global && !selectedTool.SupportsGlobal {
		return fmt.Errorf("%s does not support global configuration", selectedTool.Name)
	}
	if !global && !selectedTool.SupportsProject {
		fmt.Printf("%s only supports global configuration. Configuring globally.\n\n", selectedTool.Name)
		global = true
	}

	// Configure
	return configureTool(selectedTool, global, ckbCommand, ckbArgs)
}

func selectTool() (*aiTool, error) {
	fmt.Println("\nSelect AI tool to configure:")
	fmt.Println()
	for i, tool := range aiTools {
		fmt.Printf("  %d. %s\n", i+1, tool.Name)
	}
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("Enter choice [1-%d]: ", len(aiTools))
		input, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)
		choice, err := strconv.Atoi(input)
		if err != nil || choice < 1 || choice > len(aiTools) {
			fmt.Printf("Invalid choice. Please enter a number between 1 and %d.\n", len(aiTools))
			continue
		}

		return &aiTools[choice-1], nil
	}
}

func selectScope(tool *aiTool) (bool, error) {
	fmt.Println("\nConfigure scope:")
	fmt.Println()
	fmt.Println("  1. Project (current directory only)")
	fmt.Println("  2. Global (applies to all projects)")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("Enter choice [1-2]: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return false, fmt.Errorf("failed to read input: %w", err)
		}

		input = strings.TrimSpace(input)
		switch input {
		case "1":
			return false, nil
		case "2":
			return true, nil
		default:
			fmt.Println("Invalid choice. Please enter 1 or 2.")
		}
	}
}

func configureTool(tool *aiTool, global bool, ckbCommand string, ckbArgs []string) error {
	// Handle tools that use CLI commands for global setup
	if global && tool.GlobalUsesCmd {
		switch tool.ID {
		case "claude-code":
			return configureClaudeCodeGlobal(ckbCommand, ckbArgs)
		case "vscode":
			return configureVSCodeGlobal(ckbCommand, ckbArgs)
		}
	}

	// Get config path
	configPath := getConfigPath(tool.ID, global)
	if configPath == "" {
		return fmt.Errorf("could not determine config path for %s", tool.Name)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write config based on format
	var err error
	switch tool.Format {
	case "mcpServers":
		err = writeMcpServersConfig(configPath, ckbCommand, ckbArgs)
	case "servers":
		err = writeVSCodeConfig(configPath, ckbCommand, ckbArgs)
	case "mcp":
		err = writeOpenCodeConfig(configPath, ckbCommand, ckbArgs, setupNpx)
	default:
		err = fmt.Errorf("unknown format: %s", tool.Format)
	}

	if err != nil {
		return err
	}

	fmt.Printf("\n✓ Added CKB to %s\n", configPath)
	fmt.Printf("  Command: %s %s\n", ckbCommand, strings.Join(ckbArgs, " "))
	fmt.Printf("\nRestart %s to load the new configuration.\n", tool.Name)

	return nil
}

func getConfigPath(toolID string, global bool) string {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()

	switch toolID {
	case "claude-code":
		if global {
			// Fallback path when CLI is not available
			return filepath.Join(home, ".claude.json")
		}
		return filepath.Join(cwd, ".mcp.json")

	case "cursor":
		if global {
			return filepath.Join(home, ".cursor", "mcp.json")
		}
		return filepath.Join(cwd, ".cursor", "mcp.json")

	case "windsurf":
		// Probe multiple locations, prefer existing, default to official path
		var candidates []string
		if runtime.GOOS == "windows" {
			base := filepath.Join(os.Getenv("USERPROFILE"), ".codeium")
			candidates = []string{
				filepath.Join(base, "mcp_config.json"),
				filepath.Join(base, "windsurf", "mcp_config.json"),
			}
		} else {
			candidates = []string{
				filepath.Join(home, ".codeium", "mcp_config.json"),
				filepath.Join(home, ".codeium", "windsurf", "mcp_config.json"),
			}
		}
		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
		return candidates[0] // Default to official path

	case "vscode":
		if global {
			return "" // Use CLI command
		}
		return filepath.Join(cwd, ".vscode", "mcp.json")

	case "opencode":
		if global {
			return filepath.Join(home, ".config", "opencode", "opencode.json")
		}
		return filepath.Join(cwd, "opencode.json")

	case "claude-desktop":
		if runtime.GOOS == "windows" {
			return filepath.Join(os.Getenv("APPDATA"), "Claude", "claude_desktop_config.json")
		}
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	}

	return ""
}

func writeMcpServersConfig(path, command string, args []string) error {
	// Read existing config or create new
	config := mcpServersConfig{
		McpServers: make(map[string]mcpServer),
	}

	if data, err := os.ReadFile(path); err == nil {
		if jsonErr := json.Unmarshal(data, &config); jsonErr != nil {
			fmt.Printf("Warning: existing config is invalid, will overwrite\n")
			config.McpServers = make(map[string]mcpServer)
		}
	}

	// Add or update CKB entry
	config.McpServers["ckb"] = mcpServer{
		Command: command,
		Args:    args,
	}

	// Write config
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

func writeVSCodeConfig(path, command string, args []string) error {
	// Read existing config or create new
	config := vsCodeConfig{
		Servers: make(map[string]vsCodeServer),
	}

	if data, err := os.ReadFile(path); err == nil {
		if jsonErr := json.Unmarshal(data, &config); jsonErr != nil {
			fmt.Printf("Warning: existing config is invalid, will overwrite\n")
			config.Servers = make(map[string]vsCodeServer)
		}
	}

	// Add or update CKB entry
	config.Servers["ckb"] = vsCodeServer{
		Type:    "stdio",
		Command: command,
		Args:    args,
	}

	// Write config
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

func writeOpenCodeConfig(path, command string, args []string, useNpx bool) error {
	// Read existing config or create new
	config := openCodeConfig{
		Mcp: make(map[string]openCodeMcpEntry),
	}

	if data, err := os.ReadFile(path); err == nil {
		if jsonErr := json.Unmarshal(data, &config); jsonErr != nil {
			fmt.Printf("Warning: existing config is invalid, will overwrite\n")
			config.Mcp = make(map[string]openCodeMcpEntry)
		}
	}

	// Build command array for OpenCode format
	var cmdArray []string
	if useNpx {
		cmdArray = []string{"npx", "-y", "@tastehub/ckb", "mcp"}
	} else {
		cmdArray = append([]string{command}, args...)
	}

	// Add or update CKB entry
	config.Mcp["ckb"] = openCodeMcpEntry{
		Type:    "local",
		Command: cmdArray,
		Enabled: true,
	}

	// Write config
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

func configureClaudeCodeGlobal(ckbCommand string, ckbArgs []string) error {
	// Try using claude mcp add command first
	if isClaudeAvailable() {
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

		fmt.Println("\n✓ CKB added to Claude Code globally.")
		fmt.Println("Restart Claude Code to load the new configuration.")
		return nil
	}

	// Fallback to writing ~/.claude.json
	fmt.Println("Claude CLI not found, using fallback configuration...")
	configPath := getConfigPath("claude-code", true)
	if err := writeMcpServersConfig(configPath, ckbCommand, ckbArgs); err != nil {
		return err
	}

	fmt.Printf("\n✓ Added CKB to %s\n", configPath)
	fmt.Printf("  Command: %s %s\n", ckbCommand, strings.Join(ckbArgs, " "))
	fmt.Println("\nRestart Claude Code to load the new configuration.")
	fmt.Println("\nTip: Install Claude CLI for better integration: https://claude.ai/code")

	return nil
}

func configureVSCodeGlobal(ckbCommand string, ckbArgs []string) error {
	// Check if code command is available
	if _, err := exec.LookPath("code"); err != nil {
		return fmt.Errorf("VS Code CLI (code) not found. Please ensure VS Code is installed and 'code' is in your PATH")
	}

	// Build the MCP server JSON
	serverConfig := map[string]interface{}{
		"name":    "ckb",
		"type":    "stdio",
		"command": ckbCommand,
		"args":    ckbArgs,
	}

	jsonBytes, err := json.Marshal(serverConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal server config: %w", err)
	}

	fmt.Printf("Running: code --add-mcp '%s'\n", string(jsonBytes))

	execCmd := exec.Command("code", "--add-mcp", string(jsonBytes))
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("failed to add CKB to VS Code: %w", err)
	}

	fmt.Println("\n✓ CKB added to VS Code globally.")
	fmt.Println("Restart VS Code to load the new configuration.")

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
