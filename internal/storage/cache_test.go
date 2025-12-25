package storage

import (
	"os"
	"testing"
	"time"

	"ckb/internal/logging"
)

func TestNewCache(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-cache-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	db, err := Open(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	cache := NewCache(db)
	if cache == nil {
		t.Fatal("NewCache returned nil")
	}
	if cache.db != db {
		t.Error("cache.db should be the provided db")
	}
}

func TestQueryCache(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-cache-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	db, err := Open(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	cache := NewCache(db)

	t.Run("miss on empty cache", func(t *testing.T) {
		value, found, err := cache.GetQueryCache("nonexistent", "commit123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Error("expected not found for nonexistent key")
		}
		if value != "" {
			t.Errorf("expected empty value, got %q", value)
		}
	})

	t.Run("set and get", func(t *testing.T) {
		key := "test-key"
		valueJSON := `{"result": "test"}`
		headCommit := "abc123"
		stateID := "state-1"
		ttl := 300

		err := cache.SetQueryCache(key, valueJSON, headCommit, stateID, ttl)
		if err != nil {
			t.Fatalf("SetQueryCache failed: %v", err)
		}

		value, found, err := cache.GetQueryCache(key, headCommit)
		if err != nil {
			t.Fatalf("GetQueryCache failed: %v", err)
		}
		if !found {
			t.Error("expected to find cached value")
		}
		if value != valueJSON {
			t.Errorf("value = %q, want %q", value, valueJSON)
		}
	})

	t.Run("different commit misses", func(t *testing.T) {
		key := "test-key-2"
		valueJSON := `{"result": "test2"}`

		err := cache.SetQueryCache(key, valueJSON, "commit-a", "state-1", 300)
		if err != nil {
			t.Fatalf("SetQueryCache failed: %v", err)
		}

		// Should not find with different commit
		_, found, err := cache.GetQueryCache(key, "commit-b")
		if err != nil {
			t.Fatalf("GetQueryCache failed: %v", err)
		}
		if found {
			t.Error("expected not found for different commit")
		}
	})
}

func TestViewCache(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-cache-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	db, err := Open(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	cache := NewCache(db)

	t.Run("miss on empty cache", func(t *testing.T) {
		value, found, err := cache.GetViewCache("nonexistent", "state-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if found {
			t.Error("expected not found for nonexistent key")
		}
		if value != "" {
			t.Errorf("expected empty value, got %q", value)
		}
	})

	t.Run("set and get", func(t *testing.T) {
		key := "view-key"
		valueJSON := `{"view": "data"}`
		stateID := "state-123"
		ttl := 3600

		err := cache.SetViewCache(key, valueJSON, stateID, ttl)
		if err != nil {
			t.Fatalf("SetViewCache failed: %v", err)
		}

		value, found, err := cache.GetViewCache(key, stateID)
		if err != nil {
			t.Fatalf("GetViewCache failed: %v", err)
		}
		if !found {
			t.Error("expected to find cached value")
		}
		if value != valueJSON {
			t.Errorf("value = %q, want %q", value, valueJSON)
		}
	})

	t.Run("different state misses", func(t *testing.T) {
		key := "view-key-2"
		valueJSON := `{"view": "data2"}`

		err := cache.SetViewCache(key, valueJSON, "state-a", 3600)
		if err != nil {
			t.Fatalf("SetViewCache failed: %v", err)
		}

		// Should not find with different state
		_, found, err := cache.GetViewCache(key, "state-b")
		if err != nil {
			t.Fatalf("GetViewCache failed: %v", err)
		}
		if found {
			t.Error("expected not found for different state")
		}
	})
}

func TestNegativeCache(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-cache-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	db, err := Open(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	cache := NewCache(db)

	t.Run("miss on empty cache", func(t *testing.T) {
		entry, err := cache.GetNegativeCache("nonexistent", "state-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if entry != nil {
			t.Error("expected nil for nonexistent key")
		}
	})

	t.Run("set and get", func(t *testing.T) {
		key := "error-key"
		errorType := "symbol_not_found"
		errorMessage := "Symbol not found"
		stateID := "state-123"
		ttl := 60

		err := cache.SetNegativeCache(key, errorType, errorMessage, stateID, ttl)
		if err != nil {
			t.Fatalf("SetNegativeCache failed: %v", err)
		}

		entry, err := cache.GetNegativeCache(key, stateID)
		if err != nil {
			t.Fatalf("GetNegativeCache failed: %v", err)
		}
		if entry == nil {
			t.Fatal("expected to find cached entry")
		}
		if entry.ErrorType != errorType {
			t.Errorf("ErrorType = %q, want %q", entry.ErrorType, errorType)
		}
		if entry.ErrorMessage != errorMessage {
			t.Errorf("ErrorMessage = %q, want %q", entry.ErrorMessage, errorMessage)
		}
	})
}

func TestCacheInvalidation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-cache-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	db, err := Open(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	cache := NewCache(db)

	t.Run("invalidate query cache by pattern", func(t *testing.T) {
		// Set some entries
		err := cache.SetQueryCache("search:foo", `{"result": 1}`, "commit1", "state1", 300)
		if err != nil {
			t.Fatalf("SetQueryCache failed: %v", err)
		}
		err = cache.SetQueryCache("search:bar", `{"result": 2}`, "commit1", "state1", 300)
		if err != nil {
			t.Fatalf("SetQueryCache failed: %v", err)
		}

		// Invalidate by pattern
		err = cache.InvalidateQueryCache("search:%")
		if err != nil {
			t.Fatalf("InvalidateQueryCache failed: %v", err)
		}

		// Check they're gone
		_, found, _ := cache.GetQueryCache("search:foo", "commit1")
		if found {
			t.Error("expected search:foo to be invalidated")
		}
		_, found, _ = cache.GetQueryCache("search:bar", "commit1")
		if found {
			t.Error("expected search:bar to be invalidated")
		}
	})

	t.Run("invalidate all query cache", func(t *testing.T) {
		err := cache.SetQueryCache("key1", `{}`, "commit1", "state1", 300)
		if err != nil {
			t.Fatalf("SetQueryCache failed: %v", err)
		}

		err = cache.InvalidateAllQueryCache()
		if err != nil {
			t.Fatalf("InvalidateAllQueryCache failed: %v", err)
		}

		_, found, _ := cache.GetQueryCache("key1", "commit1")
		if found {
			t.Error("expected all query cache to be invalidated")
		}
	})

	t.Run("invalidate by state ID", func(t *testing.T) {
		stateID := "state-to-invalidate"

		err := cache.SetQueryCache("key-a", `{}`, "commit1", stateID, 300)
		if err != nil {
			t.Fatalf("SetQueryCache failed: %v", err)
		}
		err = cache.SetViewCache("key-b", `{}`, stateID, 3600)
		if err != nil {
			t.Fatalf("SetViewCache failed: %v", err)
		}

		err = cache.InvalidateByStateID(stateID)
		if err != nil {
			t.Fatalf("InvalidateByStateID failed: %v", err)
		}

		_, found, _ := cache.GetQueryCache("key-a", "commit1")
		if found {
			t.Error("expected query cache to be invalidated by state ID")
		}
		_, found, _ = cache.GetViewCache("key-b", stateID)
		if found {
			t.Error("expected view cache to be invalidated by state ID")
		}
	})
}

func TestCacheCleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-cache-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	db, err := Open(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	cache := NewCache(db)

	// Set an entry with very short TTL
	err = cache.SetQueryCache("expired-key", `{}`, "commit1", "state1", 1)
	if err != nil {
		t.Fatalf("SetQueryCache failed: %v", err)
	}

	// Wait for expiration
	time.Sleep(2 * time.Second)

	// Cleanup should remove expired entries
	err = cache.CleanupExpiredEntries()
	if err != nil {
		t.Fatalf("CleanupExpiredEntries failed: %v", err)
	}

	// The entry should be gone or marked as expired
	_, found, _ := cache.GetQueryCache("expired-key", "commit1")
	if found {
		t.Error("expected expired entry to be cleaned up")
	}
}

func TestCacheStats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ckb-cache-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})
	db, err := Open(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer func() { _ = db.Close() }()

	cache := NewCache(db)

	// Add some entries
	err = cache.SetQueryCache("key1", `{}`, "commit1", "state1", 300)
	if err != nil {
		t.Fatalf("SetQueryCache failed: %v", err)
	}
	err = cache.SetViewCache("key2", `{}`, "state1", 3600)
	if err != nil {
		t.Fatalf("SetViewCache failed: %v", err)
	}

	stats, err := cache.GetCacheStats()
	if err != nil {
		t.Fatalf("GetCacheStats failed: %v", err)
	}

	if stats == nil {
		t.Fatal("expected stats map, got nil")
	}

	// Check that we get nested cache stats
	if _, ok := stats["query_cache"]; !ok {
		t.Error("expected query_cache in stats")
	}
	if _, ok := stats["view_cache"]; !ok {
		t.Error("expected view_cache in stats")
	}
	if _, ok := stats["negative_cache"]; !ok {
		t.Error("expected negative_cache in stats")
	}

	// Check nested structure
	if qc, ok := stats["query_cache"].(map[string]interface{}); ok {
		if _, ok := qc["entries"]; !ok {
			t.Error("expected entries in query_cache stats")
		}
	}
}

func TestCacheTierConstants(t *testing.T) {
	if QueryCache != "query" {
		t.Errorf("QueryCache = %q, want %q", QueryCache, "query")
	}
	if ViewCache != "view" {
		t.Errorf("ViewCache = %q, want %q", ViewCache, "view")
	}
	if NegativeCache != "negative" {
		t.Errorf("NegativeCache = %q, want %q", NegativeCache, "negative")
	}
}
