package mcp

import (
	"github.com/SimplyLiz/CodeMCP/internal/cartographer"
	"github.com/SimplyLiz/CodeMCP/internal/errors"
	"github.com/SimplyLiz/CodeMCP/internal/envelope"
)

// v8.6 Cartographer context tool implementations.

// toolQueryContext runs Cartographer's PKG retrieval pipeline:
// BM25 content search → personalized PageRank skeleton → context health.
// Returns a ready-to-inject context bundle graded A–F.
func (s *MCPServer) toolQueryContext(params map[string]interface{}) (*envelope.Response, error) {
	if !cartographer.Available() {
		return nil, errors.NewOperationError("query context", cartographer.ErrUnavailable)
	}

	query, ok := params["query"].(string)
	if !ok || query == "" {
		return nil, errors.NewInvalidParameterError("query", "required")
	}

	opts := &cartographer.QueryContextOpts{}
	if v, ok := params["budget"].(float64); ok && v > 0 {
		opts.Budget = int(v)
	}
	if v, ok := params["model"].(string); ok && v != "" {
		opts.Model = v
	}
	if v, ok := params["maxSearchResults"].(float64); ok && v > 0 {
		opts.MaxSearchResults = int(v)
	}

	repoRoot := s.engine().GetRepoRoot()
	result, err := cartographer.QueryContext(repoRoot, query, opts)
	if err != nil {
		return nil, errors.NewOperationError("query context", err)
	}

	return NewToolResponse().Data(result).Build(), nil
}

// toolContextHealth scores a context bundle on 6 research-backed metrics,
// returning a composite 0–100 score graded A–F with per-metric breakdown.
func (s *MCPServer) toolContextHealth(params map[string]interface{}) (*envelope.Response, error) {
	if !cartographer.Available() {
		return nil, errors.NewOperationError("context health", cartographer.ErrUnavailable)
	}

	content, ok := params["content"].(string)
	if !ok || content == "" {
		return nil, errors.NewInvalidParameterError("content", "required")
	}

	opts := &cartographer.ContextHealthOpts{}
	if v, ok := params["model"].(string); ok && v != "" {
		opts.Model = v
	}
	if v, ok := params["signatureCount"].(float64); ok && v > 0 {
		opts.SignatureCount = int(v)
	}

	result, err := cartographer.ContextHealth(content, opts)
	if err != nil {
		return nil, errors.NewOperationError("context health", err)
	}

	return NewToolResponse().Data(result).Build(), nil
}
