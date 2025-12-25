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
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

	// Set custom home
	customHome := "/custom/ckb/home"
	_ = os.Setenv(CKBHomeEnvVar, customHome)

	home, err := GetCKBHome()
	if err != nil {
		t.Fatalf("GetCKBHome failed: %v", err)
	}
	if home != customHome {
		t.Errorf("Expected %s, got %s", customHome, home)
	}

	// Test without environment variable
	_ = os.Unsetenv(CKBHomeEnvVar)

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
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

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
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

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
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

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
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

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
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

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
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, filepath.Join(tempDir, ".ckb-home"))
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

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
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

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

func TestJoinRepoPath(t *testing.T) {
	result := JoinRepoPath("/repo/root", "path/to/file.go")
	expected := filepath.Join("/repo/root", "path", "to", "file.go")
	if result != expected {
		t.Errorf("JoinRepoPath: expected %s, got %s", expected, result)
	}
}

func TestIsWithinRepo(t *testing.T) {
	// Create a temp directory for testing
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Create a file inside repo
	testFile := filepath.Join(tempDir, "subdir", "test.go")
	if err := os.MkdirAll(filepath.Dir(testFile), 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}
	if err := os.WriteFile(testFile, []byte("package test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// File inside repo should return true
	if !IsWithinRepo(testFile, tempDir) {
		t.Error("Expected file to be within repo")
	}

	// File outside repo should return false
	outsideFile := filepath.Join(os.TempDir(), "outside.go")
	if IsWithinRepo(outsideFile, tempDir) {
		t.Error("Expected file outside repo to return false")
	}
}

func TestGetFederationsDir(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

	fedDir, err := GetFederationsDir()
	if err != nil {
		t.Fatalf("GetFederationsDir failed: %v", err)
	}

	expected := filepath.Join(tempDir, FederationSubdir)
	if fedDir != expected {
		t.Errorf("Expected %s, got %s", expected, fedDir)
	}
}

func TestGetFederationDir(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

	fedDir, err := GetFederationDir("my-federation")
	if err != nil {
		t.Fatalf("GetFederationDir failed: %v", err)
	}

	expected := filepath.Join(tempDir, FederationSubdir, "my-federation")
	if fedDir != expected {
		t.Errorf("Expected %s, got %s", expected, fedDir)
	}
}

func TestGetFederationConfigPath(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

	configPath, err := GetFederationConfigPath("my-federation")
	if err != nil {
		t.Fatalf("GetFederationConfigPath failed: %v", err)
	}

	if !strings.HasSuffix(configPath, FederationConfigFile) {
		t.Errorf("Expected path to end with %s, got %s", FederationConfigFile, configPath)
	}
}

func TestGetFederationIndexPath(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

	indexPath, err := GetFederationIndexPath("my-federation")
	if err != nil {
		t.Fatalf("GetFederationIndexPath failed: %v", err)
	}

	if !strings.HasSuffix(indexPath, FederationIndexFile) {
		t.Errorf("Expected path to end with %s, got %s", FederationIndexFile, indexPath)
	}
}

func TestEnsureFederationDir(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

	fedDir, err := EnsureFederationDir("my-federation")
	if err != nil {
		t.Fatalf("EnsureFederationDir failed: %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(fedDir)
	if err != nil {
		t.Fatalf("Directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Expected a directory")
	}
}

func TestListFederations_Empty(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

	federations, err := ListFederations()
	if err != nil {
		t.Fatalf("ListFederations failed: %v", err)
	}

	if len(federations) != 0 {
		t.Errorf("Expected empty list, got %v", federations)
	}
}

func TestFederationExists(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

	// Non-existent federation
	exists, err := FederationExists("nonexistent")
	if err != nil {
		t.Fatalf("FederationExists failed: %v", err)
	}
	if exists {
		t.Error("Expected federation not to exist")
	}

	// Create a federation with config
	fedDir, err := EnsureFederationDir("test-fed")
	if err != nil {
		t.Fatalf("EnsureFederationDir failed: %v", err)
	}
	configPath := filepath.Join(fedDir, FederationConfigFile)
	if err := os.WriteFile(configPath, []byte("name = \"test-fed\""), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	exists, err = FederationExists("test-fed")
	if err != nil {
		t.Fatalf("FederationExists failed: %v", err)
	}
	if !exists {
		t.Error("Expected federation to exist")
	}
}

func TestDeleteFederationDir(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

	// Create and then delete a federation
	_, err = EnsureFederationDir("to-delete")
	if err != nil {
		t.Fatalf("EnsureFederationDir failed: %v", err)
	}

	err = DeleteFederationDir("to-delete")
	if err != nil {
		t.Fatalf("DeleteFederationDir failed: %v", err)
	}

	// Verify it was deleted
	exists, err := FederationExists("to-delete")
	if err != nil {
		t.Fatalf("FederationExists failed: %v", err)
	}
	if exists {
		t.Error("Expected federation to be deleted")
	}
}

func TestGetDaemonDir(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

	daemonDir, err := GetDaemonDir()
	if err != nil {
		t.Fatalf("GetDaemonDir failed: %v", err)
	}

	expected := filepath.Join(tempDir, DaemonSubdir)
	if daemonDir != expected {
		t.Errorf("Expected %s, got %s", expected, daemonDir)
	}
}

func TestEnsureDaemonDir(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

	daemonDir, err := EnsureDaemonDir()
	if err != nil {
		t.Fatalf("EnsureDaemonDir failed: %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(daemonDir)
	if err != nil {
		t.Fatalf("Directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Expected a directory")
	}
}

func TestGetDaemonPaths(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

	// Test PID path
	pidPath, err := GetDaemonPIDPath()
	if err != nil {
		t.Fatalf("GetDaemonPIDPath failed: %v", err)
	}
	if !strings.HasSuffix(pidPath, DaemonPIDFile) {
		t.Errorf("Expected path to end with %s, got %s", DaemonPIDFile, pidPath)
	}

	// Test log path
	logPath, err := GetDaemonLogPath()
	if err != nil {
		t.Fatalf("GetDaemonLogPath failed: %v", err)
	}
	if !strings.HasSuffix(logPath, DaemonLogFile) {
		t.Errorf("Expected path to end with %s, got %s", DaemonLogFile, logPath)
	}

	// Test DB path
	dbPath, err := GetDaemonDBPath()
	if err != nil {
		t.Fatalf("GetDaemonDBPath failed: %v", err)
	}
	if !strings.HasSuffix(dbPath, DaemonDBFile) {
		t.Errorf("Expected path to end with %s, got %s", DaemonDBFile, dbPath)
	}

	// Test socket path
	socketPath, err := GetDaemonSocketPath()
	if err != nil {
		t.Fatalf("GetDaemonSocketPath failed: %v", err)
	}
	if !strings.HasSuffix(socketPath, DaemonSocketFile) {
		t.Errorf("Expected path to end with %s, got %s", DaemonSocketFile, socketPath)
	}
}

func TestGetDaemonInfo(t *testing.T) {
	// Use a temp directory as CKB home
	tempDir, err := os.MkdirTemp("", "ckb-paths-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Set CKB_HOME
	originalEnv := os.Getenv(CKBHomeEnvVar)
	_ = os.Setenv(CKBHomeEnvVar, tempDir)
	t.Cleanup(func() { _ = os.Setenv(CKBHomeEnvVar, originalEnv) })

	info, err := GetDaemonInfo()
	if err != nil {
		t.Fatalf("GetDaemonInfo failed: %v", err)
	}

	if info.Dir == "" {
		t.Error("Dir should be set")
	}
	if info.PIDPath == "" {
		t.Error("PIDPath should be set")
	}
	if info.LogPath == "" {
		t.Error("LogPath should be set")
	}
	if info.DBPath == "" {
		t.Error("DBPath should be set")
	}
	if info.SocketPath == "" {
		t.Error("SocketPath should be set")
	}

	// Verify paths are consistent
	if !strings.HasSuffix(info.PIDPath, DaemonPIDFile) {
		t.Errorf("PIDPath should end with %s, got %s", DaemonPIDFile, info.PIDPath)
	}
	if !strings.HasSuffix(info.LogPath, DaemonLogFile) {
		t.Errorf("LogPath should end with %s, got %s", DaemonLogFile, info.LogPath)
	}
	if !strings.HasSuffix(info.DBPath, DaemonDBFile) {
		t.Errorf("DBPath should end with %s, got %s", DaemonDBFile, info.DBPath)
	}
}

func TestPathConstants(t *testing.T) {
	if DefaultCKBHome != ".ckb" {
		t.Errorf("DefaultCKBHome = %q, want %q", DefaultCKBHome, ".ckb")
	}
	if ReposSubdir != "repos" {
		t.Errorf("ReposSubdir = %q, want %q", ReposSubdir, "repos")
	}
	if DecisionsSubdir != "decisions" {
		t.Errorf("DecisionsSubdir = %q, want %q", DecisionsSubdir, "decisions")
	}
	if CKBHomeEnvVar != "CKB_HOME" {
		t.Errorf("CKBHomeEnvVar = %q, want %q", CKBHomeEnvVar, "CKB_HOME")
	}
	if FederationSubdir != "federation" {
		t.Errorf("FederationSubdir = %q, want %q", FederationSubdir, "federation")
	}
	if FederationConfigFile != "config.toml" {
		t.Errorf("FederationConfigFile = %q, want %q", FederationConfigFile, "config.toml")
	}
	if DaemonSubdir != "daemon" {
		t.Errorf("DaemonSubdir = %q, want %q", DaemonSubdir, "daemon")
	}
}
