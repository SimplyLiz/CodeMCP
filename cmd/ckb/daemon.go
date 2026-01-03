package main

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"ckb/internal/config"
	"ckb/internal/daemon"
	"ckb/internal/paths"
	"ckb/internal/scheduler"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the CKB daemon",
	Long: `Manage the CKB daemon for always-on service.

The daemon provides:
- HTTP API for IDE/CI integration
- Job queue for async operations
- Scheduler for automated refresh
- File watching for git changes
- Webhooks for notifications`,
}

// Daemon flags
var (
	daemonPort       int
	daemonBind       string
	daemonForeground bool
	daemonFollow     bool
	daemonLines      int
)

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the daemon",
	Long: `Start the CKB daemon in the background.

The daemon listens on localhost:9120 by default.
Use --foreground to run in the foreground for debugging.`,
	RunE: runDaemonStart,
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the daemon",
	RunE:  runDaemonStop,
}

var daemonRestartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the daemon",
	RunE:  runDaemonRestart,
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	RunE:  runDaemonStatus,
}

var daemonLogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View daemon logs",
	RunE:  runDaemonLogs,
}

// Schedule subcommands
var daemonScheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Manage scheduled tasks",
	Long:  "List and run scheduled tasks for automated refresh, federation sync, and cleanup.",
}

var daemonScheduleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List scheduled tasks",
	Long: `List all configured scheduled tasks.

Examples:
  ckb daemon schedule list
  ckb daemon schedule list --enabled
  ckb daemon schedule list --type=refresh`,
	RunE: runScheduleList,
}

var daemonScheduleRunCmd = &cobra.Command{
	Use:   "run <schedule-id>",
	Short: "Run a schedule immediately",
	Long: `Trigger a scheduled task to run immediately.

Examples:
  ckb daemon schedule run sched_abc123`,
	Args: cobra.ExactArgs(1),
	RunE: runScheduleRun,
}

// Schedule flags
var (
	scheduleTaskType string
	scheduleEnabled  bool
	scheduleLimit    int
	scheduleFormat   string
)

func init() {
	// Add daemon command to root
	rootCmd.AddCommand(daemonCmd)

	// Add subcommands
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	daemonCmd.AddCommand(daemonRestartCmd)
	daemonCmd.AddCommand(daemonStatusCmd)
	daemonCmd.AddCommand(daemonLogsCmd)
	daemonCmd.AddCommand(daemonScheduleCmd)

	// Schedule subcommands
	daemonScheduleCmd.AddCommand(daemonScheduleListCmd)
	daemonScheduleCmd.AddCommand(daemonScheduleRunCmd)

	// Schedule list flags
	daemonScheduleListCmd.Flags().StringVar(&scheduleTaskType, "type", "", "Filter by task type (refresh, federation_sync, cleanup, health_check)")
	daemonScheduleListCmd.Flags().BoolVar(&scheduleEnabled, "enabled", false, "Show only enabled schedules")
	daemonScheduleListCmd.Flags().IntVar(&scheduleLimit, "limit", 20, "Maximum schedules to return")
	daemonScheduleListCmd.Flags().StringVar(&scheduleFormat, "format", "human", "Output format (json, human)")

	// Start flags
	daemonStartCmd.Flags().IntVar(&daemonPort, "port", 9120, "HTTP port")
	daemonStartCmd.Flags().StringVar(&daemonBind, "bind", "localhost", "Bind address")
	daemonStartCmd.Flags().BoolVar(&daemonForeground, "foreground", false, "Run in foreground")

	// Logs flags
	daemonLogsCmd.Flags().BoolVar(&daemonFollow, "follow", false, "Follow log output")
	daemonLogsCmd.Flags().IntVar(&daemonLines, "lines", 100, "Number of lines to show")
}

func runDaemonStart(cmd *cobra.Command, args []string) error {
	// Check if already running
	running, pid, err := daemon.IsRunning()
	if err != nil {
		return fmt.Errorf("failed to check daemon status: %w", err)
	}

	if running {
		fmt.Printf("Daemon is already running (PID: %d)\n", pid)
		return nil
	}

	// Load config
	cfg := config.DefaultConfig()

	// Override from flags
	if cmd.Flags().Changed("port") {
		cfg.Daemon.Port = daemonPort
	}
	if cmd.Flags().Changed("bind") {
		cfg.Daemon.Bind = daemonBind
	}

	if daemonForeground {
		// Run in foreground
		return runDaemonForeground(cfg)
	}

	// Run in background
	return runDaemonBackground()
}

