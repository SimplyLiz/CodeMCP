package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DeltaUploadRequest is sent via X-CKB-Delta-Meta header or request body
type DeltaUploadRequest struct {
	BaseCommit   string             `json:"base_commit"`   // Commit delta is relative to
	TargetCommit string             `json:"target_commit"` // New commit being uploaded
	ChangedFiles []DeltaChangedFile `json:"changed_files"` // Files that changed
}

// DeltaChangedFile represents a file change in a delta upload
type DeltaChangedFile struct {
	Path       string `json:"path"`               // New path (or current path)
	OldPath    string `json:"old_path,omitempty"` // For renames
	ChangeType string `json:"change_type"`        // "added", "modified", "deleted", "renamed"
	Hash       string `json:"hash,omitempty"`     // Content hash for validation
}

// DeltaUploadResponse is the response for POST /index/repos/{repo}/upload/delta
type DeltaUploadResponse struct {
	RepoID         string      `json:"repo_id"`
	UploadType     string      `json:"upload_type"` // Always "delta"
	Commit         string      `json:"commit"`
	BaseCommit     string      `json:"base_commit"`
	FilesChanged   int         `json:"files_changed"`
	FilesUnchanged int         `json:"files_unchanged,omitempty"`
	Stats          UploadStats `json:"stats"`
	DurationMs     int64       `json:"duration_ms"`
	// Suggestion to use full upload if too many files changed
	SuggestFullUpload bool   `json:"suggest_full_upload,omitempty"`
	SuggestReason     string `json:"suggest_reason,omitempty"`
}

// DeltaErrorResponse is returned for delta-specific errors like commit mismatch
type DeltaErrorResponse struct {
	Error         string `json:"error"`
	Code          string `json:"code"`
	CurrentCommit string `json:"current_commit,omitempty"`
}

