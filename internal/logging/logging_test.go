package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewLogger(t *testing.T) {
	t.Run("with default output", func(t *testing.T) {
		logger := NewLogger(Config{Level: InfoLevel})
		if logger == nil {
			t.Fatal("NewLogger returned nil")
		}
	})

	t.Run("with custom output", func(t *testing.T) {
		buf := &bytes.Buffer{}
		logger := NewLogger(Config{Level: InfoLevel, Output: buf})
		if logger == nil {
			t.Fatal("NewLogger returned nil")
		}
		if logger.writer != buf {
			t.Error("Logger should use provided output writer")
		}
	})
}

func TestLogLevelFiltering(t *testing.T) {
	tests := []struct {
		name       string
		configLvl  LogLevel
		logLvl     LogLevel
		shouldLog  bool
	}{
		{"debug logs debug", DebugLevel, DebugLevel, true},
		{"debug logs info", DebugLevel, InfoLevel, true},
		{"debug logs warn", DebugLevel, WarnLevel, true},
		{"debug logs error", DebugLevel, ErrorLevel, true},
		{"info skips debug", InfoLevel, DebugLevel, false},
		{"info logs info", InfoLevel, InfoLevel, true},
		{"info logs warn", InfoLevel, WarnLevel, true},
		{"info logs error", InfoLevel, ErrorLevel, true},
		{"warn skips debug", WarnLevel, DebugLevel, false},
		{"warn skips info", WarnLevel, InfoLevel, false},
		{"warn logs warn", WarnLevel, WarnLevel, true},
		{"warn logs error", WarnLevel, ErrorLevel, true},
		{"error skips debug", ErrorLevel, DebugLevel, false},
		{"error skips info", ErrorLevel, InfoLevel, false},
		{"error skips warn", ErrorLevel, WarnLevel, false},
		{"error logs error", ErrorLevel, ErrorLevel, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			logger := NewLogger(Config{Level: tt.configLvl, Output: buf})

			logger.log(tt.logLvl, "test message", nil)

			hasOutput := buf.Len() > 0
			if hasOutput != tt.shouldLog {
				t.Errorf("shouldLog = %v, but hasOutput = %v", tt.shouldLog, hasOutput)
			}
		})
	}
}

func TestDebug(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{Level: DebugLevel, Output: buf})

	logger.Debug("debug message", map[string]interface{}{"key": "value"})

	output := buf.String()
	if !strings.Contains(output, "debug") {
		t.Errorf("Debug output should contain 'debug', got: %s", output)
	}
	if !strings.Contains(output, "debug message") {
		t.Errorf("Debug output should contain message, got: %s", output)
	}
}

func TestInfo(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{Level: InfoLevel, Output: buf})

	logger.Info("info message", nil)

	output := buf.String()
	if !strings.Contains(output, "info") {
		t.Errorf("Info output should contain 'info', got: %s", output)
	}
	if !strings.Contains(output, "info message") {
		t.Errorf("Info output should contain message, got: %s", output)
	}
}

func TestWarn(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{Level: WarnLevel, Output: buf})

	logger.Warn("warning message", nil)

	output := buf.String()
	if !strings.Contains(output, "warn") {
		t.Errorf("Warn output should contain 'warn', got: %s", output)
	}
}

func TestError(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{Level: ErrorLevel, Output: buf})

	logger.Error("error message", nil)

	output := buf.String()
	if !strings.Contains(output, "error") {
		t.Errorf("Error output should contain 'error', got: %s", output)
	}
}

func TestJSONFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{
		Level:  InfoLevel,
		Format: JSONFormat,
		Output: buf,
	})

	logger.Info("test message", map[string]interface{}{
		"count": 42,
		"name":  "test",
	})

	output := buf.String()

	// Verify it's valid JSON
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &entry); err != nil {
		t.Fatalf("Output is not valid JSON: %v\nOutput: %s", err, output)
	}

	// Check required fields
	if entry["level"] != "info" {
		t.Errorf("level = %v, want 'info'", entry["level"])
	}
	if entry["message"] != "test message" {
		t.Errorf("message = %v, want 'test message'", entry["message"])
	}
	if entry["timestamp"] == nil {
		t.Error("timestamp should be present")
	}

	// Check fields
	fields, ok := entry["fields"].(map[string]interface{})
	if !ok {
		t.Fatal("fields should be a map")
	}
	if fields["count"] != float64(42) { // JSON numbers are float64
		t.Errorf("fields.count = %v, want 42", fields["count"])
	}
	if fields["name"] != "test" {
		t.Errorf("fields.name = %v, want 'test'", fields["name"])
	}
}

func TestHumanFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{
		Level:  InfoLevel,
		Format: HumanFormat,
		Output: buf,
	})

	logger.Info("human readable", map[string]interface{}{
		"key": "value",
	})

	output := buf.String()

	// Check for expected parts
	if !strings.Contains(output, "[info]") {
		t.Errorf("Output should contain '[info]', got: %s", output)
	}
	if !strings.Contains(output, "human readable") {
		t.Errorf("Output should contain message, got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("Output should contain field, got: %s", output)
	}
}

func TestHumanFormatNoFields(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{
		Level:  InfoLevel,
		Format: HumanFormat,
		Output: buf,
	})

	logger.Info("no fields", nil)

	output := buf.String()
	if strings.Contains(output, "|") {
		t.Errorf("Output without fields should not contain '|', got: %s", output)
	}
}

func TestLogLevelConstants(t *testing.T) {
	levels := []LogLevel{DebugLevel, InfoLevel, WarnLevel, ErrorLevel}

	for _, level := range levels {
		if string(level) == "" {
			t.Errorf("LogLevel %v should not be empty", level)
		}
	}

	// Verify priority order
	if logLevelPriority[DebugLevel] >= logLevelPriority[InfoLevel] {
		t.Error("Debug should have lower priority than Info")
	}
	if logLevelPriority[InfoLevel] >= logLevelPriority[WarnLevel] {
		t.Error("Info should have lower priority than Warn")
	}
	if logLevelPriority[WarnLevel] >= logLevelPriority[ErrorLevel] {
		t.Error("Warn should have lower priority than Error")
	}
}

func TestFormatConstants(t *testing.T) {
	if string(JSONFormat) == "" {
		t.Error("JSONFormat should not be empty")
	}
	if string(HumanFormat) == "" {
		t.Error("HumanFormat should not be empty")
	}
	if JSONFormat == HumanFormat {
		t.Error("JSONFormat and HumanFormat should be different")
	}
}

func TestConfigStructure(t *testing.T) {
	buf := &bytes.Buffer{}
	config := Config{
		Format: JSONFormat,
		Level:  WarnLevel,
		Output: buf,
	}

	if config.Format != JSONFormat {
		t.Errorf("Format = %v, want JSONFormat", config.Format)
	}
	if config.Level != WarnLevel {
		t.Errorf("Level = %v, want WarnLevel", config.Level)
	}
	if config.Output != buf {
		t.Error("Output should match provided writer")
	}
}

func TestShouldLog(t *testing.T) {
	logger := NewLogger(Config{Level: WarnLevel})

	if logger.shouldLog(DebugLevel) {
		t.Error("WarnLevel logger should not log DebugLevel")
	}
	if logger.shouldLog(InfoLevel) {
		t.Error("WarnLevel logger should not log InfoLevel")
	}
	if !logger.shouldLog(WarnLevel) {
		t.Error("WarnLevel logger should log WarnLevel")
	}
	if !logger.shouldLog(ErrorLevel) {
		t.Error("WarnLevel logger should log ErrorLevel")
	}
}

func TestMultipleFieldsHumanFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := NewLogger(Config{
		Level:  InfoLevel,
		Format: HumanFormat,
		Output: buf,
	})

	logger.Info("test", map[string]interface{}{
		"a": 1,
		"b": 2,
		"c": 3,
	})

	output := buf.String()

	// Should have commas between fields
	if !strings.Contains(output, ", ") {
		t.Errorf("Multiple fields should be comma-separated, got: %s", output)
	}
}
