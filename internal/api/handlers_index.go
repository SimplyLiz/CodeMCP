package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// handleIndexRepoRoutes routes requests under /index/repos/{repo}/
func (s *Server) handleIndexRepoRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Route based on path suffix and method
	switch {
	case strings.HasSuffix(path, "/upload/delta"):
		// POST /index/repos/{repo}/upload/delta - Delta upload
		s.HandleIndexDeltaUpload(w, r)
	case strings.HasSuffix(path, "/upload"):
		// POST /index/repos/{repo}/upload - Upload SCIP index
		s.HandleIndexUpload(w, r)
	case strings.HasSuffix(path, "/meta"):
		s.HandleIndexGetMeta(w, r)
	case strings.HasSuffix(path, "/files"):
		s.HandleIndexListFiles(w, r)
	case strings.HasSuffix(path, "/symbols:batchGet"):
		s.HandleIndexBatchGetSymbols(w, r)
	case strings.HasSuffix(path, "/symbols"):
		s.HandleIndexListSymbols(w, r)
	case strings.Contains(path, "/symbols/"):
		s.HandleIndexGetSymbol(w, r)
	case strings.HasSuffix(path, "/refs"):
		s.HandleIndexListRefs(w, r)
	case strings.HasSuffix(path, "/callgraph"):
		s.HandleIndexListCallgraph(w, r)
	case strings.HasSuffix(path, "/search/symbols"):
		s.HandleIndexSearchSymbols(w, r)
	case strings.HasSuffix(path, "/search/files"):
		s.HandleIndexSearchFiles(w, r)
	default:
		// Check for DELETE /index/repos/{repo} - delete repo
		if r.Method == http.MethodDelete {
			s.HandleIndexDeleteRepo(w, r)
			return
		}
		http.NotFound(w, r)
	}
}

// HandleIndexListRepos handles GET /index/repos and POST /index/repos
func (s *Server) HandleIndexListRepos(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleIndexListReposGet(w, r)
	case http.MethodPost:
		s.HandleIndexCreateRepo(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleIndexListReposGet(w http.ResponseWriter, r *http.Request) {
	if s.indexManager == nil {
		writeIndexError(w, http.StatusServiceUnavailable, "index_server_disabled", "Index server not enabled")
		return
	}

	handles := s.indexManager.ListRepos()
	repos := make([]IndexRepoInfo, len(handles))
	for i, h := range handles {
		repos[i] = h.ToRepoInfo()
	}

	WriteJSON(w, NewIndexResponse(IndexListReposResponse{Repos: repos}), http.StatusOK)
}

// HandleIndexGetMeta handles GET /index/repos/{repo}/meta
func (s *Server) HandleIndexGetMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.indexManager == nil {
		writeIndexError(w, http.StatusServiceUnavailable, "index_server_disabled", "Index server not enabled")
		return
	}

	repoID := extractRepoID(r.URL.Path, "/index/repos/", "/meta")
	if repoID == "" {
		writeIndexError(w, http.StatusBadRequest, "invalid_repo_id", "Repo ID is required")
		return
	}

	handle, err := s.indexManager.GetRepo(repoID)
	if err != nil {
		writeIndexError(w, http.StatusNotFound, "repo_not_found", err.Error())
		return
	}

	privacy := s.indexManager.Config().GetRepoPrivacy(repoID)
	maxPageSize := s.indexManager.Config().MaxPageSize

	WriteJSON(w, NewIndexResponse(handle.ToMetaResponse(privacy, maxPageSize)), http.StatusOK)
}

// HandleIndexListFiles handles GET /index/repos/{repo}/files
func (s *Server) HandleIndexListFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.indexManager == nil {
		writeIndexError(w, http.StatusServiceUnavailable, "index_server_disabled", "Index server not enabled")
		return
	}

	repoID := extractRepoID(r.URL.Path, "/index/repos/", "/files")
	if repoID == "" {
		writeIndexError(w, http.StatusBadRequest, "invalid_repo_id", "Repo ID is required")
		return
	}

	handle, err := s.indexManager.GetRepo(repoID)
	if err != nil {
		writeIndexError(w, http.StatusNotFound, "repo_not_found", err.Error())
		return
	}

	// Parse query params
	limit := QueryParamInt(r, "limit", 1000)
	if limit > s.indexManager.Config().MaxPageSize {
		limit = s.indexManager.Config().MaxPageSize
	}

	cursorStr := r.URL.Query().Get("cursor")
	cursor, err := s.indexManager.CursorManager().Decode(cursorStr)
	if err != nil {
		writeIndexError(w, http.StatusBadRequest, "invalid_cursor", err.Error())
		return
	}
	if cursor != nil {
		if err := cursor.ValidateEntity("file"); err != nil {
			writeIndexError(w, http.StatusBadRequest, "cursor_entity_mismatch", err.Error())
			return
		}
	}

	// Query files
	files, nextCursor, err := handle.QueryFiles(cursor, limit)
	if err != nil {
		writeIndexError(w, http.StatusInternalServerError, "query_error", err.Error())
		return
	}

	// Apply redaction
	privacy := s.indexManager.Config().GetRepoPrivacy(repoID)
	redactor := NewRedactor(&privacy)
	files = redactor.RedactFiles(files)

	// Build response
	meta := &IndexResponseMeta{
		SyncSeq: handle.Meta().SyncSeq,
		HasMore: nextCursor != nil,
	}
	if nextCursor != nil {
		encoded, _ := s.indexManager.CursorManager().Encode(*nextCursor)
		meta.Cursor = encoded
	}

	WriteJSON(w, NewIndexResponseWithMeta(IndexListFilesResponse{Files: files}, meta), http.StatusOK)
}

