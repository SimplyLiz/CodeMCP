package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/mcp"
	"github.com/SimplyLiz/CodeMCP/internal/query"
)

// ServerConfig holds configuration for the A2A server.
type ServerConfig struct {
	Addr      string
	AuthToken string
	CORSAllow []string
	CKBDir    string // path to .ckb directory for task DB
	BaseURL   string // public base URL for agent card
}

// Server implements the A2A protocol over HTTP.
type Server struct {
	router    *http.ServeMux
	server    *http.Server
	logger    *slog.Logger
	engine    *query.Engine
	mcpServer *mcp.MCPServer
	config    ServerConfig
	store     *TaskStore
	skills    *SkillRegistry

	// SSE subscriptions: taskID -> list of subscriber channels
	subs   map[string][]chan StreamResponse
	subsMu sync.RWMutex
}

// NewServer creates an A2A protocol server.
func NewServer(engine *query.Engine, mcpServer *mcp.MCPServer, logger *slog.Logger, config ServerConfig) (*Server, error) {
	store, err := OpenTaskStore(config.CKBDir, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open task store: %w", err)
	}

	s := &Server{
		router:    http.NewServeMux(),
		logger:    logger,
		engine:    engine,
		mcpServer: mcpServer,
		config:    config,
		store:     store,
		skills:    NewSkillRegistry(mcpServer),
		subs:      make(map[string][]chan StreamResponse),
	}

	s.registerRoutes()

	handler := s.applyMiddleware(s.router)
	s.server = &http.Server{
		Addr:         config.Addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // Disabled for SSE streaming
		IdleTimeout:  120 * time.Second,
	}

	return s, nil
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	s.logger.Info("Starting A2A server", "addr", s.config.Addr)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("A2A server error: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down A2A server")

	// Close all SSE subscriptions
	s.subsMu.Lock()
	for taskID, subs := range s.subs {
		for _, ch := range subs {
			close(ch)
		}
		delete(s.subs, taskID)
	}
	s.subsMu.Unlock()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("A2A server shutdown error: %w", err)
	}
	if err := s.store.Close(); err != nil {
		s.logger.Warn("Failed to close task store", "error", err.Error())
	}
	return nil
}

// subscribe registers a channel to receive events for a task.
func (s *Server) subscribe(taskID string) chan StreamResponse {
	ch := make(chan StreamResponse, 32)
	s.subsMu.Lock()
	s.subs[taskID] = append(s.subs[taskID], ch)
	s.subsMu.Unlock()
	return ch
}

// unsubscribe removes a channel from a task's subscribers.
func (s *Server) unsubscribe(taskID string, ch chan StreamResponse) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	subs := s.subs[taskID]
	for i, sub := range subs {
		if sub == ch {
			s.subs[taskID] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	if len(s.subs[taskID]) == 0 {
		delete(s.subs, taskID)
	}
}

// notify fans out an event to all subscribers of a task.
// Copies the subscriber slice under the lock to avoid send-on-closed-channel
// if a concurrent unsubscribe closes a channel during iteration.
func (s *Server) notify(taskID string, event StreamResponse) {
	s.subsMu.RLock()
	subs := make([]chan StreamResponse, len(s.subs[taskID]))
	copy(subs, s.subs[taskID])
	s.subsMu.RUnlock()

	for _, ch := range subs {
		// Recover from send-on-closed-channel if Shutdown closes a channel
		// between our copy and the send.
		func() {
			defer func() { _ = recover() }()
			select {
			case ch <- event:
			default:
			}
		}()
	}
}

// --- Response helpers ---

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeA2AError(w http.ResponseWriter, err *A2AError) {
	status := err.HTTPStatus
	if status == 0 {
		if s, ok := httpStatusForCode[err.Code]; ok {
			status = s
		} else {
			status = http.StatusInternalServerError
		}
	}
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    err.Code,
			"message": err.Message,
		},
	})
}
