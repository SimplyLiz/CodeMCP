// Package api provides load shedding middleware for graceful degradation.
package api

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// LoadSheddingConfig contains load shedding configuration
type LoadSheddingConfig struct {
	// Enabled enables load shedding
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	// MaxConcurrentRequests is the maximum number of concurrent requests
	MaxConcurrentRequests int `json:"maxConcurrentRequests" mapstructure:"max_concurrent_requests"`
	// QueueSize is the size of the request queue
	QueueSize int `json:"queueSize" mapstructure:"queue_size"`
	// QueueTimeout is how long to wait in queue before rejecting
	QueueTimeout time.Duration `json:"queueTimeout" mapstructure:"queue_timeout"`
	// PriorityEndpoints are endpoints that should never be shed
	PriorityEndpoints []string `json:"priorityEndpoints" mapstructure:"priority_endpoints"`
	// RetryAfterSeconds is the value for the Retry-After header
	RetryAfterSeconds int `json:"retryAfterSeconds" mapstructure:"retry_after_seconds"`
}

// DefaultLoadSheddingConfig returns default load shedding configuration
func DefaultLoadSheddingConfig() LoadSheddingConfig {
	return LoadSheddingConfig{
		Enabled:               false, // Disabled by default
		MaxConcurrentRequests: 100,
		QueueSize:             50,
		QueueTimeout:          5 * time.Second,
		PriorityEndpoints: []string{
			"/health",
			"/ready",
			"/metrics",
			"/symbol/",
			"/search",
		},
		RetryAfterSeconds: 5,
	}
}

// LoadShedder manages request load shedding
type LoadShedder struct {
	config LoadSheddingConfig

	// Current state
	inFlight     int64
	queueLength  int64
	totalShed    uint64
	lastShedTime atomic.Value // time.Time

	// Semaphore for concurrency control
	semaphore chan struct{}
	// Queue for waiting requests
	queue chan struct{}

	mu sync.RWMutex //nolint:unused
}

// NewLoadShedder creates a new load shedder
func NewLoadShedder(config LoadSheddingConfig) *LoadShedder {
	ls := &LoadShedder{
		config:    config,
		semaphore: make(chan struct{}, config.MaxConcurrentRequests),
		queue:     make(chan struct{}, config.QueueSize),
	}
	ls.lastShedTime.Store(time.Time{})
	return ls
}

// Acquire tries to acquire a slot for processing a request.
// Returns true if the request can proceed, false if it should be rejected.
func (ls *LoadShedder) Acquire(endpoint string, timeout time.Duration) bool {
	// Check if this is a priority endpoint
	if ls.isPriorityEndpoint(endpoint) {
		return true
	}

	// Try to acquire immediately
	select {
	case ls.semaphore <- struct{}{}:
		atomic.AddInt64(&ls.inFlight, 1)
		return true
	default:
		// Semaphore is full, try to queue
	}

	// Try to enter queue
	select {
	case ls.queue <- struct{}{}:
		atomic.AddInt64(&ls.queueLength, 1)
		defer func() {
			<-ls.queue
			atomic.AddInt64(&ls.queueLength, -1)
		}()
	default:
		// Queue is full, shed immediately
		ls.recordShed()
		return false
	}

	// Wait in queue for a slot
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case ls.semaphore <- struct{}{}:
		atomic.AddInt64(&ls.inFlight, 1)
		return true
	case <-timer.C:
		// Timeout waiting in queue
		ls.recordShed()
		return false
	}
}

// Release releases a slot after processing is complete.
func (ls *LoadShedder) Release(endpoint string) {
	// Priority endpoints don't hold slots
	if ls.isPriorityEndpoint(endpoint) {
		return
	}

	select {
	case <-ls.semaphore:
		atomic.AddInt64(&ls.inFlight, -1)
	default:
		// Should not happen, but don't block
	}
}

// Stats returns current load shedding statistics
func (ls *LoadShedder) Stats() LoadSheddingStats {
	lastShed, _ := ls.lastShedTime.Load().(time.Time)
	return LoadSheddingStats{
		InFlight:      atomic.LoadInt64(&ls.inFlight),
		QueueLength:   atomic.LoadInt64(&ls.queueLength),
		MaxConcurrent: ls.config.MaxConcurrentRequests,
		MaxQueue:      ls.config.QueueSize,
		TotalShed:     atomic.LoadUint64(&ls.totalShed),
		LastShedTime:  lastShed,
		Enabled:       ls.config.Enabled,
	}
}

// LoadSheddingStats contains load shedding statistics
type LoadSheddingStats struct {
	InFlight      int64     `json:"inFlight"`
	QueueLength   int64     `json:"queueLength"`
	MaxConcurrent int       `json:"maxConcurrent"`
	MaxQueue      int       `json:"maxQueue"`
	TotalShed     uint64    `json:"totalShed"`
	LastShedTime  time.Time `json:"lastShedTime,omitempty"`
	Enabled       bool      `json:"enabled"`
}

func (ls *LoadShedder) isPriorityEndpoint(endpoint string) bool {
	for _, priority := range ls.config.PriorityEndpoints {
		if strings.HasPrefix(endpoint, priority) {
			return true
		}
	}
	return false
}

func (ls *LoadShedder) recordShed() {
	atomic.AddUint64(&ls.totalShed, 1)
	ls.lastShedTime.Store(time.Now())
}

