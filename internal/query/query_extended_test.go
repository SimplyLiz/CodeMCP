package query

import (
	"context"
	"testing"
)

// TestDoctorSpecificChecks tests specific doctor checks
func TestDoctorSpecificChecks(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	testCases := []struct {
		name      string
		checkName string
	}{
		{"git check", "git"},
		{"scip check", "scip"},
		{"lsp check", "lsp"},
		{"config check", "config"},
		{"storage check", "storage"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := engine.Doctor(ctx, tc.checkName)
			if err != nil {
				t.Fatalf("Doctor failed for %s: %v", tc.checkName, err)
			}

			if len(result.Checks) == 0 {
				t.Errorf("Expected at least one check for %s", tc.checkName)
			}

			// Verify the check has valid status
			for _, check := range result.Checks {
				validStatuses := map[string]bool{"pass": true, "warn": true, "fail": true}
				if !validStatuses[check.Status] {
					t.Errorf("Invalid status '%s' for check %s", check.Status, check.Name)
				}
			}
		})
	}
}

// TestDoctorUnknownCheck tests handling of unknown check name
func TestDoctorUnknownCheck(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	result, err := engine.Doctor(ctx, "nonexistent-check")
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}

	// Should have a check with fail status for unknown check
	found := false
	for _, check := range result.Checks {
		if check.Name == "nonexistent-check" && check.Status == "fail" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected fail check for unknown check name")
	}
}

// TestDoctorQueryDuration tests that query duration is populated
func TestDoctorQueryDuration(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	result, err := engine.Doctor(ctx, "config")
	if err != nil {
		t.Fatalf("Doctor failed: %v", err)
	}

	// Duration should be positive (or at least 0)
	if result.QueryDurationMs < 0 {
		t.Errorf("Expected non-negative query duration, got %d", result.QueryDurationMs)
	}
}

// TestGenerateFixScriptEmpty tests fix script generation with no issues
func TestGenerateFixScriptEmpty(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	// Create a response with all passing checks
	response := &DoctorResponse{
		Healthy: true,
		Checks: []DoctorCheck{
			{Name: "test", Status: "pass", Message: "All good"},
		},
	}

	script := engine.GenerateFixScript(response)

	// Should still have shebang and basic structure
	if len(script) == 0 {
		t.Error("Expected non-empty script even with no fixes")
	}

	if script[0:2] != "#!" {
		t.Error("Script should start with shebang")
	}
}

// TestGenerateFixScriptWithFixes tests fix script with actual fixes
func TestGenerateFixScriptWithFixes(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	response := &DoctorResponse{
		Healthy: false,
		Checks: []DoctorCheck{
			{
				Name:    "test",
				Status:  "fail",
				Message: "Something is broken",
				SuggestedFixes: []FixAction{
					{
						Type:        "run-command",
						Command:     "echo 'fixing it'",
						Safe:        true,
						Description: "Fix the thing",
					},
				},
			},
		},
	}

	script := engine.GenerateFixScript(response)

	// Should contain the fix command
	if !containsStr(script, "echo 'fixing it'") {
		t.Error("Script should contain the fix command")
	}

	if !containsStr(script, "Fix the thing") {
		t.Error("Script should contain the fix description")
	}
}

// TestSearchSymbolsWithFilters tests symbol search with various filters
func TestSearchSymbolsWithFilters(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("with kinds filter", func(t *testing.T) {
		opts := SearchSymbolsOptions{
			Query: "test",
			Kinds: []string{"function", "class"},
			Limit: 10,
		}
		result, err := engine.SearchSymbols(ctx, opts)
		if err != nil {
			t.Fatalf("SearchSymbols failed: %v", err)
		}

		// Should not panic and return valid result
		_ = result
	})

	t.Run("with scope filter", func(t *testing.T) {
		opts := SearchSymbolsOptions{
			Query: "test",
			Scope: "internal",
			Limit: 10,
		}
		result, err := engine.SearchSymbols(ctx, opts)
		if err != nil {
			t.Fatalf("SearchSymbols failed: %v", err)
		}

		_ = result
	})
}

// TestFindReferencesWithFilters tests reference finding with filters
func TestFindReferencesWithFilters(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("with scope", func(t *testing.T) {
		opts := FindReferencesOptions{
			SymbolId: "test-symbol",
			Scope:    "internal",
			Limit:    10,
		}
		// Should not panic
		_, _ = engine.FindReferences(ctx, opts)
	})

	t.Run("exclude tests", func(t *testing.T) {
		opts := FindReferencesOptions{
			SymbolId:     "test-symbol",
			IncludeTests: false,
			Limit:        10,
		}
		// Should not panic
		_, _ = engine.FindReferences(ctx, opts)
	})
}

