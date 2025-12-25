package mcp

import (
	"testing"

	"ckb/internal/telemetry"
)

func TestMCPErrorError(t *testing.T) {
	tests := []struct {
		name    string
		err     *MCPError
		wantMsg string
	}{
		{
			name:    "simple message",
			err:     &MCPError{Code: ParseError, Message: "parse error"},
			wantMsg: "parse error",
		},
		{
			name:    "empty message",
			err:     &MCPError{Code: InternalError, Message: ""},
			wantMsg: "",
		},
		{
			name:    "with data",
			err:     &MCPError{Code: InvalidParams, Message: "invalid params", Data: map[string]string{"field": "query"}},
			wantMsg: "invalid params",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.wantMsg {
				t.Errorf("MCPError.Error() = %q, want %q", got, tt.wantMsg)
			}
		})
	}
}

func TestComputePeriodFilter(t *testing.T) {
	tests := []struct {
		period       string
		wantNonEmpty bool
	}{
		{"7d", true},
		{"30d", true},
		{"90d", true},
		{"all", false},
		{"unknown", true}, // defaults to 90d
		{"", true},        // defaults to 90d
	}

	for _, tt := range tests {
		t.Run(tt.period, func(t *testing.T) {
			result := computePeriodFilter(tt.period)
			if tt.wantNonEmpty && result == "" {
				t.Errorf("computePeriodFilter(%q) = empty, want non-empty", tt.period)
			}
			if !tt.wantNonEmpty && result != "" {
				t.Errorf("computePeriodFilter(%q) = %q, want empty", tt.period, result)
			}
		})
	}
}

func TestExtractSymbolName(t *testing.T) {
	tests := []struct {
		symbolID string
		want     string
	}{
		{"ckb:repo:sym:abc123", "ckb:repo:sym:abc123"},
		{"simple", "simple"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.symbolID, func(t *testing.T) {
			result := extractSymbolName(tt.symbolID)
			if result != tt.want {
				t.Errorf("extractSymbolName(%q) = %q, want %q", tt.symbolID, result, tt.want)
			}
		})
	}
}

