package architecture

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestComputeDirectoryMetrics_Basic(t *testing.T) {
	// Create a temp directory with some test files
	tmpDir, err := os.MkdirTemp("", "metrics-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a subdirectory with some source files
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}

	// Create a simple Go file
	goFile := filepath.Join(srcDir, "main.go")
	goContent := `package main

func main() {
	if true {
		println("hello")
	}
}
`
	if err := os.WriteFile(goFile, []byte(goContent), 0644); err != nil {
		t.Fatalf("Failed to write go file: %v", err)
	}

	// Create test directories
	directories := []DirectorySummary{
		{
			Path:      "src",
			FileCount: 1,
			LOC:       8,
		},
	}

	// Create metrics calculator (without git adapter for basic test)
	calc := NewMetricsCalculator(tmpDir, nil)

	// Compute metrics
	ctx := context.Background()
	err = calc.ComputeDirectoryMetrics(ctx, directories)
	if err != nil {
		t.Fatalf("ComputeDirectoryMetrics failed: %v", err)
	}

	// Verify metrics were computed
	if directories[0].Metrics == nil {
		t.Fatal("Expected metrics to be set")
	}

	// Verify LOC is set
	if directories[0].Metrics.LOC != directories[0].LOC {
		t.Errorf("Expected LOC=%d, got %d", directories[0].LOC, directories[0].Metrics.LOC)
	}

	// Git metrics should not be set (no git adapter)
	if directories[0].Metrics.LastModified != "" {
		t.Error("Expected empty LastModified without git adapter")
	}
	if directories[0].Metrics.Churn30d != 0 {
		t.Error("Expected zero Churn30d without git adapter")
	}
}

func TestComputeDirectoryMetrics_Empty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "metrics-test-empty")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create an empty subdirectory
	emptyDir := filepath.Join(tmpDir, "empty")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatalf("Failed to create empty dir: %v", err)
	}

	directories := []DirectorySummary{
		{
			Path:      "empty",
			FileCount: 0,
			LOC:       0,
		},
	}

	calc := NewMetricsCalculator(tmpDir, nil)
	ctx := context.Background()
	err = calc.ComputeDirectoryMetrics(ctx, directories)
	if err != nil {
		t.Fatalf("ComputeDirectoryMetrics failed: %v", err)
	}

	// Metrics should be nil for empty directory (no meaningful data)
	if directories[0].Metrics != nil {
		t.Error("Expected nil metrics for empty directory")
	}
}

func TestComputeDirectoryMetrics_IntermediateSkipped(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "metrics-test-intermediate")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	directories := []DirectorySummary{
		{
			Path:           "src",
			FileCount:      0,
			LOC:            0,
			IsIntermediate: true, // Should be skipped
		},
	}

	calc := NewMetricsCalculator(tmpDir, nil)
	ctx := context.Background()
	err = calc.ComputeDirectoryMetrics(ctx, directories)
	if err != nil {
		t.Fatalf("ComputeDirectoryMetrics failed: %v", err)
	}

	// Intermediate directories should not have metrics
	if directories[0].Metrics != nil {
		t.Error("Expected nil metrics for intermediate directory")
	}
}

func TestIsMetricsSourceFile(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{"Go file", "main.go", true},
		{"TypeScript file", "app.ts", true},
		{"TSX file", "component.tsx", true},
		{"JavaScript file", "script.js", true},
		{"Python file", "app.py", true},
		{"Rust file", "lib.rs", true},
		{"Java file", "Main.java", true},
		{"Kotlin file", "App.kt", true},
		{"Dart file", "main.dart", true},
		{"Text file", "readme.txt", false},
		{"Markdown file", "README.md", false},
		{"JSON file", "config.json", false},
		{"No extension", "Makefile", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMetricsSourceFile(tt.filename)
			if got != tt.want {
				t.Errorf("isMetricsSourceFile(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}
