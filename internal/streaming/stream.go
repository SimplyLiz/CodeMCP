// Package streaming provides SSE-based streaming for large MCP responses.
// It enables progressive delivery of results to reduce time-to-first-result
// and memory pressure for large queries.
package streaming

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// EventType represents the type of streaming event.
type EventType string

const (
	// EventMeta provides initial metadata about the stream.
	EventMeta EventType = "meta"
	// EventChunk delivers a batch of results.
	EventChunk EventType = "chunk"
	// EventProgress reports progress updates.
	EventProgress EventType = "progress"
	// EventWarning reports non-fatal issues.
	EventWarning EventType = "warning"
	// EventDone signals stream completion.
	EventDone EventType = "done"
	// EventError signals a fatal error.
	EventError EventType = "error"
	// EventHeartbeat keeps connection alive.
	EventHeartbeat EventType = "heartbeat"
)

// Event represents a single streaming event.
type Event struct {
	Type EventType   `json:"type"`
	Data interface{} `json:"data"`
}

// MetaData contains initial stream metadata.
type MetaData struct {
	Total      int      `json:"total,omitempty"`      // Total items if known
	Backends   []string `json:"backends,omitempty"`   // Contributing backends
	Confidence float64  `json:"confidence,omitempty"` // Overall confidence
	ChunkSize  int      `json:"chunkSize"`            // Items per chunk
}

// ChunkData contains a batch of results.
type ChunkData struct {
	Sequence int         `json:"sequence"`        // Chunk sequence number
	Items    interface{} `json:"items"`           // The actual items
	Count    int         `json:"count"`           // Number of items in this chunk
	HasMore  bool        `json:"hasMore"`         // More chunks coming
}

// ProgressData reports progress.
type ProgressData struct {
	Phase      string  `json:"phase"`                // Current phase
	Current    int     `json:"current,omitempty"`    // Current item index
	Total      int     `json:"total,omitempty"`      // Total items if known
	Percentage float64 `json:"percentage,omitempty"` // Completion percentage
}

// DoneData signals stream completion.
type DoneData struct {
	TotalItems int           `json:"totalItems"`
	Elapsed    time.Duration `json:"elapsed"`
	Truncated  bool          `json:"truncated"`
}

// ErrorData contains error information.
type ErrorData struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// HeartbeatData keeps the connection alive.
type HeartbeatData struct {
	Sequence int `json:"seq"`
}

// Stream represents an active streaming session.
type Stream struct {
	ID        string
	StartedAt time.Time
	ctx       context.Context
	cancel    context.CancelFunc

	// Event channel for sending events
	events chan Event

	// Configuration
	chunkSize       int
	maxBuffer       int
	heartbeatPeriod time.Duration

	// State
	mu           sync.Mutex
	sequence     int
	totalSent    int
	closed       bool
	heartbeatSeq int
}

// StreamConfig configures stream behavior.
type StreamConfig struct {
	ChunkSize       int           // Items per chunk (default: 20)
	MaxBuffer       int           // Max buffered chunks (default: 100)
	HeartbeatPeriod time.Duration // Heartbeat interval (default: 15s)
}

// DefaultConfig returns default streaming configuration.
func DefaultConfig() StreamConfig {
	return StreamConfig{
		ChunkSize:       20,
		MaxBuffer:       100,
		HeartbeatPeriod: 15 * time.Second,
	}
}

// NewStream creates a new streaming session.
func NewStream(ctx context.Context, config StreamConfig) *Stream {
	if config.ChunkSize <= 0 {
		config.ChunkSize = 20
	}
	if config.MaxBuffer <= 0 {
		config.MaxBuffer = 100
	}
	if config.HeartbeatPeriod <= 0 {
		config.HeartbeatPeriod = 15 * time.Second
	}

	ctx, cancel := context.WithCancel(ctx)

	s := &Stream{
		ID:              generateStreamID(),
		StartedAt:       time.Now(),
		ctx:             ctx,
		cancel:          cancel,
		events:          make(chan Event, config.MaxBuffer),
		chunkSize:       config.ChunkSize,
		maxBuffer:       config.MaxBuffer,
		heartbeatPeriod: config.HeartbeatPeriod,
	}

	// Start heartbeat goroutine
	go s.heartbeatLoop()

	return s
}

