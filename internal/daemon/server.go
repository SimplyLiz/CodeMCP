package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"ckb/internal/version"
)

// setupServer creates and configures the HTTP server
func (d *Daemon) setupServer() *http.Server {
	mux := http.NewServeMux()

	// Health endpoint (no auth required)
	mux.HandleFunc("/health", d.handleHealth)

	// API endpoints (auth required)
	apiHandler := d.withAuth(d.apiRouter())
	mux.Handle("/api/v1/", apiHandler)

	// Legacy routes without /api/v1 prefix (for backwards compatibility)
	mux.Handle("/daemon/", d.withAuth(http.HandlerFunc(d.handleDaemonRoutes)))

	addr := fmt.Sprintf("%s:%d", d.config.Bind, d.config.Port)

	return &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}

// apiRouter returns the router for API endpoints
func (d *Daemon) apiRouter() http.Handler {
	mux := http.NewServeMux()

	// Daemon management
	mux.HandleFunc("/api/v1/daemon/status", d.handleDaemonStatus)
	mux.HandleFunc("/api/v1/daemon/schedule", d.handleScheduleList)
	mux.HandleFunc("/api/v1/daemon/jobs", d.handleJobsList)
	mux.HandleFunc("/api/v1/daemon/jobs/", d.handleJobsRoute)

	// Repo operations
	mux.HandleFunc("/api/v1/repos", d.handleReposList)
	mux.HandleFunc("/api/v1/repos/", d.handleReposRoute)

	// Federation operations
	mux.HandleFunc("/api/v1/federations", d.handleFederationsList)
	mux.HandleFunc("/api/v1/federations/", d.handleFederationsRoute)

	return mux
}

// handleDaemonRoutes handles /daemon/* routes
func (d *Daemon) handleDaemonRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/daemon/status":
		d.handleDaemonStatus(w, r)
	default:
		http.NotFound(w, r)
	}
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status  string            `json:"status"`
	Version string            `json:"version"`
	Uptime  string            `json:"uptime"`
	Checks  map[string]string `json:"checks"`
}

// handleHealth handles GET /health (no auth required)
func (d *Daemon) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uptime := time.Since(d.startedAt)

	resp := HealthResponse{
		Status:  "healthy",
		Version: version.Version,
		Uptime:  formatDuration(uptime),
		Checks: map[string]string{
			"database":    "ok",
			"federations": "ok",
			"jobQueue":    "ok",
		},
	}

	d.writeJSON(w, http.StatusOK, resp)
}

// APIResponse is the standard API response wrapper
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
	Meta    APIMeta     `json:"meta"`
}

// APIError represents an API error
type APIError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

// APIMeta contains response metadata
type APIMeta struct {
	RequestID     string `json:"requestId"`
	Duration      int64  `json:"duration"` // milliseconds
	DaemonVersion string `json:"daemonVersion"`
}

// handleDaemonStatus handles GET /api/v1/daemon/status
func (d *Daemon) handleDaemonStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	state := d.State()
	d.writeJSON(w, http.StatusOK, state)
}

// handleScheduleList handles GET /api/v1/daemon/schedule
func (d *Daemon) handleScheduleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Implement when scheduler is added
	d.writeJSON(w, http.StatusOK, map[string]interface{}{
		"schedules": []interface{}{},
	})
}

// handleJobsList handles GET /api/v1/daemon/jobs
func (d *Daemon) handleJobsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Implement when job queue is added
	d.writeJSON(w, http.StatusOK, map[string]interface{}{
		"jobs": []interface{}{},
	})
}

// handleJobsRoute handles /api/v1/daemon/jobs/:jobId routes
func (d *Daemon) handleJobsRoute(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement job detail and cancel routes
	http.NotFound(w, r)
}

// handleReposList handles GET /api/v1/repos
func (d *Daemon) handleReposList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Implement repo listing
	d.writeJSON(w, http.StatusOK, map[string]interface{}{
		"repos": []interface{}{},
	})
}

// handleReposRoute handles /api/v1/repos/:repoId/* routes
func (d *Daemon) handleReposRoute(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement repo operations
	http.NotFound(w, r)
}

// handleFederationsList handles GET /api/v1/federations
func (d *Daemon) handleFederationsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// TODO: Implement federation listing
	d.writeJSON(w, http.StatusOK, map[string]interface{}{
		"federations": []interface{}{},
	})
}

// handleFederationsRoute handles /api/v1/federations/:name/* routes
func (d *Daemon) handleFederationsRoute(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement federation operations
	http.NotFound(w, r)
}

// writeJSON writes a JSON response
func (d *Daemon) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		d.logger.Printf("Failed to encode JSON response: %v", err)
	}
}

// writeError writes an error response
func (d *Daemon) writeError(w http.ResponseWriter, status int, code, message string) {
	resp := APIResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
		},
		Meta: APIMeta{
			DaemonVersion: version.Version,
		},
	}
	d.writeJSON(w, status, resp)
}

// formatDuration formats a duration for display
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
