package slogutil

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"ckb/internal/config"
)

func TestNewLokiHandler(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.RemoteLogConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
		},
		{
			name:    "empty endpoint",
			cfg:     &config.RemoteLogConfig{},
			wantErr: true,
		},
		{
			name: "valid config",
			cfg: &config.RemoteLogConfig{
				Type:     "loki",
				Endpoint: "http://localhost:3100",
			},
			wantErr: false,
		},
		{
			name: "with labels",
			cfg: &config.RemoteLogConfig{
				Type:     "loki",
				Endpoint: "http://localhost:3100",
				Labels: map[string]string{
					"env":  "test",
					"team": "platform",
				},
			},
			wantErr: false,
		},
		{
			name: "with custom batch settings",
			cfg: &config.RemoteLogConfig{
				Type:          "loki",
				Endpoint:      "http://localhost:3100",
				BatchSize:     50,
				FlushInterval: "10s",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewLokiHandler(tt.cfg, nil, slog.LevelInfo)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLokiHandler() error = %v, wantErr %v", err, tt.wantErr)
			}
			if handler != nil {
				// Stop immediately since we started no goroutines
				_ = handler.Stop()
			}
		})
	}
}

func TestLokiHandler_Enabled(t *testing.T) {
	cfg := &config.RemoteLogConfig{
		Type:     "loki",
		Endpoint: "http://localhost:3100",
	}

	handler, err := NewLokiHandler(cfg, nil, slog.LevelWarn)
	if err != nil {
		t.Fatalf("NewLokiHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	tests := []struct {
		level   slog.Level
		enabled bool
	}{
		{slog.LevelDebug, false},
		{slog.LevelInfo, false},
		{slog.LevelWarn, true},
		{slog.LevelError, true},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			if got := handler.Enabled(context.Background(), tt.level); got != tt.enabled {
				t.Errorf("Enabled(%v) = %v, want %v", tt.level, got, tt.enabled)
			}
		})
	}
}

func TestLokiHandler_Handle(t *testing.T) {
	cfg := &config.RemoteLogConfig{
		Type:      "loki",
		Endpoint:  "http://localhost:3100",
		BatchSize: 10, // Small batch for testing
	}

	handler, err := NewLokiHandler(cfg, map[string]string{
		"app": "test",
	}, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewLokiHandler failed: %v", err)
	}

	// Handle a log record
	record := slog.Record{
		Time:    time.Now(),
		Level:   slog.LevelInfo,
		Message: "test message",
	}
	record.AddAttrs(slog.String("key", "value"))

	err = handler.Handle(context.Background(), record)
	if err != nil {
		t.Errorf("Handle() error = %v", err)
	}

	// Check buffer has the entry
	handler.mu.Lock()
	bufLen := len(handler.buffer)
	handler.mu.Unlock()

	if bufLen != 1 {
		t.Errorf("buffer length = %d, want 1", bufLen)
	}

	_ = handler.Stop()
}

