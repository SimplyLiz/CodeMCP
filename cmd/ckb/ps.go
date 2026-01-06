package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/daemon"
)

var (
	psJSONFlag bool
)

var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List CKB processes",
	Long: `List all running CKB processes including daemon, MCP servers, and watchers.

Examples:
  ckb ps           # List all CKB processes
  ckb ps --json    # Output as JSON`,
	RunE: runPs,
}

func init() {
	psCmd.Flags().BoolVar(&psJSONFlag, "json", false, "Output as JSON")
	rootCmd.AddCommand(psCmd)
}

// ProcessInfo describes a running CKB process
type ProcessInfo struct {
	Type    string `json:"type"`              // daemon, mcp, watcher
	PID     int    `json:"pid"`               // Process ID
	Status  string `json:"status"`            // running, stopped
	Port    int    `json:"port,omitempty"`    // Port if applicable
	Uptime  string `json:"uptime,omitempty"`  // Uptime if available
	Details string `json:"details,omitempty"` // Additional info
}

// PsResponse contains the list of processes
type PsResponse struct {
	Processes []ProcessInfo `json:"processes"`
	Total     int           `json:"total"`
}

func runPs(cmd *cobra.Command, args []string) error {
	resp := PsResponse{
		Processes: make([]ProcessInfo, 0),
	}

	// Check daemon status
	daemonProc := getDaemonProcess()
	resp.Processes = append(resp.Processes, daemonProc)
	if daemonProc.Status == "running" {
		resp.Total++
	}

	// Future: Check MCP server processes
	// Future: Check watcher processes

	if psJSONFlag {
		data, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Human format
	return formatPsHuman(resp)
}

func getDaemonProcess() ProcessInfo {
	proc := ProcessInfo{
		Type:   "daemon",
		Status: "stopped",
	}

	running, pid, err := daemon.IsRunning()
	if err != nil || !running {
		return proc
	}

	proc.Status = "running"
	proc.PID = pid
	proc.Port = 9120 // default

	// Try to get uptime from health endpoint
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://localhost:9120/health")
	if err != nil {
		return proc
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var health daemon.HealthResponse
		if err := json.NewDecoder(resp.Body).Decode(&health); err == nil {
			proc.Uptime = health.Uptime
		}
	}

	return proc
}

func formatPsHuman(resp PsResponse) error {
	if resp.Total == 0 {
		fmt.Println("No CKB processes running.")
		fmt.Println()
		fmt.Println("Start the daemon with: ckb daemon start")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TYPE\tPID\tSTATUS\tPORT\tUPTIME")

	for _, p := range resp.Processes {
		if p.Status != "running" {
			continue
		}

		port := "-"
		if p.Port > 0 {
			port = fmt.Sprintf("%d", p.Port)
		}

		uptime := "-"
		if p.Uptime != "" {
			uptime = p.Uptime
		}

		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n", p.Type, p.PID, p.Status, port, uptime)
	}

	w.Flush()

	fmt.Printf("\nTotal: %d process(es)\n", resp.Total)
	return nil
}
