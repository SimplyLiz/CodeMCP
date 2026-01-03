package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"ckb/internal/auth"

	_ "modernc.org/sqlite"
)

// testLogger returns a silent logger for tests
func testMiddlewareLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// testDB creates an in-memory SQLite database for testing
func testMiddlewareDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	return db
}

func TestDetermineRequiredScope(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		expected auth.Scope
	}{
		{"GET request", "GET", "/index/repos", auth.ScopeRead},
		{"HEAD request", "HEAD", "/index/repos", auth.ScopeRead},
		{"OPTIONS request", "OPTIONS", "/index/repos", auth.ScopeRead},
		{"POST request", "POST", "/index/repos", auth.ScopeWrite},
		{"PUT request", "PUT", "/index/repos/org/repo/upload", auth.ScopeWrite},
		{"DELETE request", "DELETE", "/index/repos/org/repo", auth.ScopeAdmin},
		{"POST to tokens", "POST", "/tokens", auth.ScopeAdmin},
		{"DELETE to tokens", "DELETE", "/tokens/ckb_key_123", auth.ScopeAdmin},
		{"GET tokens", "GET", "/tokens", auth.ScopeRead},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			got := determineRequiredScope(req)
			if got != tt.expected {
				t.Errorf("determineRequiredScope() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestExtractRepoIDFromRequest(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"no index prefix", "/health", ""},
		{"repos list", "/index/repos", ""},
		{"simple repo symbols", "/index/repos/myrepo/symbols", "myrepo"},
		{"org/repo symbols", "/index/repos/org/repo/symbols", "org/repo"},
		{"org/repo files", "/index/repos/org/repo/files", "org/repo"},
		{"org/repo refs", "/index/repos/org/repo/refs", "org/repo"},
		{"org/repo callgraph", "/index/repos/org/repo/callgraph", "org/repo"},
		{"org/repo meta", "/index/repos/org/repo/meta", "org/repo"},
		{"org/repo upload", "/index/repos/org/repo/upload", "org/repo"},
		{"search symbols endpoint", "/index/repos/org/repo/search", "org/repo"},
		{"deep org path", "/index/repos/company/team/project/symbols", "company/team/project"},
		{"trailing slash", "/index/repos/org/repo/", "org/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			got := extractRepoIDFromRequest(req)
			if got != tt.expected {
				t.Errorf("extractRepoIDFromRequest() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name       string
		authHeader string
		expected   string
	}{
		{"no header", "", ""},
		{"bearer token", "Bearer my-token-123", "my-token-123"},
		{"bearer lowercase", "bearer my-token", ""},
		{"basic auth", "Basic dXNlcjpwYXNz", ""},
		{"just bearer", "Bearer ", ""},
		{"bearer with spaces", "Bearer  token-with-space", " token-with-space"},
		{"long token", "Bearer ckb_sk_a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", "ckb_sk_a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			got := extractBearerToken(req)
			if got != tt.expected {
				t.Errorf("extractBearerToken() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestWriteAuthError(t *testing.T) {
	tests := []struct {
		name           string
		result         *auth.AuthResult
		expectedStatus int
		expectedCode   string
	}{
		{
			name: "missing token",
			result: &auth.AuthResult{
				ErrorCode:    auth.ErrCodeMissingToken,
				ErrorMessage: "missing token",
			},
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   auth.ErrCodeMissingToken,
		},
		{
			name: "invalid token",
			result: &auth.AuthResult{
				ErrorCode:    auth.ErrCodeInvalidToken,
				ErrorMessage: "invalid token",
			},
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   auth.ErrCodeInvalidToken,
		},
		{
			name: "expired token",
			result: &auth.AuthResult{
				ErrorCode:    auth.ErrCodeExpiredToken,
				ErrorMessage: "token expired",
			},
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   auth.ErrCodeExpiredToken,
		},
		{
			name: "revoked token",
			result: &auth.AuthResult{
				ErrorCode:    auth.ErrCodeRevokedToken,
				ErrorMessage: "token revoked",
			},
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   auth.ErrCodeRevokedToken,
		},
		{
			name: "insufficient scope",
			result: &auth.AuthResult{
				ErrorCode:    auth.ErrCodeInsufficientScope,
				ErrorMessage: "insufficient scope",
			},
			expectedStatus: http.StatusForbidden,
			expectedCode:   auth.ErrCodeInsufficientScope,
		},
		{
			name: "repo not allowed",
			result: &auth.AuthResult{
				ErrorCode:    auth.ErrCodeRepoNotAllowed,
				ErrorMessage: "repo not allowed",
			},
			expectedStatus: http.StatusForbidden,
			expectedCode:   auth.ErrCodeRepoNotAllowed,
		},
		{
			name: "rate limited",
			result: &auth.AuthResult{
				ErrorCode:    auth.ErrCodeRateLimited,
				ErrorMessage: "rate limited",
				RetryAfter:   60,
			},
			expectedStatus: http.StatusTooManyRequests,
			expectedCode:   auth.ErrCodeRateLimited,
		},
		{
			name: "unknown error",
			result: &auth.AuthResult{
				ErrorCode:    "unknown_error",
				ErrorMessage: "unknown",
			},
			expectedStatus: http.StatusUnauthorized,
			expectedCode:   "unknown_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeAuthError(w, tt.result)

			if w.Code != tt.expectedStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.expectedStatus)
			}

			var response map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			errObj, ok := response["error"].(map[string]interface{})
			if !ok {
				t.Fatal("Response should have 'error' object")
			}

			if errObj["code"] != tt.expectedCode {
				t.Errorf("error.code = %v, want %v", errObj["code"], tt.expectedCode)
			}

			// Check retry_after for rate limited
			if tt.result.RetryAfter > 0 {
				if errObj["retry_after"] == nil {
					t.Error("rate limited response should have retry_after")
				}
			}
		})
	}
}

func TestGetAuthResult(t *testing.T) {
	// Test with no auth result in context
	ctx := context.Background()
	if result := GetAuthResult(ctx); result != nil {
		t.Error("GetAuthResult() should return nil for context without auth")
	}

	// Test with auth result in context
	expectedResult := &auth.AuthResult{
		Authenticated: true,
		KeyID:         "test-key",
	}
	ctx = context.WithValue(ctx, authResultKey, expectedResult)
	result := GetAuthResult(ctx)
	if result == nil {
		t.Fatal("GetAuthResult() should return result from context")
	}
	if result.KeyID != "test-key" {
		t.Errorf("GetAuthResult().KeyID = %q, want %q", result.KeyID, "test-key")
	}
}

func TestScopedAuthMiddleware(t *testing.T) {
	db := testMiddlewareDB(t)
	defer func() { _ = db.Close() }()

	logger := testMiddlewareLogger()

	// Create auth manager with a static key
	config := auth.ManagerConfig{
		Enabled:     true,
		RequireAuth: true,
		StaticKeys: []auth.StaticKeyConfig{
			{
				ID:           "test-key",
				Name:         "Test Key",
				Token:        "test-token-123",
				Scopes:       []string{"read", "write"},
				RepoPatterns: []string{"allowed/*"},
			},
		},
	}

	manager, err := auth.NewManager(config, db, logger)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// Create a simple handler that returns 200
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check auth result is in context
		result := GetAuthResult(r.Context())
		if result == nil {
			t.Error("Auth result should be in context")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Wrap with middleware
	wrapped := ScopedAuthMiddleware(manager, logger)(handler)

	tests := []struct {
		name           string
		method         string
		path           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "read without token",
			method:         "GET",
			path:           "/index/repos/allowed/repo/symbols",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "read with valid token",
			method:         "GET",
			path:           "/index/repos/allowed/repo/symbols",
			authHeader:     "Bearer test-token-123",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "write with valid token",
			method:         "POST",
			path:           "/index/repos/allowed/repo/upload",
			authHeader:     "Bearer test-token-123",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "admin with write-only token",
			method:         "DELETE",
			path:           "/index/repos/allowed/repo",
			authHeader:     "Bearer test-token-123",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "access to non-allowed repo",
			method:         "GET",
			path:           "/index/repos/other/repo/symbols",
			authHeader:     "Bearer test-token-123",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "invalid token",
			method:         "GET",
			path:           "/index/repos/allowed/repo/symbols",
			authHeader:     "Bearer wrong-token",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()

			wrapped.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.expectedStatus)
			}
		})
	}
}

func TestScopedAuthMiddlewareRateLimiting(t *testing.T) {
	db := testMiddlewareDB(t)
	defer func() { _ = db.Close() }()

	logger := testMiddlewareLogger()

	// Create auth manager with rate limiting
	// Use very low refill rate (1/minute = 1/60 per second) so tokens don't
	// refill between requests even on slow CI machines
	config := auth.ManagerConfig{
		Enabled:     true,
		RequireAuth: true,
		RateLimiting: auth.RateLimitConfig{
			Enabled:      true,
			DefaultLimit: 1, // 1 per minute - very slow refill to avoid CI flakiness
			BurstSize:    2, // Low burst for testing
		},
		StaticKeys: []auth.StaticKeyConfig{
			{
				ID:     "rate-test-key",
				Name:   "Rate Test Key",
				Token:  "rate-test-token",
				Scopes: []string{"read"},
			},
		},
	}

	manager, err := auth.NewManager(config, db, logger)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := ScopedAuthMiddleware(manager, logger)(handler)

	// Make requests up to burst limit
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/index/repos/any/symbols", nil)
		req.Header.Set("Authorization", "Bearer rate-test-token")
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// Next request should be rate limited
	req := httptest.NewRequest("GET", "/index/repos/any/symbols", nil)
	req.Header.Set("Authorization", "Bearer rate-test-token")
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("rate limited request: expected 429, got %d", w.Code)
	}

	// Check Retry-After header
	retryAfter := w.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("rate limited response should have Retry-After header")
	}

	// Check X-RateLimit-Remaining header
	remaining := w.Header().Get("X-RateLimit-Remaining")
	if remaining != "0" {
		t.Errorf("X-RateLimit-Remaining = %q, want %q", remaining, "0")
	}
}

