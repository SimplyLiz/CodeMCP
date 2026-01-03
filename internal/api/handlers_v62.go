package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"ckb/internal/federation"
)

// v6.2 Federation Handlers

// handleListFederations handles GET /federations
func (s *Server) handleListFederations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	names, err := federation.List()
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}

	WriteJSON(w, map[string]interface{}{
		"federations": names,
		"count":       len(names),
	}, http.StatusOK)
}

// handleFederationRoutes handles /federations/:name/* routes
func (s *Server) handleFederationRoutes(w http.ResponseWriter, r *http.Request) {
	// Extract federation name and action from path: /federations/{name}/modules, etc.
	path := strings.TrimPrefix(r.URL.Path, "/federations/")
	parts := strings.SplitN(path, "/", 2)
	fedName := parts[0]

	if fedName == "" {
		http.Error(w, "Missing federation name", http.StatusBadRequest)
		return
	}

	// Check if federation exists
	exists, err := federation.Exists(fedName)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Federation not found", http.StatusNotFound)
		return
	}

	// Determine action
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	// Open federation
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}
	defer func() { _ = fed.Close() }()

	switch action {
	case "repos":
		s.handleFederationRepos(w, r, fed)
	case "modules":
		s.handleFederationModules(w, r, fed)
	case "ownership":
		s.handleFederationOwnership(w, r, fed)
	case "hotspots":
		s.handleFederationHotspots(w, r, fed)
	case "decisions":
		s.handleFederationDecisions(w, r, fed)
	case "sync":
		s.handleFederationSync(w, r, fed)
	case "status":
		s.handleFederationStatus(w, r, fed)
	case "":
		// Return basic info about the federation
		s.handleFederationStatus(w, r, fed)
	default:
		http.Error(w, "Unknown federation action: "+action, http.StatusNotFound)
	}
}

// handleFederationRepos handles GET /federations/:name/repos
func (s *Server) handleFederationRepos(w http.ResponseWriter, r *http.Request, fed *federation.Federation) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	repos := fed.ListRepos()

	// Optionally include compatibility status
	includeCompat := QueryParamBool(r, "includeCompatibility", false)
	if includeCompat {
		checks, err := federation.CheckAllReposCompatibility(fed)
		if err == nil {
			WriteJSON(w, map[string]interface{}{
				"repos":         repos,
				"count":         len(repos),
				"compatibility": checks,
			}, http.StatusOK)
			return
		}
	}

	WriteJSON(w, map[string]interface{}{
		"repos": repos,
		"count": len(repos),
	}, http.StatusOK)
}

// handleFederationModules handles GET /federations/:name/modules
func (s *Server) handleFederationModules(w http.ResponseWriter, r *http.Request, fed *federation.Federation) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	opts := federation.SearchModulesOptions{
		Query: r.URL.Query().Get("q"),
		Limit: QueryParamInt(r, "limit", 50),
	}

	// Parse repo IDs filter
	if repoIDs := r.URL.Query().Get("repos"); repoIDs != "" {
		opts.RepoIDs = strings.Split(repoIDs, ",")
	}

	// Parse tags filter
	if tags := r.URL.Query().Get("tags"); tags != "" {
		opts.Tags = strings.Split(tags, ",")
	}

	result, err := fed.SearchModules(opts)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}

	WriteJSON(w, result, http.StatusOK)
}

// handleFederationOwnership handles GET /federations/:name/ownership
func (s *Server) handleFederationOwnership(w http.ResponseWriter, r *http.Request, fed *federation.Federation) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	opts := federation.SearchOwnershipOptions{
		PathGlob: r.URL.Query().Get("path"),
		Limit:    QueryParamInt(r, "limit", 50),
	}

	// Parse repo IDs filter
	if repoIDs := r.URL.Query().Get("repos"); repoIDs != "" {
		opts.RepoIDs = strings.Split(repoIDs, ",")
	}

	result, err := fed.SearchOwnership(opts)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}

	WriteJSON(w, result, http.StatusOK)
}

