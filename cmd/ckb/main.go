package main

import (
	"context"
	"os"
	"sync"
	"time"

	"ckb/internal/logging"
	"ckb/internal/update"
)

func main() {
	logger := logging.NewLogger(logging.Config{
		Format: "human",
		Level:  "info",
	})

	// Update check with deferred notification pattern:
	// 1. Show cached notification immediately (non-blocking)
	// 2. Start background refresh for next run
	// Skip for mcp/serve commands to avoid breaking protocol
	var refreshWg sync.WaitGroup
	if !isMCPCommand() {
		checker := update.NewChecker()
		// Show cached update notification (instant, no HTTP)
		if info := checker.CheckCached(); info != nil {
			_, _ = os.Stderr.WriteString(info.FormatUpdateMessage())
		}
		// Refresh cache in background for next run
		refreshWg.Add(1)
		go func() {
			defer refreshWg.Done()
			checker.RefreshCache(context.Background())
		}()
	}

	if err := rootCmd.Execute(); err != nil {
		logger.Error("Command execution failed", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	// Wait briefly for cache refresh to complete (max 500ms)
	// This ensures the cache gets populated for next run
	waitWithTimeout(&refreshWg, 500*time.Millisecond)
}

// waitWithTimeout waits for a WaitGroup with a maximum timeout
func waitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Completed before timeout
	case <-time.After(timeout):
		// Timeout reached, exit anyway
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
