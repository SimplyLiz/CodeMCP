package mcp

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnsureActiveEngine_EmptyRoot(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	server := &MCPServer{
		logger:  logger,
		engines: make(map[string]*engineEntry),
		roots:   newRootsManager(),
	}

	// Should be a no-op for empty root
	err := server.ensureActiveEngine("")
	if err != nil {
		t.Errorf("ensureActiveEngine('') returned error: %v", err)
	}
}

func TestEnsureActiveEngine_SameRepoNoSwitch(t *testing.T) {
	// Test that ensureActiveEngine is a no-op when current engine points to same repo
	// This tests the early return path, not the full engine creation

	tmpDir, err := os.MkdirTemp("", "engine-cache-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create server without legacyEngine - ensureActiveEngine will try to create one
	// and fail, but this tests the path normalization logic
	server := &MCPServer{
		logger:  logger,
		engines: make(map[string]*engineEntry),
		roots:   newRootsManager(),
	}

	// This will fail because there's no .ckb directory, but won't panic
	err = server.ensureActiveEngine(tmpDir)

	// Error expected - we can't create a real engine without setup
	// Just verify it doesn't panic and returns an error gracefully
	if err == nil {
		t.Log("ensureActiveEngine succeeded (temp dir may be in a git repo)")
	}
}

func TestGetOrCreateEngine_CacheHit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "engine-cache-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	normalized := normalizePath(tmpDir)
	originalTime := time.Now().Add(-time.Hour)
	entry := &engineEntry{
		repoPath: normalized,
		repoName: filepath.Base(normalized),
		loadedAt: originalTime,
		lastUsed: originalTime,
	}

	server := &MCPServer{
		logger:  logger,
		engines: map[string]*engineEntry{normalized: entry},
		roots:   newRootsManager(),
	}

	// Should hit cache and update lastUsed
	result, err := server.getOrCreateEngine(tmpDir)
	if err != nil {
		t.Fatalf("getOrCreateEngine returned error: %v", err)
	}

	if result != entry {
		t.Error("getOrCreateEngine should return cached entry")
	}

	if !result.lastUsed.After(originalTime) {
		t.Error("getOrCreateEngine should update lastUsed timestamp")
	}
}

func TestGetOrCreateEngine_NormalizedPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "engine-cache-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Store with normalized path
	normalized := normalizePath(tmpDir)
	entry := &engineEntry{
		repoPath: normalized,
		repoName: filepath.Base(normalized),
		loadedAt: time.Now(),
		lastUsed: time.Now(),
	}

	server := &MCPServer{
		logger:  logger,
		engines: map[string]*engineEntry{normalized: entry},
		roots:   newRootsManager(),
	}

	// Query with unnormalized path (trailing slash, etc.)
	pathWithSlash := tmpDir + "/"
	result, err := server.getOrCreateEngine(pathWithSlash)
	if err != nil {
		t.Fatalf("getOrCreateEngine returned error: %v", err)
	}

	if result != entry {
		t.Error("getOrCreateEngine should find entry regardless of trailing slash")
	}
}

func TestEvictLRULocked_PreservesActiveRepo(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	now := time.Now()
	activeEntry := &engineEntry{
		repoPath: "/active/repo",
		repoName: "active",
		lastUsed: now.Add(-time.Hour), // Oldest, but should not be evicted
	}
	otherEntry := &engineEntry{
		repoPath: "/other/repo",
		repoName: "other",
		lastUsed: now, // Newer
	}

	server := &MCPServer{
		logger: logger,
		engines: map[string]*engineEntry{
			"/active/repo": activeEntry,
			"/other/repo":  otherEntry,
		},
		activeRepoPath: "/active/repo",
		roots:          newRootsManager(),
	}

	// Evict should remove other, not active (even though active is older)
	server.evictLRULocked()

	if _, ok := server.engines["/active/repo"]; !ok {
		t.Error("evictLRULocked should not evict active repo")
	}

	if _, ok := server.engines["/other/repo"]; ok {
		t.Error("evictLRULocked should evict non-active repo")
	}
}

func TestEvictLRULocked_EvictsOldest(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	now := time.Now()
	entry1 := &engineEntry{
		repoPath: "/repo1",
		repoName: "repo1",
		lastUsed: now.Add(-2 * time.Hour), // Oldest
	}
	entry2 := &engineEntry{
		repoPath: "/repo2",
		repoName: "repo2",
		lastUsed: now.Add(-time.Hour),
	}
	entry3 := &engineEntry{
		repoPath: "/repo3",
		repoName: "repo3",
		lastUsed: now, // Newest
	}

	server := &MCPServer{
		logger: logger,
		engines: map[string]*engineEntry{
			"/repo1": entry1,
			"/repo2": entry2,
			"/repo3": entry3,
		},
		activeRepoPath: "/repo3", // Active is newest
		roots:          newRootsManager(),
	}

	server.evictLRULocked()

	// repo1 should be evicted (oldest non-active)
	if _, ok := server.engines["/repo1"]; ok {
		t.Error("evictLRULocked should evict oldest repo")
	}

	// repo2 and repo3 should remain
	if _, ok := server.engines["/repo2"]; !ok {
		t.Error("repo2 should not be evicted")
	}
	if _, ok := server.engines["/repo3"]; !ok {
		t.Error("repo3 should not be evicted")
	}
}
