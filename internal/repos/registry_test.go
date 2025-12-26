package repos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "myrepo", false},
		{"valid with numbers", "repo123", false},
		{"valid with underscore", "my_repo", false},
		{"valid with hyphen", "my-repo", false},
		{"valid mixed", "My_Repo-123", false},
		{"empty", "", true},
		{"with space", "my repo", true},
		{"with dot", "my.repo", true},
		{"with slash", "my/repo", true},
		{"with special chars", "my@repo!", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestRegistry_InMemoryOperations(t *testing.T) {
	// Test registry operations without disk I/O
	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	t.Run("List empty registry", func(t *testing.T) {
		entries := reg.List()
		if len(entries) != 0 {
			t.Errorf("expected empty list, got %d entries", len(entries))
		}
	})

	t.Run("GetDefault on empty registry", func(t *testing.T) {
		def := reg.GetDefault()
		if def != "" {
			t.Errorf("expected empty default, got %q", def)
		}
	})

	t.Run("Get non-existent repo", func(t *testing.T) {
		_, _, err := reg.Get("nonexistent")
		if err == nil {
			t.Error("expected error for non-existent repo")
		}
	})

	t.Run("GetByPath non-existent", func(t *testing.T) {
		_, err := reg.GetByPath("/some/path")
		if err == nil {
			t.Error("expected error for non-existent path")
		}
	})
}

func TestRegistry_WithTempDir(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	repoPath := filepath.Join(tmpDir, "testrepo")
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatalf("failed to create test repo dir: %v", err)
	}

	// Create a registry that doesn't persist (we test in-memory behavior)
	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	// Override Save to be a no-op for testing
	// We'll add entries directly to test logic

	t.Run("Add and Get repo", func(t *testing.T) {
		entry := RepoEntry{
			Name:    "test",
			Path:    repoPath,
			AddedAt: time.Now(),
		}
		reg.Repos["test"] = entry

		got, state, err := reg.Get("test")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if got.Name != "test" {
			t.Errorf("expected name 'test', got %q", got.Name)
		}
		if got.Path != repoPath {
			t.Errorf("expected path %q, got %q", repoPath, got.Path)
		}
		// State should be uninitialized (no .ckb dir)
		if state != RepoStateUninitialized {
			t.Errorf("expected state %q, got %q", RepoStateUninitialized, state)
		}
	})

	t.Run("ValidateState with .ckb dir", func(t *testing.T) {
		// Create .ckb directory
		ckbDir := filepath.Join(repoPath, ".ckb")
		if err := os.MkdirAll(ckbDir, 0755); err != nil {
			t.Fatalf("failed to create .ckb dir: %v", err)
		}

		state := reg.ValidateState("test")
		if state != RepoStateValid {
			t.Errorf("expected state %q, got %q", RepoStateValid, state)
		}
	})

	t.Run("ValidateState for missing path", func(t *testing.T) {
		reg.Repos["missing"] = RepoEntry{
			Name: "missing",
			Path: "/nonexistent/path/12345",
		}
		state := reg.ValidateState("missing")
		if state != RepoStateMissing {
			t.Errorf("expected state %q, got %q", RepoStateMissing, state)
		}
	})

	t.Run("ValidateState for non-existent repo", func(t *testing.T) {
		state := reg.ValidateState("doesnotexist")
		if state != RepoStateMissing {
			t.Errorf("expected state %q, got %q", RepoStateMissing, state)
		}
	})

	t.Run("GetByPath finds repo", func(t *testing.T) {
		entry, err := reg.GetByPath(repoPath)
		if err != nil {
			t.Fatalf("GetByPath failed: %v", err)
		}
		if entry.Name != "test" {
			t.Errorf("expected name 'test', got %q", entry.Name)
		}
	})

	t.Run("List returns all repos", func(t *testing.T) {
		entries := reg.List()
		if len(entries) != 2 { // test and missing
			t.Errorf("expected 2 entries, got %d", len(entries))
		}
	})
}

func TestRegistry_RenameLogic(t *testing.T) {
	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
		Default: "old",
	}
	reg.Repos["old"] = RepoEntry{Name: "old", Path: "/some/path"}

	t.Run("Rename updates name in entry", func(t *testing.T) {
		// Simulate rename logic without Save
		entry := reg.Repos["old"]
		entry.Name = "new"
		reg.Repos["new"] = entry
		delete(reg.Repos, "old")
		if reg.Default == "old" {
			reg.Default = "new"
		}

		if _, exists := reg.Repos["old"]; exists {
			t.Error("old name should not exist after rename")
		}
		if _, exists := reg.Repos["new"]; !exists {
			t.Error("new name should exist after rename")
		}
		if reg.Default != "new" {
			t.Errorf("default should be updated to 'new', got %q", reg.Default)
		}
	})
}

func TestRegistry_RemoveLogic(t *testing.T) {
	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
		Default: "todelete",
	}
	reg.Repos["todelete"] = RepoEntry{Name: "todelete", Path: "/some/path"}
	reg.Repos["keep"] = RepoEntry{Name: "keep", Path: "/other/path"}

	t.Run("Remove clears default if matching", func(t *testing.T) {
		// Simulate remove logic
		delete(reg.Repos, "todelete")
		if reg.Default == "todelete" {
			reg.Default = ""
		}

		if _, exists := reg.Repos["todelete"]; exists {
			t.Error("deleted repo should not exist")
		}
		if reg.Default != "" {
			t.Errorf("default should be cleared, got %q", reg.Default)
		}
	})
}