func TestLokiHandler_BatchFlush(t *testing.T) {
	var mu sync.Mutex
	var received []lokiPushRequest

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req lokiPushRequest
		if err := json.Unmarshal(body, &req); err == nil {
			mu.Lock()
			received = append(received, req)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := &config.RemoteLogConfig{
		Type:      "loki",
		Endpoint:  server.URL,
		BatchSize: 3, // Flush after 3 logs
	}

	handler, err := NewLokiHandler(cfg, map[string]string{
		"app": "test",
	}, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewLokiHandler failed: %v", err)
	}
	handler.Start()

	// Send 4 logs (should trigger one flush at 3)
	for i := 0; i < 4; i++ {
		record := slog.Record{
			Time:    time.Now(),
			Level:   slog.LevelInfo,
			Message: "test message",
		}
		_ = handler.Handle(context.Background(), record)
	}

	// Stop to flush remaining
	_ = handler.Stop()

	// Wait a bit for async sends
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	numReceived := len(received)
	mu.Unlock()

	// Should have received at least one request (the batch flush + final flush)
	if numReceived == 0 {
		t.Error("expected to receive at least one push request")
	}
}

func TestLokiHandler_Labels(t *testing.T) {
	var receivedLabels map[string]string
	var mu sync.Mutex

	// Create test server to capture labels
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req lokiPushRequest
		if err := json.Unmarshal(body, &req); err == nil && len(req.Streams) > 0 {
			mu.Lock()
			receivedLabels = req.Streams[0].Stream
			mu.Unlock()
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := &config.RemoteLogConfig{
		Type:     "loki",
		Endpoint: server.URL,
		Labels: map[string]string{
			"env": "prod",
		},
		BatchSize: 1, // Immediate flush
	}

	handler, err := NewLokiHandler(cfg, map[string]string{
		"app":       "ckb",
		"subsystem": "mcp",
	}, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewLokiHandler failed: %v", err)
	}
	handler.Start()

	// Send a log
	record := slog.Record{
		Time:    time.Now(),
		Level:   slog.LevelInfo,
		Message: "test",
	}
	_ = handler.Handle(context.Background(), record)
	_ = handler.Stop()

	// Wait for async send
	time.Sleep(100 * time.Millisecond)

	// Check labels with proper synchronization
	mu.Lock()
	labels := receivedLabels
	mu.Unlock()

	if labels == nil {
		t.Fatal("no labels received")
	}
	if labels["app"] != "ckb" {
		t.Errorf("app label = %q, want %q", labels["app"], "ckb")
	}
	if labels["subsystem"] != "mcp" {
		t.Errorf("subsystem label = %q, want %q", labels["subsystem"], "mcp")
	}
	if labels["env"] != "prod" {
		t.Errorf("env label = %q, want %q", labels["env"], "prod")
	}
	if labels["level"] != "INFO" {
		t.Errorf("level label = %q, want %q", labels["level"], "INFO")
	}
}

func TestLokiHandler_WithAttrs(t *testing.T) {
	cfg := &config.RemoteLogConfig{
		Type:     "loki",
		Endpoint: "http://localhost:3100",
	}

	handler, err := NewLokiHandler(cfg, nil, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewLokiHandler failed: %v", err)
	}

	// WithAttrs should return a new handler with attrs
	newHandler := handler.WithAttrs([]slog.Attr{
		slog.String("component", "test"),
	})

	if newHandler == handler {
		t.Error("WithAttrs should return a new handler")
	}

	// Should return a wrapper type that implements slog.Handler
	wrapper, ok := newHandler.(*lokiHandlerWithContext)
	if !ok {
		t.Fatal("WithAttrs should return *lokiHandlerWithContext")
	}

	if len(wrapper.attrs) != 1 {
		t.Errorf("attrs length = %d, want 1", len(wrapper.attrs))
	}

	_ = handler.Stop()
}

func TestLokiHandler_WithGroup(t *testing.T) {
	cfg := &config.RemoteLogConfig{
		Type:     "loki",
		Endpoint: "http://localhost:3100",
	}

	handler, err := NewLokiHandler(cfg, nil, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewLokiHandler failed: %v", err)
	}

	// WithGroup with empty name should return same handler
	sameHandler := handler.WithGroup("")
	if sameHandler != handler {
		t.Error("WithGroup(\"\") should return same handler")
	}

	// WithGroup with non-empty name should return new handler
	newHandler := handler.WithGroup("mygroup")
	if newHandler == handler {
		t.Error("WithGroup should return a new handler")
	}

	_ = handler.Stop()
}

func TestLokiHandler_FormatRecord(t *testing.T) {
	cfg := &config.RemoteLogConfig{
		Type:     "loki",
		Endpoint: "http://localhost:3100",
	}

	handler, err := NewLokiHandler(cfg, nil, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewLokiHandler failed: %v", err)
	}
	defer func() { _ = handler.Stop() }()

	record := slog.Record{
		Time:    time.Now(),
		Level:   slog.LevelInfo,
		Message: "test message",
	}
	record.AddAttrs(
		slog.String("str", "value"),
		slog.Int("num", 42),
		slog.Bool("flag", true),
		slog.Duration("dur", 5*time.Second),
	)

	line := handler.formatRecord(record)

	// Check basic format
	if line == "" {
		t.Error("formatRecord returned empty string")
	}

	// Should contain level, message, and attrs
	expectedParts := []string{
		"level=INFO",
		`msg="test message"`,
		`str="value"`,
		"num=42",
		"flag=true",
		"dur=5s",
	}

	for _, part := range expectedParts {
		if !contains(line, part) {
			t.Errorf("line %q should contain %q", line, part)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
