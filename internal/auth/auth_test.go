package auth

import (
	"database/sql"
	"os"
	"testing"
	"time"

	"ckb/internal/logging"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

// testLogger returns a silent logger for tests
func testLogger() *logging.Logger {
	return logging.NewLogger(logging.Config{
		Level:  logging.DebugLevel,
		Format: logging.JSONFormat,
		Output: os.Stderr,
	})
}

// testDB creates an in-memory SQLite database for testing
func testDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	return db
}

// TestScope tests scope validation and inclusion
func TestScope(t *testing.T) {
	tests := []struct {
		scope    Scope
		valid    bool
		includes Scope
		expected bool
	}{
		{ScopeRead, true, ScopeRead, true},
		{ScopeRead, true, ScopeWrite, false},
		{ScopeRead, true, ScopeAdmin, false},
		{ScopeWrite, true, ScopeRead, true},
		{ScopeWrite, true, ScopeWrite, true},
		{ScopeWrite, true, ScopeAdmin, false},
		{ScopeAdmin, true, ScopeRead, true},
		{ScopeAdmin, true, ScopeWrite, true},
		{ScopeAdmin, true, ScopeAdmin, true},
		{Scope("invalid"), false, ScopeRead, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.scope)+"_"+string(tt.includes), func(t *testing.T) {
			if got := tt.scope.IsValid(); got != tt.valid {
				t.Errorf("IsValid() = %v, want %v", got, tt.valid)
			}
			if got := tt.scope.Includes(tt.includes); got != tt.expected {
				t.Errorf("Includes(%s) = %v, want %v", tt.includes, got, tt.expected)
			}
		})
	}
}

// TestTokenGeneration tests token generation and validation
func TestTokenGeneration(t *testing.T) {
	// Generate a token
	token, prefix, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	// Verify format
	if !IsValidTokenFormat(token) {
		t.Errorf("Generated token has invalid format: %s", token)
	}

	// Verify prefix extraction
	if got := ExtractTokenPrefix(token); got != prefix {
		t.Errorf("ExtractTokenPrefix() = %s, want %s", got, prefix)
	}

	// Verify hash and verification
	hash, err := HashToken(token)
	if err != nil {
		t.Fatalf("HashToken() error = %v", err)
	}

	if !VerifyToken(token, hash) {
		t.Error("VerifyToken() returned false for correct token")
	}

	if VerifyToken("wrong_token", hash) {
		t.Error("VerifyToken() returned true for wrong token")
	}
}

// TestKeyIDGeneration tests key ID generation
func TestKeyIDGeneration(t *testing.T) {
	id, err := GenerateKeyID()
	if err != nil {
		t.Fatalf("GenerateKeyID() error = %v", err)
	}

	if !IsValidKeyIDFormat(id) {
		t.Errorf("Generated key ID has invalid format: %s", id)
	}

	// Verify uniqueness
	id2, _ := GenerateKeyID()
	if id == id2 {
		t.Error("GenerateKeyID() returned duplicate IDs")
	}
}

// TestAPIKeyCanAccessRepo tests repo pattern matching
func TestAPIKeyCanAccessRepo(t *testing.T) {
	tests := []struct {
		patterns []string
		repoID   string
		expected bool
	}{
		{nil, "any/repo", true},                         // No patterns = all access
		{[]string{}, "any/repo", true},                  // Empty patterns = all access
		{[]string{"*"}, "any/repo", true},               // Wildcard
		{[]string{"myorg/*"}, "myorg/repo", true},       // Org prefix
		{[]string{"myorg/*"}, "other/repo", false},      // Wrong org
		{[]string{"exact/match"}, "exact/match", true},  // Exact match
		{[]string{"exact/match"}, "other/repo", false},  // No match
		{[]string{"a/*", "b/*"}, "b/repo", true},        // Multiple patterns
	}

	for _, tt := range tests {
		key := &APIKey{RepoPatterns: tt.patterns}
		if got := key.CanAccessRepo(tt.repoID); got != tt.expected {
			t.Errorf("CanAccessRepo(%q) with patterns %v = %v, want %v",
				tt.repoID, tt.patterns, got, tt.expected)
		}
	}
}

