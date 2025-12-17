package architecture_test

import (
	"context"
	"os"
	"testing"

	"ckb/internal/config"
	"ckb/internal/logging"
	"ckb/internal/modules"
	"ckb/internal/query"
	"ckb/internal/storage"
)

func TestArchitectureIntegration(t *testing.T) {
	repoRoot := "/Users/lisa/Work/Ideas/CodeMCP"

	// Change to repo root so relative paths work
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(repoRoot)

	cfg, err := config.LoadConfig(repoRoot)
	if err != nil {
		t.Fatalf("Config error: %v", err)
	}

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

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
	repoRoot := "/Users/lisa/Work/Ideas/CodeMCP"

	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(repoRoot)

	cfg, err := config.LoadConfig(repoRoot)
	if err != nil {
		t.Fatalf("Config error: %v", err)
	}

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.DebugLevel,
	})

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
