package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ckb/internal/api"
	"ckb/internal/logging"
	"ckb/internal/repos"
	"ckb/internal/version"

	"github.com/spf13/cobra"
)

var (
	servePort        string
	serveHost        string
	serveAuthToken   string
	serveCORSAllow   string
	serveIndexServer bool
	serveIndexConfig string
	serveRepo        string
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
	serveCmd.Flags().StringVar(&serveAuthToken, "auth-token", "", "Auth token for mutating requests (env: CKB_AUTH_TOKEN)")
	serveCmd.Flags().StringVar(&serveCORSAllow, "cors-allow", "", "Comma-separated allowed CORS origins (empty=same-origin only, '*'=all)")
	serveCmd.Flags().BoolVar(&serveIndexServer, "index-server", false, "Enable index-serving endpoints for remote federation")
	serveCmd.Flags().StringVar(&serveIndexConfig, "index-config", "", "Path to index server config file (TOML)")
	serveCmd.Flags().StringVar(&serveRepo, "repo", "", "Repository path or registry name (auto-detected)")
}

func runServe(cmd *cobra.Command, args []string) error {
	// Create logger
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fmt.Printf("CKB HTTP API Server v%s\n", version.Version)

	// Build server address
	addr := fmt.Sprintf("%s:%s", serveHost, servePort)

	// Smart repo detection
	var repoRoot string
	if serveRepo != "" {
		if isRepoPath(serveRepo) {
			repoRoot = serveRepo
			fmt.Printf("Repository: %s (path)\n", repoRoot)
		} else {
			// Registry lookup
			registry, err := repos.LoadRegistry()
			if err != nil {
				return fmt.Errorf("failed to load registry: %w", err)
			}
			entry, state, err := registry.Get(serveRepo)
			if err != nil {
				return fmt.Errorf("repository '%s' not found in registry", serveRepo)
			}
			if state != repos.RepoStateValid {
				return fmt.Errorf("repository '%s' is %s", serveRepo, state)
			}
			repoRoot = entry.Path
			fmt.Printf("Repository: %s (%s) [%s]\n", serveRepo, repoRoot, state)
		}
	} else {
		repoRoot = mustGetRepoRoot()
		fmt.Printf("Repository: %s (current directory)\n", repoRoot)
	}

	// Change to repo directory
	if repoRoot != "" && repoRoot != "." {
		if err := os.Chdir(repoRoot); err != nil {
			return fmt.Errorf("failed to change to repo directory: %w", err)
		}
	}

	engine := mustGetEngine(repoRoot, logger)

	// Build server config
	serverConfig := api.DefaultServerConfig()

	// Auth token: flag > env > disabled
	authToken := serveAuthToken
	if authToken == "" {
		authToken = os.Getenv("CKB_AUTH_TOKEN")
	}
	if authToken != "" {
		serverConfig.Auth.Enabled = true
		serverConfig.Auth.Token = authToken
	} else {
		// No token = disable auth (with warning for non-localhost)
		serverConfig.Auth.Enabled = false
		if serveHost != "localhost" && serveHost != "127.0.0.1" {
			logger.Warn("Auth disabled on non-localhost bind - consider setting --auth-token or CKB_AUTH_TOKEN", map[string]interface{}{
				"host": serveHost,
			})
		}
	}

	// CORS origins
	if serveCORSAllow != "" {
		origins := strings.Split(serveCORSAllow, ",")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
		serverConfig.CORS.AllowedOrigins = origins
	}

	// Index server config
	if serveIndexServer {
		if serveIndexConfig != "" {
			indexConfig, err := api.LoadIndexServerConfig(serveIndexConfig)
			if err != nil {
				return fmt.Errorf("failed to load index config: %w", err)
			}
			serverConfig.IndexServer = indexConfig
		} else {
			// Use default config - user must still configure repos
			serverConfig.IndexServer = api.DefaultIndexServerConfig()
			logger.Warn("Index server enabled without config file - no repositories configured", nil)
		}
		serverConfig.IndexServer.Enabled = true
	}

	// Create server
	server, err := api.NewServer(addr, engine, logger, serverConfig)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

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
