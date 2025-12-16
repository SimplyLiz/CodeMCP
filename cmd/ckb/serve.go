package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ckb/internal/api"
	"ckb/internal/logging"
	"github.com/spf13/cobra"
)

var (
	servePort string
	serveHost string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start HTTP API server",
	Long: `Start the CKB HTTP API server to expose codebase comprehension
capabilities over HTTP. The server provides REST endpoints for symbol lookup,
search, references, architecture analysis, and more.`,
	RunE: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Define flags
	serveCmd.Flags().StringVar(&servePort, "port", "8080", "Port to listen on")
	serveCmd.Flags().StringVar(&serveHost, "host", "localhost", "Host to bind to")
}

func runServe(cmd *cobra.Command, args []string) error {
	// Create logger
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	// Build server address
	addr := fmt.Sprintf("%s:%s", serveHost, servePort)

	// Get repo root and create Query Engine
	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	// Create server
	server := api.NewServer(addr, engine, logger)

	// Setup graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("Starting CKB HTTP API server", map[string]interface{}{
			"addr": addr,
		})
		fmt.Printf("CKB HTTP API server listening on http://%s\n", addr)
		fmt.Println("Press Ctrl+C to stop")
		serverErr <- server.Start()
	}()

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErr:
		if err != nil {
			logger.Error("Server error", map[string]interface{}{
				"error": err.Error(),
			})
			return err
		}
	case sig := <-shutdown:
		logger.Info("Received shutdown signal", map[string]interface{}{
			"signal": sig.String(),
		})

		// Create shutdown context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := server.Shutdown(ctx); err != nil {
			logger.Error("Error during shutdown", map[string]interface{}{
				"error": err.Error(),
			})
			return err
		}

		logger.Info("Server stopped gracefully", nil)
	}

	return nil
}