// generateStreamID creates a unique stream identifier.
func generateStreamID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID
		return fmt.Sprintf("stream-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// Events returns the event channel for consumers.
func (s *Stream) Events() <-chan Event {
	return s.events
}

// Context returns the stream's context.
func (s *Stream) Context() context.Context {
	return s.ctx
}

// ChunkSize returns the configured chunk size.
func (s *Stream) ChunkSize() int {
	return s.chunkSize
}

// SendMeta sends stream metadata.
func (s *Stream) SendMeta(meta MetaData) error {
	meta.ChunkSize = s.chunkSize
	return s.send(Event{Type: EventMeta, Data: meta})
}

// SendChunk sends a chunk of items.
func (s *Stream) SendChunk(items interface{}, count int, hasMore bool) error {
	s.mu.Lock()
	s.sequence++
	seq := s.sequence
	s.totalSent += count
	s.mu.Unlock()

	return s.send(Event{
		Type: EventChunk,
		Data: ChunkData{
			Sequence: seq,
			Items:    items,
			Count:    count,
			HasMore:  hasMore,
		},
	})
}

// SendProgress sends a progress update.
func (s *Stream) SendProgress(phase string, current, total int) error {
	var pct float64
	if total > 0 {
		pct = float64(current) / float64(total) * 100
	}
	return s.send(Event{
		Type: EventProgress,
		Data: ProgressData{
			Phase:      phase,
			Current:    current,
			Total:      total,
			Percentage: pct,
		},
	})
}

// SendWarning sends a non-fatal warning.
func (s *Stream) SendWarning(message string) error {
	return s.send(Event{
		Type: EventWarning,
		Data: map[string]string{"message": message},
	})
}

// SendDone signals successful completion.
func (s *Stream) SendDone(truncated bool) error {
	s.mu.Lock()
	total := s.totalSent
	s.mu.Unlock()

	err := s.send(Event{
		Type: EventDone,
		Data: DoneData{
			TotalItems: total,
			Elapsed:    time.Since(s.StartedAt),
			Truncated:  truncated,
		},
	})

	s.Close()
	return err
}

// SendError signals a fatal error.
func (s *Stream) SendError(code, message, remediation string) error {
	err := s.send(Event{
		Type: EventError,
		Data: ErrorData{
			Code:        code,
			Message:     message,
			Remediation: remediation,
		},
	})

	s.Close()
	return err
}

// Close closes the stream.
func (s *Stream) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	s.cancel()
	close(s.events)
}

// IsClosed returns true if the stream is closed.
func (s *Stream) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// send sends an event to the channel.
func (s *Stream) send(event Event) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("stream closed")
	}
	s.mu.Unlock()

	// Check context first (important for buffered channels)
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	default:
	}

	select {
	case s.events <- event:
		return nil
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}

// heartbeatLoop sends periodic heartbeats.
func (s *Stream) heartbeatLoop() {
	ticker := time.NewTicker(s.heartbeatPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.closed {
				s.mu.Unlock()
				return
			}
			s.heartbeatSeq++
			seq := s.heartbeatSeq
			s.mu.Unlock()

			// Non-blocking send for heartbeat
			select {
			case s.events <- Event{Type: EventHeartbeat, Data: HeartbeatData{Sequence: seq}}:
			default:
				// Buffer full, skip heartbeat
			}
		}
	}
}

// MarshalJSON provides custom JSON marshaling for Event.
func (e Event) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type EventType   `json:"type"`
		Data interface{} `json:"data"`
	}{
		Type: e.Type,
		Data: e.Data,
	})
}
