package streaming

import (
	"testing"
)

func TestChunkerAdd(t *testing.T) {
	t.Parallel()

	chunker := NewChunker(ChunkerConfig{ChunkSize: 3})

	// Add items
	if chunker.Add("a") {
		t.Error("chunk should not be full after 1 item")
	}
	if chunker.Add("b") {
		t.Error("chunk should not be full after 2 items")
	}
	if !chunker.Add("c") {
		t.Error("chunk should be full after 3 items")
	}

	// Flush
	chunk := chunker.Flush()
	if len(chunk) != 3 {
		t.Errorf("expected 3 items, got %d", len(chunk))
	}

	if chunker.HasItems() {
		t.Error("chunker should be empty after flush")
	}
}

func TestChunkerByteLimit(t *testing.T) {
	t.Parallel()

	// Very small byte limit
	chunker := NewChunker(ChunkerConfig{
		ChunkSize:   100, // High item limit
		MaxByteSize: 50,  // But low byte limit
	})

	// Add items that exceed byte limit
	largeItem := "this is a moderately long string that should trigger byte limit"
	if !chunker.Add(largeItem) {
		t.Error("chunk should be full due to byte limit")
	}
}

func TestChunkerFlush(t *testing.T) {
	t.Parallel()

	chunker := NewChunker(DefaultChunkerConfig())

	chunker.Add("a")
	chunker.Add("b")

	chunk := chunker.Flush()
	if len(chunk) != 2 {
		t.Errorf("expected 2 items, got %d", len(chunk))
	}

	if chunker.Count() != 0 {
		t.Error("chunker should be empty after flush")
	}
}

func TestChunkSlice(t *testing.T) {
	t.Parallel()

	items := []int{1, 2, 3, 4, 5, 6, 7}
	chunks := ChunkSlice(items, 3)

	if len(chunks) != 3 {
		t.Errorf("expected 3 chunks, got %d", len(chunks))
	}

	if len(chunks[0]) != 3 {
		t.Errorf("first chunk should have 3 items, got %d", len(chunks[0]))
	}
	if len(chunks[1]) != 3 {
		t.Errorf("second chunk should have 3 items, got %d", len(chunks[1]))
	}
	if len(chunks[2]) != 1 {
		t.Errorf("third chunk should have 1 item, got %d", len(chunks[2]))
	}
}

func TestChunkSliceEmpty(t *testing.T) {
	t.Parallel()

	items := []int{}
	chunks := ChunkSlice(items, 3)

	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty slice, got %d", len(chunks))
	}
}

func TestChunkSliceExact(t *testing.T) {
	t.Parallel()

	items := []int{1, 2, 3, 4, 5, 6}
	chunks := ChunkSlice(items, 3)

	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks, got %d", len(chunks))
	}

	for i, chunk := range chunks {
		if len(chunk) != 3 {
			t.Errorf("chunk %d should have 3 items, got %d", i, len(chunk))
		}
	}
}

func TestChunkByBytes(t *testing.T) {
	t.Parallel()

	items := []string{"short", "medium length", "a very long string that takes up space"}
	chunks := ChunkByBytes(items, 30)

	// Should split based on size
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}

	// Verify all items are present
	total := 0
	for _, chunk := range chunks {
		total += len(chunk)
	}
	if total != 3 {
		t.Errorf("expected 3 total items, got %d", total)
	}
}

func TestChunkByBytesLargeItem(t *testing.T) {
	t.Parallel()

	items := []string{"small", "this is a very large item that exceeds the byte limit on its own"}
	chunks := ChunkByBytes(items, 20)

	// Large item should be in its own chunk
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}
}

func TestToInterfaceSlice(t *testing.T) {
	t.Parallel()

	items := []int{1, 2, 3}
	result := ToInterfaceSlice(items)

	if len(result) != 3 {
		t.Errorf("expected 3 items, got %d", len(result))
	}

	for i, item := range result {
		if item.(int) != i+1 {
			t.Errorf("expected %d, got %v", i+1, item)
		}
	}
}

func TestToInterfaceSliceNonSlice(t *testing.T) {
	t.Parallel()

	result := ToInterfaceSlice("not a slice")

	if result != nil {
		t.Error("expected nil for non-slice input")
	}
}

type testItem struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func TestChunkSliceStructs(t *testing.T) {
	t.Parallel()

	items := []testItem{
		{ID: 1, Name: "one"},
		{ID: 2, Name: "two"},
		{ID: 3, Name: "three"},
		{ID: 4, Name: "four"},
		{ID: 5, Name: "five"},
	}

	chunks := ChunkSlice(items, 2)

	if len(chunks) != 3 {
		t.Errorf("expected 3 chunks, got %d", len(chunks))
	}

	// Verify first chunk
	if len(chunks[0]) != 2 {
		t.Errorf("first chunk should have 2 items, got %d", len(chunks[0]))
	}
	if chunks[0][0].ID != 1 {
		t.Errorf("first item should have ID 1, got %d", chunks[0][0].ID)
	}
}
