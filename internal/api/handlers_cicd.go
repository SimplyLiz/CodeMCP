// Package api provides CI/CD-focused HTTP handlers.
package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"ckb/internal/audit"
	"ckb/internal/complexity"
	"ckb/internal/coupling"
	"ckb/internal/logging"
	"ckb/internal/query"
)

// handleFileComplexity handles GET /complexity?path=...
// Returns cyclomatic and cognitive complexity metrics for a file.
func (s *Server) handleFileComplexity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if complexity analysis is available (requires CGO)
	if !complexity.IsAvailable() {
		WriteJSON(w, map[string]interface{}{
			"error":   "Complexity analysis not available",
			"reason":  "Built without CGO support (tree-sitter requires CGO)",
			"suggest": "Rebuild with CGO_ENABLED=1",
		}, http.StatusServiceUnavailable)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "Missing required parameter: path", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Resolve path relative to repo root
	repoRoot, err := os.Getwd()
	if err != nil {
		InternalError(w, "Failed to get working directory", err)
		return
	}

	fullPath := path
	if !filepath.IsAbs(path) {
		fullPath = filepath.Join(repoRoot, path)
	}

	// Analyze file
	analyzer := complexity.NewAnalyzer()
	result, err := analyzer.AnalyzeFile(ctx, fullPath)
	if err != nil {
		InternalError(w, "Failed to analyze file", err)
		return
	}

	// Check for analysis errors
	if result.Error != "" {
		WriteJSON(w, map[string]interface{}{
			"path":  path,
			"error": result.Error,
		}, http.StatusOK)
		return
	}

	WriteJSON(w, result, http.StatusOK)
}

// CouplingCheckRequest represents a POST /coupling/check request body.
type CouplingCheckRequest struct {
	Files string `json:"files"` // Comma-separated list of changed files
}

// CouplingCheckResponse represents the response for missing coupled files.
type CouplingCheckResponse struct {
	ChangedFiles   []string             `json:"changedFiles"`
	MissingCoupled []MissingCoupledFile `json:"missingCoupled"`
	Recommendation string               `json:"recommendation,omitempty"`
}

// MissingCoupledFile represents a file that usually changes together but wasn't included.
type MissingCoupledFile struct {
	File          string  `json:"file"`
	CoupledTo     string  `json:"coupledTo"`
	CouplingScore float64 `json:"couplingScore"`
	CochangeCount int     `json:"cochangeCount"`
}

