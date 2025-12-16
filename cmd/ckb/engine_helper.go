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
func newLogger(format string) *logging.Logger {
	logFormat := logging.HumanFormat
	if format == "json" {
		logFormat = logging.JSONFormat
	}
	return logging.NewLogger(logging.Config{
		Format: logFormat,
		Level:  logging.InfoLevel,
	})
}
