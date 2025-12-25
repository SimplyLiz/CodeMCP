package backends

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// RateLimiter manages concurrent requests and coalesces duplicates
type RateLimiter struct {
	// Per-backend semaphores for concurrency control
	semaphores map[BackendID]*semaphore

	// Coalescing state
	coalesceWindow time.Duration
	pendingQueries map[string]*coalescedQuery
	mu             sync.RWMutex

	// done channel for graceful shutdown of cleanup goroutine
	done chan struct{}
}

// semaphore implements a counting semaphore for rate limiting
type semaphore struct {
	permits chan struct{}
}

// newSemaphore creates a semaphore with the given number of permits
func newSemaphore(permits int) *semaphore {
	s := &semaphore{
		permits: make(chan struct{}, permits),
	}
	// Fill the semaphore
	for i := 0; i < permits; i++ {
		s.permits <- struct{}{}
	}
	return s
}

// Acquire acquires a permit, blocking if none available
func (s *semaphore) Acquire(ctx context.Context) error {
	select {
	case <-s.permits:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release releases a permit back to the semaphore
func (s *semaphore) Release() {
	select {
	case s.permits <- struct{}{}:
	default:
		// Should never happen unless Release called more than Acquire
	}
}

// coalescedQuery represents a query that may be shared by multiple callers
type coalescedQuery struct {
	// Result channel that all waiters listen on
	result chan interface{}

	// Error channel for errors
	err chan error

	// Expiry time for this coalesced query
	expiry time.Time

	// Number of waiters
	waiters int

	// Mutex to protect waiters count
	mu sync.Mutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(policy *QueryPolicy) *RateLimiter {
	limiter := &RateLimiter{
		semaphores:     make(map[BackendID]*semaphore),
		coalesceWindow: time.Duration(policy.CoalesceWindowMs) * time.Millisecond,
		pendingQueries: make(map[string]*coalescedQuery),
		done:           make(chan struct{}),
	}

	// Initialize semaphores for each backend
	for backendID, maxInFlight := range policy.MaxInFlightPerBackend {
		limiter.semaphores[backendID] = newSemaphore(maxInFlight)
	}

	// Start cleanup goroutine for expired coalesced queries
	go limiter.cleanupExpiredQueries()

	return limiter
}

// Stop gracefully shuts down the rate limiter's background goroutines
func (l *RateLimiter) Stop() {
	close(l.done)
}

// Acquire acquires a permit for the given backend
func (l *RateLimiter) Acquire(ctx context.Context, backendID BackendID) error {
	sem, ok := l.semaphores[backendID]
	if !ok {
		// No limit configured for this backend
		return nil
	}
	return sem.Acquire(ctx)
}

// Release releases a permit for the given backend
func (l *RateLimiter) Release(backendID BackendID) {
	if sem, ok := l.semaphores[backendID]; ok {
		sem.Release()
	}
}

// CoalesceOrExecute checks if an identical query is pending and coalesces if so,
// otherwise executes the query function
func (l *RateLimiter) CoalesceOrExecute(
	ctx context.Context,
	req QueryRequest,
	backendID BackendID,
	fn func(context.Context, QueryRequest) (interface{}, error),
) (interface{}, error) {
	// Generate query key for coalescing
	queryKey := l.generateQueryKey(backendID, req)

	// Check if we can coalesce with an existing query
	l.mu.RLock()
	cq, exists := l.pendingQueries[queryKey]
	l.mu.RUnlock()

	if exists && time.Now().Before(cq.expiry) {
		// Coalesce with existing query
		cq.mu.Lock()
		cq.waiters++
		cq.mu.Unlock()

		select {
		case result := <-cq.result:
			return result, nil
		case err := <-cq.err:
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Create new coalesced query
	cq = &coalescedQuery{
		result:  make(chan interface{}, 1),
		err:     make(chan error, 1),
		expiry:  time.Now().Add(l.coalesceWindow),
		waiters: 1,
	}

	l.mu.Lock()
	l.pendingQueries[queryKey] = cq
	l.mu.Unlock()

	// Execute the query
	result, err := fn(ctx, req)

	// Notify all waiters
	if err != nil {
		cq.err <- err
		close(cq.err)
	} else {
		cq.result <- result
		close(cq.result)
	}

	// Remove from pending after a delay to allow coalescing
	time.AfterFunc(l.coalesceWindow, func() {
		l.mu.Lock()
		delete(l.pendingQueries, queryKey)
		l.mu.Unlock()
	})

	return result, err
}

// generateQueryKey creates a unique key for a query request
func (l *RateLimiter) generateQueryKey(backendID BackendID, req QueryRequest) string {
	// Create a deterministic representation of the request
	data := map[string]interface{}{
		"backend":    backendID,
		"type":       req.Type,
		"symbolID":   req.SymbolID,
		"query":      req.Query,
		"searchOpts": req.SearchOpts,
		"refOpts":    req.RefOpts,
	}

	jsonData, _ := json.Marshal(data)
	hash := sha256.Sum256(jsonData)
	return hex.EncodeToString(hash[:])
}

// cleanupExpiredQueries periodically removes expired coalesced queries
func (l *RateLimiter) cleanupExpiredQueries() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-l.done:
			return
		case <-ticker.C:
			now := time.Now()
			l.mu.Lock()
			for key, cq := range l.pendingQueries {
				if now.After(cq.expiry) {
					delete(l.pendingQueries, key)
				}
			}
			l.mu.Unlock()
		}
	}
}