// HandleIndexListSymbols handles GET /index/repos/{repo}/symbols
func (s *Server) HandleIndexListSymbols(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.indexManager == nil {
		writeIndexError(w, http.StatusServiceUnavailable, "index_server_disabled", "Index server not enabled")
		return
	}

	repoID := extractRepoID(r.URL.Path, "/index/repos/", "/symbols")
	if repoID == "" {
		writeIndexError(w, http.StatusBadRequest, "invalid_repo_id", "Repo ID is required")
		return
	}

	handle, err := s.indexManager.GetRepo(repoID)
	if err != nil {
		writeIndexError(w, http.StatusNotFound, "repo_not_found", err.Error())
		return
	}

	// Parse query params
	limit := QueryParamInt(r, "limit", 1000)
	if limit > s.indexManager.Config().MaxPageSize {
		limit = s.indexManager.Config().MaxPageSize
	}

	cursorStr := r.URL.Query().Get("cursor")
	cursor, err := s.indexManager.CursorManager().Decode(cursorStr)
	if err != nil {
		writeIndexError(w, http.StatusBadRequest, "invalid_cursor", err.Error())
		return
	}
	if cursor != nil {
		if err := cursor.ValidateEntity("symbol"); err != nil {
			writeIndexError(w, http.StatusBadRequest, "cursor_entity_mismatch", err.Error())
			return
		}
	}

	filters := SymbolFilters{
		Language: r.URL.Query().Get("language"),
		Kind:     r.URL.Query().Get("kind"),
		File:     r.URL.Query().Get("file"),
	}

	// Query symbols
	symbols, nextCursor, total, err := handle.QuerySymbols(cursor, limit, filters)
	if err != nil {
		writeIndexError(w, http.StatusInternalServerError, "query_error", err.Error())
		return
	}

	// Apply redaction
	privacy := s.indexManager.Config().GetRepoPrivacy(repoID)
	redactor := NewRedactor(&privacy)
	symbols = redactor.RedactSymbols(symbols)

	// Build response
	meta := &IndexResponseMeta{
		SyncSeq: handle.Meta().SyncSeq,
		HasMore: nextCursor != nil,
		Total:   total,
	}
	if nextCursor != nil {
		encoded, _ := s.indexManager.CursorManager().Encode(*nextCursor)
		meta.Cursor = encoded
	}

	WriteJSON(w, NewIndexResponseWithMeta(IndexListSymbolsResponse{Symbols: symbols}, meta), http.StatusOK)
}

