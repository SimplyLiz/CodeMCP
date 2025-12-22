// Package federation provides cross-repository federation capabilities for CKB.
// This file contains types for remote index server responses (Phase 5).
package federation

import "time"

// RemoteResponse is the standard response wrapper from remote index servers.
// Matches internal/api/index_types.go IndexResponse.
type RemoteResponse struct {
	Data  interface{}         `json:"data"`
	Meta  *RemoteResponseMeta `json:"meta,omitempty"`
	Error *RemoteErrorInfo    `json:"error,omitempty"`
}

// RemoteResponseMeta contains pagination and sync metadata from remote servers.
type RemoteResponseMeta struct {
	SyncSeq   int64  `json:"sync_seq,omitempty"`
	Timestamp int64  `json:"timestamp,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
	HasMore   bool   `json:"has_more,omitempty"`
	Total     int    `json:"total,omitempty"`
}

// RemoteErrorInfo contains error details from remote servers.
type RemoteErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RemoteRepoInfo is the summary info returned for GET /index/repos.
type RemoteRepoInfo struct {
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

// RemoteRepoMeta is the full metadata returned for GET /index/repos/{repo}/meta.
type RemoteRepoMeta struct {
	ID                     string             `json:"id"`
	Name                   string             `json:"name"`
	Description            string             `json:"description,omitempty"`
	Commit                 string             `json:"commit"`
	IndexVersion           string             `json:"index_version"`
	SyncSeq                int64              `json:"sync_seq"`
	SyncLogRetentionSeqMin int64              `json:"sync_log_retention_seq_min"`
	SchemaVersion          int                `json:"schema_version"`
	IndexedAt              int64              `json:"indexed_at"`
	Languages              []string           `json:"languages"`
	Stats                  RemoteRepoStats    `json:"stats"`
	Capabilities           RemoteCapabilities `json:"capabilities"`
	Privacy                RemotePrivacyInfo  `json:"privacy"`
}

// RemoteRepoStats contains index statistics from remote.
type RemoteRepoStats struct {
	Files     int `json:"files"`
	Symbols   int `json:"symbols"`
	Refs      int `json:"refs"`
	CallEdges int `json:"call_edges"`
}

// RemoteCapabilities describes what features a remote index server supports.
type RemoteCapabilities struct {
	SyncSeq     bool     `json:"sync_seq"`
	Search      bool     `json:"search"`
	BatchGet    bool     `json:"batch_get"`
	Compression []string `json:"compression"`
	Redaction   bool     `json:"redaction"`
	MaxPageSize int      `json:"max_page_size"`
}

// RemotePrivacyInfo describes what fields are exposed by remote.
type RemotePrivacyInfo struct {
	PathsExposed      bool `json:"paths_exposed"`
	DocsExposed       bool `json:"docs_exposed"`
	SignaturesExposed bool `json:"signatures_exposed"`
}

// RemoteSymbol represents a symbol from a remote index server.
type RemoteSymbol struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Kind          string `json:"kind,omitempty"`
	FilePath      string `json:"file_path,omitempty"`
	FileBasename  string `json:"file_basename,omitempty"`
	Line          int    `json:"line,omitempty"`
	Column        int    `json:"column,omitempty"`
	Language      string `json:"language,omitempty"`
	Documentation string `json:"documentation,omitempty"`
	Signature     string `json:"signature,omitempty"`
	Container     string `json:"container,omitempty"`
}

// RemoteFile represents a file from a remote index server.
type RemoteFile struct {
	Path        string `json:"path,omitempty"`
	Basename    string `json:"basename,omitempty"`
	Language    string `json:"language,omitempty"`
	SymbolCount int    `json:"symbol_count"`
	Hash        string `json:"hash,omitempty"`
}

// RemoteRef represents a reference from a remote index server.
type RemoteRef struct {
	FromFile   string `json:"from_file,omitempty"`
	ToSymbolID string `json:"to_symbol_id"`
	Line       int    `json:"line"`
	Col        int    `json:"col"`
	EndLine    int    `json:"end_line,omitempty"`
	EndCol     int    `json:"end_col,omitempty"`
	Kind       string `json:"kind,omitempty"`
	Language   string `json:"language,omitempty"`
}

// RemoteCallEdge represents a call graph edge from a remote index server.
type RemoteCallEdge struct {
	CallerID   string `json:"caller_id,omitempty"`
	CalleeID   string `json:"callee_id"`
	CallerFile string `json:"caller_file,omitempty"`
	CallLine   int    `json:"call_line"`
	CallCol    int    `json:"call_col"`
	EndCol     int    `json:"end_col,omitempty"`
	Language   string `json:"language,omitempty"`
}

// Response types for different endpoints

// RemoteListReposResponse is the response for GET /index/repos.
type RemoteListReposResponse struct {
	Repos []RemoteRepoInfo `json:"repos"`
}

// RemoteListSymbolsResponse is the response for GET /index/repos/{repo}/symbols.
type RemoteListSymbolsResponse struct {
	Symbols []RemoteSymbol `json:"symbols"`
}

// RemoteGetSymbolResponse is the response for GET /index/repos/{repo}/symbols/{id}.
type RemoteGetSymbolResponse struct {
	Symbol RemoteSymbol `json:"symbol"`
}

// RemoteBatchGetRequest is the request body for POST /index/repos/{repo}/symbols:batchGet.
type RemoteBatchGetRequest struct {
	IDs []string `json:"ids"`
}

// RemoteBatchGetResponse is the response for POST /index/repos/{repo}/symbols:batchGet.
type RemoteBatchGetResponse struct {
	Symbols  []RemoteSymbol `json:"symbols"`
	NotFound []string       `json:"not_found,omitempty"`
}

// RemoteListFilesResponse is the response for GET /index/repos/{repo}/files.
type RemoteListFilesResponse struct {
	Files []RemoteFile `json:"files"`
}

// RemoteListRefsResponse is the response for GET /index/repos/{repo}/refs.
type RemoteListRefsResponse struct {
	Refs []RemoteRef `json:"refs"`
}

// RemoteListCallgraphResponse is the response for GET /index/repos/{repo}/callgraph.
type RemoteListCallgraphResponse struct {
	Edges []RemoteCallEdge `json:"edges"`
}

// RemoteSearchSymbolsResponse is the response for GET /index/repos/{repo}/search/symbols.
type RemoteSearchSymbolsResponse struct {
	Truncated bool           `json:"truncated,omitempty"`
	Symbols   []RemoteSymbol `json:"symbols"`
}

// RemoteSearchFilesResponse is the response for GET /index/repos/{repo}/search/files.
type RemoteSearchFilesResponse struct {
	Truncated bool         `json:"truncated,omitempty"`
	Files     []RemoteFile `json:"files"`
}

// Query options for remote API calls

// RemoteSymbolSearchOptions contains options for symbol search queries.
type RemoteSymbolSearchOptions struct {
	Query    string
	Limit    int
	Language string
	Kind     string
}

// RemoteSymbolListOptions contains options for symbol list queries.
type RemoteSymbolListOptions struct {
	Limit    int
	Cursor   string
	Language string
	Kind     string
	File     string
}

// RemoteRefOptions contains options for reference queries.
type RemoteRefOptions struct {
	Limit      int
	Cursor     string
	FromFile   string
	ToSymbolID string
	Language   string
}

// RemoteCallGraphOptions contains options for call graph queries.
type RemoteCallGraphOptions struct {
	Limit      int
	Cursor     string
	CallerID   string
	CalleeID   string
	CallerFile string
	Language   string
}

// Hybrid query results (local + remote)

// HybridSymbolResult represents a symbol result with source attribution.
type HybridSymbolResult struct {
	Symbol     RemoteSymbol `json:"symbol"`
	Source     string       `json:"source"`      // "local" or server name
	RepoID     string       `json:"repo_id"`     // Repository identifier
	ServerURL  string       `json:"server_url"`  // URL for remote, empty for local
	ServerName string       `json:"server_name"` // Server name for remote, empty for local
}

// HybridRefResult represents a reference result with source attribution.
type HybridRefResult struct {
	Ref        RemoteRef `json:"ref"`
	Source     string    `json:"source"`
	RepoID     string    `json:"repo_id"`
	ServerURL  string    `json:"server_url"`
	ServerName string    `json:"server_name"`
}

// HybridCallEdgeResult represents a call edge result with source attribution.
type HybridCallEdgeResult struct {
	Edge       RemoteCallEdge `json:"edge"`
	Source     string         `json:"source"`
	RepoID     string         `json:"repo_id"`
	ServerURL  string         `json:"server_url"`
	ServerName string         `json:"server_name"`
}

// QuerySource represents a source that was queried.
type QuerySource struct {
	Name        string        `json:"name"`   // "local" or server name
	URL         string        `json:"url"`    // Empty for local
	Status      string        `json:"status"` // "success", "error", "timeout", "unavailable"
	ResultCount int           `json:"result_count"`
	Latency     time.Duration `json:"latency"`
}

// QueryError represents an error from a query source.
type QueryError struct {
	Source  string `json:"source"`
	URL     string `json:"url"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// HybridSearchResult is the combined result from local + remote sources.
type HybridSearchResult struct {
	Results   []HybridSymbolResult `json:"results"`
	Sources   []QuerySource        `json:"sources"`
	Errors    []QueryError         `json:"errors"`
	Truncated bool                 `json:"truncated"`
}

// HybridRefsResult is the combined refs result from local + remote sources.
type HybridRefsResult struct {
	Results []HybridRefResult `json:"results"`
	Sources []QuerySource     `json:"sources"`
	Errors  []QueryError      `json:"errors"`
}

// HybridCallGraphResult is the combined call graph result from local + remote sources.
type HybridCallGraphResult struct {
	Results []HybridCallEdgeResult `json:"results"`
	Sources []QuerySource          `json:"sources"`
	Errors  []QueryError           `json:"errors"`
}
