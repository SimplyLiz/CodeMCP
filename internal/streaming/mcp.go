package streaming

import (
	"context"
	"sync"
)

// MCPNotificationSender is the interface for sending MCP notifications.
type MCPNotificationSender interface {
	SendNotification(method string, params interface{}) error
}

// MCPStreamWriter adapts streaming to MCP notification protocol.
type MCPStreamWriter struct {
	stream *Stream
	sender MCPNotificationSender
	wg     sync.WaitGroup
}

// StreamChunkNotification is sent for each chunk.
type StreamChunkNotification struct {
	StreamID string      `json:"streamId"`
	Sequence int         `json:"sequence"`
	Chunk    interface{} `json:"chunk"`
}

// StreamProgressNotification is sent for progress updates.
type StreamProgressNotification struct {
	StreamID   string  `json:"streamId"`
	Phase      string  `json:"phase"`
	Current    int     `json:"current,omitempty"`
	Total      int     `json:"total,omitempty"`
	Percentage float64 `json:"percentage,omitempty"`
}

// StreamCompleteNotification is sent when streaming finishes.
type StreamCompleteNotification struct {
	StreamID   string `json:"streamId"`
	TotalItems int    `json:"totalItems"`
	ElapsedMs  int64  `json:"elapsedMs"`
	Truncated  bool   `json:"truncated"`
}

// StreamErrorNotification is sent on fatal error.
type StreamErrorNotification struct {
	StreamID    string `json:"streamId"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// MCP notification methods.
const (
	MethodStreamChunk    = "ckb/streamChunk"
	MethodStreamProgress = "ckb/streamProgress"
	MethodStreamComplete = "ckb/streamComplete"
	MethodStreamError    = "ckb/streamError"
)

// NewMCPStreamWriter creates a writer that sends stream events as MCP notifications.
func NewMCPStreamWriter(ctx context.Context, sender MCPNotificationSender, config StreamConfig) *MCPStreamWriter {
	w := &MCPStreamWriter{
		stream: NewStream(ctx, config),
		sender: sender,
	}

	// Start event forwarding goroutine
	w.wg.Add(1)
	go w.forwardEvents()

	return w
}

// Stream returns the underlying stream.
func (w *MCPStreamWriter) Stream() *Stream {
	return w.stream
}

// Wait waits for all events to be forwarded.
func (w *MCPStreamWriter) Wait() {
	w.wg.Wait()
}

// forwardEvents reads events from the stream and sends as MCP notifications.
func (w *MCPStreamWriter) forwardEvents() {
	defer w.wg.Done()

	for event := range w.stream.Events() {
		switch event.Type {
		case EventChunk:
			if chunk, ok := event.Data.(ChunkData); ok {
				_ = w.sender.SendNotification(MethodStreamChunk, StreamChunkNotification{
					StreamID: w.stream.ID,
					Sequence: chunk.Sequence,
					Chunk:    chunk.Items,
				})
			}

		case EventProgress:
			if progress, ok := event.Data.(ProgressData); ok {
				_ = w.sender.SendNotification(MethodStreamProgress, StreamProgressNotification{
					StreamID:   w.stream.ID,
					Phase:      progress.Phase,
					Current:    progress.Current,
					Total:      progress.Total,
					Percentage: progress.Percentage,
				})
			}

		case EventDone:
			if done, ok := event.Data.(DoneData); ok {
				_ = w.sender.SendNotification(MethodStreamComplete, StreamCompleteNotification{
					StreamID:   w.stream.ID,
					TotalItems: done.TotalItems,
					ElapsedMs:  done.Elapsed.Milliseconds(),
					Truncated:  done.Truncated,
				})
			}

		case EventError:
			if errData, ok := event.Data.(ErrorData); ok {
				_ = w.sender.SendNotification(MethodStreamError, StreamErrorNotification{
					StreamID:    w.stream.ID,
					Code:        errData.Code,
					Message:     errData.Message,
					Remediation: errData.Remediation,
				})
			}

		case EventHeartbeat:
			// Heartbeats not forwarded via MCP - connection kept alive by transport

		case EventMeta, EventWarning:
			// These are included in initial response, not as notifications
		}
	}
}

// InitialResponse creates the initial response for a streaming request.
type InitialResponse struct {
	StreamID  string      `json:"streamId"`
	Streaming bool        `json:"streaming"`
	Meta      *MetaData   `json:"meta,omitempty"`
	Warnings  []string    `json:"warnings,omitempty"`
	Data      interface{} `json:"data,omitempty"` // Optional initial data
}

// StreamableTools lists tools that support streaming.
var StreamableTools = []string{
	"findReferences",
	"searchSymbols",
	"getArchitecture",
	"explore",
	"understand",
	"prepareChange",
}

// IsStreamable returns true if the tool supports streaming.
func IsStreamable(toolName string) bool {
	for _, name := range StreamableTools {
		if name == toolName {
			return true
		}
	}
	return false
}

// StreamCapabilities returns streaming capability info for getStatus.
type StreamCapabilities struct {
	Enabled bool     `json:"enabled"`
	Tools   []string `json:"tools"`
}

// GetStreamCapabilities returns current streaming capabilities.
func GetStreamCapabilities() StreamCapabilities {
	return StreamCapabilities{
		Enabled: true,
		Tools:   StreamableTools,
	}
}
