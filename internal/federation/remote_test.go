package federation

import (
t"io"
t"log/slog"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

)

func TestExpandEnvVars(t *testing.T) {
	// Set test env vars
	t.Setenv("TEST_VAR_1", "value1")
	t.Setenv("TEST_VAR_2", "value2")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no variables",
			input:    "plain text",
			expected: "plain text",
		},
		{
			name:     "single variable",
			input:    "${TEST_VAR_1}",
			expected: "value1",
		},
		{
			name:     "variable with prefix",
			input:    "prefix_${TEST_VAR_1}",
			expected: "prefix_value1",
		},
		{
			name:     "variable with suffix",
			input:    "${TEST_VAR_1}_suffix",
			expected: "value1_suffix",
		},
		{
			name:     "multiple variables",
			input:    "${TEST_VAR_1}:${TEST_VAR_2}",
			expected: "value1:value2",
		},
		{
			name:     "undefined variable",
			input:    "${UNDEFINED_VAR}",
			expected: "",
		},
		{
			name:     "mixed defined and undefined",
			input:    "${TEST_VAR_1}:${UNDEFINED_VAR}",
			expected: "value1:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExpandEnvVars(tt.input)
			if result != tt.expected {
				t.Errorf("ExpandEnvVars(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRemoteServerValidate(t *testing.T) {
	tests := []struct {
		name    string
		server  RemoteServer
		wantErr bool
	}{
		{
			name: "valid https server",
			server: RemoteServer{
				Name: "test",
				URL:  "https://example.com",
			},
			wantErr: false,
		},
		{
			name: "valid http server",
			server: RemoteServer{
				Name: "test",
				URL:  "http://localhost:8080",
			},
			wantErr: false,
		},
		{
			name: "trailing slash removed",
			server: RemoteServer{
				Name: "test",
				URL:  "https://example.com/",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			server: RemoteServer{
				URL: "https://example.com",
			},
			wantErr: true,
		},
		{
			name: "missing url",
			server: RemoteServer{
				Name: "test",
			},
			wantErr: true,
		},
		{
			name: "invalid url scheme",
			server: RemoteServer{
				Name: "test",
				URL:  "ftp://example.com",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.server.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRemoteServerGetters(t *testing.T) {
	// Server with custom values
	server := RemoteServer{
		Name:     "test",
		URL:      "https://example.com",
		Token:    "secret-token",
		CacheTTL: Duration{Duration: 2 * time.Hour},
		Timeout:  Duration{Duration: 1 * time.Minute},
	}

	if got := server.GetCacheTTL(); got != 2*time.Hour {
		t.Errorf("GetCacheTTL() = %v, want %v", got, 2*time.Hour)
	}
	if got := server.GetTimeout(); got != 1*time.Minute {
		t.Errorf("GetTimeout() = %v, want %v", got, 1*time.Minute)
	}

	// Server with default values
	defaultServer := RemoteServer{
		Name: "test",
		URL:  "https://example.com",
	}

	if got := defaultServer.GetCacheTTL(); got != DefaultRemoteCacheTTL {
		t.Errorf("GetCacheTTL() = %v, want %v (default)", got, DefaultRemoteCacheTTL)
	}
	if got := defaultServer.GetTimeout(); got != DefaultRemoteTimeout {
		t.Errorf("GetTimeout() = %v, want %v (default)", got, DefaultRemoteTimeout)
	}
}

func TestDurationMarshalUnmarshal(t *testing.T) {
	original := Duration{Duration: 15 * time.Minute}

	// Marshal
	text, err := original.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText() error = %v", err)
	}
	if string(text) != "15m0s" {
		t.Errorf("MarshalText() = %s, want 15m0s", string(text))
	}

	// Unmarshal
	var parsed Duration
	if err := parsed.UnmarshalText([]byte("30m")); err != nil {
		t.Fatalf("UnmarshalText() error = %v", err)
	}
	if parsed.Duration != 30*time.Minute {
		t.Errorf("UnmarshalText() = %v, want %v", parsed.Duration, 30*time.Minute)
	}
}

func TestRemoteClient(t *testing.T) {
	// Create a mock server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check auth header
		auth := r.Header.Get("Authorization")

		switch r.URL.Path {
		case "/index/repos":
			resp := struct {
				Data struct {
					Repos []RemoteRepoInfo `json:"repos"`
				} `json:"data"`
			}{
				Data: struct {
					Repos []RemoteRepoInfo `json:"repos"`
				}{
					Repos: []RemoteRepoInfo{
						{ID: "repo1", Name: "Repo 1", Languages: []string{"go"}},
						{ID: "repo2", Name: "Repo 2", Languages: []string{"python"}},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		case "/index/repos/repo1/meta":
			resp := struct {
				Data RemoteRepoMeta `json:"data"`
			}{
				Data: RemoteRepoMeta{
					ID:           "repo1",
					Name:         "Repo 1",
					Commit:       "abc123",
					IndexVersion: "1.0",
					Languages:    []string{"go"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		case "/index/repos/repo1/search/symbols":
			resp := struct {
				Data struct {
					Truncated bool           `json:"truncated"`
					Symbols   []RemoteSymbol `json:"symbols"`
				} `json:"data"`
			}{
				Data: struct {
					Truncated bool           `json:"truncated"`
					Symbols   []RemoteSymbol `json:"symbols"`
				}{
					Symbols: []RemoteSymbol{
						{ID: "sym1", Name: "Function1", Kind: "function"},
					},
					Truncated: false,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		case "/index/repos/protected/meta":
			if auth != "Bearer test-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(RemoteResponse{
					Error: &RemoteErrorInfo{Code: "unauthorized", Message: "Invalid token"},
				})
				return
			}
			resp := struct {
				Data RemoteRepoMeta `json:"data"`
			}{
				Data: RemoteRepoMeta{ID: "protected", Name: "Protected Repo"},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(RemoteResponse{
				Error: &RemoteErrorInfo{Code: "not_found", Message: "Not found"},
			})
		}
	}))
	defer mockServer.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		Level:  logging.DebugLevel,
		Format: logging.JSONFormat,
		Output: os.Stderr,
	})

	t.Run("ListRepos", func(t *testing.T) {
		server := &RemoteServer{
			Name:    "test",
			URL:     mockServer.URL,
			Timeout: Duration{Duration: 5 * time.Second},
		}

		client := NewRemoteClient(server, nil, logger)
		repos, err := client.ListRepos(context.Background())
		if err != nil {
			t.Fatalf("ListRepos() error = %v", err)
		}

		if len(repos) != 2 {
			t.Errorf("ListRepos() returned %d repos, want 2", len(repos))
		}
		if repos[0].ID != "repo1" {
			t.Errorf("repos[0].ID = %q, want %q", repos[0].ID, "repo1")
		}
	})

	t.Run("GetRepoMeta", func(t *testing.T) {
		server := &RemoteServer{
			Name:    "test",
			URL:     mockServer.URL,
			Timeout: Duration{Duration: 5 * time.Second},
		}

		client := NewRemoteClient(server, nil, logger)
		meta, err := client.GetRepoMeta(context.Background(), "repo1")
		if err != nil {
			t.Fatalf("GetRepoMeta() error = %v", err)
		}

		if meta.ID != "repo1" {
			t.Errorf("meta.ID = %q, want %q", meta.ID, "repo1")
		}
		if meta.Commit != "abc123" {
			t.Errorf("meta.Commit = %q, want %q", meta.Commit, "abc123")
		}
	})

	t.Run("SearchSymbols", func(t *testing.T) {
		server := &RemoteServer{
			Name:    "test",
			URL:     mockServer.URL,
			Timeout: Duration{Duration: 5 * time.Second},
		}

		client := NewRemoteClient(server, nil, logger)
		symbols, truncated, err := client.SearchSymbols(context.Background(), "repo1", &RemoteSymbolSearchOptions{
			Query: "Function",
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("SearchSymbols() error = %v", err)
		}

		if truncated {
			t.Error("SearchSymbols() truncated = true, want false")
		}
		if len(symbols) != 1 {
			t.Errorf("SearchSymbols() returned %d symbols, want 1", len(symbols))
		}
		if symbols[0].Name != "Function1" {
			t.Errorf("symbols[0].Name = %q, want %q", symbols[0].Name, "Function1")
		}
	})

	t.Run("AuthRequired", func(t *testing.T) {
		// Without token
		server := &RemoteServer{
			Name:    "test",
			URL:     mockServer.URL,
			Timeout: Duration{Duration: 5 * time.Second},
		}

		client := NewRemoteClient(server, nil, logger)
		_, err := client.GetRepoMeta(context.Background(), "protected")
		if err == nil {
			t.Fatal("GetRepoMeta() expected error for protected repo without token")
		}

		remoteErr, ok := err.(*RemoteError)
		if !ok {
			t.Fatalf("GetRepoMeta() error type = %T, want *RemoteError", err)
		}
		if !remoteErr.IsUnauthorized() {
			t.Errorf("IsUnauthorized() = false, want true")
		}

		// With token
		serverWithToken := &RemoteServer{
			Name:    "test",
			URL:     mockServer.URL,
			Token:   "test-token",
			Timeout: Duration{Duration: 5 * time.Second},
		}

		clientWithToken := NewRemoteClient(serverWithToken, nil, logger)
		meta, err := clientWithToken.GetRepoMeta(context.Background(), "protected")
		if err != nil {
			t.Fatalf("GetRepoMeta() with token error = %v", err)
		}
		if meta.ID != "protected" {
			t.Errorf("meta.ID = %q, want %q", meta.ID, "protected")
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		server := &RemoteServer{
			Name:    "test",
			URL:     mockServer.URL,
			Timeout: Duration{Duration: 5 * time.Second},
		}

		client := NewRemoteClient(server, nil, logger)
		_, err := client.GetRepoMeta(context.Background(), "nonexistent")
		if err == nil {
			t.Fatal("GetRepoMeta() expected error for nonexistent repo")
		}

		remoteErr, ok := err.(*RemoteError)
		if !ok {
			t.Fatalf("GetRepoMeta() error type = %T, want *RemoteError", err)
		}
		if !remoteErr.IsNotFound() {
			t.Errorf("IsNotFound() = false, want true")
		}
	})
}

func TestCacheKey(t *testing.T) {
	// Same inputs should produce same key
	key1 := cacheKey("search", "symbols", "repo1", "query")
	key2 := cacheKey("search", "symbols", "repo1", "query")
	if key1 != key2 {
		t.Errorf("cacheKey() with same inputs produced different keys: %s != %s", key1, key2)
	}

	// Different inputs should produce different keys
	key3 := cacheKey("search", "symbols", "repo2", "query")
	if key1 == key3 {
		t.Errorf("cacheKey() with different inputs produced same key: %s", key1)
	}
}

func TestRemoteErrorMethods(t *testing.T) {
	tests := []struct {
		name          string
		err           *RemoteError
		isNotFound    bool
		isUnauth      bool
		isForbidden   bool
		isRateLimited bool
	}{
		{
			name:       "404 not found",
			err:        &RemoteError{StatusCode: 404, Code: "not_found", Message: "Not found"},
			isNotFound: true,
		},
		{
			name:     "401 unauthorized",
			err:      &RemoteError{StatusCode: 401, Code: "unauthorized", Message: "Invalid token"},
			isUnauth: true,
		},
		{
			name:        "403 forbidden",
			err:         &RemoteError{StatusCode: 403, Code: "forbidden", Message: "Access denied"},
			isForbidden: true,
		},
		{
			name:          "429 rate limited",
			err:           &RemoteError{StatusCode: 429, Code: "rate_limited", Message: "Too many requests"},
			isRateLimited: true,
		},
		{
			name: "500 server error",
			err:  &RemoteError{StatusCode: 500, Code: "server_error", Message: "Internal error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.IsNotFound(); got != tt.isNotFound {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.isNotFound)
			}
			if got := tt.err.IsUnauthorized(); got != tt.isUnauth {
				t.Errorf("IsUnauthorized() = %v, want %v", got, tt.isUnauth)
			}
			if got := tt.err.IsForbidden(); got != tt.isForbidden {
				t.Errorf("IsForbidden() = %v, want %v", got, tt.isForbidden)
			}
			if got := tt.err.IsRateLimited(); got != tt.isRateLimited {
				t.Errorf("IsRateLimited() = %v, want %v", got, tt.isRateLimited)
			}

			// Error() should return a non-empty string
			errStr := tt.err.Error()
			if errStr == "" {
				t.Error("Error() returned empty string")
			}
		})
	}
}