// HandleIndexGetSymbol handles GET /index/repos/{repo}/symbols/{id}
func (s *Server) HandleIndexGetSymbol(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.indexManager == nil {
		writeIndexError(w, http.StatusServiceUnavailable, "index_server_disabled", "Index server not enabled")
		return
	}

	// Extract repo and symbol ID from path: /index/repos/{repo}/symbols/{id}
	path := r.URL.Path
	prefix := "/index/repos/"
	if !strings.HasPrefix(path, prefix) {
		writeIndexError(w, http.StatusBadRequest, "invalid_path", "Invalid path")
		return
	}
	rest := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(rest, "/symbols/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeIndexError(w, http.StatusBadRequest, "invalid_path", "Repo ID and Symbol ID are required")
		return
	}
	repoID := parts[0]
	symbolID := parts[1]

	handle, err := s.indexManager.GetRepo(repoID)
	if err != nil {
		writeIndexError(w, http.StatusNotFound, "repo_not_found", err.Error())
		return
	}

	symbol, err := handle.GetSymbol(symbolID)
	if err != nil {
		writeIndexError(w, http.StatusInternalServerError, "query_error", err.Error())
		return
	}
	if symbol == nil {
		writeIndexError(w, http.StatusNotFound, "symbol_not_found", "Symbol not found: "+symbolID)
		return
	}

	// Apply redaction
	privacy := s.indexManager.Config().GetRepoPrivacy(repoID)
	redactor := NewRedactor(&privacy)
	*symbol = redactor.RedactSymbol(*symbol)

	meta := &IndexResponseMeta{
		SyncSeq: handle.Meta().SyncSeq,
	}

	WriteJSON(w, NewIndexResponseWithMeta(IndexGetSymbolResponse{Symbol: *symbol}, meta), http.StatusOK)
}

// HandleIndexBatchGetSymbols handles POST /index/repos/{repo}/symbols:batchGet
func (s *Server) HandleIndexBatchGetSymbols(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.indexManager == nil {
		writeIndexError(w, http.StatusServiceUnavailable, "index_server_disabled", "Index server not enabled")
		return
	}

	// Extract repo ID from path: /index/repos/{repo}/symbols:batchGet
	path := r.URL.Path
	prefix := "/index/repos/"
	suffix := "/symbols:batchGet"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		writeIndexError(w, http.StatusBadRequest, "invalid_path", "Invalid path")
		return
	}
	repoID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if repoID == "" {
		writeIndexError(w, http.StatusBadRequest, "invalid_repo_id", "Repo ID is required")
		return
	}

	handle, err := s.indexManager.GetRepo(repoID)
	if err != nil {
		writeIndexError(w, http.StatusNotFound, "repo_not_found", err.Error())
		return
	}

	// Parse request body
	var req IndexBatchGetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeIndexError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON: "+err.Error())
		return
	}

	if len(req.IDs) == 0 {
		writeIndexError(w, http.StatusBadRequest, "invalid_request", "IDs array is required")
		return
	}

	if len(req.IDs) > 1000 {
		writeIndexError(w, http.StatusBadRequest, "too_many_ids", "Maximum 1000 IDs per request")
		return
	}

	symbols, notFound, err := handle.BatchGetSymbols(req.IDs)
	if err != nil {
		writeIndexError(w, http.StatusInternalServerError, "query_error", err.Error())
		return
	}

	// Apply redaction
	privacy := s.indexManager.Config().GetRepoPrivacy(repoID)
	redactor := NewRedactor(&privacy)
	symbols = redactor.RedactSymbols(symbols)

	meta := &IndexResponseMeta{
		SyncSeq: handle.Meta().SyncSeq,
	}

	WriteJSON(w, NewIndexResponseWithMeta(IndexBatchGetResponse{
		Symbols:  symbols,
		NotFound: notFound,
	}, meta), http.StatusOK)
}

