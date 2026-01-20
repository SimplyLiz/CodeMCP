package mcp

import (
	"context"
	"encoding/json"
	"sync"

	"ckb/internal/envelope"
	"ckb/internal/streaming"
)

// StreamingHandler wraps a tool handler to support streaming responses.
type StreamingHandler func(params map[string]interface{}, stream *streaming.Stream) (*envelope.Response, error)

// StreamableToolInfo contains metadata about a streamable tool.
type StreamableToolInfo struct {
	Handler    StreamingHandler
	GroupKey   string // Key for grouping items in chunks (e.g., "references", "symbols")
	Streamable bool
}

// streamableTools maps tool names to their streaming info.
var (
	streamableTools   = map[string]StreamableToolInfo{}
	streamableToolsMu sync.RWMutex
)

// RegisterStreamableHandler registers a tool as streamable.
func (s *MCPServer) RegisterStreamableHandler(name string, handler StreamingHandler, groupKey string) {
	streamableToolsMu.Lock()
	defer streamableToolsMu.Unlock()
	streamableTools[name] = StreamableToolInfo{
		Handler:    handler,
		GroupKey:   groupKey,
		Streamable: true,
	}
}

// IsStreamable returns true if the tool supports streaming.
func IsStreamable(toolName string) bool {
	streamableToolsMu.RLock()
	defer streamableToolsMu.RUnlock()
	_, ok := streamableTools[toolName]
	return ok
}

// StreamingConfig returns streaming configuration from tool params.
func StreamingConfig(params map[string]interface{}) (enabled bool, chunkSize int) {
	if s, ok := params["stream"].(bool); ok {
		enabled = s
	}
	if cs, ok := params["chunkSize"].(float64); ok {
		chunkSize = int(cs)
	}
	if chunkSize <= 0 {
		chunkSize = 20 // Default
	}
	return
}

// handleStreamingCall handles a streaming tool call.
func (s *MCPServer) handleStreamingCall(toolName string, params map[string]interface{}) (*envelope.Response, error) {
	streamableToolsMu.RLock()
	info, ok := streamableTools[toolName]
	streamableToolsMu.RUnlock()
	if !ok {
		return nil, nil // Not streamable
	}

	streamEnabled, chunkSize := StreamingConfig(params)
	if !streamEnabled {
		return nil, nil // Client didn't request streaming
	}

	// Create stream
	ctx := context.Background()
	config := streaming.StreamConfig{
		ChunkSize:       chunkSize,
		MaxBuffer:       100,
		HeartbeatPeriod: 15 * 1000 * 1000 * 1000, // 15 seconds in nanoseconds
	}

	writer := streaming.NewMCPStreamWriter(ctx, s, config)
	stream := writer.Stream()

	// Run handler in goroutine
	go func() {
		defer writer.Wait()

		_, err := info.Handler(params, stream)
		if err != nil {
			_ = stream.SendError("OPERATION_FAILED", err.Error(), "Check logs for details")
		}
	}()

	// Return initial response with stream ID
	initialResp := streaming.InitialResponse{
		StreamID:  stream.ID,
		Streaming: true,
		Meta: &streaming.MetaData{
			ChunkSize: chunkSize,
		},
	}

	return envelope.Operational(initialResp), nil
}

// StreamCapabilities returns streaming capability info for getStatus.
type StreamCapabilities struct {
	Enabled bool     `json:"enabled"`
	Tools   []string `json:"tools"`
}

// GetStreamCapabilities returns current streaming capabilities.
func GetStreamCapabilities() StreamCapabilities {
	streamableToolsMu.RLock()
	defer streamableToolsMu.RUnlock()
	tools := make([]string, 0, len(streamableTools))
	for name := range streamableTools {
		tools = append(tools, name)
	}
	return StreamCapabilities{
		Enabled: true,
		Tools:   tools,
	}
}

// StreamChunkResult wraps streamed data for MCP response.
type StreamChunkResult struct {
	StreamID string      `json:"streamId"`
	Sequence int         `json:"sequence"`
	Data     interface{} `json:"data"`
	HasMore  bool        `json:"hasMore"`
}

// StreamCompleteResult signals stream completion.
type StreamCompleteResult struct {
	StreamID   string `json:"streamId"`
	TotalItems int    `json:"totalItems"`
	ElapsedMs  int64  `json:"elapsedMs"`
	Truncated  bool   `json:"truncated"`
}

// StreamToolResponse is the initial response for a streaming tool call.
type StreamToolResponse struct {
	StreamID  string                 `json:"streamId"`
	Streaming bool                   `json:"streaming"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

// wrapForStreaming checks if a tool call should use streaming and handles it.
// Returns nil, nil if streaming is not requested or not supported.
func (s *MCPServer) wrapForStreaming(toolName string, params map[string]interface{}) (*envelope.Response, error) {
	streamEnabled, _ := StreamingConfig(params)
	if !streamEnabled {
		return nil, nil
	}

	if !IsStreamable(toolName) {
		// Tool doesn't support streaming, return warning in normal response
		return nil, nil
	}

	return s.handleStreamingCall(toolName, params)
}

// MarshalStreamEvent marshals a streaming event for MCP notification.
func MarshalStreamEvent(event streaming.Event) ([]byte, error) {
	return json.Marshal(event)
}