// handleFederationHotspots handles GET /federations/:name/hotspots
func (s *Server) handleFederationHotspots(w http.ResponseWriter, r *http.Request, fed *federation.Federation) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	opts := federation.GetHotspotsOptions{
		Top:      QueryParamInt(r, "top", 20),
		MinScore: QueryParamFloat(r, "minScore", 0.3),
	}

	// Parse repo IDs filter
	if repoIDs := r.URL.Query().Get("repos"); repoIDs != "" {
		opts.RepoIDs = strings.Split(repoIDs, ",")
	}

	result, err := fed.GetHotspots(opts)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}

	WriteJSON(w, result, http.StatusOK)
}

// handleFederationDecisions handles GET /federations/:name/decisions
func (s *Server) handleFederationDecisions(w http.ResponseWriter, r *http.Request, fed *federation.Federation) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	opts := federation.SearchDecisionsOptions{
		Query:          r.URL.Query().Get("q"),
		AffectedModule: r.URL.Query().Get("module"),
		Limit:          QueryParamInt(r, "limit", 50),
	}

	// Parse repo IDs filter
	if repoIDs := r.URL.Query().Get("repos"); repoIDs != "" {
		opts.RepoIDs = strings.Split(repoIDs, ",")
	}

	// Parse status filter
	if status := r.URL.Query().Get("status"); status != "" {
		opts.Status = strings.Split(status, ",")
	}

	result, err := fed.SearchDecisions(opts)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}

	WriteJSON(w, result, http.StatusOK)
}

// handleFederationSync handles POST /federations/:name/sync
func (s *Server) handleFederationSync(w http.ResponseWriter, r *http.Request, fed *federation.Federation) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req struct {
		Force   bool     `json:"force"`
		DryRun  bool     `json:"dryRun"`
		RepoIDs []string `json:"repoIds"`
	}
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			WriteError(w, err, http.StatusBadRequest)
			return
		}
	}

	opts := federation.SyncOptions{
		Force:   req.Force,
		DryRun:  req.DryRun,
		RepoIDs: req.RepoIDs,
	}

	results, err := fed.Sync(opts)
	if err != nil {
		WriteError(w, err, http.StatusInternalServerError)
		return
	}

	// Compute summary
	success := 0
	failed := 0
	skipped := 0
	for _, r := range results {
		switch r.Status {
		case "success":
			success++
		case "failed":
			failed++
		case "skipped":
			skipped++
		}
	}

	WriteJSON(w, map[string]interface{}{
		"results": results,
		"summary": map[string]int{
			"success": success,
			"failed":  failed,
			"skipped": skipped,
			"total":   len(results),
		},
	}, http.StatusOK)
}

// handleFederationStatus handles GET /federations/:name/status
func (s *Server) handleFederationStatus(w http.ResponseWriter, r *http.Request, fed *federation.Federation) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	config := fed.Config()
	repos := fed.ListRepos()

	// Get indexed repos from index
	indexedRepos, err := fed.Index().ListRepos()
	if err != nil {
		indexedRepos = nil
	}

	// Check compatibility
	var compatible, incompatible int
	checks, err := federation.CheckAllReposCompatibility(fed)
	if err == nil {
		for _, c := range checks {
			if c.Status == federation.CompatibilityOK {
				compatible++
			} else {
				incompatible++
			}
		}
	}

	status := map[string]interface{}{
		"name":        config.Name,
		"description": config.Description,
		"createdAt":   config.CreatedAt,
		"updatedAt":   config.UpdatedAt,
		"repoCount":   len(repos),
		"repos":       repos,
		"compatibility": map[string]int{
			"compatible":   compatible,
			"incompatible": incompatible,
		},
	}

	if len(indexedRepos) > 0 {
		status["indexedRepos"] = indexedRepos
	}

	WriteJSON(w, status, http.StatusOK)
}
