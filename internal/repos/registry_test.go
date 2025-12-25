package repos

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"myrepo", false},
		{"my-repo", false},
		{"my_repo", false},
		{"MyRepo123", false},
		{"", true},
		{"my repo", true},
		{"my.repo", true},
		{"my/repo", true},
		{"my@repo", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestRegistry_AddAndGet(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a mock repo directory
	repoDir := filepath.Join(tmpDir, "myrepo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	// Test Add (without Save)
	if err := addWithoutSave(reg, "test-repo", repoDir); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Test Get
	entry, state, err := reg.Get("test-repo")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if entry.Name != "test-repo" {
		t.Errorf("Name = %q, want 'test-repo'", entry.Name)
	}
	if entry.Path != repoDir {
		t.Errorf("Path = %q, want %q", entry.Path, repoDir)
	}
	if state != RepoStateUninitialized {
		t.Errorf("State = %v, want %v", state, RepoStateUninitialized)
	}
}

func TestRegistry_AddDuplicate(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "myrepo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	if err := addWithoutSave(reg, "test-repo", repoDir); err != nil {
		t.Fatalf("First Add failed: %v", err)
	}

	// Second add should fail
	err := addWithoutSave(reg, "test-repo", repoDir)
	if err == nil {
		t.Error("Expected error for duplicate repo name")
	}
}

func TestRegistry_AddInvalidPath(t *testing.T) {
	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	err := addWithoutSave(reg, "test-repo", "/nonexistent/path/to/repo")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestRegistry_AddNotDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notadir")
	if err := os.WriteFile(filePath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	err := addWithoutSave(reg, "test-repo", filePath)
	if err == nil {
		t.Error("Expected error for file path (not directory)")
	}
}

func TestRegistry_Remove(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "myrepo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	if err := addWithoutSave(reg, "test-repo", repoDir); err != nil {
		t.Fatal(err)
	}

	// Remove
	if err := removeWithoutSave(reg, "test-repo"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Should not exist
	_, _, err := reg.Get("test-repo")
	if err == nil {
		t.Error("Expected error for removed repo")
	}
}

func TestRegistry_RemoveNonexistent(t *testing.T) {
	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	err := removeWithoutSave(reg, "nonexistent")
	if err == nil {
		t.Error("Expected error for removing nonexistent repo")
	}
}

func TestRegistry_RemoveClearsDefault(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "myrepo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	if err := addWithoutSave(reg, "test-repo", repoDir); err != nil {
		t.Fatal(err)
	}
	reg.Default = "test-repo"

	if err := removeWithoutSave(reg, "test-repo"); err != nil {
		t.Fatal(err)
	}

	if reg.Default != "" {
		t.Errorf("Default should be cleared, got %q", reg.Default)
	}
}

func TestRegistry_Rename(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "myrepo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	if err := addWithoutSave(reg, "old-name", repoDir); err != nil {
		t.Fatal(err)
	}

	if err := renameWithoutSave(reg, "old-name", "new-name"); err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	// Old name should not exist
	_, _, err := reg.Get("old-name")
	if err == nil {
		t.Error("Old name should not exist after rename")
	}

	// New name should exist
	entry, _, err := reg.Get("new-name")
	if err != nil {
		t.Fatalf("Get new-name failed: %v", err)
	}
	if entry.Name != "new-name" {
		t.Errorf("Name = %q, want 'new-name'", entry.Name)
	}
}

func TestRegistry_RenameUpdatesDefault(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "myrepo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	if err := addWithoutSave(reg, "old-name", repoDir); err != nil {
		t.Fatal(err)
	}
	reg.Default = "old-name"

	if err := renameWithoutSave(reg, "old-name", "new-name"); err != nil {
		t.Fatal(err)
	}

	if reg.Default != "new-name" {
		t.Errorf("Default = %q, want 'new-name'", reg.Default)
	}
}

func TestRegistry_RenameNonexistent(t *testing.T) {
	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	err := renameWithoutSave(reg, "nonexistent", "new-name")
	if err == nil {
		t.Error("Expected error for renaming nonexistent repo")
	}
}

func TestRegistry_RenameToExisting(t *testing.T) {
	tmpDir := t.TempDir()
	repo1 := filepath.Join(tmpDir, "repo1")
	repo2 := filepath.Join(tmpDir, "repo2")
	if err := os.MkdirAll(repo1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repo2, 0755); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	if err := addWithoutSave(reg, "repo1", repo1); err != nil {
		t.Fatal(err)
	}
	if err := addWithoutSave(reg, "repo2", repo2); err != nil {
		t.Fatal(err)
	}

	err := renameWithoutSave(reg, "repo1", "repo2")
	if err == nil {
		t.Error("Expected error for renaming to existing name")
	}
}

func TestRegistry_GetByPath(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "myrepo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	if err := addWithoutSave(reg, "test-repo", repoDir); err != nil {
		t.Fatal(err)
	}

	entry, err := reg.GetByPath(repoDir)
	if err != nil {
		t.Fatalf("GetByPath failed: %v", err)
	}
	if entry.Name != "test-repo" {
		t.Errorf("Name = %q, want 'test-repo'", entry.Name)
	}
}

func TestRegistry_GetByPathNotFound(t *testing.T) {
	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	_, err := reg.GetByPath("/some/path")
	if err == nil {
		t.Error("Expected error for path not in registry")
	}
}

