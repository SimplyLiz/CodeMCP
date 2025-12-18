package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetCKBHome(t *testing.T) {
	// Test with environment variable
	originalEnv := os.Getenv(CKBHomeEnvVar)
	defer os.Setenv(CKBHomeEnvVar, originalEnv)

	// Set custom home
	customHome := "/custom/ckb/home"
	os.Setenv(CKBHomeEnvVar, customHome)

	home, err := GetCKBHome()
	if err != nil {
		t.Fatalf("GetCKBHome failed: %v", err)
	}
	if home != customHome {
		t.Errorf("Expected %s, got %s", customHome, home)
	}

	// Test without environment variable
	os.Unsetenv(CKBHomeEnvVar)

	home, err = GetCKBHome()
	if err != nil {
		t.Fatalf("GetCKBHome failed: %v", err)
	}

	// Should end with .ckb
	if !strings.HasSuffix(home, DefaultCKBHome) {
		t.Errorf("Expected path to end with %s, got %s", DefaultCKBHome, home)
	}
}

func TestComputeRepoHash(t *testing.T) {
	// Same path should produce same hash
	hash1 := ComputeRepoHash("/some/repo/path")
	hash2 := ComputeRepoHash("/some/repo/path")
	if hash1 != hash2 {
		t.Errorf("Expected same hash for same path, got %s != %s", hash1, hash2)
	}

	// Different paths should produce different hashes
	hash3 := ComputeRepoHash("/different/repo/path")
	if hash1 == hash3 {
		t.Errorf("Expected different hash for different path, got %s == %s", hash1, hash3)
	}

	// Hash should be a valid hex string
	if len(hash1) != 16 { // 8 bytes = 16 hex chars
		t.Errorf("Expected 16 character hash, got %d: %s", len(hash1), hash1)
	}
}

func TestGetRepoDataDir(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	os.Setenv(CKBHomeEnvVar, tempDir)
	defer os.Setenv(CKBHomeEnvVar, originalEnv)

	repoRoot := "/my/repo"
	dataDir, err := GetRepoDataDir(repoRoot)
	if err != nil {
		t.Fatalf("GetRepoDataDir failed: %v", err)
	}

	// Should be under CKB_HOME/repos/
	expectedPrefix := filepath.Join(tempDir, ReposSubdir)
	if !strings.HasPrefix(dataDir, expectedPrefix) {
		t.Errorf("Expected path to start with %s, got %s", expectedPrefix, dataDir)
	}
}

