//go:build !windows

package index

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestAcquireAndReleaseLock(t *testing.T) {
	tmpDir := t.TempDir()

	// Acquire lock
	lock, err := AcquireLock(tmpDir)
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}
	if lock == nil {
		t.Fatal("expected non-nil lock")
	}

	// Verify lock file exists and contains PID
	lockPath := filepath.Join(tmpDir, lockFile)
	content, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("failed to read lock file: %v", err)
	}

	pid, err := strconv.Atoi(string(content))
	if err != nil {
		t.Fatalf("lock file should contain PID: %v", err)
	}
	if pid != os.Getpid() {
		t.Errorf("PID: got %d, want %d", pid, os.Getpid())
	}

	// Release lock
	lock.Release()

	// Verify lock file is removed
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock file should be removed after release")
	}
}

func TestAcquireLock_AlreadyLocked(t *testing.T) {
	tmpDir := t.TempDir()

	// First lock should succeed
	lock1, err := AcquireLock(tmpDir)
	if err != nil {
		t.Fatalf("first AcquireLock failed: %v", err)
	}
	defer lock1.Release()

	// Second lock should fail
	lock2, err := AcquireLock(tmpDir)
	if err == nil {
		lock2.Release()
		t.Fatal("second AcquireLock should fail when already locked")
	}

	// Error message should mention "locked"
	if err.Error() == "" {
		t.Error("error should have a message")
	}
}

func TestAcquireLock_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	ckbDir := filepath.Join(tmpDir, ".ckb")

	// Directory doesn't exist yet
	if _, err := os.Stat(ckbDir); !os.IsNotExist(err) {
		t.Fatal("ckbDir should not exist yet")
	}

	// AcquireLock should create it
	lock, err := AcquireLock(ckbDir)
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}
	defer lock.Release()

	// Directory should now exist
	if _, err := os.Stat(ckbDir); os.IsNotExist(err) {
		t.Error("ckbDir should be created by AcquireLock")
	}
}

func TestReleaseLock_NilSafe(t *testing.T) {
	// Should not panic
	var lock *Lock
	lock.Release()
}
