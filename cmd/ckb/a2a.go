package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/a2a"
	"github.com/SimplyLiz/CodeMCP/internal/config"
	"github.com/SimplyLiz/CodeMCP/internal/mcp"
	"github.com/SimplyLiz/CodeMCP/internal/repos"
	"github.com/SimplyLiz/CodeMCP/internal/slogutil"
	"github.com/SimplyLiz/CodeMCP/internal/version"

	"github.com/spf13/cobra"
)

var (
	a2aPort      string
	a2aHost      string
	a2aAuthToken string
	a2aCORSAllow string
	a2aRepo      string
	a2aBaseURL   string
)

var a2aCmd = &cobra.Command{
	Use:   "a2a",
	Short: "Start A2A protocol server",
	Long: `Start the CKB A2A (Agent-to-Agent) protocol server. This exposes
CKB's code intelligence tools as A2A skills, discoverable via the standard
agent card at /.well-known/agent-card.json.

Supports both JSON-RPC and HTTP+JSON protocol bindings, SSE streaming,
push notifications, and multi-turn task conversations.`,
	RunE: runA2A,
}

func init() {
	rootCmd.AddCommand(a2aCmd)

	a2aCmd.Flags().StringVar(&a2aPort, "port", "8081", "Port to listen on")
	a2aCmd.Flags().StringVar(&a2aHost, "host", "localhost", "Host to bind to")
	a2aCmd.Flags().StringVar(&a2aAuthToken, "auth-token", "", "Bearer token for authentication (env: CKB_A2A_TOKEN)")
	a2aCmd.Flags().StringVar(&a2aCORSAllow, "cors-allow", "", "Comma-separated allowed CORS origins")
	a2aCmd.Flags().StringVar(&a2aRepo, "repo", "", "Repository path or registry name (auto-detected)")
	a2aCmd.Flags().StringVar(&a2aBaseURL, "base-url", "", "Public base URL for agent card (auto-detected)")
}

func runA2A(cmd *cobra.Command, args []string) error {
	cliLevel := slogutil.LevelFromVerbosity(verbosity, quiet)
	if os.Getenv("CKB_DEBUG") == "1" {
		cliLevel = slog.LevelDebug
	}
	logger := slogutil.NewLogger(os.Stderr, cliLevel)

	fmt.Printf("CKB A2A Protocol Server v%s\n", version.Version)

	addr := fmt.Sprintf("%s:%s", a2aHost, a2aPort)

	// Smart repo detection (same pattern as serve.go)
	var repoRoot string
	if a2aRepo != "" {
		if isRepoPath(a2aRepo) {
			repoRoot = a2aRepo
			fmt.Printf("Repository: %s (path)\n", repoRoot)
		} else {
			registry, err := repos.LoadRegistry()
			if err != nil {
				return fmt.Errorf("failed to load registry: %w", err)
			}
			entry, state, err := registry.Get(a2aRepo)
			if err != nil {
				return fmt.Errorf("repository '%s' not found in registry", a2aRepo)
			}
			if state != repos.RepoStateValid {
				return fmt.Errorf("repository '%s' is %s", a2aRepo, state)
			}
			repoRoot = entry.Path
			fmt.Printf("Repository: %s (%s) [%s]\n", a2aRepo, repoRoot, state)
		}
	} else {
		repoRoot = mustGetRepoRoot()
		fmt.Printf("Repository: %s (current directory)\n", repoRoot)
	}

	if repoRoot != "" && repoRoot != "." {
		if err := os.Chdir(repoRoot); err != nil {
			return fmt.Errorf("failed to change to repo directory: %w", err)
		}
	}

	// Set up logging
	cfg, _ := config.LoadConfig(repoRoot)
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	factory := slogutil.NewLoggerFactory(repoRoot, cfg, cliLevel)
	defer factory.Close()

	if fileLogger, err := factory.APILogger(); err == nil {
		stderrHandler := slogutil.NewCKBHandler(os.Stderr, &slog.HandlerOptions{Level: cliLevel})
		logger = slogutil.NewTeeLogger(fileLogger.Handler(), stderrHandler)
	}

	// Initialize query engine
	engine := mustGetEngine(repoRoot, logger)

	// Create MCP server for tool access (not started — used as tool handler only)
	mcpServer := mcp.NewMCPServer(version.Version, engine, logger)

	// Resolve CKB directory
	ckbDir := filepath.Join(repoRoot, ".ckb")

	// Auth token: flag > env
	authToken := a2aAuthToken
	if authToken == "" {
		authToken = os.Getenv("CKB_A2A_TOKEN")
	}

	// CORS origins
	var corsOrigins []string
	if a2aCORSAllow != "" {
		origins := strings.Split(a2aCORSAllow, ",")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
		corsOrigins = origins
	}

	// Base URL
	baseURL := a2aBaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("http://%s", addr)
	}

	// Create A2A server
	serverConfig := a2a.ServerConfig{
		Addr:      addr,
		AuthToken: authToken,
		CORSAllow: corsOrigins,
		CKBDir:    ckbDir,
		BaseURL:   baseURL,
	}

	server, err := a2a.NewServer(engine, mcpServer, logger, serverConfig)
	if err != nil {
		return fmt.Errorf("failed to create A2A server: %w", err)
	}

	// Graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("Starting CKB A2A server", "addr", addr)
		fmt.Printf("A2A server listening on %s\n", baseURL)
		fmt.Printf("Agent card: %s%s\n", baseURL, a2a.WellKnownPath)
		fmt.Println("Press Ctrl+C to stop")
		serverErr <- server.Start()
	}()

	select {
	case err = <-serverErr:
		if err != nil {
			logger.Error("A2A server error", "error", err.Error())
			return err
		}
	case sig := <-shutdown:
		logger.Info("Received shutdown signal", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err = server.Shutdown(ctx); err != nil {
			logger.Error("Error during shutdown", "error", err.Error())
			return err
		}
		logger.Info("A2A server stopped gracefully")
	}

	return nil
}
