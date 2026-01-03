package main

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/paths"
)

var (
	logFollow bool
	logLines  int
)

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "View CKB logs",
	Long: `View CKB daemon and operation logs.

Examples:
  ckb log              # Show last 50 lines
  ckb log -n 100       # Show last 100 lines
  ckb log -f           # Follow log output (tail -f)`,
	RunE: runLog,
}

func init() {
	logCmd.Flags().BoolVarP(&logFollow, "follow", "f", false, "Follow log output")
	logCmd.Flags().IntVarP(&logLines, "lines", "n", 50, "Number of lines to show")
	rootCmd.AddCommand(logCmd)
}

func runLog(cmd *cobra.Command, args []string) error {
	logPath, err := paths.GetDaemonLogPath()
	if err != nil {
		return fmt.Errorf("failed to get log path: %w", err)
	}

	// Check if log file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		fmt.Println("No logs found.")
		fmt.Println()
		fmt.Printf("Log file location: %s\n", logPath)
		fmt.Println()
		fmt.Println("Logs are created when:")
		fmt.Println("  - Running 'ckb daemon start'")
		fmt.Println("  - Using verbose mode: CKB_LOG_LEVEL=debug ckb <command>")
		return nil
	}

	if logFollow {
		return followLogFile(logPath)
	}

	return showLogLines(logPath, logLines)
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
