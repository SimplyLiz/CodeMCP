package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestLoadShedder_Basic(t *testing.T) {
	config := LoadSheddingConfig{
		Enabled:               true,
		MaxConcurrentRequests: 5,
		QueueSize:             2,
		QueueTimeout:          100 * time.Millisecond,
		PriorityEndpoints:     []string{"/health"},
		RetryAfterSeconds:     5,
	}

	ls := NewLoadShedder(config)

	// Should acquire up to max concurrent
	for i := 0; i < 5; i++ {
		if !ls.Acquire("/test", 100*time.Millisecond) {
			t.Errorf("Failed to acquire slot %d", i)
		}
	}

	stats := ls.Stats()
	if stats.InFlight != 5 {
		t.Errorf("Expected 5 in flight, got %d", stats.InFlight)
	}

	// Release all
	for i := 0; i < 5; i++ {
		ls.Release("/test")
	}

	stats = ls.Stats()
	if stats.InFlight != 0 {
		t.Errorf("Expected 0 in flight after release, got %d", stats.InFlight)
	}
}

func TestLoadShedder_PriorityEndpoints(t *testing.T) {
	config := LoadSheddingConfig{
		Enabled:               true,
		MaxConcurrentRequests: 1,
		QueueSize:             0,
		QueueTimeout:          100 * time.Millisecond,
		PriorityEndpoints:     []string{"/health", "/metrics"},
		RetryAfterSeconds:     5,
	}

	ls := NewLoadShedder(config)

	// Fill the single slot
	if !ls.Acquire("/test", 100*time.Millisecond) {
		t.Error("Failed to acquire initial slot")
	}

	// Priority endpoint should always be allowed
	if !ls.Acquire("/health", 100*time.Millisecond) {
		t.Error("Priority endpoint /health should be allowed")
	}

	if !ls.Acquire("/metrics", 100*time.Millisecond) {
		t.Error("Priority endpoint /metrics should be allowed")
	}

	// Non-priority should be rejected
	if ls.Acquire("/test2", 100*time.Millisecond) {
		t.Error("Non-priority endpoint should be rejected when at capacity")
	}

	stats := ls.Stats()
	if stats.TotalShed == 0 {
		t.Error("Expected at least one shed")
	}
}

func TestLoadShedder_Queue(t *testing.T) {
	config := LoadSheddingConfig{
		Enabled:               true,
		MaxConcurrentRequests: 1,
		QueueSize:             2,
		QueueTimeout:          500 * time.Millisecond,
		PriorityEndpoints:     []string{},
		RetryAfterSeconds:     5,
	}

	ls := NewLoadShedder(config)

	// Fill the slot
	if !ls.Acquire("/test", 100*time.Millisecond) {
		t.Error("Failed to acquire initial slot")
	}

	// Start a request that will wait in queue
	acquiredChan := make(chan bool)
	go func() {
		acquired := ls.Acquire("/queued", 500*time.Millisecond)
		acquiredChan <- acquired
	}()

	// Give it time to enter queue
	time.Sleep(50 * time.Millisecond)

	// Release the slot
	ls.Release("/test")

	// The queued request should now succeed
	select {
	case acquired := <-acquiredChan:
		if !acquired {
			t.Error("Queued request should have succeeded after slot release")
		}
	case <-time.After(1 * time.Second):
		t.Error("Timeout waiting for queued request")
	}
}

func TestLoadShedder_QueueTimeout(t *testing.T) {
	config := LoadSheddingConfig{
		Enabled:               true,
		MaxConcurrentRequests: 1,
		QueueSize:             2,
		QueueTimeout:          100 * time.Millisecond,
		PriorityEndpoints:     []string{},
		RetryAfterSeconds:     5,
	}

	ls := NewLoadShedder(config)

	// Fill the slot and don't release
	if !ls.Acquire("/blocker", 100*time.Millisecond) {
		t.Error("Failed to acquire initial slot")
	}

	// Try to acquire with short timeout
	start := time.Now()
	acquired := ls.Acquire("/timeout-test", 100*time.Millisecond)
	elapsed := time.Since(start)

	if acquired {
		t.Error("Should not have acquired slot")
	}

	// Should have waited approximately the queue timeout
	if elapsed < 90*time.Millisecond {
		t.Errorf("Should have waited longer, only waited %v", elapsed)
	}
}

