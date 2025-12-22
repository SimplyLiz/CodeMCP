package auth

import (
	"time"
)

// Scope represents an API key permission scope
type Scope string

const (
	// ScopeRead allows GET requests, symbol lookup, search
	ScopeRead Scope = "read"
	// ScopeWrite allows POST requests, upload indexes, create repos
	ScopeWrite Scope = "write"
	// ScopeAdmin allows token management, delete repos
	ScopeAdmin Scope = "admin"
)

// ValidScopes returns all valid scope values
func ValidScopes() []Scope {
	return []Scope{ScopeRead, ScopeWrite, ScopeAdmin}
}

// IsValid checks if a scope is valid
func (s Scope) IsValid() bool {
	switch s {
	case ScopeRead, ScopeWrite, ScopeAdmin:
		return true
	default:
		return false
	}
}

// Includes checks if this scope includes the required scope
// admin includes write includes read
func (s Scope) Includes(required Scope) bool {
	switch s {
	case ScopeAdmin:
		return true // admin includes everything
	case ScopeWrite:
		return required == ScopeWrite || required == ScopeRead
	case ScopeRead:
		return required == ScopeRead
	default:
		return false
	}
}

// APIKey represents an API key for authentication
type APIKey struct {
	ID           string     `json:"id"`                      // "ckb_key_" + 16 hex chars
	Name         string     `json:"name"`                    // Human-readable name
	TokenHash    string     `json:"-"`                       // bcrypt hash (never exposed in JSON)
	TokenPrefix  string     `json:"token_prefix"`            // First 8 chars for identification
	Scopes       []Scope    `json:"scopes"`                  // Allowed scopes
	RepoPatterns []string   `json:"repo_patterns,omitempty"` // Glob patterns for allowed repos (empty = all)
	RateLimit    *int       `json:"rate_limit,omitempty"`    // Requests per minute (nil = no limit)
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`    // Expiration time (nil = never)
	CreatedAt    time.Time  `json:"created_at"`              // Creation timestamp
	CreatedBy    string     `json:"created_by,omitempty"`    // Who created this key
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`  // Last use timestamp
	Revoked      bool       `json:"revoked"`                 // Whether key is revoked
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`    // Revocation timestamp
}

// IsExpired checks if the key has expired
func (k *APIKey) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*k.ExpiresAt)
}

// IsActive checks if the key is usable (not revoked, not expired)
func (k *APIKey) IsActive() bool {
	return !k.Revoked && !k.IsExpired()
}

// HasScope checks if the key has the required scope
func (k *APIKey) HasScope(required Scope) bool {
	for _, s := range k.Scopes {
		if s.Includes(required) {
			return true
		}
	}
	return false
}

// CanAccessRepo checks if the key can access the given repo ID
func (k *APIKey) CanAccessRepo(repoID string) bool {
	// Empty patterns means access to all repos
	if len(k.RepoPatterns) == 0 {
		return true
	}

	for _, pattern := range k.RepoPatterns {
		if matchGlob(pattern, repoID) {
			return true
		}
	}
	return false
}

// matchGlob performs simple glob matching (supports * and **)
func matchGlob(pattern, value string) bool {
	// Simple implementation - supports:
	// * - matches any characters except /
	// ** - matches any characters including /
	// Exact match
	if pattern == value {
		return true
	}
	if pattern == "*" || pattern == "**" {
		return true
	}

	// Handle trailing wildcard
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		if len(prefix) > 0 && prefix[len(prefix)-1] == '*' {
			// ** pattern - match anything
			prefix = prefix[:len(prefix)-1]
			return len(value) >= len(prefix) && value[:len(prefix)] == prefix
		}
		// * pattern - match up to /
		return len(value) >= len(prefix) && value[:len(prefix)] == prefix
	}

	// Handle prefix wildcard
	if len(pattern) > 0 && pattern[0] == '*' {
		suffix := pattern[1:]
		if len(suffix) > 0 && suffix[0] == '*' {
			// ** pattern
			suffix = suffix[1:]
			return len(value) >= len(suffix) && value[len(value)-len(suffix):] == suffix
		}
		return len(value) >= len(suffix) && value[len(value)-len(suffix):] == suffix
	}

	return false
}

// AuthResult represents the result of an authentication attempt
type AuthResult struct {
	Authenticated bool     `json:"authenticated"`           // Whether authentication succeeded
	KeyID         string   `json:"key_id,omitempty"`        // ID of the authenticated key
	KeyName       string   `json:"key_name,omitempty"`      // Name of the authenticated key
	Scopes        []Scope  `json:"scopes,omitempty"`        // Scopes of the authenticated key
	RepoPatterns  []string `json:"repo_patterns,omitempty"` // Repo restrictions
	RateLimited   bool     `json:"rate_limited"`            // Whether request is rate limited
	RetryAfter    int      `json:"retry_after,omitempty"`   // Seconds until rate limit resets
	ErrorCode     string   `json:"error_code,omitempty"`    // Error code if auth failed
	ErrorMessage  string   `json:"error_message,omitempty"` // Human-readable error message
}

// Error codes for authentication failures
const (
	ErrCodeMissingToken      = "missing_token"
	ErrCodeInvalidToken      = "invalid_token"
	ErrCodeExpiredToken      = "expired_token"
	ErrCodeRevokedToken      = "revoked_token"
	ErrCodeInsufficientScope = "insufficient_scope"
	ErrCodeRepoNotAllowed    = "repo_not_allowed"
	ErrCodeRateLimited       = "rate_limited"
)

// CreateKeyOptions contains options for creating a new API key
type CreateKeyOptions struct {
	Name         string     `json:"name"`
	Scopes       []Scope    `json:"scopes"`
	RepoPatterns []string   `json:"repo_patterns,omitempty"`
	RateLimit    *int       `json:"rate_limit,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	CreatedBy    string     `json:"created_by,omitempty"`
}

// Validate checks if the options are valid
func (o *CreateKeyOptions) Validate() error {
	if o.Name == "" {
		return ErrNameRequired
	}
	if len(o.Scopes) == 0 {
		return ErrScopesRequired
	}
	for _, s := range o.Scopes {
		if !s.IsValid() {
			return ErrInvalidScope
		}
	}
	if o.RateLimit != nil && *o.RateLimit <= 0 {
		return ErrInvalidRateLimit
	}
	return nil
}

// AuditEvent represents an authentication-related event for logging
type AuditEvent struct {
	EventType  string            `json:"event_type"`
	KeyID      string            `json:"key_id,omitempty"`
	KeyName    string            `json:"key_name,omitempty"`
	IPAddress  string            `json:"ip_address,omitempty"`
	UserAgent  string            `json:"user_agent,omitempty"`
	Details    map[string]string `json:"details,omitempty"`
	OccurredAt time.Time         `json:"occurred_at"`
}

// Audit event types
const (
	AuditEventKeyCreated  = "key_created"
	AuditEventKeyRevoked  = "key_revoked"
	AuditEventKeyRotated  = "key_rotated"
	AuditEventAuthSuccess = "auth_success"
	AuditEventAuthFailed  = "auth_failed"
	AuditEventRateLimited = "rate_limited"
)
