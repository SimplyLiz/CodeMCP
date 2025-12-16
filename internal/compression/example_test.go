package compression_test

import (
	"fmt"

	"ckb/internal/compression"
	"ckb/internal/config"
	"ckb/internal/output"
)

// Example_basicCompression demonstrates basic compression usage
func Example_basicCompression() {
	// Create budget and limits
	budget := compression.DefaultBudget()
	limits := compression.DefaultLimits()

	// Create compressor
	compressor := compression.NewCompressor(budget, limits)

	// Create some test modules (more than budget allows)
	modules := []output.Module{
		{ModuleId: "mod1", Name: "Module 1", ImpactCount: 100},
		{ModuleId: "mod2", Name: "Module 2", ImpactCount: 90},
		{ModuleId: "mod3", Name: "Module 3", ImpactCount: 80},
		{ModuleId: "mod4", Name: "Module 4", ImpactCount: 70},
		{ModuleId: "mod5", Name: "Module 5", ImpactCount: 60},
		{ModuleId: "mod6", Name: "Module 6", ImpactCount: 50},
		{ModuleId: "mod7", Name: "Module 7", ImpactCount: 40},
		{ModuleId: "mod8", Name: "Module 8", ImpactCount: 30},
		{ModuleId: "mod9", Name: "Module 9", ImpactCount: 20},
		{ModuleId: "mod10", Name: "Module 10", ImpactCount: 10},
		{ModuleId: "mod11", Name: "Module 11", ImpactCount: 5},
		{ModuleId: "mod12", Name: "Module 12", ImpactCount: 1},
	}

	// Compress modules
	compressed, truncInfo := compressor.CompressModules(modules)

	fmt.Printf("Original: %d modules\n", len(modules))
	fmt.Printf("Compressed: %d modules\n", len(compressed))
	if truncInfo != nil {
		fmt.Printf("Truncation reason: %s\n", truncInfo.Reason)
		fmt.Printf("Dropped: %d modules\n", truncInfo.DroppedCount)
	}
	// Output:
	// Original: 12 modules
	// Compressed: 10 modules
	// Truncation reason: max-modules
	// Dropped: 2 modules
}

// Example_deduplication demonstrates deduplication functionality
func Example_deduplication() {
	// Create duplicate references
	refs := []output.Reference{
		{FileId: "file1.go", StartLine: 10, StartColumn: 5, EndLine: 10, EndColumn: 15},
		{FileId: "file1.go", StartLine: 10, StartColumn: 5, EndLine: 10, EndColumn: 15}, // duplicate
		{FileId: "file2.go", StartLine: 20, StartColumn: 3, EndLine: 20, EndColumn: 10},
		{FileId: "file1.go", StartLine: 10, StartColumn: 5, EndLine: 10, EndColumn: 15}, // duplicate
	}

	deduped := compression.DeduplicateReferences(refs)

	fmt.Printf("Original: %d references\n", len(refs))
	fmt.Printf("After dedup: %d references\n", len(deduped))
	// Output:
	// Original: 4 references
	// After dedup: 2 references
}

// Example_metrics demonstrates compression metrics
func Example_metrics() {
	// Create metrics
	metrics := compression.NewMetrics(100, 20)

	fmt.Printf("Input: %d items\n", metrics.InputCount)
	fmt.Printf("Output: %d items\n", metrics.OutputCount)
	fmt.Printf("Compression ratio: %.2f\n", metrics.CompressionRatio)
	fmt.Printf("Compression percentage: %.1f%%\n", metrics.CompressionPercentage())

	// Add truncation info
	truncInfo := compression.NewTruncationInfo(compression.TruncMaxModules, 100, 20)
	metrics.AddTruncation(truncInfo)

	fmt.Printf("Was truncated: %v\n", metrics.WasTruncated())
	fmt.Printf("Total dropped: %d\n", metrics.TotalDropped())
	// Output:
	// Input: 100 items
	// Output: 20 items
	// Compression ratio: 0.20
	// Compression percentage: 80.0%
	// Was truncated: true
	// Total dropped: 80
}

// Example_drilldowns demonstrates contextual drilldown generation
func Example_drilldowns() {
	budget := compression.DefaultBudget()

	// Create context with truncation
	ctx := &compression.DrilldownContext{
		TruncationReason: compression.TruncMaxModules,
		TopModule: &output.Module{
			ModuleId: "core-module",
			Name:     "Core Module",
		},
		Completeness: compression.CompletenessInfo{
			Score:            0.7,
			IsWorkspaceReady: false,
			IsBestEffort:     true,
		},
		SymbolId: "sym123",
		Budget:   budget,
	}

	drilldowns := compression.GenerateDrilldowns(ctx)

	fmt.Printf("Generated %d drilldowns\n", len(drilldowns))
	for i, d := range drilldowns {
		fmt.Printf("%d. %s (score: %.2f)\n", i+1, d.Label, d.RelevanceScore)
	}
	// Output:
	// Generated 4 drilldowns
	// 1. Explore top module: Core Module (score: 0.90)
	// 2. Check workspace status (score: 0.70)
	// 3. Retry after warmup (score: 0.80)
	// 4. Get maximum results (slower) (score: 0.65)
}

// Example_configIntegration demonstrates loading from config
func Example_configIntegration() {
	// Create a config with custom budget
	cfg := config.DefaultConfig()
	cfg.Budget.MaxModules = 5
	cfg.Budget.MaxSymbolsPerModule = 3

	// Load budget from config
	budget := compression.NewBudgetFromConfig(cfg)

	fmt.Printf("Max modules: %d\n", budget.MaxModules)
	fmt.Printf("Max symbols per module: %d\n", budget.MaxSymbolsPerModule)
	// Output:
	// Max modules: 5
	// Max symbols per module: 3
}
