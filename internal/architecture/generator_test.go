package architecture

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"ckb/internal/config"
	"ckb/internal/modules"
)

// TestArchitectureGenerator tests the basic architecture generation flow
func TestArchitectureGenerator(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()

	// Create a simple Go module structure
	goModPath := filepath.Join(tmpDir, "go.mod")
	err := os.WriteFile(goModPath, []byte("module example.com/test\n\ngo 1.21\n"), 0644)
	if err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	// Create main.go
	mainGoPath := filepath.Join(tmpDir, "main.go")
	mainGoContent := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`
	err = os.WriteFile(mainGoPath, []byte(mainGoContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create main.go: %v", err)
	}

	// Create a test config
	cfg := config.DefaultConfig()
	cfg.RepoRoot = tmpDir
	cfg.ImportScan.Enabled = true
	cfg.ImportScan.MaxFileSizeBytes = 1000000
	cfg.ImportScan.ScanTimeoutMs = 30000
	cfg.ImportScan.SkipBinary = true

	// Create logger
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create import scanner
	importScanner := modules.NewImportScanner(&cfg.ImportScan, logger)

	// Create architecture generator
	generator := NewArchitectureGenerator(tmpDir, cfg, importScanner, logger)

	// Generate architecture
	ctx := context.Background()
	opts := DefaultGeneratorOptions()
	response, err := generator.Generate(ctx, "test-state-id", opts)
	if err != nil {
		t.Fatalf("Failed to generate architecture: %v", err)
	}

	// Validate response
	if len(response.Modules) == 0 {
		t.Error("Expected at least one module")
	}

	t.Logf("Generated architecture with %d modules, %d dependencies, %d entrypoints",
		len(response.Modules),
		len(response.DependencyGraph),
		len(response.Entrypoints))

	// Check that we detected the Go module
	found := false
	for _, mod := range response.Modules {
		if mod.Language == modules.LanguageGo {
			found = true
			t.Logf("Found Go module: %s (files: %d, LOC: %d)",
				mod.Name, mod.FileCount, mod.LOC)
		}
	}
	if !found {
		t.Error("Expected to find a Go module")
	}

	// Check that we detected the main entrypoint
	foundMain := false
	for _, entry := range response.Entrypoints {
		if entry.Kind == EntrypointMain {
			foundMain = true
			t.Logf("Found main entrypoint: %s", entry.FileId)
		}
	}
	if !foundMain {
		t.Error("Expected to find a main entrypoint")
	}
}

// TestArchitectureCache tests the caching functionality
func TestArchitectureCache(t *testing.T) {
	cache := NewArchitectureCache()

	// Create a test response
	response := &ArchitectureResponse{
		Modules: []ModuleSummary{
			{
				ModuleId: "test-module",
				Name:     "test",
				RootPath: ".",
				Language: modules.LanguageGo,
			},
		},
	}

	// Test cache miss
	_, found := cache.Get("state-1")
	if found {
		t.Error("Expected cache miss")
	}

	// Test cache set
	cache.Set("state-1", response)

	// Test cache hit
	cached, found := cache.Get("state-1")
	if !found {
		t.Error("Expected cache hit")
	}
	if cached.Response != response {
		t.Error("Cached response doesn't match")
	}
	if cached.RepoStateId != "state-1" {
		t.Error("Cached state ID doesn't match")
	}

	// Test cache invalidate
	cache.Invalidate("state-1")
	_, found = cache.Get("state-1")
	if found {
		t.Error("Expected cache miss after invalidation")
	}

	// Test cache clear
	cache.Set("state-1", response)
	cache.Set("state-2", response)
	if cache.Size() != 2 {
		t.Errorf("Expected cache size 2, got %d", cache.Size())
	}
	cache.Clear()
	if cache.Size() != 0 {
		t.Errorf("Expected cache size 0 after clear, got %d", cache.Size())
	}
}

// TestFilterExternalDeps tests external dependency filtering
func TestFilterExternalDeps(t *testing.T) {
	edges := []DependencyEdge{
		{From: "mod-1", To: "mod-2", Kind: modules.LocalModule, Strength: 1},
		{From: "mod-1", To: "external:lodash", Kind: modules.ExternalDependency, Strength: 5},
		{From: "mod-2", To: "mod-3", Kind: modules.WorkspacePackage, Strength: 2},
		{From: "mod-3", To: "external:react", Kind: modules.ExternalDependency, Strength: 10},
	}

	filtered := FilterExternalDeps(edges)

	if len(filtered) != 2 {
		t.Errorf("Expected 2 edges after filtering, got %d", len(filtered))
	}

	for _, edge := range filtered {
		if edge.Kind == modules.ExternalDependency {
			t.Error("External dependency not filtered out")
		}
	}
}

// TestComputeStrength tests edge strength computation
func TestComputeStrength(t *testing.T) {
	imports := []*modules.ImportEdge{
		{From: "file1.go", To: "package1", Kind: modules.ExternalDependency},
		{From: "file2.go", To: "package1", Kind: modules.ExternalDependency},
		{From: "file3.go", To: "package1", Kind: modules.ExternalDependency},
	}

	strength := ComputeStrength(imports)
	if strength != 3 {
		t.Errorf("Expected strength 3, got %d", strength)
	}
}
