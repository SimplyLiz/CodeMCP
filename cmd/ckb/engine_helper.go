package main

import (
	"context"
	"fmt"
	"os"
	"sync"

	"ckb/internal/config"
	"ckb/internal/logging"
	"ckb/internal/query"
	"ckb/internal/storage"
)

var (
	engineOnce   sync.Once
	sharedEngine *query.Engine
	engineErr    error
)

// getEngine returns a shared Query Engine instance.
// The engine is lazily initialized on first use.
func getEngine(repoRoot string, logger *logging.Logger) (*query.Engine, error) {
	engineOnce.Do(func() {
		// Load configuration
		cfg, err := config.LoadConfig(repoRoot)
		if err != nil {
			logger.Warn("Failed to load config, using defaults", map[string]interface{}{
				"error": err.Error(),
			})
			cfg = config.DefaultConfig()
		}

		// Open storage
		db, err := storage.Open(repoRoot, logger)
		if err != nil {
			engineErr = fmt.Errorf("failed to open database: %w", err)
			return
		}

		// Create engine
		engine, err := query.NewEngine(repoRoot, db, logger, cfg)
		if err != nil {
			engineErr = fmt.Errorf("failed to create engine: %w", err)
			return
		}

		// Configure tier mode from CLI flag, env var, or config
		tierMode, err := resolveTierMode(cfg)
		if err != nil {
			engineErr = fmt.Errorf("invalid tier configuration: %w", err)
			return
		}
		engine.SetTierMode(tierMode)

		// Validate that the tier requirements can be satisfied
		if err := engine.ValidateTierMode(); err != nil {
			// Log warning but don't fail - fall back to available tier
			logger.Warn("Requested tier not available", map[string]interface{}{
				"error": err.Error(),
			})
		}

		sharedEngine = engine
	})

	return sharedEngine, engineErr
}

// mustGetEngine returns the shared Query Engine or exits on error.
func mustGetEngine(repoRoot string, logger *logging.Logger) *query.Engine {
	engine, err := getEngine(repoRoot, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing engine: %v\n", err)
		os.Exit(1)
	}
	return engine
}

// getRepoRoot returns the repository root directory.
func getRepoRoot() (string, error) {
	return os.Getwd()
}

// mustGetRepoRoot returns the repository root or exits on error.
func mustGetRepoRoot() string {
	repoRoot, err := getRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	return repoRoot
}

// newContext creates a new context for command execution.
func newContext() context.Context {
	return context.Background()
}

// newLogger creates a logger with the specified format.
// For structured output formats (json, markdown, list), logs go to stderr so stdout has clean output.
func newLogger(format string) *logging.Logger {
	output := os.Stdout
	// Keep stdout clean for structured/machine-readable output formats
	if format == "json" || format == "markdown" || format == "list" {
		output = os.Stderr
	}
	return logging.NewLogger(logging.Config{
		Format: logging.HumanFormat, // Always human-readable logs
		Level:  logging.InfoLevel,
		Output: output,
	})
}
