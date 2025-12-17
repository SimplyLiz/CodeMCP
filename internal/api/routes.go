package api

import (
	"net/http"

	"ckb/internal/version"
)

// registerRoutes registers all API routes
func (s *Server) registerRoutes() {
	// Health and readiness checks
	s.router.HandleFunc("/health", s.handleHealth)
	s.router.HandleFunc("/ready", s.handleReady)

	// System status and diagnostics
	s.router.HandleFunc("/status", s.handleStatus)
	s.router.HandleFunc("/doctor", s.handleDoctor)

	// Symbol operations
	s.router.HandleFunc("/symbol/", s.handleGetSymbol)    // GET /symbol/:id
	s.router.HandleFunc("/search", s.handleSearchSymbols) // GET /search?q=...
	s.router.HandleFunc("/refs/", s.handleFindReferences) // GET /refs/:id

	// Architecture and impact
	s.router.HandleFunc("/architecture", s.handleGetArchitecture)
	s.router.HandleFunc("/impact/", s.handleAnalyzeImpact) // GET /impact/:id

	// POST endpoints
	s.router.HandleFunc("/doctor/fix", s.handleDoctorFix)
	s.router.HandleFunc("/cache/warm", s.handleCacheWarm)
	s.router.HandleFunc("/cache/clear", s.handleCacheClear)

	// OpenAPI spec
	s.router.HandleFunc("/openapi.json", s.handleOpenAPISpec)

	// Root endpoint
	s.router.HandleFunc("/", s.handleRoot)
}

// handleRoot handles requests to the root path
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	// Only handle exact root path
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := map[string]interface{}{
		"name":    "CKB HTTP API",
		"version": version.Version,
		"endpoints": []string{
			"GET /health - Health check",
			"GET /ready - Readiness check",
			"GET /status - System status",
			"GET /doctor - Diagnostic checks",
			"GET /symbol/:id - Get symbol by ID",
			"GET /search?q=query - Search symbols",
			"GET /refs/:id - Find references",
			"GET /architecture - Architecture overview",
			"GET /impact/:id - Impact analysis",
			"POST /doctor/fix - Get fix script",
			"POST /cache/warm - Warm cache",
			"POST /cache/clear - Clear cache",
			"GET /openapi.json - OpenAPI specification",
		},
		"documentation": "/openapi.json",
	}

	WriteJSON(w, response, http.StatusOK)
}
