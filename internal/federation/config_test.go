package federation

import (
	"testing"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig("test-federation", "A test federation")

	if cfg == nil {
		t.Fatal("NewConfig returned nil")
	}
	if cfg.Name != "test-federation" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-federation")
	}
	if cfg.Description != "A test federation" {
		t.Errorf("Description = %q, want %q", cfg.Description, "A test federation")
	}
	if len(cfg.Repos) != 0 {
		t.Errorf("Repos should be empty, got %d", len(cfg.Repos))
	}
	if cfg.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
	if cfg.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should not be zero")
	}
}

func TestConfig_AddRepo(t *testing.T) {
	cfg := NewConfig("test", "")

	repo, err := cfg.AddRepo("repo1", "/path/to/repo1", []string{"backend", "go"})
	if err != nil {
		t.Fatalf("AddRepo failed: %v", err)
	}

	if repo == nil {
		t.Fatal("AddRepo returned nil repo")
	}
	if repo.RepoID != "repo1" {
		t.Errorf("RepoID = %q, want %q", repo.RepoID, "repo1")
	}
	if repo.Path != "/path/to/repo1" {
		t.Errorf("Path = %q, want %q", repo.Path, "/path/to/repo1")
	}
	if len(repo.Tags) != 2 {
		t.Errorf("Tags length = %d, want %d", len(repo.Tags), 2)
	}
	if repo.RepoUID == "" {
		t.Error("RepoUID should not be empty")
	}
	if repo.AddedAt.IsZero() {
		t.Error("AddedAt should not be zero")
	}

	// Check that it was added to config
	if len(cfg.Repos) != 1 {
		t.Errorf("Repos length = %d, want %d", len(cfg.Repos), 1)
	}
}

func TestConfig_AddRepo_Duplicate(t *testing.T) {
	cfg := NewConfig("test", "")

	_, err := cfg.AddRepo("repo1", "/path/to/repo1", nil)
	if err != nil {
		t.Fatalf("First AddRepo failed: %v", err)
	}

	// Duplicate repoID
	_, err = cfg.AddRepo("repo1", "/path/to/repo2", nil)
	if err == nil {
		t.Error("Expected error for duplicate repoID")
	}

	// Duplicate path
	_, err = cfg.AddRepo("repo2", "/path/to/repo1", nil)
	if err == nil {
		t.Error("Expected error for duplicate path")
	}
}

func TestConfig_RemoveRepo(t *testing.T) {
	cfg := NewConfig("test", "")

	_, _ = cfg.AddRepo("repo1", "/path/to/repo1", nil)
	_, _ = cfg.AddRepo("repo2", "/path/to/repo2", nil)

	if len(cfg.Repos) != 2 {
		t.Fatalf("Expected 2 repos, got %d", len(cfg.Repos))
	}

	err := cfg.RemoveRepo("repo1")
	if err != nil {
		t.Fatalf("RemoveRepo failed: %v", err)
	}

	if len(cfg.Repos) != 1 {
		t.Errorf("Expected 1 repo after removal, got %d", len(cfg.Repos))
	}

	if cfg.Repos[0].RepoID != "repo2" {
		t.Errorf("Remaining repo should be repo2, got %q", cfg.Repos[0].RepoID)
	}
}

func TestConfig_RemoveRepo_NotFound(t *testing.T) {
	cfg := NewConfig("test", "")

	err := cfg.RemoveRepo("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent repo")
	}
}

func TestConfig_RenameRepo(t *testing.T) {
	cfg := NewConfig("test", "")

	repo, _ := cfg.AddRepo("old-name", "/path/to/repo", nil)
	originalUID := repo.RepoUID

	err := cfg.RenameRepo("old-name", "new-name")
	if err != nil {
		t.Fatalf("RenameRepo failed: %v", err)
	}

	// Check that the repo was renamed
	if cfg.Repos[0].RepoID != "new-name" {
		t.Errorf("RepoID = %q, want %q", cfg.Repos[0].RepoID, "new-name")
	}

	// RepoUID should remain unchanged
	if cfg.Repos[0].RepoUID != originalUID {
		t.Error("RepoUID should not change on rename")
	}
}

func TestConfig_RenameRepo_NewNameExists(t *testing.T) {
	cfg := NewConfig("test", "")

	_, _ = cfg.AddRepo("repo1", "/path/to/repo1", nil)
	_, _ = cfg.AddRepo("repo2", "/path/to/repo2", nil)

	err := cfg.RenameRepo("repo1", "repo2")
	if err == nil {
		t.Error("Expected error when renaming to existing name")
	}
}

func TestConfig_RenameRepo_NotFound(t *testing.T) {
	cfg := NewConfig("test", "")

	err := cfg.RenameRepo("nonexistent", "new-name")
	if err == nil {
		t.Error("Expected error for non-existent repo")
	}
}

func TestConfig_GetRepo(t *testing.T) {
	cfg := NewConfig("test", "")

	_, _ = cfg.AddRepo("repo1", "/path/to/repo1", nil)
	_, _ = cfg.AddRepo("repo2", "/path/to/repo2", nil)

	repo := cfg.GetRepo("repo1")
	if repo == nil {
		t.Fatal("GetRepo returned nil")
	}
	if repo.RepoID != "repo1" {
		t.Errorf("RepoID = %q, want %q", repo.RepoID, "repo1")
	}

	// Non-existent repo
	repo = cfg.GetRepo("nonexistent")
	if repo != nil {
		t.Error("Expected nil for non-existent repo")
	}
}

func TestConfig_GetRepoByUID(t *testing.T) {
	cfg := NewConfig("test", "")

	repo1, _ := cfg.AddRepo("repo1", "/path/to/repo1", nil)
	_, _ = cfg.AddRepo("repo2", "/path/to/repo2", nil)

	repo := cfg.GetRepoByUID(repo1.RepoUID)
	if repo == nil {
		t.Fatal("GetRepoByUID returned nil")
	}
	if repo.RepoID != "repo1" {
		t.Errorf("RepoID = %q, want %q", repo.RepoID, "repo1")
	}

	// Non-existent UID
	repo = cfg.GetRepoByUID("nonexistent-uid")
	if repo != nil {
		t.Error("Expected nil for non-existent UID")
	}
}

func TestRepoConfigStruct(t *testing.T) {
	repo := RepoConfig{
		RepoUID: "uid-123",
		RepoID:  "my-repo",
		Path:    "/path/to/repo",
		Tags:    []string{"go", "backend"},
	}

	if repo.RepoUID != "uid-123" {
		t.Errorf("RepoUID = %q, want %q", repo.RepoUID, "uid-123")
	}
	if repo.RepoID != "my-repo" {
		t.Errorf("RepoID = %q, want %q", repo.RepoID, "my-repo")
	}
	if len(repo.Tags) != 2 {
		t.Errorf("Tags length = %d, want %d", len(repo.Tags), 2)
	}
}
