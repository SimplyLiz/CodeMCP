package main

import (
	"context"
	"os"

	"ckb/internal/logging"
	"ckb/internal/update"
)

func main() {
	logger := logging.NewLogger(logging.Config{
		Format: "human",
		Level:  "info",
	})

	// Start async update check (skip for mcp command to avoid breaking protocol)
	var updateCh <-chan *update.UpdateInfo
	if !isMCPCommand() {
		checker := update.NewChecker()
		updateCh = checker.CheckAsync(context.Background())
	}

	if err := rootCmd.Execute(); err != nil {
		logger.Error("Command execution failed", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	// Print update notification if available
	if updateCh != nil {
		select {
		case info := <-updateCh:
			if info != nil {
				_, _ = os.Stderr.WriteString(info.FormatUpdateMessage())
			}
		default:
			// Don't block if check hasn't completed yet
		}
	}
}

// isMCPCommand checks if the current command is 'mcp' or 'serve'
// These commands should not show update notifications as it would break protocols
func isMCPCommand() bool {
	if len(os.Args) < 2 {
		return false
	}
	cmd := os.Args[1]
	return cmd == "mcp" || cmd == "serve"
}
