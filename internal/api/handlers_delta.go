package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"ckb/internal/diff"
)

// DeltaIngestResponse represents the response for delta ingestion
type DeltaIngestResponse struct {
	Status          string    `json:"status"`
	Timestamp       time.Time `json:"timestamp"`
	DeltaID         string    `json:"deltaId,omitempty"`
	SymbolsAdded    int       `json:"symbolsAdded"`
	SymbolsModified int       `json:"symbolsModified"`
	SymbolsDeleted  int       `json:"symbolsDeleted"`
	RefsAdded       int       `json:"refsAdded"`
	RefsDeleted     int       `json:"refsDeleted"`
	ProcessingMs    int64     `json:"processingMs"`
	Warnings        []string  `json:"warnings,omitempty"`
}

// DeltaValidateResponse represents the response for delta validation
type DeltaValidateResponse struct {
	Valid           bool                `json:"valid"`
	Timestamp       time.Time           `json:"timestamp"`
	SchemaVersion   int                 `json:"schemaVersion"`
	Stats           diff.DeltaStats     `json:"stats"`
	Errors          []ValidationMessage `json:"errors,omitempty"`
	Warnings        []ValidationMessage `json:"warnings,omitempty"`
	SpotChecked     int                 `json:"spotChecked"`
	SpotCheckPassed int                 `json:"spotCheckPassed"`
}

// ValidationMessage represents a validation error or warning
type ValidationMessage struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// handleDeltaIngest handles POST /delta/ingest - ingest a delta artifact
func (s *Server) handleDeltaIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()

	// Read body
	body, err := io.ReadAll(io.LimitReader(r.Body, 50*1024*1024)) // 50MB limit
	if err != nil {
		WriteJSONError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse delta
	delta, err := diff.ParseDelta(body)
	if err != nil {
		WriteJSONError(w, "Invalid delta format: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate delta
	validator := diff.NewValidator(
		diff.WithValidationMode(diff.ValidationStrict),
		diff.WithSpotCheckPercentage(0.1), // Check 10% of entities
	)

	ctx := r.Context()

	// For ingestion, we use the delta's base snapshot for validation
	// A more sophisticated implementation would compare against the actual current state
	result := validator.Validate(delta, delta.BaseSnapshotID)
	if !result.Valid {
		// Return validation errors
		errors := make([]ValidationMessage, len(result.Errors))
		for i, e := range result.Errors {
			errors[i] = ValidationMessage{Code: e.Code, Message: e.Message}
		}
		resp := DeltaValidateResponse{
			Valid:           false,
			Timestamp:       time.Now(),
			SchemaVersion:   delta.SchemaVersion,
			Stats:           delta.Stats,
			Errors:          errors,
			SpotChecked:     result.SpotChecked,
			SpotCheckPassed: result.SpotCheckPassed,
		}
		WriteJSON(w, resp, http.StatusBadRequest)
		return
	}

	// Apply delta to storage
	warnings, err := s.engine.ApplyDelta(ctx, delta)
	if err != nil {
		WriteJSONError(w, "Failed to apply delta: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Refresh FTS index
	if err := s.engine.RefreshFTS(ctx); err != nil {
		warnings = append(warnings, "FTS refresh failed: "+err.Error())
	}

	response := DeltaIngestResponse{
		Status:          "ingested",
		Timestamp:       time.Now(),
		DeltaID:         delta.NewSnapshotID,
		SymbolsAdded:    delta.Stats.SymbolsAdded,
		SymbolsModified: delta.Stats.SymbolsModified,
		SymbolsDeleted:  delta.Stats.SymbolsDeleted,
		RefsAdded:       delta.Stats.RefsAdded,
		RefsDeleted:     delta.Stats.RefsDeleted,
		ProcessingMs:    time.Since(start).Milliseconds(),
		Warnings:        warnings,
	}

	WriteJSON(w, response, http.StatusOK)
}

// handleDeltaValidate handles POST /delta/validate - validate a delta without ingesting
func (s *Server) handleDeltaValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read body
	body, err := io.ReadAll(io.LimitReader(r.Body, 50*1024*1024)) // 50MB limit
	if err != nil {
		WriteJSONError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse delta
	delta, err := diff.ParseDelta(body)
	if err != nil {
		WriteJSONError(w, "Invalid delta format: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate delta
	validator := diff.NewValidator(
		diff.WithValidationMode(diff.ValidationStrict),
		diff.WithSpotCheckPercentage(0.2), // Check 20% for validation-only
	)

	// For validation-only, use the delta's base snapshot ID
	result := validator.Validate(delta, delta.BaseSnapshotID)

	// Build response
	errors := make([]ValidationMessage, len(result.Errors))
	for i, e := range result.Errors {
		errors[i] = ValidationMessage{Code: e.Code, Message: e.Message}
	}

	warnings := make([]ValidationMessage, len(result.Warnings))
	for i, w := range result.Warnings {
		warnings[i] = ValidationMessage{Code: w.Code, Message: w.Message}
	}

	response := DeltaValidateResponse{
		Valid:           result.Valid,
		Timestamp:       time.Now(),
		SchemaVersion:   delta.SchemaVersion,
		Stats:           delta.Stats,
		Errors:          errors,
		Warnings:        warnings,
		SpotChecked:     result.SpotChecked,
		SpotCheckPassed: result.SpotCheckPassed,
	}

	status := http.StatusOK
	if !result.Valid {
		status = http.StatusBadRequest
	}

	WriteJSON(w, response, status)
}

// handleDeltaRoutes routes /delta/* requests
func (s *Server) handleDeltaRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	case path == "/delta/ingest":
		s.handleDeltaIngest(w, r)
	case path == "/delta/validate":
		s.handleDeltaValidate(w, r)
	default:
		// Handle /delta as info endpoint
		if path == "/delta" {
			s.handleDeltaInfo(w, r)
			return
		}
		http.NotFound(w, r)
	}
}

// handleDeltaInfo returns information about delta endpoints
func (s *Server) handleDeltaInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	info := map[string]interface{}{
		"description": "Delta artifact ingestion for incremental indexing",
		"endpoints": []map[string]string{
			{
				"method":      "POST",
				"path":        "/delta/ingest",
				"description": "Ingest a delta artifact to update the index",
			},
			{
				"method":      "POST",
				"path":        "/delta/validate",
				"description": "Validate a delta artifact without ingesting",
			},
		},
		"schemaVersion": diff.SchemaVersion,
	}

	WriteJSON(w, info, http.StatusOK)
}

// WriteJSONError writes a JSON error response
func WriteJSONError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
