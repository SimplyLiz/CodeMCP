package auth

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// ManagerConfig configures the auth manager
type ManagerConfig struct {
	Enabled      bool              `toml:"enabled" json:"enabled"`
	RequireAuth  bool              `toml:"require_auth" json:"require_auth"` // If false, unauthenticated gets read-only
	LegacyToken  string            `toml:"legacy_token" json:"legacy_token"` // Backward compat single token
	StaticKeys   []StaticKeyConfig `toml:"static_keys" json:"static_keys"`   // TOML-defined keys
	RateLimiting RateLimitConfig   `toml:"rate_limiting" json:"rate_limiting"`
}

// StaticKeyConfig defines a static API key in configuration
type StaticKeyConfig struct {
	ID           string   `toml:"id" json:"id"`
	Name         string   `toml:"name" json:"name"`
	Token        string   `toml:"token" json:"token"` // Plaintext (env var expansion supported)
	Scopes       []string `toml:"scopes" json:"scopes"`
	RepoPatterns []string `toml:"repo_patterns" json:"repo_patterns"`
	RateLimit    *int     `toml:"rate_limit" json:"rate_limit"`
}

// DefaultManagerConfig returns sensible defaults
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		Enabled:      false,
		RequireAuth:  true,
		RateLimiting: DefaultRateLimitConfig(),
	}
}

// Manager handles API key authentication
type Manager struct {
	config      ManagerConfig
	store       *KeyStore
	rateLimiter *RateLimiter
	logger      *slog.Logger
	staticKeys  map[string]*APIKey // In-memory cache of static keys
	mu          sync.RWMutex
}

// NewManager creates a new auth manager
// db can be nil if only using static keys
func NewManager(config ManagerConfig, db *sql.DB, logger *slog.Logger) (*Manager, error) {
	m := &Manager{
		config:     config,
		logger:     logger,
		staticKeys: make(map[string]*APIKey),
	}

	// Initialize store if database provided
	if db != nil {
		m.store = NewKeyStore(db, logger)
		if err := m.store.InitSchema(); err != nil {
			return nil, err
		}
	}

	// Initialize rate limiter
	m.rateLimiter = NewRateLimiter(config.RateLimiting, logger)

	// Load static keys from config
	if err := m.loadStaticKeys(); err != nil {
		return nil, err
	}

	logger.Info("Auth manager initialized", map[string]interface{}{
		"enabled":       config.Enabled,
		"require_auth":  config.RequireAuth,
		"static_keys":   len(config.StaticKeys),
		"has_legacy":    config.LegacyToken != "",
		"rate_limiting": config.RateLimiting.Enabled,
	})

	return m, nil
}

// loadStaticKeys loads static keys from configuration into memory
func (m *Manager) loadStaticKeys() error {
	for _, sk := range m.config.StaticKeys {
		// Expand environment variables in token
		token := expandEnvVars(sk.Token)

		// Hash the token for comparison
		hash, err := HashToken(token)
		if err != nil {
			return err
		}

		// Convert string scopes to Scope type
		var scopes []Scope
		for _, s := range sk.Scopes {
			scopes = append(scopes, Scope(s))
		}

		key := &APIKey{
			ID:           sk.ID,
			Name:         sk.Name,
			TokenHash:    hash,
			TokenPrefix:  ExtractTokenPrefix(token),
			Scopes:       scopes,
			RepoPatterns: sk.RepoPatterns,
			RateLimit:    sk.RateLimit,
			CreatedAt:    time.Now(), // Static keys have no real creation time
		}

		m.staticKeys[sk.ID] = key
	}

	return nil
}

// expandEnvVars expands ${VAR} or $VAR in a string
func expandEnvVars(s string) string {
	// Handle ${VAR} format
	if strings.HasPrefix(s, "${") && strings.HasSuffix(s, "}") {
		varName := s[2 : len(s)-1]
		return os.Getenv(varName)
	}
	// Handle $VAR format
	if strings.HasPrefix(s, "$") {
		return os.Getenv(s[1:])
	}
	return s
}

