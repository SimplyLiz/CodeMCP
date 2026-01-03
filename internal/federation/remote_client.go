// Package federation provides cross-repository federation capabilities for CKB.
// This file implements the HTTP client for remote index servers (Phase 5).
package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RemoteClient is an HTTP client for communicating with remote index servers.
type RemoteClient struct {
	server *RemoteServer
	client *http.Client
	logger *slog.Logger
	index  *Index
}

// NewRemoteClient creates a new client for a remote server.
func NewRemoteClient(server *RemoteServer, index *Index, logger *slog.Logger) *RemoteClient {
	return &RemoteClient{
		server: server,
		client: &http.Client{
			Timeout: server.GetTimeout(),
		},
		logger: logger,
		index:  index,
	}
}

// retryConfig configures retry behavior.
type retryConfig struct {
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
}

// defaultRetryConfig returns the default retry configuration.
func defaultRetryConfig() retryConfig {
	return retryConfig{
		maxRetries: DefaultMaxRetries,
		baseDelay:  DefaultRetryBaseDelay,
		maxDelay:   5 * time.Second,
	}
}

// doRequest performs an HTTP request with retry logic.
func (c *RemoteClient) doRequest(ctx context.Context, method, path string, body io.Reader, query url.Values) (*http.Response, error) {
	cfg := defaultRetryConfig()

	// Build URL
	u, err := url.Parse(c.server.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}
	u.Path = path
	if query != nil {
		u.RawQuery = query.Encode()
	}

	var lastErr error
	for attempt := 0; attempt <= cfg.maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff delay with exponential increase
			delay := cfg.baseDelay * time.Duration(1<<uint(attempt-1))
			if delay > cfg.maxDelay {
				delay = cfg.maxDelay
			}

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}

			if c.logger != nil {
				c.logger.Debug("Retrying request",
					"server", c.server.Name,
					"attempt", attempt+1,
					"url", u.String(),
				)
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "ckb-federation-client/1.0")

		// Add authentication if configured
		if token := c.server.GetToken(); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			// Retry on network errors
			continue
		}

		// Don't retry on client errors (4xx), only on server errors (5xx) and specific cases
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return resp, nil
		}

		// Retry on server errors
		if resp.StatusCode >= 500 {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request failed after %d retries: %w", cfg.maxRetries, lastErr)
}

// get performs a GET request and returns the response body.
func (c *RemoteClient) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Limit response size
	limitedReader := io.LimitReader(resp.Body, DefaultMaxBodySize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for error response
	if resp.StatusCode >= 400 {
		return nil, c.parseErrorResponse(resp.StatusCode, data)
	}

	return data, nil
}

// post performs a POST request and returns the response body.
func (c *RemoteClient) post(ctx context.Context, path string, body interface{}, query url.Values) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = strings.NewReader(string(data))
	}

	resp, err := c.doRequest(ctx, http.MethodPost, path, bodyReader, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// Limit response size
	limitedReader := io.LimitReader(resp.Body, DefaultMaxBodySize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for error response
	if resp.StatusCode >= 400 {
		return nil, c.parseErrorResponse(resp.StatusCode, data)
	}

	return data, nil
}

// parseErrorResponse extracts error information from a response.
func (c *RemoteClient) parseErrorResponse(statusCode int, body []byte) error {
	var resp RemoteResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		// Couldn't parse as JSON, return generic error
		return &RemoteError{
			StatusCode: statusCode,
			Code:       "unknown_error",
			Message:    string(body),
		}
	}

	if resp.Error != nil {
		return &RemoteError{
			StatusCode: statusCode,
			Code:       resp.Error.Code,
			Message:    resp.Error.Message,
		}
	}

	return &RemoteError{
		StatusCode: statusCode,
		Code:       "unknown_error",
		Message:    fmt.Sprintf("HTTP %d", statusCode),
	}
}

// RemoteError represents an error from a remote server.
type RemoteError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("remote error %d (%s): %s", e.StatusCode, e.Code, e.Message)
}

// IsNotFound returns true if the error is a 404 not found error.
func (e *RemoteError) IsNotFound() bool {
	return e.StatusCode == http.StatusNotFound
}

// IsUnauthorized returns true if the error is a 401 unauthorized error.
func (e *RemoteError) IsUnauthorized() bool {
	return e.StatusCode == http.StatusUnauthorized
}

