package architecture

import (
	"context"
	"fmt"
	"time"

	"ckb/internal/config"
	"ckb/internal/logging"
	"ckb/internal/modules"
)

// ArchitectureGenerator generates architecture views of the repository
type ArchitectureGenerator struct {
	repoRoot      string
	config        *config.Config
	importScanner *modules.ImportScanner
	logger        *logging.Logger
	limits        *ArchitectureLimits
	cache         *ArchitectureCache
}

// GeneratorOptions contains options for architecture generation
type GeneratorOptions struct {
	Depth               int  // Depth of dependency analysis (default 2)
	IncludeExternalDeps bool // Include external dependencies in graph (default false)
	Refresh             bool // Force refresh, bypass cache
	MaxFilesScanned     int  // Override default max files limit
}

// DefaultGeneratorOptions returns the default generator options
func DefaultGeneratorOptions() *GeneratorOptions {
	return &GeneratorOptions{
		Depth:               2,
		IncludeExternalDeps: false,
		Refresh:             false,
		MaxFilesScanned:     0, // Use limit from config
	}
}

// NewArchitectureGenerator creates a new architecture generator
func NewArchitectureGenerator(
	repoRoot string,
	cfg *config.Config,
	importScanner *modules.ImportScanner,
	logger *logging.Logger,
) *ArchitectureGenerator {
	limits := DefaultLimits()

	// Override limits from config if available
	if cfg.BackendLimits.MaxFilesScanned > 0 {
		limits.MaxFilesScanned = cfg.BackendLimits.MaxFilesScanned
	}
	if cfg.Budget.MaxModules > 0 {
		limits.MaxModules = cfg.Budget.MaxModules
	}

	return &ArchitectureGenerator{
		repoRoot:      repoRoot,
		config:        cfg,
		importScanner: importScanner,
		logger:        logger,
		limits:        limits,
		cache:         NewArchitectureCache(),
	}
}

// Generate generates the complete architecture view
func (g *ArchitectureGenerator) Generate(ctx context.Context, repoStateId string, opts *GeneratorOptions) (*ArchitectureResponse, error) {
	startTime := time.Now()

	// Use default options if not provided
	if opts == nil {
		opts = DefaultGeneratorOptions()
	}

	// Check cache first unless refresh is requested
	if !opts.Refresh {
		if cached, found := g.cache.Get(repoStateId); found {
			g.logger.Debug("Using cached architecture", map[string]interface{}{
				"repoStateId": repoStateId,
				"age":         time.Since(cached.ComputedAt).Seconds(),
			})
			return cached.Response, nil
		}
	}

	g.logger.Info("Generating architecture view", map[string]interface{}{
		"repoStateId":           repoStateId,
		"includeExternalDeps":   opts.IncludeExternalDeps,
		"depth":                 opts.Depth,
	})

	// Step 1: Detect modules
	detectionResult, err := modules.DetectModules(
		g.repoRoot,
		g.config.Modules.Roots,
		g.config.Modules.Ignore,
		repoStateId,
		g.logger,
	)
	if err != nil {
		return nil, fmt.Errorf("module detection failed: %w", err)
	}

	g.logger.Debug("Detected modules", map[string]interface{}{
		"count":  len(detectionResult.Modules),
		"method": detectionResult.DetectionMethod,
	})

	// Check module count limit
	if err := g.limits.checkModuleCount(len(detectionResult.Modules)); err != nil {
		g.logger.Warn("Module count exceeds limit, truncating", map[string]interface{}{
			"detected": len(detectionResult.Modules),
			"limit":    g.limits.MaxModules,
		})
		detectionResult.Modules = detectionResult.Modules[:g.limits.MaxModules]
	}

	// Step 2: Aggregate module statistics
	moduleSummaries, err := g.AggregateModules(detectionResult.Modules)
	if err != nil {
		return nil, fmt.Errorf("module aggregation failed: %w", err)
	}

	// Step 3: Scan imports and build dependency graph
	importsByModule, err := g.scanImportsForModules(ctx, detectionResult.Modules)
	if err != nil {
		return nil, fmt.Errorf("import scanning failed: %w", err)
	}

	dependencyGraph, err := g.BuildDependencyGraph(detectionResult.Modules, importsByModule, opts)
	if err != nil {
		return nil, fmt.Errorf("dependency graph building failed: %w", err)
	}

	// Step 4: Detect entrypoints
	entrypoints, err := g.DetectEntrypoints(detectionResult.Modules)
	if err != nil {
		return nil, fmt.Errorf("entrypoint detection failed: %w", err)
	}

	// Build response
	response := &ArchitectureResponse{
		Modules:         moduleSummaries,
		DependencyGraph: dependencyGraph,
		Entrypoints:     entrypoints,
	}

	// Cache the response
	g.cache.Set(repoStateId, response)

	duration := time.Since(startTime)
	g.logger.Info("Architecture generation completed", map[string]interface{}{
		"durationMs":     duration.Milliseconds(),
		"modules":        len(moduleSummaries),
		"dependencies":   len(dependencyGraph),
		"entrypoints":    len(entrypoints),
	})

	return response, nil
}

// scanImportsForModules scans all imports for all modules
func (g *ArchitectureGenerator) scanImportsForModules(ctx context.Context, mods []*modules.Module) (map[string][]*modules.ImportEdge, error) {
	result := make(map[string][]*modules.ImportEdge)

	for _, mod := range mods {
		// Check context for cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Scan imports for this module
		imports, err := g.importScanner.ScanDirectory(
			g.repoRoot+"/"+mod.RootPath,
			g.repoRoot,
			g.config.Modules.Ignore,
		)
		if err != nil {
			g.logger.Warn("Failed to scan imports for module", map[string]interface{}{
				"moduleId": mod.ID,
				"error":    err.Error(),
			})
			continue
		}

		result[mod.ID] = imports
	}

	return result, nil
}

// GetCached retrieves a cached architecture response if available
func (g *ArchitectureGenerator) GetCached(repoStateId string) (*CachedArchitecture, bool) {
	return g.cache.Get(repoStateId)
}

// InvalidateCache removes cached architecture for a specific repo state
func (g *ArchitectureGenerator) InvalidateCache(repoStateId string) {
	g.cache.Invalidate(repoStateId)
}

// ClearCache clears all cached architectures
func (g *ArchitectureGenerator) ClearCache() {
	g.cache.Clear()
}