// HandleIndexDeltaUpload handles POST /index/repos/{repo}/upload/delta
// Accepts a partial SCIP index containing only changed files
func (s *Server) HandleIndexDeltaUpload(w http.ResponseWriter, r *http.Request) {
	if s.indexManager == nil {
		writeIndexError(w, http.StatusServiceUnavailable, "index_server_disabled", "Index server not enabled")
		return
	}

	// Check if delta uploads are enabled
	if s.config.IndexServer != nil && !s.config.IndexServer.EnableDeltaUpload {
		writeIndexError(w, http.StatusForbidden, "delta_disabled", "Delta uploads are disabled")
		return
	}

	// Extract repo ID from path (remove /upload/delta suffix)
	path := r.URL.Path
	if strings.HasSuffix(path, "/upload/delta") {
		path = strings.TrimSuffix(path, "/upload/delta")
	}
	repoID := extractRepoIDFromPath(path, "/index/repos/", "")
	if repoID == "" {
		writeIndexError(w, http.StatusBadRequest, "missing_repo_id", "Repo ID is required")
		return
	}

	// Check if repo exists - delta upload requires existing repo
	if _, err := s.indexManager.GetRepo(repoID); err != nil {
		writeIndexError(w, http.StatusNotFound, "repo_not_found", "Repo not found - delta upload requires existing repo")
		return
	}

	// Parse delta metadata from headers or body
	deltaMeta, err := parseDeltaMeta(r)
	if err != nil {
		writeIndexError(w, http.StatusBadRequest, "invalid_delta_meta", err.Error())
		return
	}

	// Validate base commit matches current index
	currentCommit, err := s.indexManager.GetRepoCommit(repoID)
	if err != nil {
		writeIndexError(w, http.StatusInternalServerError, "commit_check_failed", err.Error())
		return
	}

	if currentCommit != "" && deltaMeta.BaseCommit != currentCommit {
		// Return 409 Conflict with current commit info
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(DeltaErrorResponse{
			Error:         fmt.Sprintf("Base commit mismatch: index is at %s, expected %s", currentCommit, deltaMeta.BaseCommit),
			Code:          "base_commit_mismatch",
			CurrentCommit: currentCommit,
		})
		return
	}

	// Check content length if provided
	maxSize := s.config.IndexServer.MaxUploadSize
	if maxSize == 0 {
		maxSize = 500 * 1024 * 1024 // Default 500MB
	}
	if r.ContentLength > maxSize {
		writeIndexError(w, http.StatusRequestEntityTooLarge, "too_large",
			fmt.Sprintf("Upload exceeds max size of %d bytes", maxSize))
		return
	}

	// Stream upload to temp file (with decompression if needed)
	streamResult, err := s.streamUploadToFile(r, maxSize)
	if err != nil {
		writeIndexError(w, http.StatusBadRequest, "upload_failed", err.Error())
		return
	}
	defer s.indexManager.Storage().CleanupUpload(streamResult.Path)

	logFields := map[string]interface{}{
		"repo_id":       repoID,
		"upload_type":   "delta",
		"base_commit":   deltaMeta.BaseCommit,
		"target_commit": deltaMeta.TargetCommit,
		"files_changed": len(deltaMeta.ChangedFiles),
		"size":          streamResult.DecompressedSize,
	}
	s.logger.Info("Received delta upload", logFields)

	// Check if we should suggest full upload (too many files changed)
	suggestFull := false
	suggestReason := ""
	threshold := 50 // default
	if s.config.IndexServer != nil && s.config.IndexServer.DeltaThresholdPercent > 0 {
		threshold = s.config.IndexServer.DeltaThresholdPercent
	}

	// We'll calculate this properly after processing when we know total files
	// For now, just process the delta

	// Process the delta upload
	processor := s.indexManager.Processor()
	result, err := processor.ProcessDeltaUpload(repoID, streamResult.Path, deltaMeta)
	if err != nil {
		writeIndexError(w, http.StatusBadRequest, "process_failed", err.Error())
		return
	}

	// Check if we should suggest full upload based on changed percentage
	if result.TotalFiles > 0 {
		changedPercent := float64(len(deltaMeta.ChangedFiles)) / float64(result.TotalFiles) * 100
		if int(changedPercent) > threshold {
			suggestFull = true
			suggestReason = fmt.Sprintf("%.1f%% of files changed (threshold: %d%%)", changedPercent, threshold)
		}
	}

	// Reload the repo handle to pick up new data
	if err := s.indexManager.ReloadRepo(repoID); err != nil {
		s.logger.Warn("Failed to reload repo after delta upload", map[string]interface{}{
			"repo_id": repoID,
			"error":   err.Error(),
		})
	}

	// Build response
	uploadResp := DeltaUploadResponse{
		RepoID:         result.RepoID,
		UploadType:     "delta",
		Commit:         result.Commit,
		BaseCommit:     deltaMeta.BaseCommit,
		FilesChanged:   len(deltaMeta.ChangedFiles),
		FilesUnchanged: result.TotalFiles - len(deltaMeta.ChangedFiles),
		Stats: UploadStats{
			Files:     result.FileCount,
			Symbols:   result.SymbolCount,
			Refs:      result.RefCount,
			CallEdges: result.CallEdges,
		},
		DurationMs:        result.DurationMs,
		SuggestFullUpload: suggestFull,
		SuggestReason:     suggestReason,
	}

	resp := NewIndexResponse(uploadResp)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// parseDeltaMeta extracts delta metadata from headers or body
func parseDeltaMeta(r *http.Request) (*DeltaUploadRequest, error) {
	meta := &DeltaUploadRequest{}

	// First try header-based metadata (preferred for streaming)
	baseCommit := r.Header.Get("X-CKB-Base-Commit")
	targetCommit := r.Header.Get("X-CKB-Target-Commit")
	changedFilesJSON := r.Header.Get("X-CKB-Changed-Files")

	if baseCommit != "" {
		meta.BaseCommit = baseCommit
		meta.TargetCommit = targetCommit

		if changedFilesJSON != "" {
			if err := json.Unmarshal([]byte(changedFilesJSON), &meta.ChangedFiles); err != nil {
				return nil, fmt.Errorf("invalid X-CKB-Changed-Files header: %w", err)
			}
		}

		// Validate required fields
		if meta.BaseCommit == "" {
			return nil, fmt.Errorf("base_commit is required")
		}

		return meta, nil
	}

	// Fall back to body-based metadata (multipart would go here)
	return nil, fmt.Errorf("delta metadata required: set X-CKB-Base-Commit header")
}