// handleCouplingRoutes handles /coupling endpoints
func (s *Server) handleCouplingRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleCouplingAnalyze(w, r)
	case http.MethodPost:
		s.handleCouplingCheck(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleCouplingAnalyze handles GET /coupling?target=...
func (s *Server) handleCouplingAnalyze(w http.ResponseWriter, r *http.Request) {
	target := r.URL.Query().Get("target")
	if target == "" {
		http.Error(w, "Missing required parameter: target", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	repoRoot, err := os.Getwd()
	if err != nil {
		InternalError(w, "Failed to get working directory", err)
		return
	}

	logger := logging.NewLogger(logging.Config{Level: logging.WarnLevel})
	analyzer := coupling.NewAnalyzer(repoRoot, logger)

	opts := coupling.AnalyzeOptions{
		RepoRoot:       repoRoot,
		Target:         target,
		MinCorrelation: 0.3,
		WindowDays:     365,
		Limit:          20,
	}

	// Parse optional parameters
	if minCorr := r.URL.Query().Get("minCorrelation"); minCorr != "" {
		if v, err := strconv.ParseFloat(minCorr, 64); err == nil {
			opts.MinCorrelation = v
		}
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if v, err := strconv.Atoi(limit); err == nil {
			opts.Limit = v
		}
	}

	result, err := analyzer.Analyze(ctx, opts)
	if err != nil {
		InternalError(w, "Failed to analyze coupling", err)
		return
	}

	WriteJSON(w, result, http.StatusOK)
}

// handleCouplingCheck handles POST /coupling/check
// Checks if tightly-coupled files are missing from a change set.
func (s *Server) handleCouplingCheck(w http.ResponseWriter, r *http.Request) {
	var req CouplingCheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.Files == "" {
		http.Error(w, "Missing required field: files", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	repoRoot, err := os.Getwd()
	if err != nil {
		InternalError(w, "Failed to get working directory", err)
		return
	}

	changedFiles := strings.Split(req.Files, ",")
	for i := range changedFiles {
		changedFiles[i] = strings.TrimSpace(changedFiles[i])
	}

	// Create a set for quick lookup
	changedSet := make(map[string]bool)
	for _, f := range changedFiles {
		changedSet[f] = true
	}

	logger := logging.NewLogger(logging.Config{Level: logging.WarnLevel})
	analyzer := coupling.NewAnalyzer(repoRoot, logger)

	var missing []MissingCoupledFile

	// For each changed file, check if highly-coupled files are also changed
	for _, file := range changedFiles {
		result, err := analyzer.Analyze(ctx, coupling.AnalyzeOptions{
			RepoRoot:       repoRoot,
			Target:         file,
			MinCorrelation: 0.7, // Only high coupling
			WindowDays:     365,
			Limit:          10,
		})
		if err != nil {
			continue
		}

		for _, corr := range result.Correlations {
			// Skip if the coupled file is already in the change set
			if changedSet[corr.FilePath] || changedSet[corr.File] {
				continue
			}

			missing = append(missing, MissingCoupledFile{
				File:          corr.FilePath,
				CoupledTo:     file,
				CouplingScore: corr.Correlation,
				CochangeCount: corr.CoChangeCount,
			})
		}
	}

	response := CouplingCheckResponse{
		ChangedFiles:   changedFiles,
		MissingCoupled: missing,
	}

	if len(missing) > 0 {
		files := make([]string, 0, len(missing))
		for _, m := range missing {
			if len(files) < 3 {
				files = append(files, filepath.Base(m.File))
			}
		}
		response.Recommendation = "Consider updating: " + strings.Join(files, ", ")
	}

	WriteJSON(w, response, http.StatusOK)
}

// handleAudit handles GET /audit?minScore=...&limit=...&factor=...&quickWins=...
// Returns risk analysis for the codebase.
func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	repoRoot, err := os.Getwd()
	if err != nil {
		InternalError(w, "Failed to get working directory", err)
		return
	}

	opts := audit.AuditOptions{
		RepoRoot: repoRoot,
		MinScore: 40,
		Limit:    50,
	}

	// Parse optional parameters
	if minScore := r.URL.Query().Get("minScore"); minScore != "" {
		if v, err := strconv.ParseFloat(minScore, 64); err == nil {
			opts.MinScore = v
		}
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		if v, err := strconv.Atoi(limit); err == nil {
			opts.Limit = v
		}
	}
	if factor := r.URL.Query().Get("factor"); factor != "" {
		opts.Factor = factor
	}
	if quickWins := r.URL.Query().Get("quickWins"); quickWins == "true" {
		opts.QuickWins = true
	}

	logger := logging.NewLogger(logging.Config{Level: logging.WarnLevel})
	analyzer := audit.NewAnalyzer(repoRoot, logger)

	result, err := analyzer.Analyze(ctx, opts)
	if err != nil {
		InternalError(w, "Failed to run audit", err)
		return
	}

	// If quickWins=true, only return quick wins
	if opts.QuickWins {
		WriteJSON(w, map[string]interface{}{
			"quickWins": result.QuickWins,
		}, http.StatusOK)
		return
	}

	WriteJSON(w, result, http.StatusOK)
}

// DiffSummaryRequest represents a POST /diff/summary request body.
type DiffSummaryRequest struct {
	From string `json:"from"` // Git ref (tag, branch, commit)
	To   string `json:"to"`   // Git ref (tag, branch, commit)
}

// handleDiffSummary handles POST /diff/summary
// Summarizes changes between two git refs.
func (s *Server) handleDiffSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DiffSummaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.From == "" || req.To == "" {
		http.Error(w, "Missing required fields: from and to", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	opts := query.SummarizeDiffOptions{
		CommitRange: &query.CommitRangeSelector{
			Base: req.From,
			Head: req.To,
		},
	}

	result, err := s.engine.SummarizeDiff(ctx, opts)
	if err != nil {
		InternalError(w, "Failed to summarize diff", err)
		return
	}

	WriteJSON(w, result, http.StatusOK)
}
