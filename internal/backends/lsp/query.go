package lsp

import (
	"context"
	"fmt"
	"time"

	"ckb/internal/errors"
)

// LspRequest represents a request to an LSP server
type LspRequest struct {
	// Method is the LSP method name
	Method string

	// Params are the request parameters
	Params interface{}

	// Response channel for the result
	Response chan *LspResponse

	// Context for cancellation
	Context context.Context

	// CreatedAt tracks when the request was created
	CreatedAt time.Time
}

// LspResponse represents a response from an LSP server
type LspResponse struct {
	// Result is the response data
	Result interface{}

	// Error is set if the request failed
	Error error

	// Duration is how long the request took
	Duration time.Duration
}

// NewLspRequest creates a new LSP request
func NewLspRequest(ctx context.Context, method string, params interface{}) *LspRequest {
	return &LspRequest{
		Method:    method,
		Params:    params,
		Response:  make(chan *LspResponse, 1),
		Context:   ctx,
		CreatedAt: time.Now(),
	}
}

// Query sends a query to an LSP server and waits for the response
func (s *LspSupervisor) Query(languageId, method string, params interface{}) (interface{}, error) {
	return s.QueryWithContext(context.Background(), languageId, method, params)
}

// QueryWithContext sends a query with a context for cancellation
func (s *LspSupervisor) QueryWithContext(ctx context.Context, languageId, method string, params interface{}) (interface{}, error) {
	// Check if server is configured
	if _, ok := s.config.Backends.Lsp.Servers[languageId]; !ok {
		return nil, errors.NewCkbError(
			errors.BackendUnavailable,
			fmt.Sprintf("no LSP server configured for language: %s", languageId),
			nil,
			nil,
			nil,
		)
	}

	// Start server if not running
	if !s.IsReady(languageId) {
		if err := s.StartServer(languageId); err != nil {
			return nil, fmt.Errorf("failed to start LSP server: %w", err)
		}
	}

	// Create request
	req := NewLspRequest(ctx, method, params)

	// Enqueue request
	if err := s.enqueue(languageId, req); err != nil {
		return nil, err
	}

	// Wait for response
	select {
	case resp := <-req.Response:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, errors.NewCkbError(
			errors.Timeout,
			"LSP query cancelled",
			ctx.Err(),
			nil,
			nil,
		)
	}
}

// QueryDefinition queries for symbol definition
func (s *LspSupervisor) QueryDefinition(ctx context.Context, languageId, uri string, line, character int) (interface{}, error) {
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri,
		},
		"position": map[string]interface{}{
			"line":      line,
			"character": character,
		},
	}

	return s.QueryWithContext(ctx, languageId, "textDocument/definition", params)
}

// QueryReferences queries for symbol references
func (s *LspSupervisor) QueryReferences(ctx context.Context, languageId, uri string, line, character int, includeDeclaration bool) (interface{}, error) {
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri,
		},
		"position": map[string]interface{}{
			"line":      line,
			"character": character,
		},
		"context": map[string]interface{}{
			"includeDeclaration": includeDeclaration,
		},
	}

	return s.QueryWithContext(ctx, languageId, "textDocument/references", params)
}

// QueryDocumentSymbols queries for symbols in a document
func (s *LspSupervisor) QueryDocumentSymbols(ctx context.Context, languageId, uri string) (interface{}, error) {
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri,
		},
	}

	return s.QueryWithContext(ctx, languageId, "textDocument/documentSymbol", params)
}

// QueryWorkspaceSymbols queries for symbols in the workspace
func (s *LspSupervisor) QueryWorkspaceSymbols(ctx context.Context, languageId, query string) (interface{}, error) {
	params := map[string]interface{}{
		"query": query,
	}

	return s.QueryWithContext(ctx, languageId, "workspace/symbol", params)
}

// QueryHover queries for hover information
func (s *LspSupervisor) QueryHover(ctx context.Context, languageId, uri string, line, character int) (interface{}, error) {
	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri,
		},
		"position": map[string]interface{}{
			"line":      line,
			"character": character,
		},
	}

	return s.QueryWithContext(ctx, languageId, "textDocument/hover", params)
}

// NotifyDocumentOpen notifies the LSP server that a document was opened
func (s *LspSupervisor) NotifyDocumentOpen(languageId, uri, languageIdText, text string, version int) error {
	proc := s.GetProcess(languageId)
	if proc == nil || !proc.IsHealthy() {
		return errors.NewCkbError(
			errors.BackendUnavailable,
			fmt.Sprintf("LSP server not available for language: %s", languageId),
			nil,
			nil,
			nil,
		)
	}

	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        uri,
			"languageId": languageIdText,
			"version":    version,
			"text":       text,
		},
	}

	return proc.sendNotification("textDocument/didOpen", params)
}

// NotifyDocumentClose notifies the LSP server that a document was closed
func (s *LspSupervisor) NotifyDocumentClose(languageId, uri string) error {
	proc := s.GetProcess(languageId)
	if proc == nil || !proc.IsHealthy() {
		return nil // Ignore if server not running
	}

	params := map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri,
		},
	}

	return proc.sendNotification("textDocument/didClose", params)
}

// executeRequest executes a request on the LSP process
func (s *LspSupervisor) executeRequest(languageId string, req *LspRequest) *LspResponse {
	startTime := time.Now()

	proc := s.GetProcess(languageId)
	if proc == nil || !proc.IsHealthy() {
		return &LspResponse{
			Error: errors.NewCkbError(
				errors.BackendUnavailable,
				fmt.Sprintf("LSP server not available for language: %s", languageId),
				nil,
				nil,
				nil,
			),
			Duration: time.Since(startTime),
		}
	}

	// Send request
	result, err := proc.sendRequest(req.Method, req.Params)

	resp := &LspResponse{
		Result:   result,
		Error:    err,
		Duration: time.Since(startTime),
	}

	// Track success/failure
	if err != nil {
		proc.RecordFailure()
		s.logger.Error("LSP request failed", map[string]interface{}{
			"languageId": languageId,
			"method":     req.Method,
			"error":      err.Error(),
		})
	} else {
		proc.RecordSuccess()
		s.logger.Debug("LSP request succeeded", map[string]interface{}{
			"languageId": languageId,
			"method":     req.Method,
			"duration":   resp.Duration.Milliseconds(),
		})
	}

	return resp
}
