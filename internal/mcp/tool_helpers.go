package mcp

import (
	"ckb/internal/envelope"
	"ckb/internal/output"
	"ckb/internal/query"
)

// ToolResponse is a convenience builder for MCP tool responses.
type ToolResponse struct {
	builder *envelope.Builder
}

// NewToolResponse creates a new tool response builder.
func NewToolResponse() *ToolResponse {
	return &ToolResponse{
		builder: envelope.New(),
	}
}

// Data sets the payload.
func (t *ToolResponse) Data(data interface{}) *ToolResponse {
	t.builder.Data(data)
	return t
}

// WithProvenance adds provenance metadata from a query response.
func (t *ToolResponse) WithProvenance(p *query.Provenance) *ToolResponse {
	t.builder.FromProvenance(p)
	return t
}

// WithTruncation adds truncation info.
func (t *ToolResponse) WithTruncation(truncated bool, shown, total int, reason string) *ToolResponse {
	t.builder.WithTruncation(truncated, shown, total, reason)
	return t
}

// WithFreshness adds index freshness info.
func (t *ToolResponse) WithFreshness(commitsBehind int, staleReason string) *ToolResponse {
	t.builder.WithFreshness(commitsBehind, staleReason)
	return t
}

// WithDrilldowns converts drilldowns to suggested calls.
func (t *ToolResponse) WithDrilldowns(drilldowns []output.Drilldown) *ToolResponse {
	t.builder.SuggestCalls(drilldowns)
	return t
}

// Warning adds a warning message.
func (t *ToolResponse) Warning(msg string) *ToolResponse {
	t.builder.Warning(msg)
	return t
}

// CrossRepo marks this as a cross-repo query.
func (t *ToolResponse) CrossRepo() *ToolResponse {
	t.builder.CrossRepo()
	return t
}

// Build returns the envelope response.
func (t *ToolResponse) Build() *envelope.Response {
	return t.builder.Build()
}

// OperationalResponse creates a simple envelope for operational tools.
// Use for tools like getStatus, doctor, daemonStatus that return factual state.
func OperationalResponse(data interface{}) *envelope.Response {
	return envelope.Operational(data)
}
