package api

import "time"

// IndexResponse is the standard response wrapper for index-serving endpoints
type IndexResponse struct {
	Data  interface{}        `json:"data"`
	Meta  *IndexResponseMeta `json:"meta,omitempty"`
	Error *IndexErrorInfo    `json:"error,omitempty"`
}

// IndexResponseMeta contains pagination and sync metadata
type IndexResponseMeta struct {
	SyncSeq   int64  `json:"sync_seq,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
	HasMore   bool   `json:"has_more,omitempty"`
	Total     int    `json:"total,omitempty"`
}

// IndexErrorInfo contains error details
type IndexErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// IndexRepoInfo is the summary info returned for /index/repos
type IndexRepoInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Languages    []string `json:"languages"`
	Commit       string   `json:"commit"`
	IndexVersion string   `json:"index_version"`
	SyncSeq      int64    `json:"sync_seq"`
	IndexedAt    int64    `json:"indexed_at"`
	SymbolCount  int      `json:"symbol_count"`
	FileCount    int      `json:"file_count"`
}

// IndexRepoMetaResponse is the full metadata returned for /index/repos/{repo}/meta
type IndexRepoMetaResponse struct {
	ID                     string            `json:"id"`
	Name                   string            `json:"name"`
	Description            string            `json:"description,omitempty"`
	Commit                 string            `json:"commit"`
	IndexVersion           string            `json:"index_version"`
	SyncSeq                int64             `json:"sync_seq"`
	SyncLogRetentionSeqMin int64             `json:"sync_log_retention_seq_min"`
	SchemaVersion          int               `json:"schema_version"`
	IndexedAt              int64             `json:"indexed_at"`
	Languages              []string          `json:"languages"`
	Stats                  IndexRepoStats    `json:"stats"`
	Capabilities           IndexCapabilities `json:"capabilities"`
	Privacy                IndexPrivacyInfo  `json:"privacy"`
}

// IndexRepoStats contains index statistics
type IndexRepoStats struct {
	Files     int `json:"files"`
	Symbols   int `json:"symbols"`
	Refs      int `json:"refs"`
	CallEdges int `json:"call_edges"`
}

// IndexCapabilities describes what features this index server supports
type IndexCapabilities struct {
	SyncSeq     bool     `json:"sync_seq"`
	Search      bool     `json:"search"`
	BatchGet    bool     `json:"batch_get"`
	Compression []string `json:"compression"`
	Redaction   bool     `json:"redaction"`
	MaxPageSize int      `json:"max_page_size"`
}

// IndexPrivacyInfo describes what fields are exposed
type IndexPrivacyInfo struct {
	PathsExposed      bool `json:"paths_exposed"`
	DocsExposed       bool `json:"docs_exposed"`
	SignaturesExposed bool `json:"signatures_exposed"`
}

// IndexSymbol represents a symbol in index-serving responses
type IndexSymbol struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Kind          string `json:"kind,omitempty"`
	FilePath      string `json:"file_path,omitempty"`      // May be redacted
	FileBasename  string `json:"file_basename,omitempty"`  // Filename only
	Line          int    `json:"line,omitempty"`
	Column        int    `json:"column,omitempty"`
	Language      string `json:"language,omitempty"`
	Documentation string `json:"documentation,omitempty"` // May be redacted
	Signature     string `json:"signature,omitempty"`     // May be redacted
	Container     string `json:"container,omitempty"`     // Parent symbol
}

// IndexFile represents a file in index-serving responses
type IndexFile struct {
	Path        string `json:"path,omitempty"`        // May be redacted
	Basename    string `json:"basename,omitempty"`    // Filename only
	Language    string `json:"language,omitempty"`
	SymbolCount int    `json:"symbol_count"`
	Hash        string `json:"hash,omitempty"`
}

// IndexRef represents a reference in index-serving responses
type IndexRef struct {
	FromFile   string `json:"from_file,omitempty"` // May be redacted
	ToSymbolID string `json:"to_symbol_id"`
	Line       int    `json:"line"`
	Col        int    `json:"col"`
	EndLine    int    `json:"end_line,omitempty"`
	EndCol     int    `json:"end_col,omitempty"`
	Kind       string `json:"kind,omitempty"` // "call", "read", "write", etc.
	Language   string `json:"language,omitempty"`
}

// IndexCallEdge represents a call graph edge in index-serving responses
type IndexCallEdge struct {
	CallerID   string `json:"caller_id,omitempty"` // May be empty for unresolved callers
	CalleeID   string `json:"callee_id"`
	CallerFile string `json:"caller_file,omitempty"` // May be redacted
	CallLine   int    `json:"call_line"`
	CallCol    int    `json:"call_col"`
	EndCol     int    `json:"end_col,omitempty"`
	Language   string `json:"language,omitempty"`
}

// IndexListReposResponse is the response for GET /index/repos
type IndexListReposResponse struct {
	Repos []IndexRepoInfo `json:"repos"`
}

// IndexListSymbolsResponse is the response for GET /index/repos/{repo}/symbols
type IndexListSymbolsResponse struct {
	Symbols []IndexSymbol `json:"symbols"`
}

// IndexGetSymbolResponse is the response for GET /index/repos/{repo}/symbols/{id}
type IndexGetSymbolResponse struct {
	Symbol IndexSymbol `json:"symbol"`
}

// IndexBatchGetRequest is the request body for POST /index/repos/{repo}/symbols:batchGet
type IndexBatchGetRequest struct {
	IDs []string `json:"ids"`
}

// IndexBatchGetResponse is the response for POST /index/repos/{repo}/symbols:batchGet
type IndexBatchGetResponse struct {
	Symbols  []IndexSymbol `json:"symbols"`
	NotFound []string      `json:"not_found,omitempty"`
}

// IndexListFilesResponse is the response for GET /index/repos/{repo}/files
type IndexListFilesResponse struct {
	Files []IndexFile `json:"files"`
}

// IndexListRefsResponse is the response for GET /index/repos/{repo}/refs
type IndexListRefsResponse struct {
	Refs []IndexRef `json:"refs"`
}

// IndexListCallgraphResponse is the response for GET /index/repos/{repo}/callgraph
type IndexListCallgraphResponse struct {
	Edges []IndexCallEdge `json:"edges"`
}

// IndexSearchResponse is the response for search endpoints
type IndexSearchResponse struct {
	Truncated bool `json:"truncated,omitempty"` // True if results were capped
}

// IndexSearchSymbolsResponse is the response for GET /index/repos/{repo}/search/symbols
type IndexSearchSymbolsResponse struct {
	IndexSearchResponse
	Symbols []IndexSymbol `json:"symbols"`
}

// IndexSearchFilesResponse is the response for GET /index/repos/{repo}/search/files
type IndexSearchFilesResponse struct {
	IndexSearchResponse
	Files []IndexFile `json:"files"`
}

// SymbolFilters contains filters for symbol queries
type SymbolFilters struct {
	Language string
	Kind     string
	File     string
}

// RefFilters contains filters for reference queries
type RefFilters struct {
	FromFile   string
	ToSymbolID string
	Language   string
}

// CallgraphFilters contains filters for callgraph queries
type CallgraphFilters struct {
	CallerID   string
	CalleeID   string
	CallerFile string
	Language   string
}

// IndexRepoMetadata is internal metadata tracked for a repo
type IndexRepoMetadata struct {
	Commit        string
	IndexVersion  string
	SyncSeq       int64
	SchemaVersion int
	IndexedAt     time.Time
	Languages     []string
	Stats         IndexRepoStats
}

// NewIndexResponse creates a new IndexResponse with data
func NewIndexResponse(data interface{}) IndexResponse {
	return IndexResponse{
		Data: data,
		Meta: &IndexResponseMeta{
			Timestamp: time.Now().Unix(),
		},
	}
}

// NewIndexResponseWithMeta creates a new IndexResponse with data and metadata
func NewIndexResponseWithMeta(data interface{}, meta *IndexResponseMeta) IndexResponse {
	if meta.Timestamp == 0 {
		meta.Timestamp = time.Now().Unix()
	}
	return IndexResponse{
		Data: data,
		Meta: meta,
	}
}

// NewIndexErrorResponse creates a new error response
func NewIndexErrorResponse(code, message string) IndexResponse {
	return IndexResponse{
		Error: &IndexErrorInfo{
			Code:    code,
			Message: message,
		},
	}
}
