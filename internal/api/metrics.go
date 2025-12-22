// Package api provides Prometheus metrics for CKB.
package api

import (
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsConfig contains metrics configuration
type MetricsConfig struct {
	Enabled  bool   `json:"enabled" mapstructure:"enabled"`
	Endpoint string `json:"endpoint" mapstructure:"endpoint"`
}

// DefaultMetricsConfig returns default metrics configuration
func DefaultMetricsConfig() MetricsConfig {
	return MetricsConfig{
		Enabled:  true,
		Endpoint: "/metrics",
	}
}

// MetricsCollector collects and exposes Prometheus metrics
type MetricsCollector struct {
	mu sync.RWMutex //nolint:unused

	// Counters
	ingestionTotal     *Counter
	searchTotal        *Counter
	searchResultsTotal *Counter
	rateLimitExceeded  *Counter
	errorTotal         *Counter

	// Histograms
	ingestionDuration *Histogram
	searchDuration    *Histogram

	// Gauges
	storageBytes   *Gauge
	snapshotsTotal *Gauge
	symbolsTotal   *Gauge
	refsTotal      *Gauge
	cacheSize      *Gauge
	cacheHitRate   *Gauge
	goroutines     *Gauge
	memoryAlloc    *Gauge

	startTime time.Time
}

// Counter is a monotonically increasing counter
type Counter struct {
	name   string
	help   string
	labels []string
	values sync.Map // map[string]*uint64
}

// Histogram tracks distributions of values
type Histogram struct {
	name    string
	help    string
	labels  []string
	buckets []float64
	values  sync.Map // map[string]*histogramValue
}

type histogramValue struct {
	mu sync.Mutex //nolint:unused
	sum     float64
	count   uint64
	buckets []uint64
}

// Gauge is a metric that can go up and down
type Gauge struct {
	name   string
	help   string
	labels []string
	values sync.Map // map[string]*float64
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	m := &MetricsCollector{
		startTime: time.Now(),
	}

	// Initialize counters
	m.ingestionTotal = &Counter{
		name:   "ckb_ingestion_total",
		help:   "Total number of ingestion operations",
		labels: []string{"repo", "type"},
	}

	m.searchTotal = &Counter{
		name:   "ckb_search_total",
		help:   "Total number of search operations",
		labels: []string{"repo", "type"},
	}

	m.searchResultsTotal = &Counter{
		name:   "ckb_search_results_total",
		help:   "Total number of search results returned",
		labels: []string{"repo"},
	}

	m.rateLimitExceeded = &Counter{
		name:   "ckb_ratelimit_exceeded_total",
		help:   "Total number of rate limit exceeded events",
		labels: []string{"repo", "principal"},
	}

	m.errorTotal = &Counter{
		name:   "ckb_errors_total",
		help:   "Total number of errors",
		labels: []string{"type"},
	}

	// Initialize histograms
	defaultBuckets := []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

	m.ingestionDuration = &Histogram{
		name:    "ckb_ingestion_duration_seconds",
		help:    "Duration of ingestion operations in seconds",
		labels:  []string{"repo", "type"},
		buckets: defaultBuckets,
	}

	m.searchDuration = &Histogram{
		name:    "ckb_search_duration_seconds",
		help:    "Duration of search operations in seconds",
		labels:  []string{"repo", "type"},
		buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	}

	// Initialize gauges
	m.storageBytes = &Gauge{
		name:   "ckb_storage_bytes",
		help:   "Storage size in bytes",
		labels: []string{"repo", "type"},
	}

	m.snapshotsTotal = &Gauge{
		name:   "ckb_snapshots_total",
		help:   "Number of snapshots",
		labels: []string{"repo"},
	}

	m.symbolsTotal = &Gauge{
		name:   "ckb_symbols_total",
		help:   "Total number of indexed symbols",
		labels: []string{"repo"},
	}

	m.refsTotal = &Gauge{
		name:   "ckb_refs_total",
		help:   "Total number of indexed references",
		labels: []string{"repo"},
	}

	m.cacheSize = &Gauge{
		name:   "ckb_cache_size_bytes",
		help:   "Cache size in bytes",
		labels: []string{"type"},
	}

	m.cacheHitRate = &Gauge{
		name:   "ckb_cache_hit_rate",
		help:   "Cache hit rate (0-1)",
		labels: []string{"type"},
	}

	m.goroutines = &Gauge{
		name:   "ckb_goroutines",
		help:   "Number of goroutines",
		labels: []string{},
	}

	m.memoryAlloc = &Gauge{
		name:   "ckb_memory_alloc_bytes",
		help:   "Allocated memory in bytes",
		labels: []string{},
	}

	return m
}

// RecordIngestion records an ingestion operation
func (m *MetricsCollector) RecordIngestion(repo, ingestType string, duration time.Duration) {
	m.ingestionTotal.Inc(repo, ingestType)
	m.ingestionDuration.Observe(duration.Seconds(), repo, ingestType)
}

// RecordSearch records a search operation
func (m *MetricsCollector) RecordSearch(repo, searchType string, duration time.Duration, resultCount int) {
	m.searchTotal.Inc(repo, searchType)
	m.searchDuration.Observe(duration.Seconds(), repo, searchType)
	m.searchResultsTotal.Add(uint64(resultCount), repo)
}

// RecordRateLimitExceeded records a rate limit exceeded event
func (m *MetricsCollector) RecordRateLimitExceeded(repo, principal string) {
	m.rateLimitExceeded.Inc(repo, principal)
}

// RecordError records an error
func (m *MetricsCollector) RecordError(errorType string) {
	m.errorTotal.Inc(errorType)
}

// SetStorageBytes sets storage size
func (m *MetricsCollector) SetStorageBytes(repo, storageType string, bytes int64) {
	m.storageBytes.Set(float64(bytes), repo, storageType)
}

// SetSnapshots sets snapshot count
func (m *MetricsCollector) SetSnapshots(repo string, count int) {
	m.snapshotsTotal.Set(float64(count), repo)
}

// SetSymbols sets symbol count
func (m *MetricsCollector) SetSymbols(repo string, count int) {
	m.symbolsTotal.Set(float64(count), repo)
}

// SetRefs sets reference count
func (m *MetricsCollector) SetRefs(repo string, count int) {
	m.refsTotal.Set(float64(count), repo)
}

// SetCacheSize sets cache size
func (m *MetricsCollector) SetCacheSize(cacheType string, bytes int64) {
	m.cacheSize.Set(float64(bytes), cacheType)
}

// SetCacheHitRate sets cache hit rate
func (m *MetricsCollector) SetCacheHitRate(cacheType string, rate float64) {
	m.cacheHitRate.Set(rate, cacheType)
}

// WritePrometheus writes metrics in Prometheus text format
func (m *MetricsCollector) WritePrometheus(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	// Update runtime metrics
	m.goroutines.Set(float64(runtime.NumGoroutine()))
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	m.memoryAlloc.Set(float64(memStats.Alloc))

	// Write process info
	fmt.Fprintf(w, "# HELP ckb_info CKB build information\n")
	fmt.Fprintf(w, "# TYPE ckb_info gauge\n")
	fmt.Fprintf(w, "ckb_info{version=\"%s\"} 1\n\n", "7.3.0") // Should use version.Version

	// Write uptime
	fmt.Fprintf(w, "# HELP ckb_uptime_seconds Time since CKB started\n")
	fmt.Fprintf(w, "# TYPE ckb_uptime_seconds counter\n")
	fmt.Fprintf(w, "ckb_uptime_seconds %.3f\n\n", time.Since(m.startTime).Seconds())

	// Write counters
	m.writeCounter(w, m.ingestionTotal)
	m.writeCounter(w, m.searchTotal)
	m.writeCounter(w, m.searchResultsTotal)
	m.writeCounter(w, m.rateLimitExceeded)
	m.writeCounter(w, m.errorTotal)

	// Write histograms
	m.writeHistogram(w, m.ingestionDuration)
	m.writeHistogram(w, m.searchDuration)

	// Write gauges
	m.writeGauge(w, m.storageBytes)
	m.writeGauge(w, m.snapshotsTotal)
	m.writeGauge(w, m.symbolsTotal)
	m.writeGauge(w, m.refsTotal)
	m.writeGauge(w, m.cacheSize)
	m.writeGauge(w, m.cacheHitRate)
	m.writeGauge(w, m.goroutines)
	m.writeGauge(w, m.memoryAlloc)
}

func (m *MetricsCollector) writeCounter(w http.ResponseWriter, c *Counter) {
	fmt.Fprintf(w, "# HELP %s %s\n", c.name, c.help)
	fmt.Fprintf(w, "# TYPE %s counter\n", c.name)

	var keys []string
	c.values.Range(func(key, value interface{}) bool {
		keys = append(keys, key.(string))
		return true
	})
	sort.Strings(keys)

	for _, key := range keys {
		val, _ := c.values.Load(key)
		if ptr, ok := val.(*uint64); ok {
			fmt.Fprintf(w, "%s%s %d\n", c.name, key, atomic.LoadUint64(ptr))
		}
	}
	fmt.Fprintln(w)
}

func (m *MetricsCollector) writeHistogram(w http.ResponseWriter, h *Histogram) {
	fmt.Fprintf(w, "# HELP %s %s\n", h.name, h.help)
	fmt.Fprintf(w, "# TYPE %s histogram\n", h.name)

	var keys []string
	h.values.Range(func(key, value interface{}) bool {
		keys = append(keys, key.(string))
		return true
	})
	sort.Strings(keys)

	for _, key := range keys {
		val, _ := h.values.Load(key)
		if hv, ok := val.(*histogramValue); ok {
			hv.mu.Lock()
			// Write bucket counts
			cumulative := uint64(0)
			for i, bucket := range h.buckets {
				cumulative += hv.buckets[i]
				bucketLabel := key
				if bucketLabel != "" {
					bucketLabel = bucketLabel[:len(bucketLabel)-1] + fmt.Sprintf(",le=\"%.3f\"}", bucket)
				} else {
					bucketLabel = fmt.Sprintf("{le=\"%.3f\"}", bucket)
				}
				fmt.Fprintf(w, "%s_bucket%s %d\n", h.name, bucketLabel, cumulative)
			}
			// +Inf bucket
			cumulative += hv.buckets[len(h.buckets)]
			infLabel := key
			if infLabel != "" {
				infLabel = infLabel[:len(infLabel)-1] + ",le=\"+Inf\"}"
			} else {
				infLabel = "{le=\"+Inf\"}"
			}
			fmt.Fprintf(w, "%s_bucket%s %d\n", h.name, infLabel, cumulative)

			// Sum and count
			fmt.Fprintf(w, "%s_sum%s %.6f\n", h.name, key, hv.sum)
			fmt.Fprintf(w, "%s_count%s %d\n", h.name, key, hv.count)
			hv.mu.Unlock()
		}
	}
	fmt.Fprintln(w)
}

func (m *MetricsCollector) writeGauge(w http.ResponseWriter, g *Gauge) {
	fmt.Fprintf(w, "# HELP %s %s\n", g.name, g.help)
	fmt.Fprintf(w, "# TYPE %s gauge\n", g.name)

	var keys []string
	g.values.Range(func(key, value interface{}) bool {
		keys = append(keys, key.(string))
		return true
	})
	sort.Strings(keys)

	for _, key := range keys {
		val, _ := g.values.Load(key)
		if ptr, ok := val.(*float64); ok {
			fmt.Fprintf(w, "%s%s %.6f\n", g.name, key, *ptr)
		}
	}
	fmt.Fprintln(w)
}

// Counter methods
func (c *Counter) Inc(labelValues ...string) {
	c.Add(1, labelValues...)
}

func (c *Counter) Add(delta uint64, labelValues ...string) {
	key := c.labelsToKey(labelValues)
	val, _ := c.values.LoadOrStore(key, new(uint64))
	atomic.AddUint64(val.(*uint64), delta)
}

func (c *Counter) labelsToKey(values []string) string {
	if len(c.labels) == 0 || len(values) == 0 {
		return ""
	}

	pairs := make([]string, 0, len(c.labels))
	for i, label := range c.labels {
		if i < len(values) {
			pairs = append(pairs, fmt.Sprintf("%s=\"%s\"", label, values[i]))
		}
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

// Histogram methods
func (h *Histogram) Observe(value float64, labelValues ...string) {
	key := h.labelsToKey(labelValues)

	val, loaded := h.values.LoadOrStore(key, &histogramValue{
		buckets: make([]uint64, len(h.buckets)+1), // +1 for +Inf
	})

	hv := val.(*histogramValue)
	if !loaded {
		hv.buckets = make([]uint64, len(h.buckets)+1)
	}

	hv.mu.Lock()
	defer hv.mu.Unlock()

	hv.sum += value
	hv.count++

	// Find bucket
	bucketIdx := len(h.buckets) // Default to +Inf
	for i, bound := range h.buckets {
		if value <= bound {
			bucketIdx = i
			break
		}
	}
	hv.buckets[bucketIdx]++
}

func (h *Histogram) labelsToKey(values []string) string {
	if len(h.labels) == 0 || len(values) == 0 {
		return ""
	}

	pairs := make([]string, 0, len(h.labels))
	for i, label := range h.labels {
		if i < len(values) {
			pairs = append(pairs, fmt.Sprintf("%s=\"%s\"", label, values[i]))
		}
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

// Gauge methods
func (g *Gauge) Set(value float64, labelValues ...string) {
	key := g.labelsToKey(labelValues)
	ptr := new(float64)
	*ptr = value
	g.values.Store(key, ptr)
}

func (g *Gauge) labelsToKey(values []string) string {
	if len(g.labels) == 0 || len(values) == 0 {
		return ""
	}

	pairs := make([]string, 0, len(g.labels))
	for i, label := range g.labels {
		if i < len(values) {
			pairs = append(pairs, fmt.Sprintf("%s=\"%s\"", label, values[i]))
		}
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

// handleMetrics handles the /metrics endpoint
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.metrics == nil {
		http.Error(w, "Metrics not enabled", http.StatusNotImplemented)
		return
	}

	s.metrics.WritePrometheus(w)
}
