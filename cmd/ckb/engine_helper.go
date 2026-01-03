package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"ckb/internal/config"
	"ckb/internal/query"
	"ckb/internal/repos"
	"ckb/internal/slogutil"
	"ckb/internal/storage"
)

var (
	engineOnce   sync.Once
	sharedEngine *query.Engine
	engineErr    error
)

// getEngine returns a shared Query Engine instance.
// The engine is lazily initialized on first use.
func getEngine(repoRoot string, logger *slog.Logger) (*query.Engine, error) {
	engineOnce.Do(func() {
		// Load configuration
		cfg, err := config.LoadConfig(repoRoot)
		if err != nil {
			logger.Warn("Failed to load config, using defaults", "error", err.Error())
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
			logger.Warn("Requested tier not available", "error", err.Error())
		}

		sharedEngine = engine
	})

	return sharedEngine, engineErr
}

// mustGetEngine returns the shared Query Engine or exits on error.
func mustGetEngine(repoRoot string, logger *slog.Logger) *query.Engine {
	engine, err := getEngine(repoRoot, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing engine: %v\n", err)
		os.Exit(1)
	}
	return engine
}

// getRepoRoot returns the repository root directory.
// It uses the global repo resolution order:
// 1. CKB_REPO environment variable
// 2. Current directory matches a registered repo
// 3. Default repo from registry
// 4. Falls back to current working directory
func getRepoRoot() (string, error) {
	resolved, err := repos.ResolveActiveRepo("")
	if err != nil {
		// Fall back to CWD if registry can't be loaded
		return os.Getwd()
	}

	if resolved.Entry != nil {
		return resolved.Entry.Path, nil
	}

	// No registered repo found, fall back to CWD
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
// Logs always go to stderr to keep stdout clean for command output.
// Respects global -v/-q flags and CKB_DEBUG env var.
func newLogger(_ string) *slog.Logger {
	level := slogutil.LevelFromVerbosity(verbosity, quiet)
	if os.Getenv("CKB_DEBUG") == "1" {
		level = slog.LevelDebug
	}
	return slogutil.NewLogger(os.Stderr, level)
}
