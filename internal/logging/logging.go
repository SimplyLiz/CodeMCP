package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel string

const (
	// DebugLevel for debug messages
	DebugLevel LogLevel = "debug"
	// InfoLevel for informational messages
	InfoLevel LogLevel = "info"
	// WarnLevel for warning messages
	WarnLevel LogLevel = "warn"
	// ErrorLevel for error messages
	ErrorLevel LogLevel = "error"
)

var logLevelPriority = map[LogLevel]int{
	DebugLevel: 0,
	InfoLevel:  1,
	WarnLevel:  2,
	ErrorLevel: 3,
}

// Format represents the output format for logs
type Format string

const (
	// JSONFormat outputs logs as JSON
	JSONFormat Format = "json"
	// HumanFormat outputs logs in human-readable format
	HumanFormat Format = "human"
)

// Config holds logger configuration
type Config struct {
	Format Format
	Level  LogLevel
	Output io.Writer // Optional, defaults to stdout
}

// Logger provides structured logging
type Logger struct {
	config Config
	writer io.Writer
}

// NewLogger creates a new logger with the given configuration
func NewLogger(config Config) *Logger {
	writer := config.Output
	if writer == nil {
		writer = os.Stdout
	}

	return &Logger{
		config: config,
		writer: writer,
	}
}

// logEntry represents a single log entry
type logEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

func (l *Logger) shouldLog(level LogLevel) bool {
	configPriority := logLevelPriority[l.config.Level]
	messagePriority := logLevelPriority[level]
	return messagePriority >= configPriority
}

func (l *Logger) log(level LogLevel, message string, fields map[string]interface{}) {
	if !l.shouldLog(level) {
		return
	}

	entry := logEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     string(level),
		Message:   message,
		Fields:    fields,
	}

	if l.config.Format == JSONFormat {
		l.logJSON(entry)
	} else {
		l.logHuman(entry)
	}
}

func (l *Logger) logJSON(entry logEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to marshal log entry: %v\n", err)
		return
	}
	_, _ = fmt.Fprintln(l.writer, string(data))
}

func (l *Logger) logHuman(entry logEntry) {
	levelStr := fmt.Sprintf("[%s]", entry.Level)
	_, _ = fmt.Fprintf(l.writer, "%s %s %s", entry.Timestamp, levelStr, entry.Message)

	if len(entry.Fields) > 0 {
		_, _ = fmt.Fprintf(l.writer, " | ")
		first := true
		for k, v := range entry.Fields {
			if !first {
				_, _ = fmt.Fprintf(l.writer, ", ")
			}
			_, _ = fmt.Fprintf(l.writer, "%s=%v", k, v)
			first = false
		}
	}
	_, _ = fmt.Fprintln(l.writer)
}

// Debug logs a debug message
func (l *Logger) Debug(message string, fields map[string]interface{}) {
	l.log(DebugLevel, message, fields)
}

// Info logs an info message
func (l *Logger) Info(message string, fields map[string]interface{}) {
	l.log(InfoLevel, message, fields)
}

// Warn logs a warning message
func (l *Logger) Warn(message string, fields map[string]interface{}) {
	l.log(WarnLevel, message, fields)
}

// Error logs an error message
func (l *Logger) Error(message string, fields map[string]interface{}) {
	l.log(ErrorLevel, message, fields)
}
