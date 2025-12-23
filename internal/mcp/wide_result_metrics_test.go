package mcp

import (
	"sync"
	"testing"
)

func TestRecordWideResult(t *testing.T) {
	// Reset metrics before test
	ResetWideResultMetrics()

	// Record some metrics
	RecordWideResult(WideResultMetrics{
		ToolName:        "findReferences",
		TotalResults:    100,
		ReturnedResults: 50,
		TruncatedCount:  50,
		EstimatedTokens: 1000,
		ExecutionMs:     25,
	})

	RecordWideResult(WideResultMetrics{
		ToolName:        "findReferences",
		TotalResults:    200,
		ReturnedResults: 100,
		TruncatedCount:  100,
		EstimatedTokens: 2000,
		ExecutionMs:     50,
	})

	RecordWideResult(WideResultMetrics{
		ToolName:        "getCallGraph",
		TotalResults:    10,
		ReturnedResults: 10,
		TruncatedCount:  0,
		EstimatedTokens: 500,
		ExecutionMs:     10,
	})

	// Check aggregated metrics
	summary := GetWideResultSummary()

	// Check findReferences
	fr, ok := summary["findReferences"]
	if !ok {
		t.Fatal("findReferences not found in summary")
	}
	if fr.QueryCount != 2 {
		t.Errorf("expected QueryCount=2, got %d", fr.QueryCount)
	}
	if fr.TotalResults != 300 {
		t.Errorf("expected TotalResults=300, got %d", fr.TotalResults)
	}
	if fr.TotalReturned != 150 {
		t.Errorf("expected TotalReturned=150, got %d", fr.TotalReturned)
	}
	if fr.TotalTruncated != 150 {
		t.Errorf("expected TotalTruncated=150, got %d", fr.TotalTruncated)
	}
	if fr.TotalTokens != 3000 {
		t.Errorf("expected TotalTokens=3000, got %d", fr.TotalTokens)
	}
	if fr.TotalMs != 75 {
		t.Errorf("expected TotalMs=75, got %d", fr.TotalMs)
	}

	// Check getCallGraph
	cg, ok := summary["getCallGraph"]
	if !ok {
		t.Fatal("getCallGraph not found in summary")
	}
	if cg.QueryCount != 1 {
		t.Errorf("expected QueryCount=1, got %d", cg.QueryCount)
	}
	if cg.TotalTruncated != 0 {
		t.Errorf("expected TotalTruncated=0, got %d", cg.TotalTruncated)
	}
}

func TestToolMetricsSummaryCalculations(t *testing.T) {
	ResetWideResultMetrics()

	// Record metrics with known values
	RecordWideResult(WideResultMetrics{
		ToolName:        "testTool",
		TotalResults:    100,
		ReturnedResults: 25,
		TruncatedCount:  75,
		EstimatedTokens: 1000,
		ExecutionMs:     100,
	})

	RecordWideResult(WideResultMetrics{
		ToolName:        "testTool",
		TotalResults:    100,
		ReturnedResults: 25,
		TruncatedCount:  75,
		EstimatedTokens: 1000,
		ExecutionMs:     100,
	})

	summary := GetWideResultSummary()
	tt := summary["testTool"]

	// Check truncation rate: 150 / 200 = 0.75
	rate := tt.AvgTruncationRate()
	if rate < 0.74 || rate > 0.76 {
		t.Errorf("expected AvgTruncationRate ~0.75, got %f", rate)
	}

	// Check avg tokens: 2000 / 2 = 1000
	avgTokens := tt.AvgTokens()
	if avgTokens != 1000 {
		t.Errorf("expected AvgTokens=1000, got %f", avgTokens)
	}

	// Check avg latency: 200 / 2 = 100
	avgLatency := tt.AvgLatencyMs()
	if avgLatency != 100 {
		t.Errorf("expected AvgLatencyMs=100, got %f", avgLatency)
	}
}

func TestWideResultMetricsTruncationRate(t *testing.T) {
	m := WideResultMetrics{
		ToolName:        "test",
		TotalResults:    100,
		ReturnedResults: 20,
		TruncatedCount:  80,
	}

	rate := m.TruncationRate()
	if rate != 0.8 {
		t.Errorf("expected TruncationRate=0.8, got %f", rate)
	}

	// Test zero division
	m2 := WideResultMetrics{
		TotalResults: 0,
	}
	if m2.TruncationRate() != 0 {
		t.Error("expected TruncationRate=0 for zero total results")
	}
}

func TestResetWideResultMetrics(t *testing.T) {
	// Record something
	RecordWideResult(WideResultMetrics{
		ToolName:     "test",
		TotalResults: 10,
	})

	// Verify it's there
	summary := GetWideResultSummary()
	if len(summary) == 0 {
		t.Fatal("expected metrics to be recorded")
	}

	// Reset
	ResetWideResultMetrics()

	// Verify empty
	summary = GetWideResultSummary()
	if len(summary) != 0 {
		t.Errorf("expected empty metrics after reset, got %d", len(summary))
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		jsonBytes int
		expected  int
	}{
		{0, 0},
		{4, 1},
		{100, 25},
		{1000, 250},
	}

	for _, tt := range tests {
		got := EstimateTokens(tt.jsonBytes)
		if got != tt.expected {
			t.Errorf("EstimateTokens(%d) = %d, want %d", tt.jsonBytes, got, tt.expected)
		}
	}
}

func TestWideResultTimer(t *testing.T) {
	timer := NewWideResultTimer()

	// Sleep briefly
	// Note: we just check it returns non-negative, not exact timing
	elapsed := timer.ElapsedMs()
	if elapsed < 0 {
		t.Errorf("expected non-negative elapsed time, got %d", elapsed)
	}
}

func TestConcurrentRecording(t *testing.T) {
	ResetWideResultMetrics()

	var wg sync.WaitGroup
	iterations := 100

	// Concurrently record metrics from multiple goroutines
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				RecordWideResult(WideResultMetrics{
					ToolName:        "concurrentTest",
					TotalResults:    10,
					ReturnedResults: 10,
					TruncatedCount:  0,
					ExecutionMs:     1,
				})
			}
		}()
	}

	wg.Wait()

	// Verify total count
	summary := GetWideResultSummary()
	ct := summary["concurrentTest"]
	if ct.QueryCount != 1000 {
		t.Errorf("expected QueryCount=1000 after concurrent recording, got %d", ct.QueryCount)
	}
}

func TestGetWideResultSummaryReturnsACopy(t *testing.T) {
	ResetWideResultMetrics()

	RecordWideResult(WideResultMetrics{
		ToolName:     "test",
		TotalResults: 10,
	})

	// Get summary
	summary1 := GetWideResultSummary()

	// Record more
	RecordWideResult(WideResultMetrics{
		ToolName:     "test",
		TotalResults: 10,
	})

	// Original summary should be unchanged (it's a copy)
	if summary1["test"].QueryCount != 1 {
		t.Error("GetWideResultSummary should return a copy, not a reference")
	}

	// New summary should show updated count
	summary2 := GetWideResultSummary()
	if summary2["test"].QueryCount != 2 {
		t.Errorf("expected QueryCount=2, got %d", summary2["test"].QueryCount)
	}
}