// TestGetArchitectureWithRefresh tests architecture retrieval with refresh
func TestGetArchitectureWithRefresh(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	opts := GetArchitectureOptions{
		Depth:   1,
		Refresh: true,
	}
	result, err := engine.GetArchitecture(ctx, opts)
	if err != nil {
		t.Fatalf("GetArchitecture failed: %v", err)
	}

	if result.Provenance == nil {
		t.Error("Provenance should not be nil")
	}
}

// TestAnalyzeImpactWithDepth tests impact analysis with different depths
func TestAnalyzeImpactWithDepth(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	depths := []int{1, 2, 3, 5}

	for _, depth := range depths {
		t.Run("depth="+string(rune('0'+depth)), func(t *testing.T) {
			opts := AnalyzeImpactOptions{
				SymbolId: "test-symbol",
				Depth:    depth,
			}
			// Should not panic with any valid depth
			_, _ = engine.AnalyzeImpact(ctx, opts)
		})
	}
}

// TestCallGraphDirections tests all call graph directions
func TestCallGraphDirections(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	directions := []string{"callers", "callees", "both"}

	for _, dir := range directions {
		t.Run("direction="+dir, func(t *testing.T) {
			opts := CallGraphOptions{
				SymbolId:  "test-symbol",
				Direction: dir,
				Depth:     1,
			}
			// Should not panic with any valid direction
			_, _ = engine.GetCallGraph(ctx, opts)
		})
	}
}

// TestCallGraphDepthLimits tests call graph depth limits
func TestCallGraphDepthLimits(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("depth=0 uses default", func(t *testing.T) {
		opts := CallGraphOptions{
			SymbolId:  "test-symbol",
			Direction: "both",
			Depth:     0,
		}
		// Should not panic, should use default depth
		_, _ = engine.GetCallGraph(ctx, opts)
	})

	t.Run("depth=4 max", func(t *testing.T) {
		opts := CallGraphOptions{
			SymbolId:  "test-symbol",
			Direction: "both",
			Depth:     4,
		}
		// Should not panic
		_, _ = engine.GetCallGraph(ctx, opts)
	})
}

// TestExplainPathRoles tests different file role classifications
func TestExplainPathRoles(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	// Test that ExplainPath returns consistent results for various file types
	testCases := []struct {
		path         string
		expectedRole string
	}{
		{"internal/query/engine_test.go", "test-only"},
		{"config.json", "config"},
		{"settings.yaml", "config"},
		{".env.example", "config"},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			opts := ExplainPathOptions{
				FilePath: tc.path,
			}
			result, err := engine.ExplainPath(ctx, opts)
			if err != nil {
				t.Fatalf("ExplainPath failed: %v", err)
			}

			if result.Role != tc.expectedRole {
				t.Errorf("Expected role '%s' for %s, got '%s'", tc.expectedRole, tc.path, result.Role)
			}
		})
	}
}

// TestExplainPathReturnsValidRole tests that ExplainPath always returns a valid role
func TestExplainPathReturnsValidRole(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	validRoles := map[string]bool{
		"core":      true,
		"glue":      true,
		"test-only": true,
		"config":    true,
		"legacy":    true,
		"unknown":   true,
	}

	testPaths := []string{
		"internal/query/engine.go",
		"cmd/main.go",
		"pkg/util/helper.go",
		"random/file.go",
	}

	for _, path := range testPaths {
		t.Run(path, func(t *testing.T) {
			opts := ExplainPathOptions{
				FilePath: path,
			}
			result, err := engine.ExplainPath(ctx, opts)
			if err != nil {
				t.Fatalf("ExplainPath failed: %v", err)
			}

			if !validRoles[result.Role] {
				t.Errorf("Invalid role '%s' for %s", result.Role, path)
			}
		})
	}
}

// TestHotspotsLimitEnforcement tests that hotspots limit is enforced
func TestHotspotsLimitEnforcement(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("limit 0 uses default", func(t *testing.T) {
		opts := GetHotspotsOptions{
			Limit: 0,
		}
		result, err := engine.GetHotspots(ctx, opts)
		if err != nil {
			return // Git may not be available
		}

		// Default is usually 20
		if len(result.Hotspots) > 20 {
			t.Errorf("Expected at most 20 hotspots with default limit, got %d", len(result.Hotspots))
		}
	})

	t.Run("limit 50 max", func(t *testing.T) {
		opts := GetHotspotsOptions{
			Limit: 100, // Request more than max
		}
		result, err := engine.GetHotspots(ctx, opts)
		if err != nil {
			return
		}

		// Should be capped at 50
		if len(result.Hotspots) > 50 {
			t.Errorf("Expected at most 50 hotspots, got %d", len(result.Hotspots))
		}
	})
}