// TestAPIKeyIsActive tests key active status
func TestAPIKeyIsActive(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	tests := []struct {
		name     string
		key      *APIKey
		expected bool
	}{
		{
			name:     "active key",
			key:      &APIKey{},
			expected: true,
		},
		{
			name:     "revoked key",
			key:      &APIKey{Revoked: true},
			expected: false,
		},
		{
			name:     "expired key",
			key:      &APIKey{ExpiresAt: &past},
			expected: false,
		},
		{
			name:     "not yet expired",
			key:      &APIKey{ExpiresAt: &future},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.key.IsActive(); got != tt.expected {
				t.Errorf("IsActive() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestKeyStore tests the key store CRUD operations
func TestKeyStore(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	store := NewKeyStore(db, testLogger())
	if err := store.InitSchema(); err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}

	// Create a key
	token, prefix, _ := GenerateToken()
	hash, _ := HashToken(token)
	keyID, _ := GenerateKeyID()

	key := &APIKey{
		ID:           keyID,
		Name:         "Test Key",
		TokenHash:    hash,
		TokenPrefix:  prefix,
		Scopes:       []Scope{ScopeRead, ScopeWrite},
		RepoPatterns: []string{"myorg/*"},
		CreatedAt:    time.Now(),
	}

	// Save
	if err := store.Save(key); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Get by ID
	got, err := store.GetByID(keyID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Name != "Test Key" {
		t.Errorf("GetByID().Name = %q, want %q", got.Name, "Test Key")
	}
	if len(got.Scopes) != 2 {
		t.Errorf("GetByID().Scopes len = %d, want 2", len(got.Scopes))
	}

	// Get by prefix
	keys, err := store.GetByTokenPrefix(prefix)
	if err != nil {
		t.Fatalf("GetByTokenPrefix() error = %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("GetByTokenPrefix() returned %d keys, want 1", len(keys))
	}

	// List
	keys, err = store.List(false)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("List() returned %d keys, want 1", len(keys))
	}

	// Update
	key.Name = "Updated Key"
	key.Revoked = true
	now := time.Now()
	key.RevokedAt = &now
	if err := store.Update(key); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, _ = store.GetByID(keyID)
	if got.Name != "Updated Key" {
		t.Errorf("After Update(), Name = %q, want %q", got.Name, "Updated Key")
	}
	if !got.Revoked {
		t.Error("After Update(), Revoked = false, want true")
	}

	// List should exclude revoked by default
	keys, _ = store.List(false)
	if len(keys) != 0 {
		t.Errorf("List(false) returned %d revoked keys, want 0", len(keys))
	}

	// List with revoked
	keys, _ = store.List(true)
	if len(keys) != 1 {
		t.Errorf("List(true) returned %d keys, want 1", len(keys))
	}

	// Delete
	if err := store.Delete(keyID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = store.GetByID(keyID)
	if err != ErrKeyNotFound {
		t.Errorf("GetByID() after Delete() error = %v, want ErrKeyNotFound", err)
	}
}

// TestRateLimiter tests the rate limiter
func TestRateLimiter(t *testing.T) {
	config := RateLimitConfig{
		Enabled:      true,
		DefaultLimit: 60, // 1 per second
		BurstSize:    5,
	}

	limiter := NewRateLimiter(config, testLogger())

	keyID := "test-key"

	// Should allow up to burst size initially
	for i := 0; i < 5; i++ {
		allowed, _ := limiter.Allow(keyID, nil)
		if !allowed {
			t.Errorf("Request %d should be allowed (burst)", i+1)
		}
	}

	// Next request should be rate limited
	allowed, retryAfter := limiter.Allow(keyID, nil)
	if allowed {
		t.Error("Request after burst should be rate limited")
	}
	if retryAfter <= 0 {
		t.Errorf("RetryAfter should be positive, got %d", retryAfter)
	}

	// Test remaining count
	remaining := limiter.GetRemaining(keyID)
	if remaining != 0 {
		t.Errorf("GetRemaining() = %d, want 0", remaining)
	}

	// Test reset
	limiter.Reset(keyID)
	remaining = limiter.GetRemaining(keyID)
	if remaining != 5 {
		t.Errorf("GetRemaining() after Reset() = %d, want 5", remaining)
	}
}

// TestRateLimiterDisabled tests rate limiter when disabled
func TestRateLimiterDisabled(t *testing.T) {
	config := RateLimitConfig{
		Enabled: false,
	}

	limiter := NewRateLimiter(config, testLogger())

	// Should always allow when disabled
	for i := 0; i < 100; i++ {
		allowed, _ := limiter.Allow("any-key", nil)
		if !allowed {
			t.Error("Disabled rate limiter should always allow")
		}
	}

	// GetRemaining should return -1 (unlimited)
	remaining := limiter.GetRemaining("any-key")
	if remaining != -1 {
		t.Errorf("GetRemaining() = %d, want -1 (unlimited)", remaining)
	}
}

// TestManagerAuthenticate tests the auth manager authentication
func TestManagerAuthenticate(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	config := ManagerConfig{
		Enabled:     true,
		RequireAuth: true,
		StaticKeys: []StaticKeyConfig{
			{
				ID:     "static-read",
				Name:   "Static Read Key",
				Token:  "static-token-123",
				Scopes: []string{"read"},
			},
		},
		LegacyToken: "legacy-token-456",
	}

	manager, err := NewManager(config, db, testLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	tests := []struct {
		name     string
		token    string
		scope    Scope
		repoID   string
		wantAuth bool
		wantErr  string
	}{
		{
			name:     "legacy token full access",
			token:    "legacy-token-456",
			scope:    ScopeAdmin,
			wantAuth: true,
		},
		{
			name:     "static token read access",
			token:    "static-token-123",
			scope:    ScopeRead,
			wantAuth: true,
		},
		{
			name:     "static token insufficient scope",
			token:    "static-token-123",
			scope:    ScopeWrite,
			wantAuth: false,
			wantErr:  ErrCodeInsufficientScope,
		},
		{
			name:     "invalid token",
			token:    "wrong-token",
			scope:    ScopeRead,
			wantAuth: false,
			wantErr:  ErrCodeInvalidToken,
		},
		{
			name:     "missing token",
			token:    "",
			scope:    ScopeWrite,
			wantAuth: false,
			wantErr:  ErrCodeMissingToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.Authenticate(tt.token, tt.scope, tt.repoID)
			if result.Authenticated != tt.wantAuth {
				t.Errorf("Authenticated = %v, want %v", result.Authenticated, tt.wantAuth)
			}
			if tt.wantErr != "" && result.ErrorCode != tt.wantErr {
				t.Errorf("ErrorCode = %q, want %q", result.ErrorCode, tt.wantErr)
			}
		})
	}
}

// TestManagerAuthDisabled tests authentication when auth is disabled
func TestManagerAuthDisabled(t *testing.T) {
	config := ManagerConfig{
		Enabled: false,
	}

	manager, err := NewManager(config, nil, testLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Should always authenticate when disabled
	result := manager.Authenticate("", ScopeAdmin, "any/repo")
	if !result.Authenticated {
		t.Error("Disabled auth should always authenticate")
	}
}

// TestManagerCreateKey tests key creation
func TestManagerCreateKey(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	config := ManagerConfig{
		Enabled: true,
	}

	manager, err := NewManager(config, db, testLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	opts := CreateKeyOptions{
		Name:         "Test Key",
		Scopes:       []Scope{ScopeRead, ScopeWrite},
		RepoPatterns: []string{"myorg/*"},
	}

	key, rawToken, err := manager.CreateKey(opts)
	if err != nil {
		t.Fatalf("CreateKey() error = %v", err)
	}

	if key.Name != "Test Key" {
		t.Errorf("CreateKey().Name = %q, want %q", key.Name, "Test Key")
	}
	if !IsValidTokenFormat(rawToken) {
		t.Errorf("CreateKey() returned invalid token format: %s", rawToken)
	}

	// Verify the token works for authentication
	result := manager.Authenticate(rawToken, ScopeRead, "myorg/repo")
	if !result.Authenticated {
		t.Errorf("Created token failed authentication: %v", result.ErrorCode)
	}

	// Verify repo restriction
	result = manager.Authenticate(rawToken, ScopeRead, "other/repo")
	if result.Authenticated {
		t.Error("Created token should not work for other repos")
	}
}

// TestCreateKeyOptionsValidate tests key creation options validation
func TestCreateKeyOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    CreateKeyOptions
		wantErr error
	}{
		{
			name:    "empty name",
			opts:    CreateKeyOptions{Scopes: []Scope{ScopeRead}},
			wantErr: ErrNameRequired,
		},
		{
			name:    "empty scopes",
			opts:    CreateKeyOptions{Name: "Test"},
			wantErr: ErrScopesRequired,
		},
		{
			name:    "invalid scope",
			opts:    CreateKeyOptions{Name: "Test", Scopes: []Scope{"invalid"}},
			wantErr: ErrInvalidScope,
		},
		{
			name: "invalid rate limit",
			opts: CreateKeyOptions{
				Name:      "Test",
				Scopes:    []Scope{ScopeRead},
				RateLimit: intPtr(-1),
			},
			wantErr: ErrInvalidRateLimit,
		},
		{
			name: "valid",
			opts: CreateKeyOptions{
				Name:   "Test",
				Scopes: []Scope{ScopeRead},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func intPtr(i int) *int {
	return &i
}
