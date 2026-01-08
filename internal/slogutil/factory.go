package slogutil

import (
	"io"
	"log/slog"

	"ckb/internal/config"
	"ckb/internal/paths"
)

// LoggerFactory creates appropriately configured loggers for different subsystems.
// It respects the configuration precedence: CLI flags > subsystem config > global config.
type LoggerFactory struct {
	repoRoot string
	config   *config.Config
	cliLevel slog.Level // from CLI flags (0 means not set)
	closers  []io.Closer
}

// NewLoggerFactory creates a new logger factory.
// cliLevel should be 0 if no CLI override was specified.
func NewLoggerFactory(repoRoot string, cfg *config.Config, cliLevel slog.Level) *LoggerFactory {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	return &LoggerFactory{
		repoRoot: repoRoot,
		config:   cfg,
		cliLevel: cliLevel,
		closers:  make([]io.Closer, 0),
	}
}

// MCPLogger creates a logger for the MCP server.
// Writes to <repoRoot>/.ckb/logs/mcp.log
func (f *LoggerFactory) MCPLogger() (*slog.Logger, error) {
	if f.repoRoot == "" {
		return NewDiscardLogger(), nil
	}

	logPath, err := paths.GetMCPLogPath(f.repoRoot)
	if err != nil {
		return NewDiscardLogger(), nil
	}

	if _, err := paths.EnsureRepoLogsDir(f.repoRoot); err != nil {
		return NewDiscardLogger(), nil
	}

	level := f.effectiveLevel("mcp")
	logger, closer, err := f.createFileLogger(logPath, level)
	if err != nil {
		return NewDiscardLogger(), nil
	}

	f.closers = append(f.closers, closer)
	return logger, nil
}

// APILogger creates a logger for the HTTP API server.
// Writes to <repoRoot>/.ckb/logs/api.log
func (f *LoggerFactory) APILogger() (*slog.Logger, error) {
	if f.repoRoot == "" {
		return NewDiscardLogger(), nil
	}

	logPath, err := paths.GetAPILogPath(f.repoRoot)
	if err != nil {
		return NewDiscardLogger(), nil
	}

	if _, err := paths.EnsureRepoLogsDir(f.repoRoot); err != nil {
		return NewDiscardLogger(), nil
	}

	level := f.effectiveLevel("api")
	logger, closer, err := f.createFileLogger(logPath, level)
	if err != nil {
		return NewDiscardLogger(), nil
	}

	f.closers = append(f.closers, closer)
	return logger, nil
}

// IndexLogger creates a logger for indexing operations.
// Writes to <repoRoot>/.ckb/logs/index.log
func (f *LoggerFactory) IndexLogger() (*slog.Logger, error) {
	if f.repoRoot == "" {
		return NewDiscardLogger(), nil
	}

	logPath, err := paths.GetIndexLogPath(f.repoRoot)
	if err != nil {
		return NewDiscardLogger(), nil
	}

	if _, err := paths.EnsureRepoLogsDir(f.repoRoot); err != nil {
		return NewDiscardLogger(), nil
	}

	level := f.effectiveLevel("index")
	logger, closer, err := f.createFileLogger(logPath, level)
	if err != nil {
		return NewDiscardLogger(), nil
	}

	f.closers = append(f.closers, closer)
	return logger, nil
}

// DaemonLogger creates a logger for the background daemon.
// Writes to ~/.ckb/daemon/daemon.log (existing location)
func (f *LoggerFactory) DaemonLogger() (*slog.Logger, error) {
	logPath, err := paths.GetDaemonLogPath()
	if err != nil {
		return NewDiscardLogger(), nil
	}

	if _, err := paths.EnsureDaemonDir(); err != nil {
		return NewDiscardLogger(), nil
	}

	level := f.effectiveLevel("daemon")
	logger, closer, err := f.createFileLogger(logPath, level)
	if err != nil {
		return NewDiscardLogger(), nil
	}

	f.closers = append(f.closers, closer)
	return logger, nil
}

// SystemLogger creates a logger for global system operations.
// Writes to ~/.ckb/logs/system.log
func (f *LoggerFactory) SystemLogger() (*slog.Logger, error) {
	logPath, err := paths.GetSystemLogPath()
	if err != nil {
		return NewDiscardLogger(), nil
	}

	if _, err := paths.EnsureGlobalLogsDir(); err != nil {
		return NewDiscardLogger(), nil
	}

	level := f.effectiveLevel("system")
	logger, closer, err := f.createFileLogger(logPath, level)
	if err != nil {
		return NewDiscardLogger(), nil
	}

	f.closers = append(f.closers, closer)
	return logger, nil
}

// createFileLogger creates a file logger with optional rotation based on config
func (f *LoggerFactory) createFileLogger(path string, level slog.Level) (*slog.Logger, io.Closer, error) {
	// Check if rotation is configured
	if f.config.Logging.MaxSize != "" {
		return NewFileLoggerWithRotation(path, level, f.config.Logging.MaxSize, f.config.Logging.MaxBackups)
	}
	// No rotation, use regular file logger
	return NewFileLogger(path, level)
}

// effectiveLevel returns the effective log level for a subsystem.
// Precedence: CLI flag > subsystem config > global config > default (info)
func (f *LoggerFactory) effectiveLevel(subsystem string) slog.Level {
	// CLI flag takes highest precedence
	if f.cliLevel != 0 {
		return f.cliLevel
	}

	// Check subsystem-specific config
	var subsystemLevel string
	switch subsystem {
	case "mcp":
		subsystemLevel = f.config.Logging.MCP
	case "api":
		subsystemLevel = f.config.Logging.API
	case "index":
		subsystemLevel = f.config.Logging.Index
	}

	if subsystemLevel != "" {
		return LevelFromString(subsystemLevel)
	}

	// Fall back to global config level
	if f.config.Logging.Level != "" {
		return LevelFromString(f.config.Logging.Level)
	}

	// Default
	return slog.LevelInfo
}

// Close closes all open log files.
func (f *LoggerFactory) Close() error {
	var firstErr error
	for _, c := range f.closers {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	f.closers = nil
	return firstErr
}