// HandleIndexListRefs handles GET /index/repos/{repo}/refs
func (s *Server) HandleIndexListRefs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.indexManager == nil {
		writeIndexError(w, http.StatusServiceUnavailable, "index_server_disabled", "Index server not enabled")
		return
	}

	repoID := extractRepoID(r.URL.Path, "/index/repos/", "/refs")
	if repoID == "" {
		writeIndexError(w, http.StatusBadRequest, "invalid_repo_id", "Repo ID is required")
		return
	}

	handle, err := s.indexManager.GetRepo(repoID)
	if err != nil {
		writeIndexError(w, http.StatusNotFound, "repo_not_found", err.Error())
		return
	}

	// Parse query params
	limit := QueryParamInt(r, "limit", 1000)
	if limit > s.indexManager.Config().MaxPageSize {
		limit = s.indexManager.Config().MaxPageSize
	}

	cursorStr := r.URL.Query().Get("cursor")
	cursor, err := s.indexManager.CursorManager().Decode(cursorStr)
	if err != nil {
		writeIndexError(w, http.StatusBadRequest, "invalid_cursor", err.Error())
		return
	}
	if cursor != nil {
		if validateErr := cursor.ValidateEntity("ref"); validateErr != nil {
			writeIndexError(w, http.StatusBadRequest, "cursor_entity_mismatch", validateErr.Error())
			return
		}
	}

	filters := RefFilters{
		FromFile:   r.URL.Query().Get("from_file"),
		ToSymbolID: r.URL.Query().Get("to_symbol_id"),
		Language:   r.URL.Query().Get("language"),
	}

	// Query refs
	refs, nextCursor, err := handle.QueryRefs(cursor, limit, filters)
	if err != nil {
		writeIndexError(w, http.StatusInternalServerError, "query_error", err.Error())
		return
	}

	// Apply redaction
	privacy := s.indexManager.Config().GetRepoPrivacy(repoID)
	redactor := NewRedactor(&privacy)
	refs = redactor.RedactRefs(refs)

	// Build response
	meta := &IndexResponseMeta{
		SyncSeq: handle.Meta().SyncSeq,
		HasMore: nextCursor != nil,
	}
	if nextCursor != nil {
		encoded, _ := s.indexManager.CursorManager().Encode(*nextCursor)
		meta.Cursor = encoded
	}

	WriteJSON(w, NewIndexResponseWithMeta(IndexListRefsResponse{Refs: refs}, meta), http.StatusOK)
}

// HandleIndexListCallgraph handles GET /index/repos/{repo}/callgraph
func (s *Server) HandleIndexListCallgraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.indexManager == nil {
		writeIndexError(w, http.StatusServiceUnavailable, "index_server_disabled", "Index server not enabled")
		return
	}

	repoID := extractRepoID(r.URL.Path, "/index/repos/", "/callgraph")
	if repoID == "" {
		writeIndexError(w, http.StatusBadRequest, "invalid_repo_id", "Repo ID is required")
		return
	}

	handle, err := s.indexManager.GetRepo(repoID)
	if err != nil {
		writeIndexError(w, http.StatusNotFound, "repo_not_found", err.Error())
		return
	}

	// Parse query params
	limit := QueryParamInt(r, "limit", 1000)
	if limit > s.indexManager.Config().MaxPageSize {
		limit = s.indexManager.Config().MaxPageSize
	}

	cursorStr := r.URL.Query().Get("cursor")
	cursor, err := s.indexManager.CursorManager().Decode(cursorStr)
	if err != nil {
		writeIndexError(w, http.StatusBadRequest, "invalid_cursor", err.Error())
		return
	}
	if cursor != nil {
		if validateErr := cursor.ValidateEntity("callgraph"); validateErr != nil {
			writeIndexError(w, http.StatusBadRequest, "cursor_entity_mismatch", validateErr.Error())
			return
		}
	}

	filters := CallgraphFilters{
		CallerID:   r.URL.Query().Get("caller_id"),
		CalleeID:   r.URL.Query().Get("callee_id"),
		CallerFile: r.URL.Query().Get("caller_file"),
		Language:   r.URL.Query().Get("language"),
	}

	// Query callgraph
	edges, nextCursor, err := handle.QueryCallgraph(cursor, limit, filters)
	if err != nil {
		writeIndexError(w, http.StatusInternalServerError, "query_error", err.Error())
		return
	}

	// Apply redaction
	privacy := s.indexManager.Config().GetRepoPrivacy(repoID)
	redactor := NewRedactor(&privacy)
	edges = redactor.RedactCallEdges(edges)

	// Build response
	meta := &IndexResponseMeta{
		SyncSeq: handle.Meta().SyncSeq,
		HasMore: nextCursor != nil,
	}
	if nextCursor != nil {
		encoded, _ := s.indexManager.CursorManager().Encode(*nextCursor)
		meta.Cursor = encoded
	}

	WriteJSON(w, NewIndexResponseWithMeta(IndexListCallgraphResponse{Edges: edges}, meta), http.StatusOK)
}

