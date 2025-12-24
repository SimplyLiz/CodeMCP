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
