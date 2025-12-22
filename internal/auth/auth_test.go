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

// TestManagerRevokeKey tests revoking an API key
func TestManagerRevokeKey(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	config := ManagerConfig{
		Enabled: true,
	}

	manager, err := NewManager(config, db, testLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Create a key first
	opts := CreateKeyOptions{
		Name:   "Key To Revoke",
		Scopes: []Scope{ScopeRead},
	}

	key, rawToken, err := manager.CreateKey(opts)
	if err != nil {
		t.Fatalf("CreateKey() error = %v", err)
	}

	// Verify the key works
	result := manager.Authenticate(rawToken, ScopeRead, "")
	if !result.Authenticated {
		t.Error("Key should work before revocation")
	}

	// Revoke the key
	if err := manager.RevokeKey(key.ID); err != nil {
		t.Fatalf("RevokeKey() error = %v", err)
	}

	// Verify the key no longer works
	result = manager.Authenticate(rawToken, ScopeRead, "")
	if result.Authenticated {
		t.Error("Key should not work after revocation")
	}
	if result.ErrorCode != ErrCodeRevokedToken {
		t.Errorf("ErrorCode = %q, want %q", result.ErrorCode, ErrCodeRevokedToken)
	}

	// Test revoking non-existent key - should return some error
	err = manager.RevokeKey("ckb_key_nonexistent")
	if err == nil {
		t.Error("RevokeKey(nonexistent) should return an error")
	}
}

// TestManagerRevokeKeyNoStore tests RevokeKey without a store
func TestManagerRevokeKeyNoStore(t *testing.T) {
	config := ManagerConfig{
		Enabled: true,
	}

	manager, err := NewManager(config, nil, testLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	err = manager.RevokeKey("any-key")
	if err != ErrStoreNotInitialized {
		t.Errorf("RevokeKey() error = %v, want ErrStoreNotInitialized", err)
	}
}

// TestManagerListKeys tests listing API keys
func TestManagerListKeys(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	config := ManagerConfig{
		Enabled: true,
		StaticKeys: []StaticKeyConfig{
			{
				ID:     "static-key-1",
				Name:   "Static Key 1",
				Token:  "static-token-1",
				Scopes: []string{"read"},
			},
		},
	}

	manager, err := NewManager(config, db, testLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Create a database key
	opts := CreateKeyOptions{
		Name:   "DB Key 1",
		Scopes: []Scope{ScopeWrite},
	}
	key1, _, err := manager.CreateKey(opts)
	if err != nil {
		t.Fatalf("CreateKey() error = %v", err)
	}

	// Create another and revoke it
	opts.Name = "DB Key 2 (Revoked)"
	key2, _, err := manager.CreateKey(opts)
	if err != nil {
		t.Fatalf("CreateKey() error = %v", err)
	}
	if err := manager.RevokeKey(key2.ID); err != nil {
		t.Fatalf("RevokeKey() error = %v", err)
	}

	// List without revoked
	keys, err := manager.ListKeys(false)
	if err != nil {
		t.Fatalf("ListKeys(false) error = %v", err)
	}

	// Should have static key + 1 active db key
	if len(keys) != 2 {
		t.Errorf("ListKeys(false) returned %d keys, want 2", len(keys))
	}

	// Verify token hashes are redacted
	for _, k := range keys {
		if k.TokenHash != "" {
			t.Errorf("Key %s: TokenHash should be redacted", k.ID)
		}
	}

	// Check we have the expected keys
	foundStatic, foundDB := false, false
	for _, k := range keys {
		if k.ID == "static-key-1" {
			foundStatic = true
		}
		if k.ID == key1.ID {
			foundDB = true
		}
	}
	if !foundStatic {
		t.Error("Static key not found in list")
	}
	if !foundDB {
		t.Error("DB key not found in list")
	}

	// List with revoked
	keys, err = manager.ListKeys(true)
	if err != nil {
		t.Fatalf("ListKeys(true) error = %v", err)
	}

	// Should have static key + 2 db keys
	if len(keys) != 3 {
		t.Errorf("ListKeys(true) returned %d keys, want 3", len(keys))
	}
}

// TestManagerGetKey tests getting a single API key
func TestManagerGetKey(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	config := ManagerConfig{
		Enabled: true,
		StaticKeys: []StaticKeyConfig{
			{
				ID:     "static-key",
				Name:   "Static Key",
				Token:  "static-token",
				Scopes: []string{"admin"},
			},
		},
	}

	manager, err := NewManager(config, db, testLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Create a database key
	opts := CreateKeyOptions{
		Name:   "DB Key",
		Scopes: []Scope{ScopeRead},
	}
	dbKey, _, err := manager.CreateKey(opts)
	if err != nil {
		t.Fatalf("CreateKey() error = %v", err)
	}

	// Get static key
	key, err := manager.GetKey("static-key")
	if err != nil {
		t.Fatalf("GetKey(static) error = %v", err)
	}
	if key.Name != "Static Key" {
		t.Errorf("GetKey(static).Name = %q, want %q", key.Name, "Static Key")
	}
	if key.TokenHash != "" {
		t.Error("Static key TokenHash should be redacted")
	}

	// Get database key
	key, err = manager.GetKey(dbKey.ID)
	if err != nil {
		t.Fatalf("GetKey(db) error = %v", err)
	}
	if key.Name != "DB Key" {
		t.Errorf("GetKey(db).Name = %q, want %q", key.Name, "DB Key")
	}
	if key.TokenHash != "" {
		t.Error("DB key TokenHash should be redacted")
	}

	// Get non-existent key
	_, err = manager.GetKey("nonexistent")
	if err != ErrKeyNotFound {
		t.Errorf("GetKey(nonexistent) error = %v, want ErrKeyNotFound", err)
	}
}

// TestManagerGetKeyNoStore tests GetKey without a store
func TestManagerGetKeyNoStore(t *testing.T) {
	config := ManagerConfig{
		Enabled: true,
		StaticKeys: []StaticKeyConfig{
			{
				ID:     "static-key",
				Name:   "Static Key",
				Token:  "static-token",
				Scopes: []string{"read"},
			},
		},
	}

	manager, err := NewManager(config, nil, testLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Should still be able to get static key
	key, err := manager.GetKey("static-key")
	if err != nil {
		t.Fatalf("GetKey(static) error = %v", err)
	}
	if key.Name != "Static Key" {
		t.Errorf("GetKey().Name = %q, want %q", key.Name, "Static Key")
	}

	// Non-existent key should fail
	_, err = manager.GetKey("nonexistent")
	if err != ErrKeyNotFound {
		t.Errorf("GetKey(nonexistent) error = %v, want ErrKeyNotFound", err)
	}
}

// TestManagerRotateKey tests rotating an API key
func TestManagerRotateKey(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	config := ManagerConfig{
		Enabled: true,
		StaticKeys: []StaticKeyConfig{
			{
				ID:     "static-key",
				Name:   "Static Key",
				Token:  "static-token",
				Scopes: []string{"read"},
			},
		},
	}

	manager, err := NewManager(config, db, testLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Create a key to rotate
	opts := CreateKeyOptions{
		Name:   "Key To Rotate",
		Scopes: []Scope{ScopeWrite},
	}
	key, oldToken, err := manager.CreateKey(opts)
	if err != nil {
		t.Fatalf("CreateKey() error = %v", err)
	}

	// Rotate the key
	rotatedKey, newToken, err := manager.RotateKey(key.ID)
	if err != nil {
		t.Fatalf("RotateKey() error = %v", err)
	}

	// Key ID should be the same
	if rotatedKey.ID != key.ID {
		t.Errorf("Rotated key ID = %q, want %q", rotatedKey.ID, key.ID)
	}

	// Token should be different
	if newToken == oldToken {
		t.Error("Rotated token should be different from old token")
	}

	// New token should work
	result := manager.Authenticate(newToken, ScopeWrite, "")
	if !result.Authenticated {
		t.Error("New token should work after rotation")
	}

	// Old token should NOT work
	result = manager.Authenticate(oldToken, ScopeWrite, "")
	if result.Authenticated {
		t.Error("Old token should not work after rotation")
	}

	// Token hash should be redacted in returned key
	if rotatedKey.TokenHash != "" {
		t.Error("Rotated key TokenHash should be redacted")
	}

	// Test rotating static key (should fail)
	_, _, err = manager.RotateKey("static-key")
	if err != ErrKeyNotFound {
		t.Errorf("RotateKey(static) error = %v, want ErrKeyNotFound", err)
	}

	// Test rotating non-existent key
	_, _, err = manager.RotateKey("nonexistent")
	if err != ErrKeyNotFound {
		t.Errorf("RotateKey(nonexistent) error = %v, want ErrKeyNotFound", err)
	}
}

// TestManagerRotateKeyNoStore tests RotateKey without a store
func TestManagerRotateKeyNoStore(t *testing.T) {
	config := ManagerConfig{
		Enabled: true,
	}

	manager, err := NewManager(config, nil, testLogger())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	_, _, err = manager.RotateKey("any-key")
	if err != ErrStoreNotInitialized {
		t.Errorf("RotateKey() error = %v, want ErrStoreNotInitialized", err)
	}
}

// TestMaskToken tests token masking for display
func TestMaskToken(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		expected string
	}{
		{"empty", "", "****"},
		{"short", "abc", "****"},
		{"too short for prefix", "ckb_sk_", "****"},
		{"valid token", "ckb_sk_a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", "ckb_sk_a1b2c3d4****...****"},
		{"minimum valid", "ckb_sk_12345678", "ckb_sk_12345678****...****"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaskToken(tt.token)
			if got != tt.expected {
				t.Errorf("MaskToken(%q) = %q, want %q", tt.token, got, tt.expected)
			}
		})
	}
}

// TestValidScopes tests the ValidScopes function
func TestValidScopes(t *testing.T) {
	scopes := ValidScopes()

	if len(scopes) != 3 {
		t.Errorf("ValidScopes() returned %d scopes, want 3", len(scopes))
	}

	// Verify all expected scopes are present
	expected := map[Scope]bool{
		ScopeRead:  false,
		ScopeWrite: false,
		ScopeAdmin: false,
	}

	for _, s := range scopes {
		if _, ok := expected[s]; !ok {
			t.Errorf("Unexpected scope: %s", s)
		}
		expected[s] = true
	}

	for scope, found := range expected {
		if !found {
			t.Errorf("Missing scope: %s", scope)
		}
	}
}

// TestDefaultManagerConfig tests the default manager configuration
func TestDefaultManagerConfig(t *testing.T) {
	config := DefaultManagerConfig()

	if config.Enabled {
		t.Error("Default config should have Enabled = false")
	}
	// Default is RequireAuth = true (secure by default)
	if !config.RequireAuth {
		t.Error("Default config should have RequireAuth = true")
	}
	if len(config.StaticKeys) != 0 {
		t.Errorf("Default config should have no static keys, got %d", len(config.StaticKeys))
	}
}

// TestDefaultRateLimitConfig tests the default rate limit configuration
func TestDefaultRateLimitConfig(t *testing.T) {
	config := DefaultRateLimitConfig()

	if config.Enabled {
		t.Error("Default rate limit config should have Enabled = false")
	}
	if config.DefaultLimit <= 0 {
		t.Errorf("Default rate limit should be positive, got %d", config.DefaultLimit)
	}
	if config.BurstSize <= 0 {
		t.Errorf("Default burst size should be positive, got %d", config.BurstSize)
	}
}

// TestRateLimiterCustomKeyLimit tests per-key custom rate limits
func TestRateLimiterCustomKeyLimit(t *testing.T) {
	config := RateLimitConfig{
		Enabled:      true,
		DefaultLimit: 60,
		BurstSize:    5,
	}

	limiter := NewRateLimiter(config, testLogger())

	// Custom limit for the key
	customLimit := intPtr(120) // Higher limit

	// Should allow more requests with custom limit
	for i := 0; i < 5; i++ {
		allowed, _ := limiter.Allow("custom-key", customLimit)
		if !allowed {
			t.Errorf("Request %d with custom limit should be allowed", i+1)
		}
	}
}