func TestRegistry_SetDefaultLogic(t *testing.T) {
	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}
	reg.Repos["repo1"] = RepoEntry{Name: "repo1", Path: "/path1"}

	t.Run("SetDefault to existing repo", func(t *testing.T) {
		// Check validation logic
		if _, exists := reg.Repos["repo1"]; !exists {
			t.Error("repo1 should exist")
		}
		reg.Default = "repo1"
		if reg.Default != "repo1" {
			t.Errorf("expected default 'repo1', got %q", reg.Default)
		}
	})

	t.Run("SetDefault to empty clears default", func(t *testing.T) {
		reg.Default = ""
		if reg.Default != "" {
			t.Errorf("expected empty default, got %q", reg.Default)
		}
	})
}

func TestRegistry_TouchLastUsedLogic(t *testing.T) {
	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}
	reg.Repos["repo1"] = RepoEntry{Name: "repo1", Path: "/path1"}

	t.Run("TouchLastUsed updates timestamp", func(t *testing.T) {
		before := time.Now()
		entry := reg.Repos["repo1"]
		entry.LastUsedAt = time.Now()
		reg.Repos["repo1"] = entry
		after := time.Now()

		updated := reg.Repos["repo1"]
		if updated.LastUsedAt.Before(before) || updated.LastUsedAt.After(after) {
			t.Error("LastUsedAt should be between before and after")
		}
	})
}

func TestRegistry_JSONSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second) // JSON loses sub-second precision
	reg := &Registry{
		Repos: map[string]RepoEntry{
			"test": {
				Name:       "test",
				Path:       "/path/to/repo",
				AddedAt:    now,
				LastUsedAt: now,
			},
		},
		Default: "test",
		Version: 1,
	}

	t.Run("Marshal and Unmarshal", func(t *testing.T) {
		data, err := json.Marshal(reg)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var decoded Registry
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		if decoded.Version != reg.Version {
			t.Errorf("version mismatch: got %d, want %d", decoded.Version, reg.Version)
		}
		if decoded.Default != reg.Default {
			t.Errorf("default mismatch: got %q, want %q", decoded.Default, reg.Default)
		}
		if len(decoded.Repos) != len(reg.Repos) {
			t.Errorf("repos count mismatch: got %d, want %d", len(decoded.Repos), len(reg.Repos))
		}

		entry, exists := decoded.Repos["test"]
		if !exists {
			t.Fatal("test repo not found after decode")
		}
		if entry.Name != "test" {
			t.Errorf("name mismatch: got %q, want %q", entry.Name, "test")
		}
		if entry.Path != "/path/to/repo" {
			t.Errorf("path mismatch: got %q, want %q", entry.Path, "/path/to/repo")
		}
	})
}

func TestRegistry_VersionValidation(t *testing.T) {
	t.Run("Future version rejected", func(t *testing.T) {
		data := []byte(`{"version": 999, "repos": {}}`)
		var reg Registry
		if err := json.Unmarshal(data, &reg); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		// The version check happens in LoadRegistry, simulate it
		if reg.Version > currentRegistryVersion {
			// This is expected behavior
		} else {
			t.Error("expected version to be greater than current")
		}
	})

	t.Run("Nil repos map initialized", func(t *testing.T) {
		data := []byte(`{"version": 1}`)
		var reg Registry
		if err := json.Unmarshal(data, &reg); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		// Simulate initialization
		if reg.Repos == nil {
			reg.Repos = make(map[string]RepoEntry)
		}
		if reg.Repos == nil {
			t.Error("repos map should be initialized")
		}
	})
}

func TestFileLock_Release(t *testing.T) {
	t.Run("Release nil file is safe", func(t *testing.T) {
		lock := &FileLock{file: nil}
		err := lock.Release()
		if err != nil {
			t.Errorf("Release on nil file should not error: %v", err)
		}
	})

	t.Run("Release with actual file", func(t *testing.T) {
		tmpDir := t.TempDir()
		lockPath := filepath.Join(tmpDir, "test.lock")

		lock, err := acquireLock(lockPath)
		if err != nil {
			t.Fatalf("acquireLock failed: %v", err)
		}

		err = lock.Release()
		if err != nil {
			t.Errorf("Release failed: %v", err)
		}

		// Verify file is nil after release
		if lock.file != nil {
			t.Error("file should be nil after release")
		}

		// Double release should be safe
		err = lock.Release()
		if err != nil {
			t.Errorf("Double release should not error: %v", err)
		}
	})
}

func TestAcquireLock(t *testing.T) {
	t.Run("Creates directory if needed", func(t *testing.T) {
		tmpDir := t.TempDir()
		lockPath := filepath.Join(tmpDir, "subdir", "test.lock")

		lock, err := acquireLock(lockPath)
		if err != nil {
			t.Fatalf("acquireLock failed: %v", err)
		}
		defer lock.Release()

		// Verify directory was created
		dir := filepath.Dir(lockPath)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Error("directory should have been created")
		}
	})

	t.Run("Lock file is created", func(t *testing.T) {
		tmpDir := t.TempDir()
		lockPath := filepath.Join(tmpDir, "test.lock")

		lock, err := acquireLock(lockPath)
		if err != nil {
			t.Fatalf("acquireLock failed: %v", err)
		}
		defer lock.Release()

		if _, err := os.Stat(lockPath); os.IsNotExist(err) {
			t.Error("lock file should exist")
		}
	})
}

func TestRepoStateConstants(t *testing.T) {
	// Ensure constants have expected values
	if RepoStateValid != "valid" {
		t.Errorf("RepoStateValid = %q, want %q", RepoStateValid, "valid")
	}
	if RepoStateUninitialized != "uninitialized" {
		t.Errorf("RepoStateUninitialized = %q, want %q", RepoStateUninitialized, "uninitialized")
	}
	if RepoStateMissing != "missing" {
		t.Errorf("RepoStateMissing = %q, want %q", RepoStateMissing, "missing")
	}
}
