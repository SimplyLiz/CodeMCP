package slogutil

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// NewLogger creates a new slog.Logger with CKB's custom format.
func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(NewCKBHandler(w, &slog.HandlerOptions{Level: level}))
}

// NewFileLogger creates a new slog.Logger that writes to a file.
// The file is opened in append mode and created if it doesn't exist.
func NewFileLogger(path string, level slog.Level) (*slog.Logger, *os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, err
	}
	return NewLogger(f, level), f, nil
}

// NewDiscardLogger creates a logger that discards all output.
// Useful for tests or when logging should be completely suppressed.
func NewDiscardLogger() *slog.Logger {
	return slog.New(NewCKBHandler(io.Discard, &slog.HandlerOptions{Level: slog.Level(100)}))
}

// LevelFromString converts a string to a slog.Level.
// Supports: debug, info, warn, error (case-insensitive).
// Returns slog.LevelInfo for unrecognized strings.
func LevelFromString(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// LevelFromVerbosity converts CLI verbosity flags to a slog.Level.
// - quiet=true: returns a level that suppresses all logs
// - verbosity=0: warn (default for CLI)
// - verbosity=1: info
// - verbosity>=2: debug
func LevelFromVerbosity(verbosity int, quiet bool) slog.Level {
	if quiet {
		return slog.Level(100) // Above all standard levels
	}
	switch verbosity {
	case 0:
		return slog.LevelWarn
	case 1:
		return slog.LevelInfo
	default:
		return slog.LevelDebug
	}
}

// TeeHandler writes logs to multiple handlers.
type TeeHandler struct {
	handlers []slog.Handler
}

// NewTeeHandler creates a handler that writes to all provided handlers.
func NewTeeHandler(handlers ...slog.Handler) *TeeHandler {
	return &TeeHandler{handlers: handlers}
}

// Enabled returns true if any handler is enabled for the level.
func (t *TeeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range t.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

// Handle writes the record to all handlers.
func (t *TeeHandler) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, h := range t.handlers {
		if h.Enabled(ctx, r.Level) {
			if err := h.Handle(ctx, r); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// WithAttrs returns a new TeeHandler with attributes added to all handlers.
func (t *TeeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(t.handlers))
	for i, h := range t.handlers {
		newHandlers[i] = h.WithAttrs(attrs)
	}
	return &TeeHandler{handlers: newHandlers}
}

// WithGroup returns a new TeeHandler with the group added to all handlers.
func (t *TeeHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(t.handlers))
	for i, h := range t.handlers {
		newHandlers[i] = h.WithGroup(name)
	}
	return &TeeHandler{handlers: newHandlers}
}

// NewTeeLogger creates a logger that writes to multiple destinations.
func NewTeeLogger(handlers ...slog.Handler) *slog.Logger {
	return slog.New(NewTeeHandler(handlers...))
}
