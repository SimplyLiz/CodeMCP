package streaming

import (
	"encoding/json"
	"reflect"
)

// Chunker batches items into appropriately-sized chunks.
type Chunker struct {
	chunkSize    int
	maxByteSize  int
	currentChunk []interface{}
	currentBytes int
}

// ChunkerConfig configures chunker behavior.
type ChunkerConfig struct {
	ChunkSize   int // Max items per chunk (default: 20)
	MaxByteSize int // Max bytes per chunk (default: 16KB)
}

// DefaultChunkerConfig returns default chunker configuration.
func DefaultChunkerConfig() ChunkerConfig {
	return ChunkerConfig{
		ChunkSize:   20,
		MaxByteSize: 16 * 1024, // 16KB
	}
}

// NewChunker creates a new chunker.
func NewChunker(config ChunkerConfig) *Chunker {
	if config.ChunkSize <= 0 {
		config.ChunkSize = 20
	}
	if config.MaxByteSize <= 0 {
		config.MaxByteSize = 16 * 1024
	}

	return &Chunker{
		chunkSize:    config.ChunkSize,
		maxByteSize:  config.MaxByteSize,
		currentChunk: make([]interface{}, 0, config.ChunkSize),
	}
}

// Add adds an item to the current chunk and returns true if chunk is full.
func (c *Chunker) Add(item interface{}) bool {
	itemBytes := estimateSize(item)
	c.currentChunk = append(c.currentChunk, item)
	c.currentBytes += itemBytes

	return len(c.currentChunk) >= c.chunkSize || c.currentBytes >= c.maxByteSize
}

// Flush returns the current chunk and resets.
func (c *Chunker) Flush() []interface{} {
	chunk := c.currentChunk
	c.currentChunk = make([]interface{}, 0, c.chunkSize)
	c.currentBytes = 0
	return chunk
}

// HasItems returns true if there are pending items.
func (c *Chunker) HasItems() bool {
	return len(c.currentChunk) > 0
}

// Count returns the number of pending items.
func (c *Chunker) Count() int {
	return len(c.currentChunk)
}

// estimateSize estimates JSON size of an item.
func estimateSize(item interface{}) int {
	data, err := json.Marshal(item)
	if err != nil {
		return 100 // Conservative fallback
	}
	return len(data)
}

// ChunkSlice chunks a slice into batches.
func ChunkSlice[T any](items []T, chunkSize int) [][]T {
	if chunkSize <= 0 {
		chunkSize = 20
	}

	var chunks [][]T
	for i := 0; i < len(items); i += chunkSize {
		end := i + chunkSize
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[i:end])
	}
	return chunks
}

// ChunkByBytes chunks a slice respecting byte size limits.
func ChunkByBytes[T any](items []T, maxBytes int) [][]T {
	if maxBytes <= 0 {
		maxBytes = 16 * 1024
	}

	var chunks [][]T
	var current []T
	currentBytes := 0

	for _, item := range items {
		itemBytes := estimateSize(item)

		// If single item exceeds limit, put it in its own chunk
		if itemBytes >= maxBytes {
			if len(current) > 0 {
				chunks = append(chunks, current)
				current = nil
				currentBytes = 0
			}
			chunks = append(chunks, []T{item})
			continue
		}

		// Check if adding item would exceed limit
		if currentBytes+itemBytes > maxBytes && len(current) > 0 {
			chunks = append(chunks, current)
			current = nil
			currentBytes = 0
		}

		current = append(current, item)
		currentBytes += itemBytes
	}

	if len(current) > 0 {
		chunks = append(chunks, current)
	}

	return chunks
}

// StreamSlice streams a slice through a stream, chunking automatically.
func StreamSlice[T any](stream *Stream, items []T, groupKey string) error {
	if len(items) == 0 {
		return stream.SendDone(false)
	}

	chunks := ChunkSlice(items, stream.ChunkSize())
	total := len(items)

	// Send metadata
	if err := stream.SendMeta(MetaData{Total: total}); err != nil {
		return err
	}

	// Send chunks
	for i, chunk := range chunks {
		hasMore := i < len(chunks)-1

		// Create chunk data with the appropriate key
		data := map[string]interface{}{
			groupKey: chunk,
		}

		if err := stream.SendChunk(data, len(chunk), hasMore); err != nil {
			return err
		}

		// Send progress every few chunks
		if i > 0 && i%5 == 0 {
			sent := 0
			for j := 0; j <= i; j++ {
				sent += len(chunks[j])
			}
			if err := stream.SendProgress("streaming", sent, total); err != nil {
				return err
			}
		}
	}

	return stream.SendDone(false)
}

// ToInterfaceSlice converts a typed slice to []interface{}.
func ToInterfaceSlice(slice interface{}) []interface{} {
	v := reflect.ValueOf(slice)
	if v.Kind() != reflect.Slice {
		return nil
	}

	result := make([]interface{}, v.Len())
	for i := 0; i < v.Len(); i++ {
		result[i] = v.Index(i).Interface()
	}
	return result
}
