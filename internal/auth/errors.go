package auth

import "errors"

var (
	// Validation errors
	ErrNameRequired     = errors.New("name is required")
	ErrScopesRequired   = errors.New("at least one scope is required")
	ErrInvalidScope     = errors.New("invalid scope")
	ErrInvalidRateLimit = errors.New("rate limit must be positive")

	// Key lookup errors
	ErrKeyNotFound = errors.New("API key not found")
	ErrKeyRevoked  = errors.New("API key has been revoked")
	ErrKeyExpired  = errors.New("API key has expired")

	// Store errors
	ErrStoreNotInitialized = errors.New("key store not initialized")
	ErrDuplicateKeyID      = errors.New("duplicate key ID")
)