func runDaemonForeground(cfg *config.Config) error {
	fmt.Printf("Starting CKB daemon on %s:%d (foreground mode)\n", cfg.Daemon.Bind, cfg.Daemon.Port)

	d, err := daemon.New(&cfg.Daemon)
	if err != nil {
		return fmt.Errorf("failed to create daemon: %w", err)
	}

	if err := d.Start(); err != nil {
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	// Wait for shutdown signal
	d.Wait()

	// Stop gracefully
	return d.Stop()
}

func runDaemonBackground() error {
	// Get the path to the current executable
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Build command to run daemon in foreground
	args := []string{"daemon", "start", "--foreground"}
	if daemonPort != 9120 {
		args = append(args, fmt.Sprintf("--port=%d", daemonPort))
	}
	if daemonBind != "localhost" {
		args = append(args, fmt.Sprintf("--bind=%s", daemonBind))
	}

	// Start the process
	cmd := exec.Command(executable, args...)

	// Detach from parent (platform-specific)
	setDaemonSysProcAttr(cmd)

	// Redirect stdout/stderr to log file
	logPath, err := paths.GetDaemonLogPath()
	if err != nil {
		return fmt.Errorf("failed to get log path: %w", err)
	}

	// Ensure daemon directory exists
	if _, dirErr := paths.EnsureDaemonDir(); dirErr != nil {
		return fmt.Errorf("failed to create daemon directory: %w", dirErr)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("failed to start daemon: %w", err)
	}

	_ = logFile.Close()

	fmt.Printf("Daemon started (PID: %d)\n", cmd.Process.Pid)
	fmt.Printf("Listening on %s:%d\n", daemonBind, daemonPort)
	fmt.Printf("Log file: %s\n", logPath)

	return nil
}

func runDaemonStop(cmd *cobra.Command, args []string) error {
	running, pid, err := daemon.IsRunning()
	if err != nil {
		return fmt.Errorf("failed to check daemon status: %w", err)
	}

	if !running {
		fmt.Println("Daemon is not running")
		return nil
	}

	fmt.Printf("Stopping daemon (PID: %d)...\n", pid)

	if err := daemon.StopRemote(); err != nil {
		return fmt.Errorf("failed to stop daemon: %w", err)
	}

	fmt.Println("Daemon stopped")
	return nil
}

func runDaemonRestart(cmd *cobra.Command, args []string) error {
	// Stop if running
	running, _, _ := daemon.IsRunning()
	if running {
		if err := runDaemonStop(cmd, args); err != nil {
			return err
		}
	}

	// Start
	return runDaemonStart(cmd, args)
}

func runDaemonStatus(cmd *cobra.Command, args []string) error {
	running, pid, err := daemon.IsRunning()
	if err != nil {
		return fmt.Errorf("failed to check daemon status: %w", err)
	}

	if !running {
		fmt.Println("Status: stopped")
		return nil
	}

	fmt.Printf("Status: running\n")
	fmt.Printf("PID: %d\n", pid)

	// Try to get more info from the HTTP API
	// TODO: Add HTTP client to query /health endpoint

	return nil
}

func runDaemonLogs(cmd *cobra.Command, args []string) error {
	logPath, err := paths.GetDaemonLogPath()
	if err != nil {
		return fmt.Errorf("failed to get log path: %w", err)
	}

	// Check if log file exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		fmt.Println("No log file found")
		return nil
	}

	if daemonFollow {
		// Follow mode - tail -f behavior
		return followLogs(logPath)
	}

	// Show last N lines
	return showLastLines(logPath, daemonLines)
}

func showLastLines(path string, n int) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	// Simple implementation: read all lines and show last N
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

func followLogs(path string) error {
	// Open file
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	// Seek to end
	file.Seek(0, 2)

	// Read and print new lines
	scanner := bufio.NewScanner(file)
	for {
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return err
		}

		// Sleep briefly and create new scanner from current position
		// This is a simple implementation; production would use fsnotify
		select {}
	}
}

// Schedule command handlers

func runScheduleList(cmd *cobra.Command, args []string) error {
	// Get daemon directory
	daemonDir, err := paths.GetDaemonDir()
	if err != nil {
		return fmt.Errorf("daemon not configured: %w", err)
	}

	// Create logger (silent for CLI commands)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Open scheduler
	sched, err := scheduler.New(daemonDir, logger, scheduler.DefaultConfig())
	if err != nil {
		return fmt.Errorf("failed to access scheduler: %w", err)
	}

	// Build filter options
	opts := scheduler.ListSchedulesOptions{
		Limit: scheduleLimit,
	}

	if scheduleTaskType != "" {
		opts.TaskType = []scheduler.TaskType{scheduler.TaskType(scheduleTaskType)}
	}

	if cmd.Flags().Changed("enabled") {
		opts.Enabled = &scheduleEnabled
	}

	result, err := sched.ListSchedules(opts)
	if err != nil {
		return fmt.Errorf("failed to list schedules: %w", err)
	}

	// Format and output
	output, err := FormatResponse(result, OutputFormat(scheduleFormat))
	if err != nil {
		return fmt.Errorf("error formatting output: %w", err)
	}

	fmt.Println(output)
	return nil
}

func runScheduleRun(cmd *cobra.Command, args []string) error {
	scheduleID := args[0]

	// Get daemon directory
	daemonDir, err := paths.GetDaemonDir()
	if err != nil {
		return fmt.Errorf("daemon not configured: %w", err)
	}

	// Create logger (silent for CLI commands)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Open scheduler
	sched, err := scheduler.New(daemonDir, logger, scheduler.DefaultConfig())
	if err != nil {
		return fmt.Errorf("failed to access scheduler: %w", err)
	}

	// Run the schedule
	if err := sched.RunNow(scheduleID); err != nil {
		return fmt.Errorf("failed to run schedule: %w", err)
	}

	fmt.Printf("Schedule %s triggered successfully\n", scheduleID)
	return nil
}
