package mcp

import (
	"testing"

	"ckb/internal/logging"
)

func TestPresetFiltering(t *testing.T) {
	logger := logging.NewLogger(logging.Config{
		Level: logging.ErrorLevel,
	})

	server := NewMCPServer("test", nil, logger)

	// Test core preset (default)
	coreTools := server.GetFilteredTools()
	if len(coreTools) != 14 {
		t.Errorf("expected 14 core tools, got %d", len(coreTools))
	}

	// Verify core tools are in correct order (core-first)
	expectedFirst := []string{
		"searchSymbols", "getSymbol", "explainSymbol", "explainFile",
		"findReferences", "getCallGraph", "traceUsage",
		"getArchitecture", "getModuleOverview", "listKeyConcepts",
		"analyzeImpact", "getHotspots", "getStatus", "expandToolset",
	}
	for i, expected := range expectedFirst {
		if i >= len(coreTools) {
			t.Errorf("missing tool at position %d: %s", i, expected)
			continue
		}
		if coreTools[i].Name != expected {
			t.Errorf("position %d: expected %s, got %s", i, expected, coreTools[i].Name)
		}
	}

	// Test full preset
	if err := server.SetPreset("full"); err != nil {
		t.Fatalf("failed to set full preset: %v", err)
	}
	fullTools := server.GetFilteredTools()
	if len(fullTools) != 76 {
		t.Errorf("expected 76 full tools, got %d", len(fullTools))
	}

	// Full preset should still have core tools first
	for i, expected := range expectedFirst[:5] {
		if fullTools[i].Name != expected {
			t.Errorf("full preset position %d: expected %s, got %s", i, expected, fullTools[i].Name)
		}
	}
}

func TestPagination(t *testing.T) {
	logger := logging.NewLogger(logging.Config{
		Level: logging.ErrorLevel,
	})

	server := NewMCPServer("test", nil, logger)

	// Set to full preset for more tools to paginate
	_ = server.SetPreset("full")
	allTools := server.GetFilteredTools()
	hash := server.GetToolsetHash()

	// First page
	page1, cursor1, err := PaginateTools(allTools, 0, 15, "full", hash)
	if err != nil {
		t.Fatalf("pagination error: %v", err)
	}
	if len(page1) != 15 {
		t.Errorf("expected 15 tools on page 1, got %d", len(page1))
	}
	if cursor1 == "" {
		t.Error("expected nextCursor for page 1")
	}

	// Decode and validate cursor
	offset, err := DecodeToolsCursor(cursor1, "full", hash)
	if err != nil {
		t.Fatalf("failed to decode cursor: %v", err)
	}
	if offset != 15 {
		t.Errorf("expected offset 15, got %d", offset)
	}

	// Second page
	page2, cursor2, err := PaginateTools(allTools, offset, 15, "full", hash)
	if err != nil {
		t.Fatalf("pagination error: %v", err)
	}
	if len(page2) != 15 {
		t.Errorf("expected 15 tools on page 2, got %d", len(page2))
	}
	if cursor2 == "" {
		t.Error("expected nextCursor for page 2")
	}

	// Last page
	lastOffset := len(allTools) - 10
	lastPage, lastCursor, err := PaginateTools(allTools, lastOffset, 15, "full", hash)
	if err != nil {
		t.Fatalf("pagination error: %v", err)
	}
	if len(lastPage) != 10 {
		t.Errorf("expected 10 tools on last page, got %d", len(lastPage))
	}
	if lastCursor != "" {
		t.Error("expected no nextCursor for last page")
	}
}

func TestCursorInvalidation(t *testing.T) {
	// Test that cursor is invalid when preset changes
	hash := ComputeToolsetHash([]Tool{{Name: "test", Description: "test"}})

	cursor := EncodeToolsCursor("core", 15, hash)

	// Same preset and hash - should work
	offset, err := DecodeToolsCursor(cursor, "core", hash)
	if err != nil {
		t.Errorf("unexpected error for valid cursor: %v", err)
	}
	if offset != 15 {
		t.Errorf("expected offset 15, got %d", offset)
	}

	// Different preset - should fail
	_, err = DecodeToolsCursor(cursor, "full", hash)
	if err == nil {
		t.Error("expected error for mismatched preset")
	}

	// Different hash - should fail
	_, err = DecodeToolsCursor(cursor, "core", "different-hash")
	if err == nil {
		t.Error("expected error for mismatched hash")
	}

	// Invalid cursor string - should fail
	_, err = DecodeToolsCursor("invalid", "core", hash)
	if err == nil {
		t.Error("expected error for invalid cursor")
	}

	// Empty cursor - should return 0 (first page)
	offset, err = DecodeToolsCursor("", "core", hash)
	if err != nil {
		t.Errorf("unexpected error for empty cursor: %v", err)
	}
	if offset != 0 {
		t.Errorf("expected offset 0 for empty cursor, got %d", offset)
	}
}

func TestExpandToolsetRateLimit(t *testing.T) {
	logger := logging.NewLogger(logging.Config{
		Level: logging.ErrorLevel,
	})

	server := NewMCPServer("test", nil, logger)

	// Initially not expanded
	if server.IsExpanded() {
		t.Error("server should not be expanded initially")
	}

	// Mark as expanded
	server.MarkExpanded()

	if !server.IsExpanded() {
		t.Error("server should be expanded after MarkExpanded")
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		tokens   int
		expected string
	}{
		{0, "~0 tokens"},
		{500, "~500 tokens"},
		{999, "~999 tokens"},
		{1000, "~1k tokens"},  // (1000+500)/1000 = 1
		{1499, "~1k tokens"},  // (1499+500)/1000 = 1
		{1500, "~2k tokens"},  // (1500+500)/1000 = 2 (rounds up at .5)
		{1501, "~2k tokens"},  // (1501+500)/1000 = 2
		{9040, "~9k tokens"},  // (9040+500)/1000 = 9
		{9499, "~9k tokens"},  // (9499+500)/1000 = 9
		{9500, "~10k tokens"}, // (9500+500)/1000 = 10 (rounds up at .5)
	}

	for _, tc := range tests {
		result := FormatTokens(tc.tokens)
		if result != tc.expected {
			t.Errorf("FormatTokens(%d) = %q, want %q", tc.tokens, result, tc.expected)
		}
	}
}

func TestToolsetHash(t *testing.T) {
	tools1 := []Tool{
		{Name: "a", Description: "desc a"},
		{Name: "b", Description: "desc b"},
	}
	tools2 := []Tool{
		{Name: "a", Description: "desc a"},
		{Name: "b", Description: "desc b"},
	}
	tools3 := []Tool{
		{Name: "a", Description: "different desc"},
		{Name: "b", Description: "desc b"},
	}

	hash1 := ComputeToolsetHash(tools1)
	hash2 := ComputeToolsetHash(tools2)
	hash3 := ComputeToolsetHash(tools3)

	if hash1 != hash2 {
		t.Error("identical tools should have identical hashes")
	}
	if hash1 == hash3 {
		t.Error("different descriptions should have different hashes")
	}
}
