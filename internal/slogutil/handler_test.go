package slogutil

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestCKBHandler_Format(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, slog.LevelInfo)

	logger.Info("Test message", "key", "value", "count", 42)

	output := buf.String()

	// Check format: TIMESTAMP [level] Message | key=value
	if !strings.Contains(output, "[info]") {
		t.Errorf("expected [info] in output, got: %s", output)
	}
	if !strings.Contains(output, "Test message") {
		t.Errorf("expected 'Test message' in output, got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("expected 'key=value' in output, got: %s", output)
	}
	if !strings.Contains(output, "count=42") {
		t.Errorf("expected 'count=42' in output, got: %s", output)
	}
	if !strings.Contains(output, " | ") {
		t.Errorf("expected ' | ' separator in output, got: %s", output)
	}
}

func TestCKBHandler_Levels(t *testing.T) {
	tests := []struct {
		level    slog.Level
		logFunc  func(*slog.Logger)
		expected string
	}{
		{slog.LevelDebug, func(l *slog.Logger) { l.Debug("debug") }, "[debug]"},
		{slog.LevelInfo, func(l *slog.Logger) { l.Info("info") }, "[info]"},
		{slog.LevelWarn, func(l *slog.Logger) { l.Warn("warn") }, "[warn]"},
		{slog.LevelError, func(l *slog.Logger) { l.Error("error") }, "[error]"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewLogger(&buf, slog.LevelDebug) // Enable all levels
			tt.logFunc(logger)

			output := buf.String()
			if !strings.Contains(output, tt.expected) {
				t.Errorf("expected %s in output, got: %s", tt.expected, output)
			}
		})
	}
}

func TestCKBHandler_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, slog.LevelWarn)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()

	if strings.Contains(output, "debug message") {
		t.Error("debug message should be filtered")
	}
	if strings.Contains(output, "info message") {
		t.Error("info message should be filtered")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("warn message should be included")
	}
	if !strings.Contains(output, "error message") {
		t.Error("error message should be included")
	}
}

func TestLevelFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"ERROR", slog.LevelError},
		{"unknown", slog.LevelInfo}, // default
		{"", slog.LevelInfo},        // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := LevelFromString(tt.input)
			if got != tt.expected {
				t.Errorf("LevelFromString(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLevelFromVerbosity(t *testing.T) {
	tests := []struct {
		verbosity int
		quiet     bool
		expected  slog.Level
	}{
		{0, false, slog.LevelWarn},
		{1, false, slog.LevelInfo},
		{2, false, slog.LevelDebug},
		{3, false, slog.LevelDebug},
		{0, true, slog.Level(100)}, // silent
		{5, true, slog.Level(100)}, // quiet overrides verbosity
	}

	for _, tt := range tests {
		got := LevelFromVerbosity(tt.verbosity, tt.quiet)
		if got != tt.expected {
			t.Errorf("LevelFromVerbosity(%d, %v) = %v, want %v",
				tt.verbosity, tt.quiet, got, tt.expected)
		}
	}
}

func TestNewDiscardLogger(t *testing.T) {
	logger := NewDiscardLogger()

	// Should not panic
	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")
}

func TestTeeHandler(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	h1 := NewCKBHandler(&buf1, &slog.HandlerOptions{Level: slog.LevelInfo})
	h2 := NewCKBHandler(&buf2, &slog.HandlerOptions{Level: slog.LevelWarn})

	logger := slog.New(NewTeeHandler(h1, h2))
	logger.Info("info message")
	logger.Warn("warn message")

	// buf1 should have both (info level)
	if !strings.Contains(buf1.String(), "info message") {
		t.Error("buf1 should contain info message")
	}
	if !strings.Contains(buf1.String(), "warn message") {
		t.Error("buf1 should contain warn message")
	}

	// buf2 should only have warn (warn level)
	if strings.Contains(buf2.String(), "info message") {
		t.Error("buf2 should not contain info message")
	}
	if !strings.Contains(buf2.String(), "warn message") {
		t.Error("buf2 should contain warn message")
	}
}