// Authenticate validates a bearer token and returns auth result
func (m *Manager) Authenticate(token string, requiredScope Scope, repoID string) *AuthResult {
	result := &AuthResult{}

	// Check if auth is enabled
	if !m.config.Enabled {
		result.Authenticated = true
		result.Scopes = []Scope{ScopeAdmin} // Full access when auth disabled
		return result
	}

	// Handle missing token
	if token == "" {
		if !m.config.RequireAuth && requiredScope == ScopeRead {
			// Allow unauthenticated read access
			result.Authenticated = true
			result.Scopes = []Scope{ScopeRead}
			return result
		}
		result.ErrorCode = ErrCodeMissingToken
		result.ErrorMessage = "Authorization header required"
		return result
	}

	// Check legacy token first
	if m.config.LegacyToken != "" {
		legacyToken := expandEnvVars(m.config.LegacyToken)
		if token == legacyToken {
			result.Authenticated = true
			result.KeyID = "legacy"
			result.KeyName = "Legacy Token"
			result.Scopes = []Scope{ScopeAdmin} // Legacy token gets full access
			return result
		}
	}

	// Look up key by token prefix
	prefix := ExtractTokenPrefix(token)
	key := m.findKeyByPrefix(prefix, token)

	if key == nil {
		result.ErrorCode = ErrCodeInvalidToken
		result.ErrorMessage = "Invalid API key"
		return result
	}

	// Verify token matches
	if !VerifyToken(token, key.TokenHash) {
		result.ErrorCode = ErrCodeInvalidToken
		result.ErrorMessage = "Invalid API key"
		return result
	}

	// Check if key is active
	if key.Revoked {
		result.ErrorCode = ErrCodeRevokedToken
		result.ErrorMessage = "API key has been revoked"
		return result
	}

	if key.IsExpired() {
		result.ErrorCode = ErrCodeExpiredToken
		result.ErrorMessage = "API key has expired"
		return result
	}

	// Check scope
	if !key.HasScope(requiredScope) {
		result.ErrorCode = ErrCodeInsufficientScope
		result.ErrorMessage = "Insufficient scope for this operation"
		return result
	}

	// Check repo access
	if repoID != "" && !key.CanAccessRepo(repoID) {
		result.ErrorCode = ErrCodeRepoNotAllowed
		result.ErrorMessage = "API key not authorized for this repository"
		return result
	}

	// Check rate limit
	if m.rateLimiter != nil {
		allowed, retryAfter := m.rateLimiter.Allow(key.ID, key.RateLimit)
		if !allowed {
			result.RateLimited = true
			result.RetryAfter = retryAfter
			result.ErrorCode = ErrCodeRateLimited
			result.ErrorMessage = "Rate limit exceeded"

			// Log rate limit event
			m.logAuditEvent(AuditEvent{
				EventType:  AuditEventRateLimited,
				KeyID:      key.ID,
				KeyName:    key.Name,
				OccurredAt: time.Now(),
			})

			return result
		}
	}

	// Update last used timestamp (async, don't block)
	go m.updateLastUsed(key.ID)

	// Success
	result.Authenticated = true
	result.KeyID = key.ID
	result.KeyName = key.Name
	result.Scopes = key.Scopes
	result.RepoPatterns = key.RepoPatterns

	return result
}

// findKeyByPrefix looks up a key by token prefix
func (m *Manager) findKeyByPrefix(prefix, fullToken string) *APIKey {
	// Check static keys first
	m.mu.RLock()
	for _, key := range m.staticKeys {
		if key.TokenPrefix == prefix && VerifyToken(fullToken, key.TokenHash) {
			m.mu.RUnlock()
			return key
		}
	}
	m.mu.RUnlock()

	// Check database keys
	if m.store != nil {
		keys, err := m.store.GetByTokenPrefix(prefix)
		if err != nil {
			m.logger.Error("Failed to lookup key by prefix", map[string]interface{}{
				"error": err.Error(),
			})
			return nil
		}
		for _, key := range keys {
			if VerifyToken(fullToken, key.TokenHash) {
				return key
			}
		}
	}

	return nil
}

// updateLastUsed updates the last_used_at timestamp for a key
func (m *Manager) updateLastUsed(keyID string) {
	// Skip static keys (they don't have persistent timestamps)
	m.mu.RLock()
	_, isStatic := m.staticKeys[keyID]
	m.mu.RUnlock()
	if isStatic {
		return
	}

	if m.store != nil {
		if err := m.store.UpdateLastUsed(keyID, time.Now()); err != nil {
			m.logger.Warn("Failed to update last used", map[string]interface{}{
				"key_id": keyID,
				"error":  err.Error(),
			})
		}
	}
}

// CreateKey generates a new API key
// Returns: key (without hash), raw token, error
func (m *Manager) CreateKey(opts CreateKeyOptions) (*APIKey, string, error) {
	if err := opts.Validate(); err != nil {
		return nil, "", err
	}

	if m.store == nil {
		return nil, "", ErrStoreNotInitialized
	}

	// Generate key ID and token
	keyID, err := GenerateKeyID()
	if err != nil {
		return nil, "", err
	}

	rawToken, prefix, err := GenerateToken()
	if err != nil {
		return nil, "", err
	}

	tokenHash, err := HashToken(rawToken)
	if err != nil {
		return nil, "", err
	}

	key := &APIKey{
		ID:           keyID,
		Name:         opts.Name,
		TokenHash:    tokenHash,
		TokenPrefix:  prefix,
		Scopes:       opts.Scopes,
		RepoPatterns: opts.RepoPatterns,
		RateLimit:    opts.RateLimit,
		ExpiresAt:    opts.ExpiresAt,
		CreatedAt:    time.Now(),
		CreatedBy:    opts.CreatedBy,
	}

	if err := m.store.Save(key); err != nil {
		return nil, "", err
	}

	// Log audit event
	m.logAuditEvent(AuditEvent{
		EventType:  AuditEventKeyCreated,
		KeyID:      key.ID,
		KeyName:    key.Name,
		OccurredAt: time.Now(),
		Details: map[string]string{
			"created_by": opts.CreatedBy,
		},
	})

	return key, rawToken, nil
}

