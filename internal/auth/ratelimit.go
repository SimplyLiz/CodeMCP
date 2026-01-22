package auth

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// RateLimitConfig configures rate limiting behavior
type RateLimitConfig struct {
	Enabled         bool `toml:"enabled" json:"enabled"`
	DefaultLimit    int  `toml:"default_limit" json:"default_limit"`       // Requests per minute
	BurstSize       int  `toml:"burst_size" json:"burst_size"`             // Token bucket burst
	CleanupInterval int  `toml:"cleanup_interval" json:"cleanup_interval"` // Seconds between cleanup runs
}

// DefaultRateLimitConfig returns sensible defaults
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Enabled:         false,
		DefaultLimit:    60,  // 60 requests per minute
		BurstSize:       10,  // Allow 10 request burst
		CleanupInterval: 300, // Clean up every 5 minutes
	}
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	config  RateLimitConfig
	buckets map[string]*tokenBucket
	mu      sync.RWMutex
	logger  *slog.Logger
}

type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
	limit      int // Tokens per minute (custom per key, or default)
	burst      int
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(config RateLimitConfig, logger *slog.Logger) *RateLimiter {
	if config.DefaultLimit <= 0 {
		config.DefaultLimit = 60
	}
	if config.BurstSize <= 0 {
		config.BurstSize = 10
	}
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 300
	}

	return &RateLimiter{
		config:  config,
		buckets: make(map[string]*tokenBucket),
		logger:  logger,
	}
}

// Allow checks if a request is allowed and consumes a token
// Returns: allowed (bool), retryAfter (seconds until next token available)
func (r *RateLimiter) Allow(keyID string, customLimit *int) (bool, int) {
	if !r.config.Enabled {
		return true, 0
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	bucket, exists := r.buckets[keyID]
	if !exists {
		limit := r.config.DefaultLimit
		if customLimit != nil && *customLimit > 0 {
			limit = *customLimit
		}
		bucket = &tokenBucket{
			tokens:     float64(r.config.BurstSize),
			lastRefill: time.Now(),
			limit:      limit,
			burst:      r.config.BurstSize,
		}
		r.buckets[keyID] = bucket
	}

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill)
	bucket.lastRefill = now

	// Calculate tokens to add (limit per minute = limit/60 per second)
	tokensToAdd := elapsed.Seconds() * (float64(bucket.limit) / 60.0)
	bucket.tokens += tokensToAdd

	// Cap at burst size
	if bucket.tokens > float64(bucket.burst) {
		bucket.tokens = float64(bucket.burst)
	}

	// Try to consume a token
	if bucket.tokens >= 1.0 {
		bucket.tokens -= 1.0
		return true, 0
	}

	// Calculate retry-after (time until we have 1 token)
	tokensNeeded := 1.0 - bucket.tokens
	secondsUntilToken := tokensNeeded / (float64(bucket.limit) / 60.0)
	retryAfter := int(secondsUntilToken) + 1 // Round up

	return false, retryAfter
}

// GetRemaining returns the number of tokens remaining for a key
func (r *RateLimiter) GetRemaining(keyID string) int {
	if !r.config.Enabled {
		return -1 // Unlimited
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	bucket, exists := r.buckets[keyID]
	if !exists {
		return r.config.BurstSize
	}

	// Calculate current tokens (with refill)
	elapsed := time.Since(bucket.lastRefill)
	tokensToAdd := elapsed.Seconds() * (float64(bucket.limit) / 60.0)
	tokens := bucket.tokens + tokensToAdd

	if tokens > float64(bucket.burst) {
		tokens = float64(bucket.burst)
	}

	return int(tokens)
}

// Reset resets the rate limit for a key (e.g., after rotation)
func (r *RateLimiter) Reset(keyID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.buckets, keyID)
}

// StartCleanup starts a background goroutine to clean up stale buckets
func (r *RateLimiter) StartCleanup(ctx context.Context) {
	if !r.config.Enabled {
		return
	}

	go func() {
		ticker := time.NewTicker(time.Duration(r.config.CleanupInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.cleanup()
			}
		}
	}()
}

// cleanup removes buckets that haven't been used recently
func (r *RateLimiter) cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove buckets unused for more than 10 minutes
	cutoff := time.Now().Add(-10 * time.Minute)
	removed := 0

	for keyID, bucket := range r.buckets {
		if bucket.lastRefill.Before(cutoff) {
			delete(r.buckets, keyID)
			removed++
		}
	}

	if removed > 0 && r.logger != nil {
		r.logger.Debug("Rate limit cleanup",
			"removed_buckets", removed,
			"remaining", len(r.buckets),
		)
	}
}

// Stats returns rate limiter statistics
func (r *RateLimiter) Stats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return map[string]interface{}{
		"enabled":       r.config.Enabled,
		"default_limit": r.config.DefaultLimit,
		"burst_size":    r.config.BurstSize,
		"active_keys":   len(r.buckets),
	}
}
