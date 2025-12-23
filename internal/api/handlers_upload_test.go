package api

import (
	"os"
	"strings"
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
		defer func() { _ = file.Close() }()
		defer func() { _ = storage.CleanupUpload(path) }()

		if !strings.HasPrefix(path, storage.uploadDir) {
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

// Phase 3 tests

func TestPhase3ConfigValidation(t *testing.T) {
	t.Run("valid_delta_threshold", func(t *testing.T) {
		config := &IndexServerConfig{
			Enabled:               true,
			MaxPageSize:           1000,
			AllowCreateRepo:       true,
			DeltaThresholdPercent: 50,
		}

		err := config.Validate()
		if err != nil {
			t.Errorf("Validate() should pass with valid delta threshold: %v", err)
		}
	})

	t.Run("invalid_delta_threshold_negative", func(t *testing.T) {
		config := &IndexServerConfig{
			Enabled:               true,
			MaxPageSize:           1000,
			AllowCreateRepo:       true,
			DeltaThresholdPercent: -10,
		}

		err := config.Validate()
		if err == nil {
			t.Error("Validate() should fail with negative DeltaThresholdPercent")
		}
	})

	t.Run("invalid_delta_threshold_over_100", func(t *testing.T) {
		config := &IndexServerConfig{
			Enabled:               true,
			MaxPageSize:           1000,
			AllowCreateRepo:       true,
			DeltaThresholdPercent: 150,
		}

		err := config.Validate()
		if err == nil {
			t.Error("Validate() should fail with DeltaThresholdPercent > 100")
		}
	})

	t.Run("valid_supported_encodings", func(t *testing.T) {
		config := &IndexServerConfig{
			Enabled:            true,
			MaxPageSize:        1000,
			AllowCreateRepo:    true,
			SupportedEncodings: []string{"gzip", "zstd"},
		}

		err := config.Validate()
		if err != nil {
			t.Errorf("Validate() should pass with valid encodings: %v", err)
		}
	})

	t.Run("invalid_supported_encoding", func(t *testing.T) {
		config := &IndexServerConfig{
			Enabled:            true,
			MaxPageSize:        1000,
			AllowCreateRepo:    true,
			SupportedEncodings: []string{"gzip", "brotli"}, // brotli not supported
		}

		err := config.Validate()
		if err == nil {
			t.Error("Validate() should fail with unsupported encoding")
		}
	})
}

func TestIsEncodingSupported(t *testing.T) {
	config := &IndexServerConfig{
		EnableCompression:  true,
		SupportedEncodings: []string{"gzip", "zstd"},
	}

	tests := []struct {
		encoding  string
		supported bool
	}{
		{"", true},         // No compression always supported
		{"identity", true}, // Identity always supported
		{"gzip", true},
		{"zstd", true},
		{"brotli", false},
		{"deflate", false},
	}

	for _, tt := range tests {
		t.Run(tt.encoding, func(t *testing.T) {
			if got := config.IsEncodingSupported(tt.encoding); got != tt.supported {
				t.Errorf("IsEncodingSupported(%q) = %v, want %v", tt.encoding, got, tt.supported)
			}
		})
	}

	// Test with compression disabled
	t.Run("compression_disabled", func(t *testing.T) {
		configDisabled := &IndexServerConfig{
			EnableCompression:  false,
			SupportedEncodings: []string{"gzip", "zstd"},
		}

		// gzip should not be supported when compression is disabled
		if configDisabled.IsEncodingSupported("gzip") {
			t.Error("gzip should not be supported when compression is disabled")
		}

		// But identity should still work
		if !configDisabled.IsEncodingSupported("identity") {
			t.Error("identity should always be supported")
		}
	})
}

func TestCountingReader(t *testing.T) {
	data := []byte("hello world")
	reader := &countingReader{
		reader: &mockReader{data: data},
	}

	buf := make([]byte, 5)
	n, _ := reader.Read(buf)
	if n != 5 {
		t.Errorf("Read() returned %d, want 5", n)
	}
	if reader.count != 5 {
		t.Errorf("count = %d, want 5", reader.count)
	}

	n, _ = reader.Read(buf)
	if n != 5 {
		t.Errorf("Read() returned %d, want 5", n)
	}
	if reader.count != 10 {
		t.Errorf("count = %d, want 10", reader.count)
	}
}

// mockReader is a simple reader for testing
type mockReader struct {
	data []byte
	pos  int
}

func (r *mockReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, nil
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func TestProgressWriter(t *testing.T) {
	var progressCalls []int64
	pw := &progressWriter{
		writer:   &mockWriter{},
		total:    100,
		interval: 10,
		callback: func(written, total int64) {
			progressCalls = append(progressCalls, written)
		},
	}

	// Write 25 bytes - should trigger 1 callback (single write exceeds interval)
	// Note: callbacks only fire at end of Write, not in a loop
	data := make([]byte, 25)
	_, _ = pw.Write(data)

	if pw.written != 25 {
		t.Errorf("written = %d, want 25", pw.written)
	}
	if len(progressCalls) != 1 {
		t.Errorf("got %d progress callbacks, want 1", len(progressCalls))
	}
	if len(progressCalls) > 0 && progressCalls[0] != 25 {
		t.Errorf("first callback got written=%d, want 25", progressCalls[0])
	}

	// Write another 15 bytes (total 40) - should trigger another callback
	_, _ = pw.Write(make([]byte, 15))
	if pw.written != 40 {
		t.Errorf("written = %d, want 40", pw.written)
	}
	if len(progressCalls) != 2 {
		t.Errorf("got %d progress callbacks after second write, want 2", len(progressCalls))
	}
}

// mockWriter discards all written data
type mockWriter struct{}

func (w *mockWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func TestDeltaChangedFileChangeTypes(t *testing.T) {
	tests := []struct {
		changeType string
		valid      bool
	}{
		{"added", true},
		{"modified", true},
		{"deleted", true},
		{"renamed", true},
	}

	for _, tt := range tests {
		t.Run(tt.changeType, func(t *testing.T) {
			cf := DeltaChangedFile{
				Path:       "src/main.go",
				ChangeType: tt.changeType,
			}
			// Just verify we can create the struct - validation happens in handler
			if cf.ChangeType != tt.changeType {
				t.Errorf("ChangeType = %q, want %q", cf.ChangeType, tt.changeType)
			}
		})
	}
}

func TestDeltaUploadRequestTypes(t *testing.T) {
	req := DeltaUploadRequest{
		BaseCommit:   "abc123",
		TargetCommit: "def456",
		ChangedFiles: []DeltaChangedFile{
			{Path: "src/main.go", ChangeType: "modified"},
			{Path: "src/old.go", OldPath: "src/renamed.go", ChangeType: "renamed"},
			{Path: "src/deleted.go", ChangeType: "deleted"},
		},
	}

	if req.BaseCommit != "abc123" {
		t.Errorf("BaseCommit = %q, want %q", req.BaseCommit, "abc123")
	}
	if len(req.ChangedFiles) != 3 {
		t.Errorf("len(ChangedFiles) = %d, want 3", len(req.ChangedFiles))
	}
	if req.ChangedFiles[1].OldPath != "src/renamed.go" {
		t.Errorf("OldPath = %q, want %q", req.ChangedFiles[1].OldPath, "src/renamed.go")
	}
}