// RevokeKey revokes an API key
func (m *Manager) RevokeKey(id string) error {
	if m.store == nil {
		return ErrStoreNotInitialized
	}

	key, err := m.store.GetByID(id)
	if err != nil {
		return err
	}

	now := time.Now()
	key.Revoked = true
	key.RevokedAt = &now

	if err := m.store.Update(key); err != nil {
		return err
	}

	// Reset rate limit for this key
	if m.rateLimiter != nil {
		m.rateLimiter.Reset(id)
	}

	// Log audit event
	m.logAuditEvent(AuditEvent{
		EventType:  AuditEventKeyRevoked,
		KeyID:      key.ID,
		KeyName:    key.Name,
		OccurredAt: time.Now(),
	})

	return nil
}

// ListKeys returns all API keys (token hashes redacted)
func (m *Manager) ListKeys(includeRevoked bool) ([]*APIKey, error) {
	var keys []*APIKey

	// Add static keys
	m.mu.RLock()
	for _, key := range m.staticKeys {
		if includeRevoked || !key.Revoked {
			// Clone to avoid exposing hash
			k := *key
			k.TokenHash = "" // Redact
			keys = append(keys, &k)
		}
	}
	m.mu.RUnlock()

	// Add database keys
	if m.store != nil {
		dbKeys, err := m.store.List(includeRevoked)
		if err != nil {
			return nil, err
		}
		for _, key := range dbKeys {
			key.TokenHash = "" // Redact
			keys = append(keys, key)
		}
	}

	return keys, nil
}

// GetKey returns a single API key by ID
func (m *Manager) GetKey(id string) (*APIKey, error) {
	// Check static keys
	m.mu.RLock()
	if key, ok := m.staticKeys[id]; ok {
		m.mu.RUnlock()
		k := *key
		k.TokenHash = "" // Redact
		return &k, nil
	}
	m.mu.RUnlock()

	// Check database
	if m.store == nil {
		return nil, ErrKeyNotFound
	}

	key, err := m.store.GetByID(id)
	if err != nil {
		return nil, err
	}

	key.TokenHash = "" // Redact
	return key, nil
}

// RotateKey creates a new token for an existing key
func (m *Manager) RotateKey(id string) (*APIKey, string, error) {
	if m.store == nil {
		return nil, "", ErrStoreNotInitialized
	}

	// Check if it's a static key
	m.mu.RLock()
	if _, ok := m.staticKeys[id]; ok {
		m.mu.RUnlock()
		return nil, "", ErrKeyNotFound // Can't rotate static keys
	}
	m.mu.RUnlock()

	key, err := m.store.GetByID(id)
	if err != nil {
		return nil, "", err
	}

	// Generate new token
	rawToken, prefix, err := GenerateToken()
	if err != nil {
		return nil, "", err
	}

	tokenHash, err := HashToken(rawToken)
	if err != nil {
		return nil, "", err
	}

	key.TokenHash = tokenHash
	key.TokenPrefix = prefix

	if err := m.store.Update(key); err != nil {
		return nil, "", err
	}

	// Reset rate limit for rotated key
	if m.rateLimiter != nil {
		m.rateLimiter.Reset(id)
	}

	// Log audit event
	m.logAuditEvent(AuditEvent{
		EventType:  AuditEventKeyRotated,
		KeyID:      key.ID,
		KeyName:    key.Name,
		OccurredAt: time.Now(),
	})

	key.TokenHash = "" // Redact for return
	return key, rawToken, nil
}

// logAuditEvent logs an audit event if store is available
func (m *Manager) logAuditEvent(event AuditEvent) {
	if m.store != nil {
		if err := m.store.LogAuditEvent(event); err != nil {
			m.logger.Warn("Failed to log audit event", map[string]interface{}{
				"event_type": event.EventType,
				"error":      err.Error(),
			})
		}
	}
}

// StartBackgroundTasks starts background maintenance tasks
func (m *Manager) StartBackgroundTasks(ctx context.Context) {
	if m.rateLimiter != nil {
		m.rateLimiter.StartCleanup(ctx)
	}
}

// Stats returns manager statistics
func (m *Manager) Stats() map[string]interface{} {
	stats := map[string]interface{}{
		"enabled":      m.config.Enabled,
		"require_auth": m.config.RequireAuth,
		"has_legacy":   m.config.LegacyToken != "",
	}

	m.mu.RLock()
	stats["static_keys"] = len(m.staticKeys)
	m.mu.RUnlock()

	if m.rateLimiter != nil {
		stats["rate_limiter"] = m.rateLimiter.Stats()
	}

	return stats
}