// IsForbidden returns true if the error is a 403 forbidden error.
func (e *RemoteError) IsForbidden() bool {
	return e.StatusCode == http.StatusForbidden
}

// IsRateLimited returns true if the error is a 429 rate limit error.
func (e *RemoteError) IsRateLimited() bool {
	return e.StatusCode == http.StatusTooManyRequests
}

// parseResponse parses a remote response and extracts the data.
func parseResponse[T any](body []byte) (*T, *RemoteResponseMeta, error) {
	var resp struct {
		Data  T                   `json:"data"`
		Meta  *RemoteResponseMeta `json:"meta,omitempty"`
		Error *RemoteErrorInfo    `json:"error,omitempty"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.Error != nil {
		return nil, nil, &RemoteError{
			Code:    resp.Error.Code,
			Message: resp.Error.Message,
		}
	}

	return &resp.Data, resp.Meta, nil
}

// Server returns the remote server configuration.
func (c *RemoteClient) Server() *RemoteServer {
	return c.server
}

// Ping checks if the remote server is reachable.
func (c *RemoteClient) Ping(ctx context.Context) error {
	_, err := c.get(ctx, "/index/repos", nil)
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	return nil
}

// ListRepos lists all repositories on the remote server.
func (c *RemoteClient) ListRepos(ctx context.Context) ([]RemoteRepoInfo, error) {
	body, err := c.get(ctx, "/index/repos", nil)
	if err != nil {
		return nil, err
	}

	data, _, err := parseResponse[RemoteListReposResponse](body)
	if err != nil {
		return nil, err
	}

	return data.Repos, nil
}

// GetRepoMeta gets metadata for a specific repository.
func (c *RemoteClient) GetRepoMeta(ctx context.Context, repoID string) (*RemoteRepoMeta, error) {
	path := fmt.Sprintf("/index/repos/%s/meta", url.PathEscape(repoID))
	body, err := c.get(ctx, path, nil)
	if err != nil {
		return nil, err
	}

	data, _, err := parseResponse[RemoteRepoMeta](body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// SearchSymbols searches for symbols in a repository.
func (c *RemoteClient) SearchSymbols(ctx context.Context, repoID string, opts *RemoteSymbolSearchOptions) ([]RemoteSymbol, bool, error) {
	path := fmt.Sprintf("/index/repos/%s/search/symbols", url.PathEscape(repoID))

	query := url.Values{}
	query.Set("q", opts.Query)
	if opts.Limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", opts.Limit))
	}
	if opts.Language != "" {
		query.Set("language", opts.Language)
	}
	if opts.Kind != "" {
		query.Set("kind", opts.Kind)
	}

	body, err := c.get(ctx, path, query)
	if err != nil {
		return nil, false, err
	}

	data, _, err := parseResponse[RemoteSearchSymbolsResponse](body)
	if err != nil {
		return nil, false, err
	}

	return data.Symbols, data.Truncated, nil
}

// ListSymbols lists symbols in a repository with pagination.
func (c *RemoteClient) ListSymbols(ctx context.Context, repoID string, opts *RemoteSymbolListOptions) ([]RemoteSymbol, string, int, error) {
	path := fmt.Sprintf("/index/repos/%s/symbols", url.PathEscape(repoID))

	query := url.Values{}
	if opts.Limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", opts.Limit))
	}
	if opts.Cursor != "" {
		query.Set("cursor", opts.Cursor)
	}
	if opts.Language != "" {
		query.Set("language", opts.Language)
	}
	if opts.Kind != "" {
		query.Set("kind", opts.Kind)
	}
	if opts.File != "" {
		query.Set("file", opts.File)
	}

	body, err := c.get(ctx, path, query)
	if err != nil {
		return nil, "", 0, err
	}

	data, meta, err := parseResponse[RemoteListSymbolsResponse](body)
	if err != nil {
		return nil, "", 0, err
	}

	var nextCursor string
	var total int
	if meta != nil {
		nextCursor = meta.Cursor
		total = meta.Total
	}

	return data.Symbols, nextCursor, total, nil
}

// GetSymbol gets a specific symbol by ID.
func (c *RemoteClient) GetSymbol(ctx context.Context, repoID, symbolID string) (*RemoteSymbol, error) {
	path := fmt.Sprintf("/index/repos/%s/symbols/%s", url.PathEscape(repoID), url.PathEscape(symbolID))
	body, err := c.get(ctx, path, nil)
	if err != nil {
		return nil, err
	}

	data, _, err := parseResponse[RemoteGetSymbolResponse](body)
	if err != nil {
		return nil, err
	}

	return &data.Symbol, nil
}

// BatchGetSymbols gets multiple symbols by ID.
func (c *RemoteClient) BatchGetSymbols(ctx context.Context, repoID string, ids []string) ([]RemoteSymbol, []string, error) {
	path := fmt.Sprintf("/index/repos/%s/symbols:batchGet", url.PathEscape(repoID))

	req := RemoteBatchGetRequest{IDs: ids}
	body, err := c.post(ctx, path, req, nil)
	if err != nil {
		return nil, nil, err
	}

	data, _, err := parseResponse[RemoteBatchGetResponse](body)
	if err != nil {
		return nil, nil, err
	}

	return data.Symbols, data.NotFound, nil
}

// ListRefs lists references in a repository with pagination.
func (c *RemoteClient) ListRefs(ctx context.Context, repoID string, opts *RemoteRefOptions) ([]RemoteRef, string, error) {
	path := fmt.Sprintf("/index/repos/%s/refs", url.PathEscape(repoID))

	query := url.Values{}
	if opts.Limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", opts.Limit))
	}
	if opts.Cursor != "" {
		query.Set("cursor", opts.Cursor)
	}
	if opts.FromFile != "" {
		query.Set("from_file", opts.FromFile)
	}
	if opts.ToSymbolID != "" {
		query.Set("to_symbol_id", opts.ToSymbolID)
	}
	if opts.Language != "" {
		query.Set("language", opts.Language)
	}

	body, err := c.get(ctx, path, query)
	if err != nil {
		return nil, "", err
	}

	data, meta, err := parseResponse[RemoteListRefsResponse](body)
	if err != nil {
		return nil, "", err
	}

	var nextCursor string
	if meta != nil {
		nextCursor = meta.Cursor
	}

	return data.Refs, nextCursor, nil
}

// ListCallGraph lists call graph edges in a repository with pagination.
func (c *RemoteClient) ListCallGraph(ctx context.Context, repoID string, opts *RemoteCallGraphOptions) ([]RemoteCallEdge, string, error) {
	path := fmt.Sprintf("/index/repos/%s/callgraph", url.PathEscape(repoID))

	query := url.Values{}
	if opts.Limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", opts.Limit))
	}
	if opts.Cursor != "" {
		query.Set("cursor", opts.Cursor)
	}
	if opts.CallerID != "" {
		query.Set("caller_id", opts.CallerID)
	}
	if opts.CalleeID != "" {
		query.Set("callee_id", opts.CalleeID)
	}
	if opts.CallerFile != "" {
		query.Set("caller_file", opts.CallerFile)
	}
	if opts.Language != "" {
		query.Set("language", opts.Language)
	}

	body, err := c.get(ctx, path, query)
	if err != nil {
		return nil, "", err
	}

	data, meta, err := parseResponse[RemoteListCallgraphResponse](body)
	if err != nil {
		return nil, "", err
	}

	var nextCursor string
	if meta != nil {
		nextCursor = meta.Cursor
	}

	return data.Edges, nextCursor, nil
}

// ListFiles lists files in a repository with pagination.
func (c *RemoteClient) ListFiles(ctx context.Context, repoID string, limit int, cursor string) ([]RemoteFile, string, error) {
	path := fmt.Sprintf("/index/repos/%s/files", url.PathEscape(repoID))

	query := url.Values{}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	if cursor != "" {
		query.Set("cursor", cursor)
	}

	body, err := c.get(ctx, path, query)
	if err != nil {
		return nil, "", err
	}

	data, meta, err := parseResponse[RemoteListFilesResponse](body)
	if err != nil {
		return nil, "", err
	}

	var nextCursor string
	if meta != nil {
		nextCursor = meta.Cursor
	}

	return data.Files, nextCursor, nil
}

// SearchFiles searches for files in a repository.
func (c *RemoteClient) SearchFiles(ctx context.Context, repoID, query string, limit int) ([]RemoteFile, bool, error) {
	path := fmt.Sprintf("/index/repos/%s/search/files", url.PathEscape(repoID))

	params := url.Values{}
	params.Set("q", query)
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}

	body, err := c.get(ctx, path, params)
	if err != nil {
		return nil, false, err
	}

	data, _, err := parseResponse[RemoteSearchFilesResponse](body)
	if err != nil {
		return nil, false, err
	}

	return data.Files, data.Truncated, nil
}
