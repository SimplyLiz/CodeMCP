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
	s.router.HandleFunc("/architecture/refresh", s.handleRefreshArchitecture) // POST with async support
	s.router.HandleFunc("/impact/", s.handleAnalyzeImpact)                    // GET /impact/:id

	// v6.0 Architectural Memory endpoints
	s.router.HandleFunc("/hotspots", s.handleGetHotspots)
	s.router.HandleFunc("/ownership", s.handleGetOwnership)
	s.router.HandleFunc("/decisions", s.handleDecisions) // GET list, POST create
	s.router.HandleFunc("/modules/", s.handleModules)    // GET /modules/:id/overview, /modules/:id/responsibilities
	s.router.HandleFunc("/callgraph/", s.handleGetCallGraph)
	s.router.HandleFunc("/explain/symbol/", s.handleExplainSymbol)
	s.router.HandleFunc("/justify/", s.handleJustifySymbol)

	// v6.1 Job management endpoints
	s.router.HandleFunc("/jobs", s.handleListJobs)   // GET
	s.router.HandleFunc("/jobs/", s.handleJobRoutes) // GET /:id, POST /:id/cancel

	// v6.1 CI/CD endpoints
	s.router.HandleFunc("/pr/summary", s.handleSummarizePR)         // GET/POST
	s.router.HandleFunc("/ownership/drift", s.handleOwnershipDrift) // GET

	// v6.2 Federation endpoints
	s.router.HandleFunc("/federations", s.handleListFederations)   // GET
	s.router.HandleFunc("/federations/", s.handleFederationRoutes) // /federations/:name/*

	// v6.4 Telemetry endpoints
	s.router.HandleFunc("/telemetry/", s.handleTelemetryRoutes) // /telemetry/status, /telemetry/usage/:id, /telemetry/dead-code
	s.router.HandleFunc("/telemetry", s.handleTelemetryStatus)  // GET /telemetry (alias for /telemetry/status)

	// Delta ingestion endpoints (incremental indexing)
	s.router.HandleFunc("/delta", s.handleDeltaRoutes)
	s.router.HandleFunc("/delta/", s.handleDeltaRoutes)

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
			"POST /architecture/refresh - Refresh architectural model (async support)",
			"GET /impact/:id - Impact analysis",
			"GET /hotspots - Get code hotspots",
			"GET /ownership?path=... - Get file/path ownership",
			"GET /decisions - List architectural decisions",
			"POST /decisions - Record architectural decision",
			"GET /modules/:id/overview - Module overview",
			"GET /modules/:id/responsibilities - Module responsibilities",
			"GET /callgraph/:id - Get call graph",
			"GET /explain/symbol/:id - Explain symbol",
			"GET /justify/:id - Justify symbol (keep/investigate/remove)",
			"GET /jobs - List background jobs",
			"GET /jobs/:id - Get job status",
			"POST /jobs/:id/cancel - Cancel job",
			"GET/POST /pr/summary - Summarize PR changes with risk assessment",
			"GET /ownership/drift - Detect ownership drift between CODEOWNERS and git-blame",
			"GET /federations - List all federations",
			"GET /federations/:name/status - Federation status",
			"GET /federations/:name/repos - List repos in federation",
			"GET /federations/:name/modules?q=... - Search modules across federation",
			"GET /federations/:name/ownership?path=... - Search ownership across federation",
			"GET /federations/:name/hotspots - Get hotspots across federation",
			"GET /federations/:name/decisions?q=... - Search decisions across federation",
			"POST /federations/:name/sync - Sync federation index",
			"GET /telemetry/status - Telemetry system status and coverage",
			"GET /telemetry/usage/:id - Observed usage for a symbol",
			"GET /telemetry/dead-code - Find dead code candidates",
			"GET /delta - Delta ingestion info",
			"POST /delta/ingest - Ingest delta artifact for incremental indexing",
			"POST /delta/validate - Validate delta artifact without ingesting",
			"POST /doctor/fix - Get fix script",
			"POST /cache/warm - Warm cache",
			"POST /cache/clear - Clear cache",
			"GET /openapi.json - OpenAPI specification",
		},
		"documentation": "/openapi.json",
	}

	WriteJSON(w, response, http.StatusOK)
}