func TestLoadSheddingMiddleware_Disabled(t *testing.T) {
	config := LoadSheddingConfig{
		Enabled: false,
	}

	middleware := LoadSheddingMiddleware(config)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", rec.Code)
	}
}

func TestLoadSheddingMiddleware_Enabled(t *testing.T) {
	config := LoadSheddingConfig{
		Enabled:               true,
		MaxConcurrentRequests: 2,
		QueueSize:             1,
		QueueTimeout:          50 * time.Millisecond,
		PriorityEndpoints:     []string{"/health"},
		RetryAfterSeconds:     10,
	}

	middleware := LoadSheddingMiddleware(config)

	slowHandler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	// Start concurrent requests
	var wg sync.WaitGroup
	results := make(chan int, 10)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/test", nil)
			rec := httptest.NewRecorder()
			slowHandler.ServeHTTP(rec, req)
			results <- rec.Code
		}()
	}

	wg.Wait()
	close(results)

	var ok, shed int
	for code := range results {
		if code == http.StatusOK {
			ok++
		} else if code == http.StatusServiceUnavailable {
			shed++
		}
	}

	// Some should succeed, some should be shed
	if ok == 0 {
		t.Error("Expected some requests to succeed")
	}
	if shed == 0 {
		t.Error("Expected some requests to be shed")
	}
}

func TestCircuitBreaker_Basic(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)

	// Should start closed
	if cb.State() != CircuitClosed {
		t.Error("Circuit should start closed")
	}

	// Record failures to open
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Error("Circuit should be open after 3 failures")
	}

	// Should not allow requests when open
	if cb.Allow() {
		t.Error("Should not allow when open")
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// Should transition to half-open
	if !cb.Allow() {
		t.Error("Should allow after timeout (half-open)")
	}
	if cb.State() != CircuitHalfOpen {
		t.Error("Should be half-open after timeout")
	}

	// Record successes to close
	cb.RecordSuccess()
	cb.RecordSuccess()

	if cb.State() != CircuitClosed {
		t.Error("Circuit should close after successes in half-open")
	}
}

func TestCircuitBreaker_FailureInHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(2, 2, 50*time.Millisecond)

	// Open the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)
	cb.Allow() // Transition to half-open

	if cb.State() != CircuitHalfOpen {
		t.Error("Should be half-open")
	}

	// Failure in half-open should reopen
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Error("Should be open after failure in half-open")
	}
}

func TestCircuitBreaker_Stats(t *testing.T) {
	cb := NewCircuitBreaker(3, 2, 100*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()

	stats := cb.Stats()

	if stats.State != "closed" {
		t.Errorf("Expected closed state, got %s", stats.State)
	}
	if stats.Failures != 2 {
		t.Errorf("Expected 2 failures, got %d", stats.Failures)
	}
}

func TestDefaultLoadSheddingConfig(t *testing.T) {
	cfg := DefaultLoadSheddingConfig()

	if cfg.Enabled {
		t.Error("Load shedding should be disabled by default")
	}
	if cfg.MaxConcurrentRequests != 100 {
		t.Errorf("Expected MaxConcurrentRequests=100, got %d", cfg.MaxConcurrentRequests)
	}
	if len(cfg.PriorityEndpoints) == 0 {
		t.Error("Expected some priority endpoints")
	}
}

func TestAdaptiveLoadShedder(t *testing.T) {
	config := LoadSheddingConfig{
		Enabled:               true,
		MaxConcurrentRequests: 10,
		QueueSize:             5,
		QueueTimeout:          100 * time.Millisecond,
		PriorityEndpoints:     []string{},
	}

	als := NewAdaptiveLoadShedder(config, 50) // Target 50ms latency

	// Record low latencies
	for i := 0; i < 10; i++ {
		als.RecordLatency(30) // 30ms
	}

	if als.ShouldShed() {
		t.Error("Should not shed with low latency")
	}

	// Record high latencies
	for i := 0; i < 100; i++ {
		als.RecordLatency(100) // 100ms - 2x target
	}

	if !als.ShouldShed() {
		t.Error("Should shed with high latency")
	}
}