func TestScopedAuthMiddlewareDisabled(t *testing.T) {
	logger := testMiddlewareLogger()

	// Create auth manager with auth disabled
	config := auth.ManagerConfig{
		Enabled: false,
	}

	manager, err := auth.NewManager(config, nil, logger)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := ScopedAuthMiddleware(manager, logger)(handler)

	// Request without token should succeed when auth is disabled
	req := httptest.NewRequest("POST", "/index/repos/any/upload", nil)
	w := httptest.NewRecorder()
	wrapped.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("disabled auth: expected 200, got %d", w.Code)
	}
}

func TestScopedAuthMiddlewareHealthEndpointExemption(t *testing.T) {
	db := testMiddlewareDB(t)
	defer func() { _ = db.Close() }()

	logger := testMiddlewareLogger()

	// Create auth manager with strict auth enabled
	config := auth.ManagerConfig{
		Enabled:     true,
		RequireAuth: true,
		StaticKeys: []auth.StaticKeyConfig{
			{
				ID:     "test-key",
				Name:   "Test Key",
				Token:  "test-token-123",
				Scopes: []string{"read"},
			},
		},
	}

	manager, err := auth.NewManager(config, db, logger)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	wrapped := ScopedAuthMiddleware(manager, logger)(handler)

	// Health endpoints should be accessible without authentication
	// This is critical for Kubernetes probes and load balancer health checks
	exemptPaths := []string{"/health", "/ready", "/health/detailed"}

	for _, path := range exemptPaths {
		t.Run("GET "+path+" without token", func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			wrapped.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("health endpoint %s should be exempt from auth, got status %d", path, w.Code)
			}
		})
	}

	// Verify that other endpoints still require auth
	t.Run("GET /status requires auth", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/status", nil)
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("/status should require auth, got status %d", w.Code)
		}
	})
}
