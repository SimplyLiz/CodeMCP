package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"ckb/internal/logging"
	"ckb/internal/query"
)

// Server represents the HTTP API server
type Server struct {
	router *http.ServeMux
	server *http.Server
	addr   string
	logger *logging.Logger
	engine *query.Engine
}

// NewServer creates a new HTTP server instance
func NewServer(addr string, engine *query.Engine, logger *logging.Logger) *Server {
	s := &Server{
		addr:   addr,
		logger: logger,
		engine: engine,
		router: http.NewServeMux(),
	}

	// Register routes
	s.registerRoutes()

	// Create HTTP server with configured router and middleware
	handler := s.applyMiddleware(s.router)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.logger.Info("Starting HTTP server", map[string]interface{}{
		"addr": s.addr,
	})

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down HTTP server", nil)

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	s.logger.Info("Server shut down successfully", nil)
	return nil
}

// ServeHTTP implements http.Handler for testing
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.server.Handler.ServeHTTP(w, r)
}

// applyMiddleware wraps the handler with middleware in the correct order
func (s *Server) applyMiddleware(handler http.Handler) http.Handler {
	// Apply middleware in reverse order (last one wraps first)
	handler = RecoveryMiddleware(s.logger)(handler)
	handler = LoggingMiddleware(s.logger)(handler)
	handler = RequestIDMiddleware()(handler)
	handler = CORSMiddleware()(handler)
	return handler
}