// HandleIndexSearchSymbols handles GET /index/repos/{repo}/search/symbols
func (s *Server) HandleIndexSearchSymbols(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.indexManager == nil {
		writeIndexError(w, http.StatusServiceUnavailable, "index_server_disabled", "Index server not enabled")
		return
	}

	repoID := extractRepoID(r.URL.Path, "/index/repos/", "/search/symbols")
	if repoID == "" {
		writeIndexError(w, http.StatusBadRequest, "invalid_repo_id", "Repo ID is required")
		return
	}

	handle, err := s.indexManager.GetRepo(repoID)
	if err != nil {
		writeIndexError(w, http.StatusNotFound, "repo_not_found", err.Error())
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		writeIndexError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	limit := QueryParamInt(r, "limit", 100)
	if limit > s.indexManager.Config().MaxPageSize {
		limit = s.indexManager.Config().MaxPageSize
	}

	filters := SymbolFilters{
		Language: r.URL.Query().Get("language"),
		Kind:     r.URL.Query().Get("kind"),
	}

	symbols, truncated, err := handle.SearchSymbols(query, limit, filters)
	if err != nil {
		writeIndexError(w, http.StatusInternalServerError, "search_error", err.Error())
		return
	}

	// Apply redaction
	privacy := s.indexManager.Config().GetRepoPrivacy(repoID)
	redactor := NewRedactor(&privacy)
	symbols = redactor.RedactSymbols(symbols)

	meta := &IndexResponseMeta{
		SyncSeq: handle.Meta().SyncSeq,
	}

	resp := struct {
		IndexSearchResponse
		Symbols []IndexSymbol `json:"symbols"`
	}{
		IndexSearchResponse: IndexSearchResponse{Truncated: truncated},
		Symbols:             symbols,
	}

	WriteJSON(w, NewIndexResponseWithMeta(resp, meta), http.StatusOK)
}

// HandleIndexSearchFiles handles GET /index/repos/{repo}/search/files
func (s *Server) HandleIndexSearchFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.indexManager == nil {
		writeIndexError(w, http.StatusServiceUnavailable, "index_server_disabled", "Index server not enabled")
		return
	}

	repoID := extractRepoID(r.URL.Path, "/index/repos/", "/search/files")
	if repoID == "" {
		writeIndexError(w, http.StatusBadRequest, "invalid_repo_id", "Repo ID is required")
		return
	}

	handle, err := s.indexManager.GetRepo(repoID)
	if err != nil {
		writeIndexError(w, http.StatusNotFound, "repo_not_found", err.Error())
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		writeIndexError(w, http.StatusBadRequest, "missing_query", "Query parameter 'q' is required")
		return
	}

	limit := QueryParamInt(r, "limit", 100)
	if limit > s.indexManager.Config().MaxPageSize {
		limit = s.indexManager.Config().MaxPageSize
	}

	files, truncated, err := handle.SearchFiles(query, limit)
	if err != nil {
		writeIndexError(w, http.StatusInternalServerError, "search_error", err.Error())
		return
	}

	// Apply redaction
	privacy := s.indexManager.Config().GetRepoPrivacy(repoID)
	redactor := NewRedactor(&privacy)
	files = redactor.RedactFiles(files)

	meta := &IndexResponseMeta{
		SyncSeq: handle.Meta().SyncSeq,
	}

	resp := struct {
		IndexSearchResponse
		Files []IndexFile `json:"files"`
	}{
		IndexSearchResponse: IndexSearchResponse{Truncated: truncated},
		Files:               files,
	}

	WriteJSON(w, NewIndexResponseWithMeta(resp, meta), http.StatusOK)
}

// extractRepoID extracts repo ID from a path like /index/repos/{repo}/suffix
func extractRepoID(path, prefix, suffix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	if suffix != "" {
		if !strings.HasSuffix(rest, suffix) {
			return ""
		}
		rest = strings.TrimSuffix(rest, suffix)
	}
	return rest
}

// writeIndexError writes an error response in the index API format
func writeIndexError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := IndexResponse{
		Error: &IndexErrorInfo{
			Code:    code,
			Message: message,
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}
