// Package pkg provides HTTP handling and business logic.
package pkg

// Server coordinates handlers and services.
type Server struct {
	handler *Handler
}

// NewServer creates a new Server with default configuration.
func NewServer() *Server {
	svc := NewDefaultService()
	cached := NewCachingService(svc)
	handler := NewHandler(cached)

	return &Server{
		handler: handler,
	}
}

// RunServer starts the server and processes a sample request.
// This is the main entry point called from main().
func (s *Server) RunServer() {
	// Process a sample request to demonstrate the call chain
	_ = s.handler.Handle("hello world")
}

// GetHandler returns the server's handler.
func (s *Server) GetHandler() *Handler {
	return s.handler
}
