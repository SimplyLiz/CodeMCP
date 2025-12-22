package api

import (
	"os"
	"path/filepath"
	"testing"

	"ckb/internal/logging"
)

func TestIsValidRepoID(t *testing.T) {
	tests := []struct {
		id    string
		valid bool
	}{
		{"company/core-lib", true},
		{"simple", true},
		{"my-org/my-repo", true},
		{"org_name/repo.name", true},
		{"123/456", true},
		{"a/b/c", true}, // multi-level org
		{"", false},
		{"/invalid", false},
		{"invalid/", false},
		{"has//double", false},
		{"has spaces", false},
		{"has@special", false},
		{"has#special", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := isValidRepoID(tt.id); got != tt.valid {
				t.Errorf("isValidRepoID(%q) = %v, want %v", tt.id, got, tt.valid)
			}
		})
	}
}

func TestExtractRepoIDFromPath(t *testing.T) {
	tests := []struct {
		path   string
		prefix string
		suffix string
		want   string
	}{
		{"/index/repos/company/core-lib/upload", "/index/repos/", "/upload", "company/core-lib"},
		{"/index/repos/simple/meta", "/index/repos/", "/meta", "simple"},
		{"/index/repos/org/repo", "/index/repos/", "", "org/repo"},
		{"/other/path", "/index/repos/", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := extractRepoIDFromPath(tt.path, tt.prefix, tt.suffix); got != tt.want {
				t.Errorf("extractRepoIDFromPath(%q, %q, %q) = %q, want %q",
					tt.path, tt.prefix, tt.suffix, got, tt.want)
			}
		})
	}
}

func TestParseLanguagesHeader(t *testing.T) {
	tests := []struct {
		header string
		want   []string
	}{
		{"", nil},
		{"go", []string{"go"}},
		{"go, typescript", []string{"go", "typescript"}},
		{"  go  ,  python  ", []string{"go", "python"}},
		{"go,python,rust", []string{"go", "python", "rust"}},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			got := parseLanguagesHeader(tt.header)
			if len(got) != len(tt.want) {
				t.Errorf("parseLanguagesHeader(%q) = %v, want %v", tt.header, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseLanguagesHeader(%q)[%d] = %q, want %q", tt.header, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIndexStorage(t *testing.T) {
	// Create temp directory for tests
	tmpDir, err := os.MkdirTemp("", "ckb-storage-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create logger
	logger := logging.NewLogger(logging.Config{Level: logging.ErrorLevel})

	// Create storage
	storage, err := NewIndexStorage(tmpDir, logger)
	if err != nil {
		t.Fatalf("NewIndexStorage failed: %v", err)
	}

	t.Run("CreateRepo", func(t *testing.T) {
		err := storage.CreateRepo("test/repo", "Test Repo", "A test repository")
		if err != nil {
			t.Errorf("CreateRepo failed: %v", err)
		}

		// Verify directory exists
		repoPath := storage.RepoPath("test/repo")
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			t.Error("Repo directory not created")
		}

		// Verify meta.json exists
		metaPath := storage.MetaPath("test/repo")
		if _, err := os.Stat(metaPath); os.IsNotExist(err) {
			t.Error("meta.json not created")
		}
	})

	t.Run("LoadMeta", func(t *testing.T) {
		meta, err := storage.LoadMeta("test/repo")
		if err != nil {
			t.Errorf("LoadMeta failed: %v", err)
		}

		if meta.ID != "test/repo" {
			t.Errorf("meta.ID = %q, want %q", meta.ID, "test/repo")
		}
		if meta.Name != "Test Repo" {
			t.Errorf("meta.Name = %q, want %q", meta.Name, "Test Repo")
		}
		if meta.Source != "uploaded" {
			t.Errorf("meta.Source = %q, want %q", meta.Source, "uploaded")
		}
	})

	t.Run("RepoExists", func(t *testing.T) {
		if !storage.RepoExists("test/repo") {
			t.Error("RepoExists returned false for existing repo")
		}
		if storage.RepoExists("nonexistent/repo") {
			t.Error("RepoExists returned true for nonexistent repo")
		}
	})

	t.Run("ListRepos", func(t *testing.T) {
		repos, err := storage.ListRepos()
		if err != nil {
			t.Errorf("ListRepos failed: %v", err)
		}
		if len(repos) != 1 {
			t.Errorf("ListRepos returned %d repos, want 1", len(repos))
		}
		if len(repos) > 0 && repos[0] != "test/repo" {
			t.Errorf("ListRepos[0] = %q, want %q", repos[0], "test/repo")
		}
	})

	t.Run("CreateUploadFile", func(t *testing.T) {
		file, path, err := storage.CreateUploadFile()
		if err != nil {
			t.Errorf("CreateUploadFile failed: %v", err)
		}
		defer file.Close()
		defer storage.CleanupUpload(path)

		if !filepath.HasPrefix(path, storage.uploadDir) {
			t.Errorf("Upload path %q not in upload dir %q", path, storage.uploadDir)
		}

		// Write some data
		_, err = file.Write([]byte("test data"))
		if err != nil {
			t.Errorf("Failed to write to upload file: %v", err)
		}
	})

	t.Run("DeleteRepo", func(t *testing.T) {
		err := storage.DeleteRepo("test/repo")
		if err != nil {
			t.Errorf("DeleteRepo failed: %v", err)
		}

		if storage.RepoExists("test/repo") {
			t.Error("Repo still exists after deletion")
		}
	})

	t.Run("DeleteNonexistent", func(t *testing.T) {
		err := storage.DeleteRepo("nonexistent/repo")
		if err == nil {
			t.Error("DeleteRepo should fail for nonexistent repo")
		}
	})
}

func TestSanitizeRepoID(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"company/core-lib", "company-core-lib"},
		{"simple", "simple"},
		{"a/b/c", "a-b-c"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := sanitizeRepoID(tt.id); got != tt.want {
				t.Errorf("sanitizeRepoID(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestConfigValidationWithUpload(t *testing.T) {
	t.Run("enabled_no_repos_with_create_allowed", func(t *testing.T) {
		config := &IndexServerConfig{
			Enabled:         true,
			Repos:           []IndexRepoConfig{},
			MaxPageSize:     1000,
			AllowCreateRepo: true, // This should make it valid
		}

		err := config.Validate()
		if err != nil {
			t.Errorf("Validate() should pass when AllowCreateRepo is true: %v", err)
		}
	})

	t.Run("enabled_no_repos_without_create", func(t *testing.T) {
		config := &IndexServerConfig{
			Enabled:         true,
			Repos:           []IndexRepoConfig{},
			MaxPageSize:     1000,
			AllowCreateRepo: false,
		}

		err := config.Validate()
		if err == nil {
			t.Error("Validate() should fail when no repos and AllowCreateRepo is false")
		}
	})

	t.Run("negative_upload_size", func(t *testing.T) {
		config := &IndexServerConfig{
			Enabled:         true,
			Repos:           []IndexRepoConfig{},
			MaxPageSize:     1000,
			AllowCreateRepo: true,
			MaxUploadSize:   -1,
		}

		err := config.Validate()
		if err == nil {
			t.Error("Validate() should fail with negative MaxUploadSize")
		}
	})
}
