package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/paths"
	"ckb/internal/repos"
)

var (
	logFollow bool
	logLines  int
	logType   string
	logClear  bool
	logPath   bool
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "View CKB logs",
	Long: `View CKB logs for different subsystems.

Log types:
  daemon  - Background daemon logs (default, global)
  system  - Global system operations
  mcp     - MCP server logs (per-repo)
  api     - HTTP API logs (per-repo)
  index   - Indexing operation logs (per-repo)

Examples:
  ckb log                    # Show daemon logs (backward compatible)
  ckb log -t mcp             # Show MCP logs for active repo
  ckb log -t api             # Show API logs for active repo
  ckb log -t mcp -n 100      # Show last 100 lines of MCP logs
  ckb log -t mcp -f          # Follow MCP log output
  ckb log --path -t mcp      # Print log file path
  ckb log --clear -t mcp     # Clear MCP log file`,
	RunE: runLog,
}

func init() {
	logCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, "Follow log output")
	logCmd.Flags().IntVarP(&logLines, "lines", "n", 50, "Number of lines to show")
	logCmd.Flags().StringVarP(&logType, "type", "t", "", "Log type: daemon, system, mcp, api, index")
	logCmd.Flags().BoolVar(&logClear, "clear", false, "Clear the log file")
	logCmd.Flags().BoolVar(&logPath, "path", false, "Print log file path instead of contents")
	rootCmd.AddCommand(logCmd)
}

func runLog(cmd *cobra.Command, args []string) error {
	var logFile string
	var err error

	// Determine log file path based on type
	switch logType {
	case "daemon", "":
		// Default: daemon log (backward compatible)
		logFile, err = paths.GetDaemonLogPath()
		if err != nil {
			return fmt.Errorf("failed to get daemon log path: %w", err)
		}
	case "system":
		logFile, err = paths.GetSystemLogPath()
		if err != nil {
			return fmt.Errorf("failed to get system log path: %w", err)
		}
	case "mcp", "api", "index":
		// Per-repo logs - need active repo
		repoRoot, err := getActiveRepoRoot()
		if err != nil {
			return fmt.Errorf("failed to get active repo: %w\nUse 'ckb use <path>' to set an active repo", err)
		}
		switch logType {
		case "mcp":
			logFile, err = paths.GetMCPLogPath(repoRoot)
		case "api":
			logFile, err = paths.GetAPILogPath(repoRoot)
		case "index":
			logFile, err = paths.GetIndexLogPath(repoRoot)
		}
		if err != nil {
			return fmt.Errorf("failed to get %s log path: %w", logType, err)
		}
	default:
		return fmt.Errorf("unknown log type: %s (valid: daemon, system, mcp, api, index)", logType)
	}

	// Handle --path flag
	if logPath {
		fmt.Println(logFile)
		return nil
	}

	// Handle --clear flag
	if logClear {
		if _, err := os.Stat(logFile); os.IsNotExist(err) {
			fmt.Printf("Log file does not exist: %s\n", logFile)
			return nil
		}
		if err := os.Truncate(logFile, 0); err != nil {
			return fmt.Errorf("failed to clear log file: %w", err)
		}
		fmt.Printf("Cleared: %s\n", logFile)
		return nil
	}

	// Check if log file exists
	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		fmt.Println("No logs found.")
		fmt.Println()
		fmt.Printf("Log file location: %s\n", logFile)
		fmt.Println()
		if logType == "" || logType == "daemon" {
			fmt.Println("Logs are created when:")
			fmt.Println("  - Running 'ckb daemon start'")
		} else {
			fmt.Printf("Logs are created when using 'ckb %s' commands.\n", logType)
		}
		return nil
	}

	if logFollow {
		return followLogFile(logFile)
	}

	return showLogLines(logFile, logLines)
}

// getActiveRepoRoot returns the active repository root from registry or CWD
func getActiveRepoRoot() (string, error) {
	resolved, err := repos.ResolveActiveRepo("")
	if err != nil {
		// Fall back to CWD
		return os.Getwd()
	}
	if resolved.Entry != nil {
		return resolved.Entry.Path, nil
	}
	// No registered repo, fall back to CWD
	return os.Getwd()
}

func showLogLines(path string, n int) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	// Read all lines and keep last N
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}

	for _, line := range lines {
		fmt.Println(line)
	}

	return scanner.Err()
}

func followLogFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	// Seek to end
	_, _ = file.Seek(0, 2)

	fmt.Printf("Following %s (Ctrl+C to stop)\n\n", path)

	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// No new data, wait and retry
			time.Sleep(100 * time.Millisecond)
			continue
		}
		fmt.Print(line)
	}
}
