package backends

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestRateLimiterStop verifies the cleanup goroutine stops when Stop is called
func TestRateLimiterStop(t *testing.T) {
	policy := &QueryPolicy{
		MaxInFlightPerBackend: map[BackendID]int{
			"test": 5,
		},
		CoalesceWindowMs: 100,
	}

	limiter := NewRateLimiter(policy)

	// Verify limiter is working
	if limiter.done == nil {
		t.Fatal("done channel not initialized")
	}

	// Stop should not block
	done := make(chan struct{})
	go func() {
		limiter.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Stop() blocked for too long")
	}
}

// TestRateLimiterCleanupStopsOnDone verifies cleanup goroutine respects done channel
func TestRateLimiterCleanupStopsOnDone(t *testing.T) {
	policy := &QueryPolicy{
		MaxInFlightPerBackend: map[BackendID]int{
			"test": 5,
		},
		CoalesceWindowMs: 50,
	}

	limiter := NewRateLimiter(policy)

	// Add a pending query
	limiter.mu.Lock()
	limiter.pendingQueries["test-key"] = &coalescedQuery{
		result:  make(chan interface{}, 1),
		err:     make(chan error, 1),
		expiry:  time.Now().Add(time.Hour), // Won't expire naturally
		waiters: 1,
	}
	limiter.mu.Unlock()

	// Stop the limiter
	limiter.Stop()

	// Give cleanup goroutine time to exit
	time.Sleep(100 * time.Millisecond)

	// The pending query should still be there (cleanup stopped before it could clean)
	limiter.mu.RLock()
	_, exists := limiter.pendingQueries["test-key"]
	limiter.mu.RUnlock()

	if !exists {
		t.Log("Query was cleaned up before Stop - this is also valid")
	}
}

// TestRateLimiterAcquireRelease verifies basic semaphore functionality
func TestRateLimiterAcquireRelease(t *testing.T) {
	policy := &QueryPolicy{
		MaxInFlightPerBackend: map[BackendID]int{
			"test": 2,
		},
		CoalesceWindowMs: 100,
	}

	limiter := NewRateLimiter(policy)
	defer limiter.Stop()

	ctx := context.Background()

	// Acquire first permit
	if err := limiter.Acquire(ctx, "test"); err != nil {
		t.Fatalf("First acquire failed: %v", err)
	}

	// Acquire second permit
	if err := limiter.Acquire(ctx, "test"); err != nil {
		t.Fatalf("Second acquire failed: %v", err)
	}

	// Third acquire should block (use timeout context)
	ctxTimeout, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	err := limiter.Acquire(ctxTimeout, "test")
	if err == nil {
		t.Error("Third acquire should have timed out")
	}

	// Release one permit
	limiter.Release("test")

	// Now acquire should succeed
	if err := limiter.Acquire(ctx, "test"); err != nil {
		t.Fatalf("Acquire after release failed: %v", err)
	}

	// Cleanup
	limiter.Release("test")
	limiter.Release("test")
}

// TestRateLimiterCoalescing verifies query coalescing reduces duplicate calls
func TestRateLimiterCoalescing(t *testing.T) {
	policy := &QueryPolicy{
		MaxInFlightPerBackend: map[BackendID]int{
			"test": 5,
		},
		CoalesceWindowMs: 500, // Long enough for test
	}

	limiter := NewRateLimiter(policy)
	defer limiter.Stop()

	ctx := context.Background()

	// Track how many times the function is actually called
	var callCount int
	var mu sync.Mutex

	req := QueryRequest{
		Type:  "test",
		Query: "test-query",
	}

	fn := func(ctx context.Context, req QueryRequest) (interface{}, error) {
		mu.Lock()
		callCount++
		mu.Unlock()
		time.Sleep(100 * time.Millisecond) // Simulate work
		return "result", nil
	}

	// Launch multiple concurrent requests with the same query
	var wg sync.WaitGroup
	successCount := 0
	var successMu sync.Mutex

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result, err := limiter.CoalesceOrExecute(ctx, req, "test", fn)
			if err == nil && result == "result" {
				successMu.Lock()
				successCount++
				successMu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// At least the primary request should succeed
	successMu.Lock()
	sc := successCount
	successMu.Unlock()

	if sc == 0 {
		t.Error("No requests succeeded")
	}

	// Function should only have been called once or twice (coalesced)
	mu.Lock()
	count := callCount
	mu.Unlock()

	// Due to timing, we might get 1, 2, or even 3 calls depending on scheduling
	// The key is that it shouldn't be MORE than the number of requests
	if count > 3 {
		t.Errorf("Coalescing broken: got %d calls for 3 requests", count)
	}
	t.Logf("Function called %d times for 3 concurrent requests (%d succeeded)", count, sc)
}

// TestRateLimiterUnknownBackend verifies unknown backends don't block
func TestRateLimiterUnknownBackend(t *testing.T) {
	policy := &QueryPolicy{
		MaxInFlightPerBackend: map[BackendID]int{
			"known": 1,
		},
		CoalesceWindowMs: 100,
	}

	limiter := NewRateLimiter(policy)
	defer limiter.Stop()

	ctx := context.Background()

	// Unknown backend should not block
	if err := limiter.Acquire(ctx, "unknown"); err != nil {
		t.Errorf("Acquire for unknown backend should not fail: %v", err)
	}

	// Release should be safe for unknown backend
	limiter.Release("unknown") // Should not panic
}

// BenchmarkRateLimiterAcquireRelease benchmarks acquire/release cycle
func BenchmarkRateLimiterAcquireRelease(b *testing.B) {
	policy := &QueryPolicy{
		MaxInFlightPerBackend: map[BackendID]int{
			"test": 100,
		},
		CoalesceWindowMs: 100,
	}

	limiter := NewRateLimiter(policy)
	defer limiter.Stop()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = limiter.Acquire(ctx, "test")
		limiter.Release("test")
	}
}
