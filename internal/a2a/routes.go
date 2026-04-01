package a2a

import "net/http"

// registerRoutes registers all A2A protocol routes.
func (s *Server) registerRoutes() {
	// Agent discovery (no auth)
	s.router.HandleFunc("GET /.well-known/agent-card.json", s.handleAgentCard)
	s.router.HandleFunc("GET /extendedAgentCard", s.handleExtendedAgentCard)

	// JSON-RPC endpoint (handles all A2A methods)
	s.router.HandleFunc("POST /", s.handleJSONRPC)

	// HTTP+JSON binding
	s.router.HandleFunc("POST /message:send", s.handleHTTPMessageSend)
	s.router.HandleFunc("POST /message:stream", s.handleHTTPMessageStream)
	s.router.HandleFunc("GET /tasks/{id}", s.handleHTTPGetTask)
	s.router.HandleFunc("GET /tasks", s.handleHTTPListTasks)
	s.router.HandleFunc("POST /tasks/{id}:cancel", s.handleHTTPCancelTask)
	s.router.HandleFunc("POST /tasks/{id}:subscribe", s.handleHTTPSubscribeTask)

	// Push notification config CRUD
	s.router.HandleFunc("POST /tasks/{id}/pushNotificationConfigs", s.handleHTTPCreatePushConfig)
	s.router.HandleFunc("GET /tasks/{id}/pushNotificationConfigs/{configId}", s.handleHTTPGetPushConfig)
	s.router.HandleFunc("GET /tasks/{id}/pushNotificationConfigs", s.handleHTTPListPushConfigs)
	s.router.HandleFunc("DELETE /tasks/{id}/pushNotificationConfigs/{configId}", s.handleHTTPDeletePushConfig)

	// Health
	s.router.HandleFunc("GET /health", s.handleHealth)
}

// handleHealth returns a simple health check response.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "healthy",
		"protocol": "a2a",
		"version":  ProtocolVersion,
	})
}