// TestKeyConceptsLimit tests that key concepts limit is enforced
func TestKeyConceptsLimit(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("limit 0 uses default", func(t *testing.T) {
		opts := ListKeyConceptsOptions{
			Limit: 0,
		}
		result, err := engine.ListKeyConcepts(ctx, opts)
		if err != nil {
			t.Fatalf("ListKeyConcepts failed: %v", err)
		}

		// Default is usually 12
		if len(result.Concepts) > 12 {
			t.Errorf("Expected at most 12 concepts with default limit, got %d", len(result.Concepts))
		}
	})

	t.Run("limit enforced", func(t *testing.T) {
		opts := ListKeyConceptsOptions{
			Limit: 3,
		}
		result, err := engine.ListKeyConcepts(ctx, opts)
		if err != nil {
			t.Fatalf("ListKeyConcepts failed: %v", err)
		}

		if len(result.Concepts) > 3 {
			t.Errorf("Expected at most 3 concepts, got %d", len(result.Concepts))
		}
	})
}

// TestTraceUsageMaxPaths tests max paths limit
func TestTraceUsageMaxPaths(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("default max paths", func(t *testing.T) {
		opts := TraceUsageOptions{
			SymbolId: "test-symbol",
			MaxPaths: 0, // Use default
		}
		// Should not panic
		_, _ = engine.TraceUsage(ctx, opts)
	})

	t.Run("custom max paths", func(t *testing.T) {
		opts := TraceUsageOptions{
			SymbolId: "test-symbol",
			MaxPaths: 5,
			MaxDepth: 3,
		}
		// Should not panic
		_, _ = engine.TraceUsage(ctx, opts)
	})
}

// TestEntrypointsLimit tests entrypoints limit
func TestEntrypointsLimit(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("default limit", func(t *testing.T) {
		opts := ListEntrypointsOptions{
			Limit: 0, // Use default
		}
		result, err := engine.ListEntrypoints(ctx, opts)
		if err != nil {
			t.Fatalf("ListEntrypoints failed: %v", err)
		}

		// Should not panic and return result
		_ = result
	})

	t.Run("custom limit", func(t *testing.T) {
		opts := ListEntrypointsOptions{
			Limit: 5,
		}
		result, err := engine.ListEntrypoints(ctx, opts)
		if err != nil {
			t.Fatalf("ListEntrypoints failed: %v", err)
		}

		if len(result.Entrypoints) > 5 {
			t.Errorf("Expected at most 5 entrypoints, got %d", len(result.Entrypoints))
		}
	})
}

// TestStatusResponse tests the structure of status response
func TestStatusResponse(t *testing.T) {
	engine, cleanup := testEngine(t)
	defer cleanup()

	ctx := context.Background()

	status, err := engine.GetStatus(ctx)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	// Verify all required fields
	if status.CkbVersion == "" {
		t.Error("CkbVersion should not be empty")
	}

	if status.RepoState == nil {
		t.Error("RepoState should not be nil")
	}

	if len(status.Backends) == 0 {
		t.Error("Backends should not be empty")
	}

	if status.Cache == nil {
		t.Error("Cache should not be nil")
	}

	// Verify backend entries have required fields
	for _, backend := range status.Backends {
		if backend.Id == "" {
			t.Error("Backend should have Id field")
		}
		// Capabilities might be empty for some backends
	}
}

// Helper function
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// BenchmarkDoctor benchmarks the doctor command
func BenchmarkDoctor(b *testing.B) {
	engine, cleanup := testEngine(&testing.T{})
	defer cleanup()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.Doctor(ctx, "")
	}
}

// BenchmarkSearchSymbols benchmarks symbol search
func BenchmarkSearchSymbols(b *testing.B) {
	engine, cleanup := testEngine(&testing.T{})
	defer cleanup()

	ctx := context.Background()
	opts := SearchSymbolsOptions{
		Query: "test",
		Limit: 10,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.SearchSymbols(ctx, opts)
	}
}

// BenchmarkExplainPath benchmarks path explanation
func BenchmarkExplainPath(b *testing.B) {
	engine, cleanup := testEngine(&testing.T{})
	defer cleanup()

	ctx := context.Background()
	opts := ExplainPathOptions{
		FilePath: "internal/query/engine.go",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = engine.ExplainPath(ctx, opts)
	}
}