func TestComputeTrend(t *testing.T) {
	tests := []struct {
		name   string
		usages []telemetry.ObservedUsage
		want   telemetry.UsageTrend
	}{
		{
			name:   "empty",
			usages: []telemetry.ObservedUsage{},
			want:   telemetry.TrendStable,
		},
		{
			name:   "single element",
			usages: []telemetry.ObservedUsage{{CallCount: 100}},
			want:   telemetry.TrendStable,
		},
		{
			name: "stable usage",
			usages: []telemetry.ObservedUsage{
				{CallCount: 100},
				{CallCount: 100},
				{CallCount: 100},
				{CallCount: 100},
			},
			want: telemetry.TrendStable,
		},
		{
			name: "increasing usage",
			usages: []telemetry.ObservedUsage{
				{CallCount: 200},
				{CallCount: 200},
				{CallCount: 50},
				{CallCount: 50},
			},
			want: telemetry.TrendIncreasing,
		},
		{
			name: "decreasing usage",
			usages: []telemetry.ObservedUsage{
				{CallCount: 50},
				{CallCount: 50},
				{CallCount: 200},
				{CallCount: 200},
			},
			want: telemetry.TrendDecreasing,
		},
		{
			name: "new calls from zero",
			usages: []telemetry.ObservedUsage{
				{CallCount: 100},
				{CallCount: 0},
			},
			want: telemetry.TrendIncreasing,
		},
		{
			name: "both halves zero",
			usages: []telemetry.ObservedUsage{
				{CallCount: 0},
				{CallCount: 0},
			},
			want: telemetry.TrendStable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeTrend(tt.usages)
			if result != tt.want {
				t.Errorf("computeTrend() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestComputeBlendedConfidence(t *testing.T) {
	tests := []struct {
		name               string
		staticConfidence   float64
		observedConfidence float64
		wantMin            float64
		wantMax            float64
	}{
		{
			name:               "static higher",
			staticConfidence:   0.9,
			observedConfidence: 0.7,
			wantMin:            0.9,
			wantMax:            1.0,
		},
		{
			name:               "observed higher",
			staticConfidence:   0.7,
			observedConfidence: 0.9,
			wantMin:            0.9,
			wantMax:            1.0,
		},
		{
			name:               "both agree high",
			staticConfidence:   0.8,
			observedConfidence: 0.8,
			wantMin:            0.83, // 0.8 + 0.03 boost
			wantMax:            0.84,
		},
		{
			name:               "both low no boost",
			staticConfidence:   0.4,
			observedConfidence: 0.4,
			wantMin:            0.4,
			wantMax:            0.41,
		},
		{
			name:               "cap at 1.0",
			staticConfidence:   0.99,
			observedConfidence: 0.99,
			wantMin:            1.0,
			wantMax:            1.0,
		},
		{
			name:               "both zero",
			staticConfidence:   0.0,
			observedConfidence: 0.0,
			wantMin:            0.0,
			wantMax:            0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeBlendedConfidence(tt.staticConfidence, tt.observedConfidence)
			if result < tt.wantMin || result > tt.wantMax {
				t.Errorf("computeBlendedConfidence(%f, %f) = %f, want between %f and %f",
					tt.staticConfidence, tt.observedConfidence, result, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello..."},
		{"hello world", 5, "he..."},
		{"", 10, ""},
		{"ab", 5, "ab"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncateString(tt.input, tt.max)
			if result != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.max, result, tt.want)
			}
		})
	}
}

func TestToolMetricsSummaryAvgBytes(t *testing.T) {
	tests := []struct {
		name    string
		summary ToolMetricsSummary
		wantAvg float64
	}{
		{
			name:    "zero queries",
			summary: ToolMetricsSummary{QueryCount: 0, TotalBytes: 1000},
			wantAvg: 0,
		},
		{
			name:    "simple average",
			summary: ToolMetricsSummary{QueryCount: 10, TotalBytes: 1000},
			wantAvg: 100,
		},
		{
			name:    "fractional average",
			summary: ToolMetricsSummary{QueryCount: 3, TotalBytes: 100},
			wantAvg: 33.333333,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.summary.AvgBytes()
			// Allow small floating point difference
			diff := result - tt.wantAvg
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.001 {
				t.Errorf("AvgBytes() = %f, want ~%f", result, tt.wantAvg)
			}
		})
	}
}

func TestErrorCodeConstants(t *testing.T) {
	// Verify standard JSON-RPC error codes
	if ParseError != -32700 {
		t.Errorf("ParseError = %d, want -32700", ParseError)
	}
	if InvalidRequest != -32600 {
		t.Errorf("InvalidRequest = %d, want -32600", InvalidRequest)
	}
	if MethodNotFound != -32601 {
		t.Errorf("MethodNotFound = %d, want -32601", MethodNotFound)
	}
	if InvalidParams != -32602 {
		t.Errorf("InvalidParams = %d, want -32602", InvalidParams)
	}
	if InternalError != -32603 {
		t.Errorf("InternalError = %d, want -32603", InternalError)
	}
}

func TestMaxMessageSizeConstant(t *testing.T) {
	// MaxMessageSize should be 1MB
	if MaxMessageSize != 1024*1024 {
		t.Errorf("MaxMessageSize = %d, want %d (1MB)", MaxMessageSize, 1024*1024)
	}
}

func TestMCPMessageIsRequest(t *testing.T) {
	tests := []struct {
		name string
		msg  MCPMessage
		want bool
	}{
		{
			name: "request",
			msg:  MCPMessage{Method: "test", Id: 1},
			want: true,
		},
		{
			name: "notification (no id)",
			msg:  MCPMessage{Method: "test", Id: nil},
			want: false,
		},
		{
			name: "response",
			msg:  MCPMessage{Result: "ok", Id: 1},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.msg.IsRequest(); got != tt.want {
				t.Errorf("IsRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMCPMessageIsNotification(t *testing.T) {
	tests := []struct {
		name string
		msg  MCPMessage
		want bool
	}{
		{
			name: "notification",
			msg:  MCPMessage{Method: "test", Id: nil},
			want: true,
		},
		{
			name: "request (has id)",
			msg:  MCPMessage{Method: "test", Id: 1},
			want: false,
		},
		{
			name: "response",
			msg:  MCPMessage{Result: "ok", Id: 1},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.msg.IsNotification(); got != tt.want {
				t.Errorf("IsNotification() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMCPMessageIsResponse(t *testing.T) {
	tests := []struct {
		name string
		msg  MCPMessage
		want bool
	}{
		{
			name: "success response",
			msg:  MCPMessage{Result: "ok", Id: 1},
			want: true,
		},
		{
			name: "error response",
			msg:  MCPMessage{Error: &MCPError{Code: -1, Message: "err"}, Id: 1},
			want: true,
		},
		{
			name: "request",
			msg:  MCPMessage{Method: "test", Id: 1},
			want: false,
		},
		{
			name: "notification",
			msg:  MCPMessage{Method: "test"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.msg.IsResponse(); got != tt.want {
				t.Errorf("IsResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ============================================================================
// Preset Tests
// ============================================================================

func TestValidPresets(t *testing.T) {
	presets := ValidPresets()

	// Should return all defined presets
	expected := []string{"core", "review", "refactor", "federation", "docs", "ops", "full"}
	if len(presets) != len(expected) {
		t.Errorf("ValidPresets() returned %d presets, want %d", len(presets), len(expected))
	}

	for _, exp := range expected {
		found := false
		for _, p := range presets {
			if p == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ValidPresets() missing %q", exp)
		}
	}
}

func TestIsValidPreset(t *testing.T) {
	tests := []struct {
		preset string
		want   bool
	}{
		{"core", true},
		{"review", true},
		{"refactor", true},
		{"federation", true},
		{"docs", true},
		{"ops", true},
		{"full", true},
		{"invalid", false},
		{"", false},
		{"CORE", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			if got := IsValidPreset(tt.preset); got != tt.want {
				t.Errorf("IsValidPreset(%q) = %v, want %v", tt.preset, got, tt.want)
			}
		})
	}
}

func TestGetPresetTools(t *testing.T) {
	t.Run("core preset", func(t *testing.T) {
		tools := GetPresetTools("core")
		if len(tools) == 0 {
			t.Error("GetPresetTools(core) returned empty")
		}
		// Core should include essential tools
		hasSearch := false
		for _, tool := range tools {
			if tool == "searchSymbols" {
				hasSearch = true
				break
			}
		}
		if !hasSearch {
			t.Error("Core preset missing searchSymbols")
		}
	})

	t.Run("invalid preset returns core", func(t *testing.T) {
		tools := GetPresetTools("invalid")
		coreTools := GetPresetTools("core")
		if len(tools) != len(coreTools) {
			t.Errorf("GetPresetTools(invalid) = %d tools, want %d (core)", len(tools), len(coreTools))
		}
	})

	t.Run("full preset has wildcard", func(t *testing.T) {
		tools := GetPresetTools("full")
		if len(tools) != 1 || tools[0] != "*" {
			t.Errorf("GetPresetTools(full) = %v, want [*]", tools)
		}
	})
}

func TestFilterAndOrderTools(t *testing.T) {
	// Create mock tools
	allTools := []Tool{
		{Name: "zebra"},
		{Name: "searchSymbols"},
		{Name: "apple"},
		{Name: "getSymbol"},
		{Name: "explainSymbol"},
	}

	t.Run("core preset filters and orders", func(t *testing.T) {
		result := FilterAndOrderTools(allTools, "core")
		// Should only include core tools, not zebra/apple
		for _, tool := range result {
			if tool.Name == "zebra" || tool.Name == "apple" {
				t.Errorf("FilterAndOrderTools should filter out %q", tool.Name)
			}
		}
	})

	t.Run("full preset keeps all tools", func(t *testing.T) {
		result := FilterAndOrderTools(allTools, "full")
		if len(result) != len(allTools) {
			t.Errorf("FilterAndOrderTools(full) = %d tools, want %d", len(result), len(allTools))
		}
	})

	t.Run("core tools ordered first", func(t *testing.T) {
		result := FilterAndOrderTools(allTools, "full")
		// searchSymbols should come before zebra/apple
		searchIdx := -1
		zebraIdx := -1
		for i, tool := range result {
			if tool.Name == "searchSymbols" {
				searchIdx = i
			}
			if tool.Name == "zebra" {
				zebraIdx = i
			}
		}
		if searchIdx == -1 {
			t.Error("searchSymbols not found in result")
		}
		if zebraIdx == -1 {
			t.Error("zebra not found in result")
		}
		if searchIdx > zebraIdx {
			t.Errorf("searchSymbols (idx %d) should come before zebra (idx %d)", searchIdx, zebraIdx)
		}
	})
}

func TestComputeToolsetHash(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		tools := []Tool{
			{Name: "a", Description: "desc a"},
			{Name: "b", Description: "desc b"},
		}
		hash1 := ComputeToolsetHash(tools)
		hash2 := ComputeToolsetHash(tools)
		if hash1 != hash2 {
			t.Errorf("ComputeToolsetHash not deterministic: %q != %q", hash1, hash2)
		}
	})

	t.Run("order independent", func(t *testing.T) {
		tools1 := []Tool{
			{Name: "a", Description: "desc a"},
			{Name: "b", Description: "desc b"},
		}
		tools2 := []Tool{
			{Name: "b", Description: "desc b"},
			{Name: "a", Description: "desc a"},
		}
		hash1 := ComputeToolsetHash(tools1)
		hash2 := ComputeToolsetHash(tools2)
		if hash1 != hash2 {
			t.Errorf("ComputeToolsetHash should be order independent: %q != %q", hash1, hash2)
		}
	})

	t.Run("different content different hash", func(t *testing.T) {
		tools1 := []Tool{{Name: "a", Description: "desc"}}
		tools2 := []Tool{{Name: "a", Description: "different"}}
		hash1 := ComputeToolsetHash(tools1)
		hash2 := ComputeToolsetHash(tools2)
		if hash1 == hash2 {
			t.Error("ComputeToolsetHash should differ for different descriptions")
		}
	})

	t.Run("empty tools", func(t *testing.T) {
		hash := ComputeToolsetHash([]Tool{})
		if len(hash) != 10 {
			t.Errorf("ComputeToolsetHash(empty) = %q, want 10-char hash", hash)
		}
	})
}

// ============================================================================
// Cursor Tests
// ============================================================================

func TestEncodeToolsCursor(t *testing.T) {
	t.Run("encodes valid cursor", func(t *testing.T) {
		cursor := EncodeToolsCursor("core", 15, "abc123")
		if cursor == "" {
			t.Error("EncodeToolsCursor returned empty string")
		}
	})

	t.Run("deterministic", func(t *testing.T) {
		c1 := EncodeToolsCursor("core", 15, "abc123")
		c2 := EncodeToolsCursor("core", 15, "abc123")
		if c1 != c2 {
			t.Errorf("EncodeToolsCursor not deterministic: %q != %q", c1, c2)
		}
	})
}

func TestDecodeToolsCursor(t *testing.T) {
	t.Run("empty cursor returns 0", func(t *testing.T) {
		offset, err := DecodeToolsCursor("", "core", "hash")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if offset != 0 {
			t.Errorf("DecodeToolsCursor('') = %d, want 0", offset)
		}
	})

	t.Run("roundtrip", func(t *testing.T) {
		preset := "review"
		expectedOffset := 30
		hash := "testhash"

		encoded := EncodeToolsCursor(preset, expectedOffset, hash)
		offset, err := DecodeToolsCursor(encoded, preset, hash)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if offset != expectedOffset {
			t.Errorf("roundtrip offset = %d, want %d", offset, expectedOffset)
		}
	})

	t.Run("preset mismatch error", func(t *testing.T) {
		encoded := EncodeToolsCursor("core", 15, "hash")
		_, err := DecodeToolsCursor(encoded, "review", "hash")
		if err == nil {
			t.Error("expected error for preset mismatch")
		}
	})

	t.Run("hash mismatch error", func(t *testing.T) {
		encoded := EncodeToolsCursor("core", 15, "hash1")
		_, err := DecodeToolsCursor(encoded, "core", "hash2")
		if err == nil {
			t.Error("expected error for hash mismatch")
		}
	})

	t.Run("invalid encoding error", func(t *testing.T) {
		_, err := DecodeToolsCursor("not-valid-base64!!!", "core", "hash")
		if err == nil {
			t.Error("expected error for invalid encoding")
		}
	})

	t.Run("invalid json error", func(t *testing.T) {
		// Valid base64 but not valid JSON
		_, err := DecodeToolsCursor("bm90LWpzb24", "core", "hash")
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestPaginateTools(t *testing.T) {
	tools := make([]Tool, 50)
	for i := range tools {
		tools[i] = Tool{Name: string(rune('a' + i%26))}
	}

	t.Run("first page", func(t *testing.T) {
		page, cursor, err := PaginateTools(tools, 0, 15, "core", "hash")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(page) != 15 {
			t.Errorf("first page = %d tools, want 15", len(page))
		}
		if cursor == "" {
			t.Error("expected next cursor for first page")
		}
	})

	t.Run("last page no cursor", func(t *testing.T) {
		page, cursor, err := PaginateTools(tools, 45, 15, "core", "hash")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(page) != 5 {
			t.Errorf("last page = %d tools, want 5", len(page))
		}
		if cursor != "" {
			t.Errorf("last page should have no cursor, got %q", cursor)
		}
	})

	t.Run("offset past end", func(t *testing.T) {
		page, cursor, err := PaginateTools(tools, 100, 15, "core", "hash")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(page) != 0 {
			t.Errorf("past-end page = %d tools, want 0", len(page))
		}
		if cursor != "" {
			t.Errorf("past-end page should have no cursor, got %q", cursor)
		}
	})

	t.Run("negative offset treated as 0", func(t *testing.T) {
		page, _, err := PaginateTools(tools, -5, 15, "core", "hash")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(page) != 15 {
			t.Errorf("negative offset page = %d tools, want 15", len(page))
		}
	})

	t.Run("zero pageSize uses default", func(t *testing.T) {
		page, _, err := PaginateTools(tools, 0, 0, "core", "hash")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(page) != DefaultPageSize {
			t.Errorf("zero pageSize = %d tools, want %d", len(page), DefaultPageSize)
		}
	})
}

// ============================================================================
// WideResultMetrics Additional Tests
// ============================================================================

func TestToolMetricsSummaryAvgMethods(t *testing.T) {
	t.Run("AvgTruncationRate", func(t *testing.T) {
		summary := &ToolMetricsSummary{TotalResults: 100, TotalTruncated: 25}
		if got := summary.AvgTruncationRate(); got != 0.25 {
			t.Errorf("AvgTruncationRate() = %f, want 0.25", got)
		}

		// Zero case
		zero := &ToolMetricsSummary{TotalResults: 0, TotalTruncated: 10}
		if got := zero.AvgTruncationRate(); got != 0 {
			t.Errorf("AvgTruncationRate(zero) = %f, want 0", got)
		}
	})

	t.Run("AvgTokens", func(t *testing.T) {
		summary := &ToolMetricsSummary{QueryCount: 10, TotalTokens: 1000}
		if got := summary.AvgTokens(); got != 100 {
			t.Errorf("AvgTokens() = %f, want 100", got)
		}

		// Zero case
		zero := &ToolMetricsSummary{QueryCount: 0, TotalTokens: 1000}
		if got := zero.AvgTokens(); got != 0 {
			t.Errorf("AvgTokens(zero) = %f, want 0", got)
		}
	})

	t.Run("AvgLatencyMs", func(t *testing.T) {
		summary := &ToolMetricsSummary{QueryCount: 10, TotalMs: 500}
		if got := summary.AvgLatencyMs(); got != 50 {
			t.Errorf("AvgLatencyMs() = %f, want 50", got)
		}

		// Zero case
		zero := &ToolMetricsSummary{QueryCount: 0, TotalMs: 500}
		if got := zero.AvgLatencyMs(); got != 0 {
			t.Errorf("AvgLatencyMs(zero) = %f, want 0", got)
		}
	})
}

func TestMeasureJSONSize(t *testing.T) {
	t.Run("simple struct", func(t *testing.T) {
		data := map[string]string{"key": "value"}
		size := MeasureJSONSize(data)
		if size == 0 {
			t.Error("MeasureJSONSize returned 0 for valid data")
		}
		// {"key":"value"} is 15 bytes
		if size != 15 {
			t.Errorf("MeasureJSONSize = %d, want 15", size)
		}
	})

	t.Run("unmarshalable returns 0", func(t *testing.T) {
		// Functions can't be marshaled
		size := MeasureJSONSize(func() {})
		if size != 0 {
			t.Errorf("MeasureJSONSize(func) = %d, want 0", size)
		}
	})
}