func TestEnsureRepoDataDir(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	os.Setenv(CKBHomeEnvVar, tempDir)
	defer os.Setenv(CKBHomeEnvVar, originalEnv)

	repoRoot := "/my/repo"
	dataDir, err := EnsureRepoDataDir(repoRoot)
	if err != nil {
		t.Fatalf("EnsureRepoDataDir failed: %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(dataDir)
	if err != nil {
		t.Fatalf("Directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Expected a directory")
	}
}

func TestGetDecisionsDir(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	os.Setenv(CKBHomeEnvVar, tempDir)
	defer os.Setenv(CKBHomeEnvVar, originalEnv)

	repoRoot := "/my/repo"
	decisionsDir, err := GetDecisionsDir(repoRoot)
	if err != nil {
		t.Fatalf("GetDecisionsDir failed: %v", err)
	}

	// Should end with /decisions
	if !strings.HasSuffix(decisionsDir, DecisionsSubdir) {
		t.Errorf("Expected path to end with %s, got %s", DecisionsSubdir, decisionsDir)
	}
}

func TestEnsureDecisionsDir(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	os.Setenv(CKBHomeEnvVar, tempDir)
	defer os.Setenv(CKBHomeEnvVar, originalEnv)

	repoRoot := "/my/repo"
	decisionsDir, err := EnsureDecisionsDir(repoRoot)
	if err != nil {
		t.Fatalf("EnsureDecisionsDir failed: %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(decisionsDir)
	if err != nil {
		t.Fatalf("Directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Expected a directory")
	}
}

func TestGetRepoDatabasePath(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	os.Setenv(CKBHomeEnvVar, tempDir)
	defer os.Setenv(CKBHomeEnvVar, originalEnv)

	repoRoot := "/my/repo"
	dbPath, err := GetRepoDatabasePath(repoRoot)
	if err != nil {
		t.Fatalf("GetRepoDatabasePath failed: %v", err)
	}

	// Should end with ckb.db
	if !strings.HasSuffix(dbPath, "ckb.db") {
		t.Errorf("Expected path to end with ckb.db, got %s", dbPath)
	}
}

func TestGetLocalDatabasePath(t *testing.T) {
	repoRoot := "/my/repo"
	dbPath := GetLocalDatabasePath(repoRoot)

	expected := filepath.Join(repoRoot, ".ckb", "ckb.db")
	if dbPath != expected {
		t.Errorf("Expected %s, got %s", expected, dbPath)
	}
}

func TestGetSCIPIndexPath(t *testing.T) {
	repoRoot := "/my/repo"

	// Default path
	path := GetSCIPIndexPath(repoRoot, "")
	expected := filepath.Join(repoRoot, "index.scip")
	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}

	// Relative configured path
	path = GetSCIPIndexPath(repoRoot, "custom/index.scip")
	expected = filepath.Join(repoRoot, "custom/index.scip")
	if path != expected {
		t.Errorf("Expected %s, got %s", expected, path)
	}

	// Absolute configured path
	path = GetSCIPIndexPath(repoRoot, "/absolute/index.scip")
	if path != "/absolute/index.scip" {
		t.Errorf("Expected /absolute/index.scip, got %s", path)
	}
}

func TestGetRepoInfo(t *testing.T) {
	// Use a temp directory as CKB home and repo
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	os.Setenv(CKBHomeEnvVar, filepath.Join(tempDir, ".ckb-home"))
	defer os.Setenv(CKBHomeEnvVar, originalEnv)

	// Create a repo dir
	repoDir := filepath.Join(tempDir, "my-repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatalf("Failed to create repo dir: %v", err)
	}

	info, err := GetRepoInfo(repoDir)
	if err != nil {
		t.Fatalf("GetRepoInfo failed: %v", err)
	}

	// Verify all paths are set
	if info.Root == "" {
		t.Error("Root should be set")
	}
	if info.Hash == "" {
		t.Error("Hash should be set")
	}
	if info.LocalCKBDir == "" {
		t.Error("LocalCKBDir should be set")
	}
	if info.GlobalDataDir == "" {
		t.Error("GlobalDataDir should be set")
	}
	if info.LocalDatabasePath == "" {
		t.Error("LocalDatabasePath should be set")
	}
	if info.GlobalDatabasePath == "" {
		t.Error("GlobalDatabasePath should be set")
	}
	if info.DecisionsDir == "" {
		t.Error("DecisionsDir should be set")
	}

	// Verify paths are consistent
	if !strings.HasSuffix(info.LocalCKBDir, ".ckb") {
		t.Errorf("LocalCKBDir should end with .ckb, got %s", info.LocalCKBDir)
	}
	if !strings.HasSuffix(info.LocalDatabasePath, "ckb.db") {
		t.Errorf("LocalDatabasePath should end with ckb.db, got %s", info.LocalDatabasePath)
	}
	if !strings.HasSuffix(info.GlobalDatabasePath, "ckb.db") {
		t.Errorf("GlobalDatabasePath should end with ckb.db, got %s", info.GlobalDatabasePath)
	}
	if !strings.HasSuffix(info.DecisionsDir, DecisionsSubdir) {
		t.Errorf("DecisionsDir should end with %s, got %s", DecisionsSubdir, info.DecisionsDir)
	}
}

func TestCanonicalizePath(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file
	testFile := filepath.Join(tempDir, "subdir", "test.go")
	if err := os.MkdirAll(filepath.Dir(testFile), 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}
	if err := os.WriteFile(testFile, []byte("package test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test canonicalization
	canonical, err := CanonicalizePath(testFile, tempDir)
	if err != nil {
		t.Fatalf("CanonicalizePath failed: %v", err)
	}

	expected := "subdir/test.go"
	if canonical != expected {
		t.Errorf("Expected %s, got %s", expected, canonical)
	}
}

func TestNormalizePath(t *testing.T) {
	// Test that forward slashes are preserved
	result := NormalizePath("path/to/file")
	expected := "path/to/file"
	if result != expected {
		t.Errorf("NormalizePath(path/to/file): expected %s, got %s", expected, result)
	}

	// Note: filepath.ToSlash only converts the OS-specific separator
	// On Unix, backslashes are valid filename characters and won't be converted
	// On Windows, backslashes would be converted to forward slashes
}
