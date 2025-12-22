package api

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// CreateRepoRequest is the request body for POST /index/repos
type CreateRepoRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CreateRepoResponse is the response for POST /index/repos
type CreateRepoResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// UploadResponse is the response for POST /index/repos/{repo}/upload
type UploadResponse struct {
	RepoID           string      `json:"repo_id"`
	Commit           string      `json:"commit,omitempty"`
	Languages        []string    `json:"languages"`
	Stats            UploadStats `json:"stats"`
	DurationMs       int64       `json:"duration_ms"`
	UploadType       string      `json:"upload_type"`                 // "full" or "delta"
	CompressedSize   int64       `json:"compressed_size,omitempty"`   // Original upload size if compressed
	CompressionRatio float64     `json:"compression_ratio,omitempty"` // Ratio (e.g., 0.15 = 85% savings)
}

// UploadStats contains processing statistics
type UploadStats struct {
	Files     int `json:"files"`
	Symbols   int `json:"symbols"`
	Refs      int `json:"refs"`
	CallEdges int `json:"call_edges"`
}

// DeleteRepoResponse is the response for DELETE /index/repos/{repo}
type DeleteRepoResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// HandleIndexCreateRepo handles POST /index/repos
// Creates a new empty repo ready for upload
func (s *Server) HandleIndexCreateRepo(w http.ResponseWriter, r *http.Request) {
	if s.indexManager == nil {
		writeIndexError(w, http.StatusServiceUnavailable, "index_server_disabled", "Index server not enabled")
		return
	}

	// Check if repo creation is allowed
	if !s.config.IndexServer.AllowCreateRepo {
		writeIndexError(w, http.StatusForbidden, "create_repo_disabled", "Creating repos via API is disabled")
		return
	}

	// Parse request
	var req CreateRepoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeIndexError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	// Validate
	if req.ID == "" {
		writeIndexError(w, http.StatusBadRequest, "missing_id", "Repo ID is required")
		return
	}

	// Validate ID format (alphanumeric, /, -, _)
	if !isValidRepoID(req.ID) {
		writeIndexError(w, http.StatusBadRequest, "invalid_id", "Repo ID must be alphanumeric with optional / - _")
		return
	}

	// Check if already exists
	if _, err := s.indexManager.GetRepo(req.ID); err == nil {
		writeIndexError(w, http.StatusConflict, "repo_exists", "Repo already exists")
		return
	}

	// Create repo in storage
	name := req.Name
	if name == "" {
		name = req.ID
	}

	if err := s.indexManager.CreateUploadedRepo(req.ID, name, req.Description); err != nil {
		writeIndexError(w, http.StatusInternalServerError, "create_failed", err.Error())
		return
	}

	// Return success
	resp := NewIndexResponse(CreateRepoResponse{
		ID:     req.ID,
		Name:   name,
		Status: "created",
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// HandleIndexDeleteRepo handles DELETE /index/repos/{repo}
// Deletes an uploaded repo and all its data
func (s *Server) HandleIndexDeleteRepo(w http.ResponseWriter, r *http.Request) {
	if s.indexManager == nil {
		writeIndexError(w, http.StatusServiceUnavailable, "index_server_disabled", "Index server not enabled")
		return
	}

	// Extract repo ID from path
	repoID := extractRepoIDFromPath(r.URL.Path, "/index/repos/", "")
	if repoID == "" {
		writeIndexError(w, http.StatusBadRequest, "missing_repo_id", "Repo ID is required")
		return
	}

	// Check if repo exists
	handle, err := s.indexManager.GetRepo(repoID)
	if err != nil {
		writeIndexError(w, http.StatusNotFound, "repo_not_found", "Repo not found")
		return
	}

	// Don't allow deleting config-based repos
	if !s.indexManager.IsUploadedRepo(repoID) {
		writeIndexError(w, http.StatusForbidden, "config_repo", "Cannot delete config-based repos via API")
		return
	}

	// Close and delete
	if err := s.indexManager.RemoveRepo(repoID); err != nil {
		writeIndexError(w, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}

	// Return success
	resp := NewIndexResponse(DeleteRepoResponse{
		ID:     handle.ID,
		Status: "deleted",
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleIndexUpload handles POST /index/repos/{repo}/upload
// Accepts SCIP index file and processes it into the database
func (s *Server) HandleIndexUpload(w http.ResponseWriter, r *http.Request) {
	if s.indexManager == nil {
		writeIndexError(w, http.StatusServiceUnavailable, "index_server_disabled", "Index server not enabled")
		return
	}

	// Extract repo ID from path (remove /upload suffix)
	path := strings.TrimSuffix(r.URL.Path, "/upload")
	repoID := extractRepoIDFromPath(path, "/index/repos/", "")
	if repoID == "" {
		writeIndexError(w, http.StatusBadRequest, "missing_repo_id", "Repo ID is required")
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

	// Create repo if it doesn't exist (auto-create on upload)
	if _, err := s.indexManager.GetRepo(repoID); err != nil {
		if s.config.IndexServer.AllowCreateRepo {
			if err := s.indexManager.CreateUploadedRepo(repoID, repoID, ""); err != nil {
				writeIndexError(w, http.StatusInternalServerError, "create_failed", err.Error())
				return
			}
		} else {
			writeIndexError(w, http.StatusNotFound, "repo_not_found", "Repo not found and auto-create is disabled")
			return
		}
	}

	// Stream upload to temp file (with decompression if needed)
	streamResult, err := s.streamUploadToFile(r, maxSize)
	if err != nil {
		writeIndexError(w, http.StatusBadRequest, "upload_failed", err.Error())
		return
	}
	defer s.indexManager.Storage().CleanupUpload(streamResult.Path)

	logFields := map[string]interface{}{
		"repo_id":           repoID,
		"decompressed_size": streamResult.DecompressedSize,
		"path":              streamResult.Path,
	}
	if streamResult.Encoding != "" {
		logFields["encoding"] = streamResult.Encoding
		logFields["compressed_size"] = streamResult.CompressedSize
	}
	s.logger.Info("Received upload", logFields)

	// Parse upload metadata from headers
	meta := parseUploadMetaFromHeaders(r)

	// Process the SCIP file
	processor := s.indexManager.Processor()
	result, err := processor.ProcessUpload(repoID, streamResult.Path, meta)
	if err != nil {
		writeIndexError(w, http.StatusBadRequest, "process_failed", err.Error())
		return
	}

	// Reload the repo handle to pick up new data
	if err := s.indexManager.ReloadRepo(repoID); err != nil {
		s.logger.Warn("Failed to reload repo after upload", map[string]interface{}{
			"repo_id": repoID,
			"error":   err.Error(),
		})
	}

	// Build response with compression stats
	uploadResp := UploadResponse{
		RepoID:    result.RepoID,
		Commit:    result.Commit,
		Languages: result.Languages,
		Stats: UploadStats{
			Files:     result.FileCount,
			Symbols:   result.SymbolCount,
			Refs:      result.RefCount,
			CallEdges: result.CallEdges,
		},
		DurationMs: result.DurationMs,
		UploadType: "full",
	}

	// Add compression stats if compressed
	if streamResult.CompressedSize > 0 {
		uploadResp.CompressedSize = streamResult.CompressedSize
		uploadResp.CompressionRatio = float64(streamResult.CompressedSize) / float64(streamResult.DecompressedSize)
	}

	resp := NewIndexResponse(uploadResp)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// StreamResult contains the result of streaming an upload to a file
type StreamResult struct {
	Path             string // Path to temp file
	DecompressedSize int64  // Size after decompression
	CompressedSize   int64  // Original compressed size (0 if not compressed)
	Encoding         string // Content-Encoding used ("gzip", "zstd", or "")
}

// countingReader wraps an io.Reader and counts bytes read
type countingReader struct {
	reader io.Reader
	count  int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.count += int64(n)
	return n, err
}

// progressWriter wraps an io.Writer with progress callbacks
type progressWriter struct {
	writer     io.Writer
	total      int64 // Expected total (-1 if unknown)
	written    int64
	callback   func(written, total int64)
	interval   int64 // Report every N bytes
	lastReport int64
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	pw.written += int64(n)

	// Report progress at intervals
	if pw.callback != nil && pw.written-pw.lastReport >= pw.interval {
		pw.callback(pw.written, pw.total)
		pw.lastReport = pw.written
	}
	return n, err
}

// streamUploadToFile streams the request body to a temp file
// Supports Content-Encoding: gzip, zstd, or identity (no compression)
func (s *Server) streamUploadToFile(r *http.Request, maxSize int64) (*StreamResult, error) {
	storage := s.indexManager.Storage()

	// Create temp file
	file, path, err := storage.CreateUploadFile()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Wrap body in counting reader to track compressed size
	counter := &countingReader{reader: r.Body}
	var reader io.Reader = counter

	// Detect compression from Content-Encoding header
	encoding := r.Header.Get("Content-Encoding")

	// Check if encoding is supported (uses config if available)
	if s.config.IndexServer != nil {
		if !s.config.IndexServer.IsEncodingSupported(encoding) {
			storage.CleanupUpload(path)
			return nil, fmt.Errorf("Content-Encoding %q not supported (compression may be disabled)", encoding)
		}
	}

	switch encoding {
	case "gzip":
		var gr *gzip.Reader
		gr, err = gzip.NewReader(reader)
		if err != nil {
			storage.CleanupUpload(path)
			return nil, fmt.Errorf("invalid gzip stream: %w", err)
		}
		defer gr.Close()
		reader = gr
	case "zstd":
		var zr *zstd.Decoder
		zr, err = zstd.NewReader(reader)
		if err != nil {
			storage.CleanupUpload(path)
			return nil, fmt.Errorf("invalid zstd stream: %w", err)
		}
		defer zr.Close()
		reader = zr
	case "", "identity":
		// No compression, use raw body
		encoding = ""
	default:
		storage.CleanupUpload(path)
		return nil, fmt.Errorf("unsupported Content-Encoding: %s", encoding)
	}

	// Create progress writer with logging callback
	contentLength := r.ContentLength            // -1 if unknown
	progressInterval := int64(10 * 1024 * 1024) // Log every 10MB
	pw := &progressWriter{
		writer:   file,
		total:    contentLength,
		interval: progressInterval,
		callback: func(written, total int64) {
			fields := map[string]interface{}{
				"phase": "receiving",
				"bytes": written,
				"mb":    float64(written) / (1024 * 1024),
			}
			if total > 0 {
				fields["total"] = total
				fields["percent"] = float64(written) / float64(total) * 100
			}
			s.logger.Info("Upload progress", fields)
		},
	}

	// Stream with size limit (applied to decompressed size)
	limitReader := io.LimitReader(reader, maxSize+1)
	written, err := io.Copy(pw, limitReader)
	if err != nil {
		storage.CleanupUpload(path)
		return nil, fmt.Errorf("failed to write upload: %w", err)
	}

	// Check if decompressed size hit the limit
	if written > maxSize {
		storage.CleanupUpload(path)
		return nil, fmt.Errorf("decompressed upload exceeds max size of %d bytes", maxSize)
	}

	// Check minimum size (SCIP files should be at least a few bytes)
	if written < 10 {
		storage.CleanupUpload(path)
		return nil, fmt.Errorf("upload too small to be a valid SCIP index")
	}

	result := &StreamResult{
		Path:             path,
		DecompressedSize: written,
		Encoding:         encoding,
	}

	// Set compressed size if compression was used
	if encoding != "" {
		result.CompressedSize = counter.count
	}

	return result, nil
}

// parseUploadMetaFromHeaders extracts upload metadata from request headers
func parseUploadMetaFromHeaders(r *http.Request) UploadMeta {
	return UploadMeta{
		Commit:      r.Header.Get("X-CKB-Commit"),
		IndexerName: r.Header.Get("X-CKB-Indexer-Name"),
		IndexerVer:  r.Header.Get("X-CKB-Indexer-Version"),
		Languages:   parseLanguagesHeader(r.Header.Get("X-CKB-Language")),
	}
}

// parseLanguagesHeader parses comma-separated languages
func parseLanguagesHeader(header string) []string {
	if header == "" {
		return nil
	}
	parts := strings.Split(header, ",")
	var langs []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			langs = append(langs, p)
		}
	}
	return langs
}

// isValidRepoID validates a repo ID
func isValidRepoID(id string) bool {
	if len(id) == 0 || len(id) > 256 {
		return false
	}
	for _, c := range id {
		if !isValidRepoIDChar(c) {
			return false
		}
	}
	// Don't allow consecutive slashes or starting/ending with special chars
	if strings.Contains(id, "//") || strings.HasPrefix(id, "/") || strings.HasSuffix(id, "/") {
		return false
	}
	return true
}

func isValidRepoIDChar(c rune) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '/' || c == '-' || c == '_' || c == '.'
}

// extractRepoIDFromPath extracts repo ID from URL path
func extractRepoIDFromPath(path, prefix, suffix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	id := strings.TrimPrefix(path, prefix)
	if suffix != "" && strings.HasSuffix(id, suffix) {
		id = strings.TrimSuffix(id, suffix)
	}
	return id
}
