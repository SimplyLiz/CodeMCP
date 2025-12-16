package api

import (
	"net/http"
	"time"
)

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}

// ReadyResponse represents the readiness check response
type ReadyResponse struct {
	Status    string               `json:"status"`
	Timestamp time.Time            `json:"timestamp"`
	Backends  map[string]bool      `json:"backends"`
	Details   map[string]string    `json:"details,omitempty"`
}

// handleHealth responds to health check requests (simple liveness check)
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().UTC(),
		Version:   "0.1.0", // TODO: Get from version constant
	}

	WriteJSON(w, response, http.StatusOK)
}

// handleReady responds to readiness check requests (checks backend availability)
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Actually check backend availability
	// For now, return a placeholder response
	backends := map[string]bool{
		"scip": true,  // Placeholder
		"lsp":  true,  // Placeholder
		"git":  true,  // Placeholder
	}

	// Determine overall readiness
	ready := true
	for _, available := range backends {
		if !available {
			ready = false
			break
		}
	}

	status := "ready"
	statusCode := http.StatusOK
	if !ready {
		status = "not_ready"
		statusCode = http.StatusServiceUnavailable
	}

	response := ReadyResponse{
		Status:    status,
		Timestamp: time.Now().UTC(),
		Backends:  backends,
	}

	WriteJSON(w, response, statusCode)
}
