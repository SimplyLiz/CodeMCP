package architecture

// This file demonstrates how to integrate the Architecture Generator
// with a getArchitecture MCP handler.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"ckb/internal/config"
	"ckb/internal/modules"
)

// Example: getArchitecture MCP handler implementation
//
// This would be called by the MCP server when handling a getArchitecture request.
// The actual MCP integration would be in internal/mcp/handler.go or similar.

// GetArchitectureRequest represents the incoming MCP request
type GetArchitectureRequest struct {
	Depth               *int  `json:"depth,omitempty"`
	IncludeExternalDeps *bool `json:"includeExternalDeps,omitempty"`
	Refresh             *bool `json:"refresh,omitempty"`
}

// GetArchitectureHandler handles the getArchitecture MCP request
func GetArchitectureHandler(
	ctx context.Context,
	repoRoot string,
	repoStateId string,
	request *GetArchitectureRequest,
	cfg *config.Config,
	logger *slog.Logger,
) (*ArchitectureResponse, error) {
	// Create import scanner
	importScanner := modules.NewImportScanner(&cfg.ImportScan, logger)

	// Create architecture generator
	generator := NewArchitectureGenerator(repoRoot, cfg, importScanner, logger)

	// Build options from request
	opts := DefaultGeneratorOptions()

	if request.Depth != nil {
		opts.Depth = *request.Depth
	}
	if request.IncludeExternalDeps != nil {
		opts.IncludeExternalDeps = *request.IncludeExternalDeps
	}
	if request.Refresh != nil {
		opts.Refresh = *request.Refresh
	}

	// Generate architecture
	response, err := generator.Generate(ctx, repoStateId, opts)
	if err != nil {
		logger.Error("Architecture generation failed", "error", err.Error(), "repoStateId", repoStateId)
		return nil, fmt.Errorf("failed to generate architecture: %w", err)
	}

	logger.Info("Architecture generated successfully",
		"modules", len(response.Modules),
		"dependencies", len(response.DependencyGraph),
		"entrypoints", len(response.Entrypoints),
	)

	return response, nil
}

// Example usage in an HTTP handler or MCP server:
//
// func handleGetArchitecture(w http.ResponseWriter, r *http.Request) {
//     var request GetArchitectureRequest
//     if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
//         http.Error(w, err.Error(), http.StatusBadRequest)
//         return
//     }
//
//     response, err := GetArchitectureHandler(
//         r.Context(),
//         cfg.RepoRoot,
//         currentRepoStateId,
//         &request,
//         cfg,
//         logger,
//     )
//     if err != nil {
//         http.Error(w, err.Error(), http.StatusInternalServerError)
//         return
//     }
//
//     json.NewEncoder(w).Encode(response)
// }

// ExampleUsage demonstrates typical usage patterns
func ExampleUsage() {
	// Setup (this would be done once at application startup)
	cfg := config.DefaultConfig()
	// Use slogutil.NewLogger(os.Stderr, slog.LevelInfo) to create a logger
	var logger *slog.Logger // = slogutil.NewLogger(os.Stderr, slog.LevelInfo)
	importScanner := modules.NewImportScanner(&cfg.ImportScan, logger)

	generator := NewArchitectureGenerator(
		"/path/to/repo",
		cfg,
		importScanner,
		logger,
	)

	// Generate with default options
	ctx := context.Background()
	opts := DefaultGeneratorOptions()

	response, err := generator.Generate(ctx, "state-abc123", opts)
	if err != nil {
		panic(err)
	}

	// Print summary
	fmt.Printf("Repository Architecture:\n")
	fmt.Printf("  Modules: %d\n", len(response.Modules))
	fmt.Printf("  Dependencies: %d\n", len(response.DependencyGraph))
	fmt.Printf("  Entry Points: %d\n", len(response.Entrypoints))

	// Print module details
	fmt.Println("\nModules:")
	for _, mod := range response.Modules {
		fmt.Printf("  - %s (%s): %d files, %d LOC\n",
			mod.Name, mod.Language, mod.FileCount, mod.LOC)
	}

	// Print entrypoints
	fmt.Println("\nEntry Points:")
	for _, entry := range response.Entrypoints {
		fmt.Printf("  - %s (%s): %s\n",
			entry.Name, entry.Kind, entry.FileId)
	}

	// Convert to JSON for MCP response
	jsonData, _ := json.MarshalIndent(response, "", "  ")
	fmt.Printf("\nJSON Response:\n%s\n", string(jsonData))
}

// Example: Using with cache
func ExampleWithCache() {
	cfg := config.DefaultConfig()
	// Use slogutil.NewLogger(os.Stderr, slog.LevelInfo) to create a logger
	var logger *slog.Logger // = slogutil.NewLogger(os.Stderr, slog.LevelInfo)
	importScanner := modules.NewImportScanner(&cfg.ImportScan, logger)

	generator := NewArchitectureGenerator(
		"/path/to/repo",
		cfg,
		importScanner,
		logger,
	)

	ctx := context.Background()
	repoStateId := "state-abc123"

	// Check cache first
	if cached, found := generator.GetCached(repoStateId); found {
		fmt.Println("Using cached architecture")
		// Use cached.Response
		_ = cached.Response
		return
	}

	// Generate fresh
	opts := DefaultGeneratorOptions()
	response, err := generator.Generate(ctx, repoStateId, opts)
	if err != nil {
		panic(err)
	}

	// Use response
	_ = response
}

// Example: Filtering external dependencies
func ExampleExternalDependencies() {
	// By default, external dependencies are excluded
	opts := DefaultGeneratorOptions()
	fmt.Printf("IncludeExternalDeps: %v (default)\n", opts.IncludeExternalDeps)

	// To include external dependencies in the graph:
	opts.IncludeExternalDeps = true

	// Or filter after generation:
	edges := []DependencyEdge{
		{From: "mod-1", To: "mod-2", Kind: modules.LocalModule},
		{From: "mod-1", To: "external:lodash", Kind: modules.ExternalDependency},
	}

	filtered := FilterExternalDeps(edges)
	fmt.Printf("Filtered edges: %d\n", len(filtered))
}
