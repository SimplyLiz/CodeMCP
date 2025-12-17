package query

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ckb/internal/config"
	"ckb/internal/logging"
	"ckb/internal/storage"
)

// testEngine creates a test engine with minimal configuration
func testEngine(t *testing.T) (*Engine, func()) {
	t.Helper()

	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "ckb-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Create .ckb directory
	ckbDir := filepath.Join(tmpDir, ".ckb")
	if mkdirErr := os.MkdirAll(ckbDir, 0755); mkdirErr != nil {
		_ = os.RemoveAll(tmpDir)
		t.Fatalf("failed to create .ckb dir: %v", mkdirErr)
	}

	// Create test logger (silent)
	logger := logging.NewLogger(logging.Config{
		Format: logging.JSONFormat,
		Level:  logging.ErrorLevel,
	})

	// Create test database
	db, err := storage.Open(tmpDir, logger)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create test db: %v", err)
	}

	// Create test config
	cfg := config.DefaultConfig()
	cfg.RepoRoot = tmpDir

	// Create engine
	engine, err := NewEngine(tmpDir, db, logger, cfg)
	if err != nil {
		db.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create engine: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return engine, cleanup
}

func TestNewEngine(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	if engine == nil {
		t.Fatal("engine should not be nil")
	}
}

func TestGetStatus(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()
	status, err := engine.GetStatus(ctx)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	// Verify required fields
	if status.CkbVersion == "" {
		t.Error("CkbVersion should not be empty")
	}

	if status.RepoState == nil {
		t.Error("RepoState should not be nil")
	}

	if status.Cache == nil {
		t.Error("Cache should not be nil")
	}

	// Verify backends are reported
	if len(status.Backends) == 0 {
		t.Error("Backends should not be empty")
	}
}

func TestDoctor(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("all checks", func(t *testing.T) {
		result, err := engine.Doctor(ctx, "")
		if err != nil {
			t.Fatalf("Doctor failed: %v", err)
		}

		if len(result.Checks) == 0 {
			t.Error("Checks should not be empty")
		}

		// Verify each check has required fields
		for _, check := range result.Checks {
			if check.Name == "" {
				t.Error("Check name should not be empty")
			}
			if check.Status == "" {
				t.Error("Check status should not be empty")
			}
			if check.Status != "pass" && check.Status != "warn" && check.Status != "fail" {
				t.Errorf("Invalid check status: %s", check.Status)
			}
		}
	})

	t.Run("specific check", func(t *testing.T) {
		result, err := engine.Doctor(ctx, "config")
		if err != nil {
			t.Fatalf("Doctor failed: %v", err)
		}

		// Should have filtered to just the config check
		found := false
		for _, check := range result.Checks {
			if check.Name == "config" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Config check should be present")
		}
	})
}

func TestSearchSymbols(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("empty query", func(t *testing.T) {
		opts := SearchSymbolsOptions{
			Query: "",
			Limit: 10,
		}
		result, err := engine.SearchSymbols(ctx, opts)
		if err != nil {
			t.Fatalf("SearchSymbols failed: %v", err)
		}

		// Empty query should return empty results
		if result.TotalCount != 0 {
			t.Errorf("Expected 0 results for empty query, got %d", result.TotalCount)
		}
	})

	t.Run("with query", func(t *testing.T) {
		opts := SearchSymbolsOptions{
			Query: "nonexistent",
			Limit: 10,
		}
		result, err := engine.SearchSymbols(ctx, opts)
		if err != nil {
			t.Fatalf("SearchSymbols failed: %v", err)
		}

		// Should have provenance
		if result.Provenance == nil {
			t.Error("Provenance should not be nil")
		}
	})

	t.Run("with limit", func(t *testing.T) {
		opts := SearchSymbolsOptions{
			Query: "test",
			Limit: 5,
		}
		result, err := engine.SearchSymbols(ctx, opts)
		if err != nil {
			t.Fatalf("SearchSymbols failed: %v", err)
		}

		if len(result.Symbols) > 5 {
			t.Errorf("Expected at most 5 results, got %d", len(result.Symbols))
		}
	})
}

func TestGetSymbol(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("invalid symbol id", func(t *testing.T) {
		opts := GetSymbolOptions{
			SymbolId: "invalid-symbol-id",
		}
		result, err := engine.GetSymbol(ctx, opts)

		// Either returns error or empty result for invalid symbol
		if err != nil {
			// Error is acceptable for invalid symbol
			return
		}

		// If no error, should return empty result
		if result.Symbol != nil && result.Symbol.StableId != "" {
			t.Error("Expected nil or empty symbol for invalid ID")
		}
	})
}

func TestFindReferences(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("invalid symbol id", func(t *testing.T) {
		opts := FindReferencesOptions{
			SymbolId: "invalid-symbol-id",
			Limit:    10,
		}
		result, err := engine.FindReferences(ctx, opts)

		// Either returns error or empty result for invalid symbol
		if err != nil {
			// Error is acceptable for invalid symbol
			return
		}

		// Should return empty references for invalid symbol
		if result.TotalCount != 0 {
			t.Errorf("Expected 0 references for invalid symbol, got %d", result.TotalCount)
		}
	})

	t.Run("with valid options structure", func(t *testing.T) {
		opts := FindReferencesOptions{
			SymbolId:     "test-symbol",
			IncludeTests: true,
			Limit:        50,
		}
		// Just test that the function doesn't panic with valid options structure
		_, _ = engine.FindReferences(ctx, opts)
	})
}

func TestGetArchitecture(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("basic architecture", func(t *testing.T) {
		opts := GetArchitectureOptions{
			Depth: 2,
		}
		result, err := engine.GetArchitecture(ctx, opts)
		if err != nil {
			t.Fatalf("GetArchitecture failed: %v", err)
		}

		// Should have provenance
		if result.Provenance == nil {
			t.Error("Provenance should not be nil")
		}

		// Modules might be empty for test directory, that's OK
	})

	t.Run("with external deps", func(t *testing.T) {
		opts := GetArchitectureOptions{
			Depth:               2,
			IncludeExternalDeps: true,
		}
		result, err := engine.GetArchitecture(ctx, opts)
		if err != nil {
			t.Fatalf("GetArchitecture failed: %v", err)
		}

		// Should not panic or error
		_ = result
	})
}

func TestAnalyzeImpact(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("invalid symbol", func(t *testing.T) {
		opts := AnalyzeImpactOptions{
			SymbolId: "invalid-symbol-id",
			Depth:    2,
		}
		result, err := engine.AnalyzeImpact(ctx, opts)

		// Either returns error or empty result for invalid symbol
		if err != nil {
			// Error is acceptable for invalid symbol
			return
		}

		// Should return empty impact for invalid symbol
		if len(result.DirectImpact) != 0 {
			t.Errorf("Expected 0 direct impact for invalid symbol, got %d", len(result.DirectImpact))
		}
	})

	t.Run("with valid options structure", func(t *testing.T) {
		opts := AnalyzeImpactOptions{
			SymbolId: "test-symbol",
			Depth:    3,
		}
		// Just test that the function doesn't panic with valid options structure
		_, _ = engine.AnalyzeImpact(ctx, opts)
	})
}

func TestGenerateFixScript(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Run doctor to get a response
	doctorResp, err := engine.Doctor(ctx, "")
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}

	// Generate fix script
	script := engine.GenerateFixScript(doctorResp)

	// Script should be non-empty string
	if script == "" {
		t.Error("Fix script should not be empty")
	}

	// Script should contain shebang
	if len(script) > 0 && script[0:2] != "#!" {
		t.Error("Fix script should start with shebang")
	}
}
