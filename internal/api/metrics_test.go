package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsCollector_Counter(t *testing.T) {
	m := NewMetricsCollector()

	// Record some ingestions
	m.RecordIngestion("test-repo", "delta", 100*time.Millisecond)
	m.RecordIngestion("test-repo", "delta", 200*time.Millisecond)
	m.RecordIngestion("other-repo", "full", 500*time.Millisecond)

	// Record some searches
	m.RecordSearch("test-repo", "fts5", 50*time.Millisecond, 10)
	m.RecordSearch("test-repo", "like", 100*time.Millisecond, 5)

	// Capture output
	recorder := httptest.NewRecorder()
	m.WritePrometheus(recorder)

	output := recorder.Body.String()

	// Verify counter increments
	if !strings.Contains(output, "ckb_ingestion_total") {
		t.Error("Expected ingestion total counter")
	}
	if !strings.Contains(output, "ckb_search_total") {
		t.Error("Expected search total counter")
	}
	if !strings.Contains(output, "ckb_search_results_total") {
		t.Error("Expected search results counter")
	}
}

func TestMetricsCollector_Histogram(t *testing.T) {
	m := NewMetricsCollector()

	// Record various durations to test buckets
	durations := []time.Duration{
		1 * time.Millisecond,
		10 * time.Millisecond,
		100 * time.Millisecond,
		500 * time.Millisecond,
		2 * time.Second,
	}

	for _, d := range durations {
		m.RecordIngestion("test-repo", "delta", d)
	}

	recorder := httptest.NewRecorder()
	m.WritePrometheus(recorder)

	output := recorder.Body.String()

	// Verify histogram output
	if !strings.Contains(output, "ckb_ingestion_duration_seconds_bucket") {
		t.Error("Expected ingestion duration histogram buckets")
	}
	if !strings.Contains(output, "ckb_ingestion_duration_seconds_sum") {
		t.Error("Expected ingestion duration histogram sum")
	}
	if !strings.Contains(output, "ckb_ingestion_duration_seconds_count") {
		t.Error("Expected ingestion duration histogram count")
	}
}

func TestMetricsCollector_Gauge(t *testing.T) {
	m := NewMetricsCollector()

	// Set various gauges
	m.SetStorageBytes("test-repo", "database", 1024*1024)
	m.SetSnapshots("test-repo", 5)
	m.SetSymbols("test-repo", 10000)
	m.SetRefs("test-repo", 50000)
	m.SetCacheSize("query", 512*1024)
	m.SetCacheHitRate("query", 0.85)

	recorder := httptest.NewRecorder()
	m.WritePrometheus(recorder)

	output := recorder.Body.String()

	// Verify gauge output
	if !strings.Contains(output, "ckb_storage_bytes") {
		t.Error("Expected storage bytes gauge")
	}
	if !strings.Contains(output, "ckb_snapshots_total") {
		t.Error("Expected snapshots gauge")
	}
	if !strings.Contains(output, "ckb_symbols_total") {
		t.Error("Expected symbols gauge")
	}
	if !strings.Contains(output, "ckb_cache_hit_rate") {
		t.Error("Expected cache hit rate gauge")
	}
}

func TestMetricsCollector_RuntimeMetrics(t *testing.T) {
	m := NewMetricsCollector()

	recorder := httptest.NewRecorder()
	m.WritePrometheus(recorder)

	output := recorder.Body.String()

	// Verify runtime metrics
	if !strings.Contains(output, "ckb_goroutines") {
		t.Error("Expected goroutines gauge")
	}
	if !strings.Contains(output, "ckb_memory_alloc_bytes") {
		t.Error("Expected memory alloc gauge")
	}
	if !strings.Contains(output, "ckb_uptime_seconds") {
		t.Error("Expected uptime counter")
	}
}

func TestMetricsCollector_Concurrency(t *testing.T) {
	m := NewMetricsCollector()

	// Concurrent writes to verify thread safety
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				m.RecordIngestion("repo", "type", time.Duration(j)*time.Millisecond)
				m.RecordSearch("repo", "fts5", time.Duration(j)*time.Millisecond, j)
				m.SetStorageBytes("repo", "db", int64(j*1024))
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic and produce valid output
	recorder := httptest.NewRecorder()
	m.WritePrometheus(recorder)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", recorder.Code)
	}
}

func TestDefaultMetricsConfig(t *testing.T) {
	cfg := DefaultMetricsConfig()

	if !cfg.Enabled {
		t.Error("Expected metrics to be enabled by default")
	}
	if cfg.Endpoint != "/metrics" {
		t.Errorf("Expected endpoint /metrics, got %s", cfg.Endpoint)
	}
}