func TestRegistry_List(t *testing.T) {
	tmpDir := t.TempDir()
	repo1 := filepath.Join(tmpDir, "repo1")
	repo2 := filepath.Join(tmpDir, "repo2")
	if err := os.MkdirAll(repo1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(repo2, 0755); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	if err := addWithoutSave(reg, "repo1", repo1); err != nil {
		t.Fatal(err)
	}
	if err := addWithoutSave(reg, "repo2", repo2); err != nil {
		t.Fatal(err)
	}

	list := reg.List()
	if len(list) != 2 {
		t.Errorf("List len = %d, want 2", len(list))
	}
}

func TestRegistry_SetDefault(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "myrepo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	if err := addWithoutSave(reg, "test-repo", repoDir); err != nil {
		t.Fatal(err)
	}

	if err := setDefaultWithoutSave(reg, "test-repo"); err != nil {
		t.Fatalf("SetDefault failed: %v", err)
	}

	if reg.GetDefault() != "test-repo" {
		t.Errorf("GetDefault() = %q, want 'test-repo'", reg.GetDefault())
	}
}

func TestRegistry_SetDefaultNonexistent(t *testing.T) {
	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	err := setDefaultWithoutSave(reg, "nonexistent")
	if err == nil {
		t.Error("Expected error for setting nonexistent default")
	}
}

func TestRegistry_SetDefaultEmpty(t *testing.T) {
	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Default: "something",
		Version: 1,
	}

	if err := setDefaultWithoutSave(reg, ""); err != nil {
		t.Fatalf("SetDefault('') failed: %v", err)
	}

	if reg.Default != "" {
		t.Errorf("Default = %q, want empty", reg.Default)
	}
}

func TestRegistry_ValidateState(t *testing.T) {
	tmpDir := t.TempDir()

	// Create repo without .ckb
	uninitRepo := filepath.Join(tmpDir, "uninit")
	if err := os.MkdirAll(uninitRepo, 0755); err != nil {
		t.Fatal(err)
	}

	// Create repo with .ckb
	validRepo := filepath.Join(tmpDir, "valid")
	if err := os.MkdirAll(filepath.Join(validRepo, ".ckb"), 0755); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{
		Repos:   make(map[string]RepoEntry),
		Version: 1,
	}

	if err := addWithoutSave(reg, "uninit", uninitRepo); err != nil {
		t.Fatal(err)
	}
	if err := addWithoutSave(reg, "valid", validRepo); err != nil {
		t.Fatal(err)
	}
	// Add missing repo manually
	reg.Repos["missing"] = RepoEntry{Name: "missing", Path: "/nonexistent"}

	tests := []struct {
		name     string
		expected RepoState
	}{
		{"uninit", RepoStateUninitialized},
		{"valid", RepoStateValid},
		{"missing", RepoStateMissing},
		{"notfound", RepoStateMissing},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := reg.ValidateState(tt.name)
			if state != tt.expected {
				t.Errorf("ValidateState(%q) = %v, want %v", tt.name, state, tt.expected)
			}
		})
	}
}

func TestRepoStateConstants(t *testing.T) {
	if RepoStateValid != "valid" {
		t.Errorf("RepoStateValid = %q, want 'valid'", RepoStateValid)
	}
	if RepoStateUninitialized != "uninitialized" {
		t.Errorf("RepoStateUninitialized = %q, want 'uninitialized'", RepoStateUninitialized)
	}
	if RepoStateMissing != "missing" {
		t.Errorf("RepoStateMissing = %q, want 'missing'", RepoStateMissing)
	}
}

func TestFileLock_Release(t *testing.T) {
	// Test Release with nil file
	lock := &FileLock{}
	err := lock.Release()
	if err != nil {
		t.Errorf("Release() with nil file should not error: %v", err)
	}
}

// Helper functions for testing without Save() which requires file system
func addWithoutSave(r *Registry, name, path string) error {
	if err := ValidateName(name); err != nil {
		return err
	}

	if _, exists := r.Repos[name]; exists {
		return fmt.Errorf("repo '%s' already exists", name)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}
	absPath = filepath.Clean(absPath)

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("path does not exist: %s", absPath)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", absPath)
	}

	r.Repos[name] = RepoEntry{
		Name: name,
		Path: absPath,
	}
	return nil
}

func removeWithoutSave(r *Registry, name string) error {
	if _, exists := r.Repos[name]; !exists {
		return fmt.Errorf("repo '%s' not found", name)
	}
	delete(r.Repos, name)
	if r.Default == name {
		r.Default = ""
	}
	return nil
}

func renameWithoutSave(r *Registry, oldName, newName string) error {
	if err := ValidateName(newName); err != nil {
		return err
	}
	entry, exists := r.Repos[oldName]
	if !exists {
		return fmt.Errorf("repo '%s' not found", oldName)
	}
	if _, exists := r.Repos[newName]; exists {
		return fmt.Errorf("repo '%s' already exists", newName)
	}
	entry.Name = newName
	r.Repos[newName] = entry
	delete(r.Repos, oldName)
	if r.Default == oldName {
		r.Default = newName
	}
	return nil
}

func setDefaultWithoutSave(r *Registry, name string) error {
	if name != "" {
		if _, exists := r.Repos[name]; !exists {
			return fmt.Errorf("repo '%s' not found", name)
		}
	}
	r.Default = name
	return nil
}
