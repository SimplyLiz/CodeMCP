package architecture_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"ckb/internal/config"
	"ckb/internal/modules"
	"ckb/internal/query"
	"ckb/internal/storage"
)

// findRepoRoot finds the repository root by looking for go.mod
func findRepoRoot() (string, error) {
	// Start from current working directory
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up until we find go.mod
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod in any parent directory")
		}
		dir = parent
	}
}

func TestArchitectureIntegration(t *testing.T) {
	// Get repo root from working directory (tests run from repo root via go test ./...)
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("Could not find repo root: %v", err)
	}

	// Change to repo root so relative paths work
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(repoRoot)

	cfg, err := config.LoadConfig(repoRoot)
	if err != nil {
		t.Fatalf("Config error: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := storage.Open(repoRoot, logger)
	if err != nil {
		t.Skipf("Storage not available: %v", err)
	}
	defer func() { _ = db.Close() }()

	engine, err := query.NewEngine(repoRoot, db, logger, cfg)
	if err != nil {
		t.Fatalf("Engine error: %v", err)
	}
	defer func() { _ = engine.Close() }()

	ctx := context.Background()
	arch, err := engine.GetArchitecture(ctx, query.GetArchitectureOptions{
		Depth:   2,
		Refresh: true,
	})
	if err != nil {
		t.Fatalf("Architecture error: %v", err)
	}

	t.Logf("=== Architecture Results ===")
	t.Logf("Modules: %d", len(arch.Modules))
	t.Logf("Dependencies: %d", len(arch.DependencyGraph))
	t.Logf("Entrypoints: %d", len(arch.Entrypoints))

	t.Log("\n--- Modules ---")
	for _, m := range arch.Modules {
		t.Logf("  %s: %d symbols, path=%s", m.Name, m.SymbolCount, m.Path)
	}

	if len(arch.DependencyGraph) > 0 {
		t.Log("\n--- Sample Dependencies ---")
		for i, e := range arch.DependencyGraph {
			if i >= 10 {
				break
			}
			t.Logf("  %s -> %s (%s, strength=%d)", e.From, e.To, e.Kind, e.Strength)
		}
	}

	// Assertions
	if len(arch.Modules) == 0 {
		t.Error("Expected at least one module")
	}

	// Check that we have some modules with symbol counts
	totalSymbols := 0
	for _, m := range arch.Modules {
		totalSymbols += m.SymbolCount
	}
	t.Logf("Total symbols across all modules: %d", totalSymbols)
}

func TestModuleDetection(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Skipf("Could not find repo root: %v", err)
	}

	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(repoRoot)

	cfg, err := config.LoadConfig(repoRoot)
	if err != nil {
		t.Fatalf("Config error: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	result, err := modules.DetectModules(".", cfg.Modules.Roots, cfg.Modules.Ignore, "test", logger)
	if err != nil {
		t.Fatalf("Detection error: %v", err)
	}

	t.Logf("Detection method: %s", result.DetectionMethod)
	t.Logf("Found %d modules:", len(result.Modules))
	for _, mod := range result.Modules {
		t.Logf("  ID=%s Name=%s RootPath=%s Language=%s",
			mod.ID, mod.Name, mod.RootPath, mod.Language)
	}

	// Test import scanning for a specific module
	scanner := modules.NewImportScanner(&cfg.ImportScan, logger)

	// Find the query module
	for _, mod := range result.Modules {
		if mod.RootPath == "internal/query" {
			t.Logf("\n=== Imports from internal/query ===")
			imports, err := scanner.ScanDirectory(
				repoRoot+"/"+mod.RootPath,
				repoRoot,
				cfg.Modules.Ignore,
			)
			if err != nil {
				t.Errorf("Scan error: %v", err)
				continue
			}
			for i, imp := range imports {
				if i >= 20 {
					t.Logf("... and %d more", len(imports)-20)
					break
				}
				t.Logf("  %s -> %s (line %d)", imp.From, imp.To, imp.Line)
			}
			break
		}
	}
}
