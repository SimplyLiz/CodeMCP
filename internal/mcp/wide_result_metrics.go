package mcp

import (
	"encoding/json"
	"sync"
	"time"

	"ckb/internal/storage"
)

// MeasureJSONSize returns the approximate byte size of a value when JSON-encoded
func MeasureJSONSize(v interface{}) int {
	data, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return len(data)
}

// WideResultMetrics captures data about wide-result tool responses
type WideResultMetrics struct {
	ToolName        string
	TotalResults    int
	ReturnedResults int
	TruncatedCount  int
	EstimatedTokens int
	ResponseBytes   int // Size of actual JSON response
	ExecutionMs     int64
}

// TruncationRate returns the percentage of results that were truncated (0.0-1.0)
func (m WideResultMetrics) TruncationRate() float64 {
	if m.TotalResults == 0 {
		return 0
	}
	return float64(m.TruncatedCount) / float64(m.TotalResults)
}

// WideResultAggregator collects metrics across tool invocations
type WideResultAggregator struct {
	mu      sync.Mutex
	metrics map[string]*ToolMetricsSummary
	db      *storage.DB // optional SQLite persistence
}

// ToolMetricsSummary holds aggregated stats for a single tool
type ToolMetricsSummary struct {
	ToolName       string  `json:"toolName"`
	QueryCount     int64   `json:"queryCount"`
	TotalResults   int64   `json:"totalResults"`
	TotalReturned  int64   `json:"totalReturned"`
	TotalTruncated int64   `json:"totalTruncated"`
	TotalTokens    int64   `json:"totalTokens"`
	TotalBytes     int64   `json:"totalBytes"`
	TotalMs        int64   `json:"totalMs"`
	AvgTruncation  float64 `json:"avgTruncationRate"` // computed on read
}

// AvgTruncationRate returns the average truncation rate
func (s *ToolMetricsSummary) AvgTruncationRate() float64 {
	if s.TotalResults == 0 {
		return 0
	}
	return float64(s.TotalTruncated) / float64(s.TotalResults)
}

// AvgTokens returns the average tokens per query
func (s *ToolMetricsSummary) AvgTokens() float64 {
	if s.QueryCount == 0 {
		return 0
	}
	return float64(s.TotalTokens) / float64(s.QueryCount)
}

// AvgLatencyMs returns the average latency in milliseconds
func (s *ToolMetricsSummary) AvgLatencyMs() float64 {
	if s.QueryCount == 0 {
		return 0
	}
	return float64(s.TotalMs) / float64(s.QueryCount)
}

// AvgBytes returns the average response bytes per query
func (s *ToolMetricsSummary) AvgBytes() float64 {
	if s.QueryCount == 0 {
		return 0
	}
	return float64(s.TotalBytes) / float64(s.QueryCount)
}

// Global aggregator instance
var globalWideResultAggregator = &WideResultAggregator{
	metrics: make(map[string]*ToolMetricsSummary),
}

// SetMetricsDB sets the database for persistent storage
// Call this during MCP server initialization
func SetMetricsDB(db *storage.DB) {
	globalWideResultAggregator.mu.Lock()
	defer globalWideResultAggregator.mu.Unlock()
	globalWideResultAggregator.db = db
}

// RecordWideResult records metrics for a wide-result tool invocation
func RecordWideResult(m WideResultMetrics) {
	globalWideResultAggregator.mu.Lock()
	defer globalWideResultAggregator.mu.Unlock()

	// In-memory aggregation
	summary, ok := globalWideResultAggregator.metrics[m.ToolName]
	if !ok {
		summary = &ToolMetricsSummary{ToolName: m.ToolName}
		globalWideResultAggregator.metrics[m.ToolName] = summary
	}

	summary.QueryCount++
	summary.TotalResults += int64(m.TotalResults)
	summary.TotalReturned += int64(m.ReturnedResults)
	summary.TotalTruncated += int64(m.TruncatedCount)
	summary.TotalTokens += int64(m.EstimatedTokens)
	summary.TotalBytes += int64(m.ResponseBytes)
	summary.TotalMs += m.ExecutionMs

	// SQLite persistence (non-blocking, errors are logged but not returned)
	if globalWideResultAggregator.db != nil {
		go func() {
			_ = globalWideResultAggregator.db.RecordWideResult(
				m.ToolName,
				m.TotalResults,
				m.ReturnedResults,
				m.TruncatedCount,
				m.EstimatedTokens,
				m.ResponseBytes,
				m.ExecutionMs,
			)
		}()
	}
}

// GetWideResultSummary returns a copy of all aggregated metrics
func GetWideResultSummary() map[string]*ToolMetricsSummary {
	globalWideResultAggregator.mu.Lock()
	defer globalWideResultAggregator.mu.Unlock()

	result := make(map[string]*ToolMetricsSummary, len(globalWideResultAggregator.metrics))
	for k, v := range globalWideResultAggregator.metrics {
		// Copy and compute derived fields
		copy := *v
		copy.AvgTruncation = v.AvgTruncationRate()
		result[k] = &copy
	}
	return result
}

// ResetWideResultMetrics clears all metrics (for testing)
func ResetWideResultMetrics() {
	globalWideResultAggregator.mu.Lock()
	defer globalWideResultAggregator.mu.Unlock()
	globalWideResultAggregator.metrics = make(map[string]*ToolMetricsSummary)
}

// EstimateTokens provides a rough token count from JSON byte size
// Conservative estimate: 1 token ~ 4 bytes for code/structured data
func EstimateTokens(jsonBytes int) int {
	return jsonBytes / 4
}

// WideResultTimer is a helper for timing tool execution
type WideResultTimer struct {
	start time.Time
}

// NewWideResultTimer starts a new timer
func NewWideResultTimer() *WideResultTimer {
	return &WideResultTimer{start: time.Now()}
}

// ElapsedMs returns elapsed milliseconds
func (t *WideResultTimer) ElapsedMs() int64 {
	return time.Since(t.start).Milliseconds()
}
