package slogutil

import (
	"io"
	"log/slog"
	"path/filepath"

	"ckb/internal/config"
	"ckb/internal/paths"
)

// LoggerFactory creates appropriately configured loggers for different subsystems.
// It respects the configuration precedence: CLI flags > subsystem config > global config.
type LoggerFactory struct {
	repoRoot     string
	config       *config.Config
	cliLevel     slog.Level // from CLI flags (0 means not set)
	closers      []io.Closer
	lokiHandlers []*LokiHandler
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
	logger, closer, err := f.createFileLogger(logPath, level, "mcp")
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
	logger, closer, err := f.createFileLogger(logPath, level, "api")
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
	logger, closer, err := f.createFileLogger(logPath, level, "index")
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
	logger, closer, err := f.createFileLogger(logPath, level, "daemon")
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
	logger, closer, err := f.createFileLogger(logPath, level, "system")
	if err != nil {
		return NewDiscardLogger(), nil
	}

	f.closers = append(f.closers, closer)
	return logger, nil
}

// createFileLogger creates a file logger with optional rotation and remote logging.
func (f *LoggerFactory) createFileLogger(path string, level slog.Level, subsystem string) (*slog.Logger, io.Closer, error) {
	var fileLogger *slog.Logger
	var closer io.Closer
	var err error

	// Check if rotation is configured
	if f.config.Logging.MaxSize != "" {
		fileLogger, closer, err = NewFileLoggerWithRotation(path, level, f.config.Logging.MaxSize, f.config.Logging.MaxBackups)
	} else {
		// No rotation, use regular file logger
		fileLogger, closer, err = NewFileLogger(path, level)
	}
	if err != nil {
		return nil, nil, err
	}

	// Check if Loki remote logging is configured
	if f.config.Logging.Remote != nil && f.config.Logging.Remote.Type == "loki" {
		repoName := filepath.Base(f.repoRoot)
		if repoName == "" || repoName == "." {
			repoName = "unknown"
		}

		lokiHandler, lokiErr := NewLokiHandler(f.config.Logging.Remote, map[string]string{
			"app":       "ckb",
			"repo":      repoName,
			"subsystem": subsystem,
		}, level)

		if lokiErr == nil {
			lokiHandler.Start()
			f.lokiHandlers = append(f.lokiHandlers, lokiHandler)

			// Create tee logger with both file and Loki handlers
			return slog.New(NewTeeHandler(fileLogger.Handler(), lokiHandler)), closer, nil
		}
		// If Loki setup fails, just use file logger (best effort)
	}

	return fileLogger, closer, nil
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

// Close closes all open log files and stops Loki handlers.
func (f *LoggerFactory) Close() error {
	var firstErr error

	// Stop Loki handlers first (flush remaining logs)
	for _, lh := range f.lokiHandlers {
		if err := lh.Stop(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	f.lokiHandlers = nil

	// Close file handles
	for _, c := range f.closers {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	f.closers = nil

	return firstErr
}
