package streaming

import (
	"context"
	"testing"
	"time"
)

func TestNewStream(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stream := NewStream(ctx, DefaultConfig())
	defer stream.Close()

	if stream.ID == "" {
		t.Error("stream should have an ID")
	}

	if stream.ChunkSize() != 20 {
		t.Errorf("expected chunk size 20, got %d", stream.ChunkSize())
	}

	if stream.IsClosed() {
		t.Error("stream should not be closed initially")
	}
}

func TestStreamSendMeta(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stream := NewStream(ctx, DefaultConfig())
	defer stream.Close()

	go func() {
		err := stream.SendMeta(MetaData{
			Total:      100,
			Backends:   []string{"scip", "git"},
			Confidence: 0.95,
		})
		if err != nil {
			t.Errorf("SendMeta failed: %v", err)
		}
	}()

	select {
	case event := <-stream.Events():
		if event.Type != EventMeta {
			t.Errorf("expected EventMeta, got %s", event.Type)
		}
		meta, ok := event.Data.(MetaData)
		if !ok {
			t.Error("expected MetaData")
			return
		}
		if meta.Total != 100 {
			t.Errorf("expected total 100, got %d", meta.Total)
		}
		if meta.ChunkSize != 20 {
			t.Errorf("expected chunk size 20, got %d", meta.ChunkSize)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for meta event")
	}
}

func TestStreamSendChunk(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stream := NewStream(ctx, DefaultConfig())
	defer stream.Close()

	go func() {
		items := []string{"a", "b", "c"}
		err := stream.SendChunk(items, len(items), true)
		if err != nil {
			t.Errorf("SendChunk failed: %v", err)
		}
	}()

	select {
	case event := <-stream.Events():
		if event.Type != EventChunk {
			t.Errorf("expected EventChunk, got %s", event.Type)
		}
		chunk, ok := event.Data.(ChunkData)
		if !ok {
			t.Error("expected ChunkData")
			return
		}
		if chunk.Sequence != 1 {
			t.Errorf("expected sequence 1, got %d", chunk.Sequence)
		}
		if chunk.Count != 3 {
			t.Errorf("expected count 3, got %d", chunk.Count)
		}
		if !chunk.HasMore {
			t.Error("expected hasMore to be true")
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for chunk event")
	}
}

func TestStreamSendProgress(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stream := NewStream(ctx, DefaultConfig())
	defer stream.Close()

	go func() {
		err := stream.SendProgress("searching", 50, 100)
		if err != nil {
			t.Errorf("SendProgress failed: %v", err)
		}
	}()

	select {
	case event := <-stream.Events():
		if event.Type != EventProgress {
			t.Errorf("expected EventProgress, got %s", event.Type)
		}
		progress, ok := event.Data.(ProgressData)
		if !ok {
			t.Error("expected ProgressData")
			return
		}
		if progress.Phase != "searching" {
			t.Errorf("expected phase 'searching', got %s", progress.Phase)
		}
		if progress.Percentage != 50 {
			t.Errorf("expected percentage 50, got %f", progress.Percentage)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for progress event")
	}
}

func TestStreamSendDone(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stream := NewStream(ctx, DefaultConfig())

	// Send some items first
	go func() {
		_ = stream.SendChunk([]string{"a"}, 1, false)
		_ = stream.SendDone(false)
	}()

	// Consume events
	events := make([]Event, 0)
	for event := range stream.Events() {
		events = append(events, event)
	}

	if len(events) < 2 {
		t.Errorf("expected at least 2 events, got %d", len(events))
		return
	}

	lastEvent := events[len(events)-1]
	if lastEvent.Type != EventDone {
		t.Errorf("expected last event to be EventDone, got %s", lastEvent.Type)
	}

	if !stream.IsClosed() {
		t.Error("stream should be closed after SendDone")
	}
}

func TestStreamSendError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stream := NewStream(ctx, DefaultConfig())

	go func() {
		_ = stream.SendError("RESOURCE_NOT_FOUND", "Symbol not found", "Check symbol ID")
	}()

	// Consume events
	events := make([]Event, 0)
	for event := range stream.Events() {
		events = append(events, event)
	}

	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
		return
	}

	if events[0].Type != EventError {
		t.Errorf("expected EventError, got %s", events[0].Type)
	}

	if !stream.IsClosed() {
		t.Error("stream should be closed after SendError")
	}
}

func TestStreamClose(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stream := NewStream(ctx, DefaultConfig())

	stream.Close()

	if !stream.IsClosed() {
		t.Error("stream should be closed")
	}

	// Should be safe to close again
	stream.Close()

	// Sending should fail
	err := stream.SendMeta(MetaData{})
	if err == nil {
		t.Error("expected error sending to closed stream")
	}
}

func TestStreamContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	stream := NewStream(ctx, DefaultConfig())

	cancel()

	// Give some time for cancellation to propagate
	time.Sleep(50 * time.Millisecond)

	err := stream.SendMeta(MetaData{})
	if err == nil {
		t.Error("expected error when context is cancelled")
	}
}

func TestStreamMultipleChunks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	stream := NewStream(ctx, DefaultConfig())

	go func() {
		for i := 0; i < 5; i++ {
			hasMore := i < 4
			_ = stream.SendChunk([]int{i}, 1, hasMore)
		}
		_ = stream.SendDone(false)
	}()

	chunkCount := 0
	for event := range stream.Events() {
		if event.Type == EventChunk {
			chunk := event.Data.(ChunkData)
			if chunk.Sequence != chunkCount+1 {
				t.Errorf("expected sequence %d, got %d", chunkCount+1, chunk.Sequence)
			}
			chunkCount++
		}
	}

	if chunkCount != 5 {
		t.Errorf("expected 5 chunks, got %d", chunkCount)
	}
}