// LoadSheddingMiddleware creates middleware for load shedding
func LoadSheddingMiddleware(config LoadSheddingConfig) func(http.Handler) http.Handler {
	if !config.Enabled {
		// Return pass-through middleware if disabled
		return func(next http.Handler) http.Handler {
			return next
		}
	}

	shedder := NewLoadShedder(config)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !shedder.Acquire(r.URL.Path, config.QueueTimeout) {
				// Request was shed
				w.Header().Set("Retry-After", strconv.Itoa(config.RetryAfterSeconds))
				w.Header().Set("X-Load-Shed", "true")
				http.Error(w, "Service temporarily overloaded. Please retry.", http.StatusServiceUnavailable)
				return
			}

			defer shedder.Release(r.URL.Path)
			next.ServeHTTP(w, r)
		})
	}
}

// AdaptiveLoadShedder extends LoadShedder with adaptive behavior
type AdaptiveLoadShedder struct {
	*LoadShedder

	// Adaptive thresholds
	targetLatencyMs  int64
	currentLatencyMs int64
	shedThreshold    float64

	// Metrics for adaptation
	recentLatencies []int64
	latencyMu       sync.Mutex
	maxSamples      int
}

// NewAdaptiveLoadShedder creates an adaptive load shedder
func NewAdaptiveLoadShedder(config LoadSheddingConfig, targetLatencyMs int64) *AdaptiveLoadShedder {
	return &AdaptiveLoadShedder{
		LoadShedder:     NewLoadShedder(config),
		targetLatencyMs: targetLatencyMs,
		shedThreshold:   1.5, // Start shedding when latency is 1.5x target
		maxSamples:      100,
		recentLatencies: make([]int64, 0, 100),
	}
}

// RecordLatency records a request latency for adaptive adjustment
func (als *AdaptiveLoadShedder) RecordLatency(latencyMs int64) {
	als.latencyMu.Lock()
	defer als.latencyMu.Unlock()

	als.recentLatencies = append(als.recentLatencies, latencyMs)
	if len(als.recentLatencies) > als.maxSamples {
		als.recentLatencies = als.recentLatencies[1:]
	}

	// Update current latency (simple moving average)
	if len(als.recentLatencies) > 0 {
		var sum int64
		for _, l := range als.recentLatencies {
			sum += l
		}
		atomic.StoreInt64(&als.currentLatencyMs, sum/int64(len(als.recentLatencies)))
	}
}

// ShouldShed returns true if we should start shedding load based on latency
func (als *AdaptiveLoadShedder) ShouldShed() bool {
	current := atomic.LoadInt64(&als.currentLatencyMs)
	threshold := float64(als.targetLatencyMs) * als.shedThreshold
	return float64(current) > threshold
}

// AcquireAdaptive tries to acquire a slot with adaptive behavior
func (als *AdaptiveLoadShedder) AcquireAdaptive(endpoint string, timeout time.Duration) bool {
	// If we're under load, reduce timeout to shed faster
	if als.ShouldShed() {
		timeout = timeout / 2
		if timeout < 100*time.Millisecond {
			timeout = 100 * time.Millisecond
		}
	}

	return als.LoadShedder.Acquire(endpoint, timeout)
}

// CircuitBreaker provides circuit breaker pattern for external dependencies
type CircuitBreaker struct {
	mu sync.RWMutex //nolint:unused

	// State
	state           CircuitState
	failures        int
	successes       int
	lastFailureTime time.Time
	lastStateChange time.Time

	// Configuration
	failureThreshold int
	successThreshold int
	timeout          time.Duration
}

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(failureThreshold, successThreshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            CircuitClosed,
		failureThreshold: failureThreshold,
		successThreshold: successThreshold,
		timeout:          timeout,
		lastStateChange:  time.Now(),
	}
}

// Allow checks if a request should be allowed through
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if timeout has passed
		if time.Since(cb.lastFailureTime) > cb.timeout {
			cb.state = CircuitHalfOpen
			cb.lastStateChange = time.Now()
			cb.successes = 0
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitHalfOpen:
		cb.successes++
		if cb.successes >= cb.successThreshold {
			cb.state = CircuitClosed
			cb.lastStateChange = time.Now()
			cb.failures = 0
		}
	case CircuitClosed:
		cb.failures = 0 // Reset failure count on success
	}
}

// RecordFailure records a failed operation
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitClosed:
		if cb.failures >= cb.failureThreshold {
			cb.state = CircuitOpen
			cb.lastStateChange = time.Now()
		}
	case CircuitHalfOpen:
		cb.state = CircuitOpen
		cb.lastStateChange = time.Now()
	}
}

// State returns the current circuit state
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Stats returns circuit breaker statistics
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerStats{
		State:           cb.state.String(),
		Failures:        cb.failures,
		Successes:       cb.successes,
		LastFailure:     cb.lastFailureTime,
		LastStateChange: cb.lastStateChange,
	}
}

// CircuitBreakerStats contains circuit breaker statistics
type CircuitBreakerStats struct {
	State           string    `json:"state"`
	Failures        int       `json:"failures"`
	Successes       int       `json:"successes"`
	LastFailure     time.Time `json:"lastFailure,omitempty"`
	LastStateChange time.Time `json:"lastStateChange"`
}
