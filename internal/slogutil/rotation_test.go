package slogutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSize(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"", 0},
		{"invalid", 0},
		{"100", 100},
		{"100B", 100},
		{"100b", 100},
		{"1KB", 1024},
		{"1kb", 1024},
		{"10KB", 10240},
		{"1MB", 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
		{"1GB", 1024 * 1024 * 1024},
		{"1.5MB", int64(1.5 * 1024 * 1024)},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseSize(tt.input)
			if result != tt.expected {
				t.Errorf("ParseSize(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRotatingFile_Write(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	// Create rotating file with 100 byte max size and 2 backups
	rf, err := OpenRotatingFile(path, 100, 2)
	if err != nil {
		t.Fatalf("OpenRotatingFile failed: %v", err)
	}
	defer rf.Close()

	// Write some data
	data := []byte("hello world\n")
	for i := 0; i < 5; i++ {
		_, err := rf.Write(data)
		if err != nil {
			t.Fatalf("Write failed: %v", err)
		}
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("Log file should exist")
	}
}

func TestRotatingFile_Rotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	// Create rotating file with 50 byte max size and 2 backups
	rf, err := OpenRotatingFile(path, 50, 2)
	if err != nil {
		t.Fatalf("OpenRotatingFile failed: %v", err)
	}

	// Write enough data to trigger rotation
	data := make([]byte, 30)
	for i := range data {
		data[i] = 'a'
	}
	data[len(data)-1] = '\n'

	// Write multiple times to trigger rotation
	for i := 0; i < 5; i++ {
		_, err := rf.Write(data)
		if err != nil {
			t.Fatalf("Write %d failed: %v", i, err)
		}
	}

	rf.Close()

	// Check that backup files exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("Main log file should exist")
	}
	if _, err := os.Stat(path + ".1"); os.IsNotExist(err) {
		t.Error("Backup .1 should exist")
	}
}

func TestNewFileLoggerWithRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	// Test with rotation
	logger, closer, err := NewFileLoggerWithRotation(path, LevelVerbose, "1MB", 3)
	if err != nil {
		t.Fatalf("NewFileLoggerWithRotation failed: %v", err)
	}
	defer closer.Close()

	if logger == nil {
		t.Error("Logger should not be nil")
	}

	// Test without rotation (empty maxSize)
	path2 := filepath.Join(dir, "test2.log")
	logger2, closer2, err := NewFileLoggerWithRotation(path2, LevelVerbose, "", 3)
	if err != nil {
		t.Fatalf("NewFileLoggerWithRotation without rotation failed: %v", err)
	}
	defer closer2.Close()

	if logger2 == nil {
		t.Error("Logger2 should not be nil")
	}
}
