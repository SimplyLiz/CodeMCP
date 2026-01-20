// Package slogutil provides custom slog handlers and utilities for CKB logging.
package slogutil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
)

// CKBHandler is a custom slog handler that formats logs in CKB's format:
// TIMESTAMP [level] Message | key=value, key=value
type CKBHandler struct {
	w      io.Writer
	level  slog.Leveler
	attrs  []slog.Attr
	groups []string
	mu     *sync.Mutex
}

// NewCKBHandler creates a new CKB log handler.
func NewCKBHandler(w io.Writer, opts *slog.HandlerOptions) *CKBHandler {
	level := slog.LevelInfo
	if opts != nil && opts.Level != nil {
		level = opts.Level.Level()
	}
	return &CKBHandler{
		w:     w,
		level: level,
		mu:    &sync.Mutex{},
	}
}

// Enabled reports whether the handler handles records at the given level.
func (h *CKBHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

// Handle formats and writes the log record.
func (h *CKBHandler) Handle(_ context.Context, r slog.Record) error {
	var buf bytes.Buffer

	// Timestamp
	buf.WriteString(r.Time.UTC().Format(time.RFC3339))

	// Level
	buf.WriteString(" [")
	buf.WriteString(levelString(r.Level))
	buf.WriteString("] ")

	// Message
	buf.WriteString(r.Message)

	// Collect all attributes (pre-set + record attrs)
	attrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, h.resolveAttr(a))
		return true
	})

	// Format attributes
	if len(attrs) > 0 {
		buf.WriteString(" |")
		for _, a := range attrs {
			if a.Key == "" {
				continue
			}
			buf.WriteString(" ")
			buf.WriteString(a.Key)
			buf.WriteString("=")
			buf.WriteString(formatValue(a.Value))
		}
	}

	buf.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(buf.Bytes())
	return err
}

// WithAttrs returns a new handler with the given attributes added.
func (h *CKBHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs), len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)

	for _, a := range attrs {
		newAttrs = append(newAttrs, h.resolveAttr(a))
	}

	return &CKBHandler{
		w:      h.w,
		level:  h.level,
		attrs:  newAttrs,
		groups: h.groups,
		mu:     h.mu,
	}
}

// WithGroup returns a new handler with the given group name added.
func (h *CKBHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	return &CKBHandler{
		w:      h.w,
		level:  h.level,
		attrs:  h.attrs,
		groups: newGroups,
		mu:     h.mu,
	}
}

// resolveAttr applies group prefixes to attribute keys.
func (h *CKBHandler) resolveAttr(a slog.Attr) slog.Attr {
	if len(h.groups) == 0 {
		return a
	}
	// Prefix key with group names
	key := a.Key
	for i := len(h.groups) - 1; i >= 0; i-- {
		key = h.groups[i] + "." + key
	}
	return slog.Attr{Key: key, Value: a.Value}
}

// levelString returns a lowercase string for the log level.
func levelString(level slog.Level) string {
	switch {
	case level < slog.LevelInfo:
		return "debug"
	case level < slog.LevelWarn:
		return "info"
	case level < slog.LevelError:
		return "warn"
	default:
		return "error"
	}
}

// formatValue formats a slog.Value for display.
func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	case slog.KindDuration:
		return v.Duration().String()
	default:
		return fmt.Sprint(v.Any())
	}
}
