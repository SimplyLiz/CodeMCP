package architecture

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"time"

	"ckb/internal/config"
	"ckb/internal/modules"
)

// ArchitectureGenerator generates architecture views of the repository
type ArchitectureGenerator struct {
	repoRoot      string
	config        *config.Config
	importScanner *modules.ImportScanner
	logger        *slog.Logger
	limits        *ArchitectureLimits
	cache         *ArchitectureCache
}

// GeneratorOptions contains options for architecture generation
type GeneratorOptions struct {
	Depth               int  // Depth of dependency analysis (default 2)
	IncludeExternalDeps bool // Include external dependencies in graph (default false)
	Refresh             bool // Force refresh, bypass cache
	MaxFilesScanned     int  // Override default max files limit

	// v8.0: Granularity options
	Granularity  Granularity // "module", "directory", "file" (default: "module")
	InferModules bool        // Infer modules from directory structure when no explicit modules exist (default: true)
	TargetPath   string      // Optional path to focus on (relative to repo root)
	MaxDepth     int         // Max directory depth for directory/file views (default: 4)
}

// DefaultGeneratorOptions returns the default generator options
func DefaultGeneratorOptions() *GeneratorOptions {
	return &GeneratorOptions{
		Depth:               2,
		IncludeExternalDeps: false,
		Refresh:             false,
		MaxFilesScanned:     0, // Use limit from config
		Granularity:         GranularityModule,
		InferModules:        true,
		MaxDepth:            4,
	}
}

// NewArchitectureGenerator creates a new architecture generator
func NewArchitectureGenerator(
	repoRoot string,
	cfg *config.Config,
	importScanner *modules.ImportScanner,
	logger *slog.Logger,
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

	// Dispatch based on granularity (v8.0)
	switch opts.Granularity {
	case GranularityFile:
		return g.generateFileLevel(ctx, repoStateId, opts, startTime)
	case GranularityDirectory:
		return g.generateDirectoryLevel(ctx, repoStateId, opts, startTime)
	default:
		// GranularityModule - existing behavior with optional inferred modules
		return g.generateModuleLevel(ctx, repoStateId, opts, startTime)
	}
}

// generateModuleLevel generates the module-level architecture view (existing behavior)
func (g *ArchitectureGenerator) generateModuleLevel(ctx context.Context, repoStateId string, opts *GeneratorOptions, startTime time.Time) (*ArchitectureResponse, error) {
	// Check cache first unless refresh is requested
	if !opts.Refresh {
		if cached, found := g.cache.Get(repoStateId); found {
			g.logger.Debug("Using cached architecture",
				"repoStateId", repoStateId,
				"age", time.Since(cached.ComputedAt).Seconds(),
			)
			return cached.Response, nil
		}
	}

	g.logger.Info("Generating architecture view",
		"repoStateId", repoStateId,
		"includeExternalDeps", opts.IncludeExternalDeps,
		"depth", opts.Depth,
	)

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

	g.logger.Debug("Detected modules",
		"count", len(detectionResult.Modules),
		"method", detectionResult.DetectionMethod,
	)

	// Check module count limit
	if limitErr := g.limits.checkModuleCount(len(detectionResult.Modules)); limitErr != nil {
		g.logger.Warn("Module count exceeds limit, truncating",
			"detected", len(detectionResult.Modules),
			"limit", g.limits.MaxModules,
		)
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
		Granularity:     GranularityModule,
		DetectionMethod: detectionResult.DetectionMethod,
	}

	// Cache the response
	g.cache.Set(repoStateId, response)

	duration := time.Since(startTime)
	g.logger.Info("Architecture generation completed",
		"durationMs", duration.Milliseconds(),
		"modules", len(moduleSummaries),
		"dependencies", len(dependencyGraph),
		"entrypoints", len(entrypoints),
	)

	return response, nil
}

// generateDirectoryLevel generates directory-level architecture view
func (g *ArchitectureGenerator) generateDirectoryLevel(ctx context.Context, repoStateId string, opts *GeneratorOptions, startTime time.Time) (*ArchitectureResponse, error) {
	g.logger.Info("Generating directory-level architecture view",
		"repoStateId", repoStateId,
		"targetPath", opts.TargetPath,
		"maxDepth", opts.MaxDepth,
	)

	// Detect interesting directories using the inference algorithm
	inferOpts := modules.DefaultInferOptions()
	inferOpts.MaxDepth = opts.MaxDepth
	inferOpts.TargetPath = opts.TargetPath
	inferOpts.IgnoreDirs = append(inferOpts.IgnoreDirs, g.config.Modules.Ignore...)
	inferOpts.Logger = g.logger

	directories, err := modules.DetectInferredDirectories(g.repoRoot, inferOpts)
	if err != nil {
		return nil, fmt.Errorf("directory detection failed: %w", err)
	}

	g.logger.Debug("Detected directories",
		"count", len(directories),
	)

	// Apply limits
	maxDirectories := 50 // Hard cap for directory-level view
	if len(directories) > maxDirectories {
		directories = directories[:maxDirectories]
	}

	// Scan all imports from the repository
	allImports, err := g.scanAllImports(ctx, opts.TargetPath)
	if err != nil {
		return nil, fmt.Errorf("import scanning failed: %w", err)
	}

	// Build directory-level edges
	dirEdges := g.buildDirectoryEdges(directories, allImports, opts)

	// Convert InferredDirectory to DirectorySummary
	dirSummaries := make([]DirectorySummary, 0, len(directories))
	for _, dir := range directories {
		// Calculate LOC
		loc, _ := modules.AggregateDirectoryStats(g.repoRoot, dir)

		summary := DirectorySummary{
			Path:           dir.Path,
			FileCount:      dir.FileCount,
			Language:       dir.Language,
			LOC:            loc,
			HasIndexFile:   dir.HasIndexFile,
			IsIntermediate: dir.IsIntermediate,
		}
		dirSummaries = append(dirSummaries, summary)
	}

	// Compute edge counts
	incomingCounts := make(map[string]int)
	outgoingCounts := make(map[string]int)
	for _, edge := range dirEdges {
		outgoingCounts[edge.From]++
		incomingCounts[edge.To]++
	}
	for i := range dirSummaries {
		dirSummaries[i].IncomingEdges = incomingCounts[dirSummaries[i].Path]
		dirSummaries[i].OutgoingEdges = outgoingCounts[dirSummaries[i].Path]
	}

	response := &ArchitectureResponse{
		Directories:           dirSummaries,
		DirectoryDependencies: dirEdges,
		Granularity:           GranularityDirectory,
		DetectionMethod:       "inferred",
	}

	duration := time.Since(startTime)
	g.logger.Info("Directory-level architecture generation completed",
		"durationMs", duration.Milliseconds(),
		"directories", len(dirSummaries),
		"dependencies", len(dirEdges),
	)

	return response, nil
}

// generateFileLevel generates file-level architecture view
func (g *ArchitectureGenerator) generateFileLevel(ctx context.Context, repoStateId string, opts *GeneratorOptions, startTime time.Time) (*ArchitectureResponse, error) {
	g.logger.Info("Generating file-level architecture view",
		"repoStateId", repoStateId,
		"targetPath", opts.TargetPath,
	)

	// Scan all imports from the repository
	allImports, err := g.scanAllImports(ctx, opts.TargetPath)
	if err != nil {
		return nil, fmt.Errorf("import scanning failed: %w", err)
	}

	// Build file summaries and edges
	fileSummaries, fileEdges := g.buildFileLevelData(allImports, opts)

	// Apply limits
	maxFiles := 200 // Hard cap for file-level view
	maxEdges := 500
	if len(fileSummaries) > maxFiles {
		fileSummaries = fileSummaries[:maxFiles]
	}
	if len(fileEdges) > maxEdges {
		fileEdges = fileEdges[:maxEdges]
	}

	response := &ArchitectureResponse{
		Files:            fileSummaries,
		FileDependencies: fileEdges,
		Granularity:      GranularityFile,
		DetectionMethod:  "import-scan",
	}

	duration := time.Since(startTime)
	g.logger.Info("File-level architecture generation completed",
		"durationMs", duration.Milliseconds(),
		"files", len(fileSummaries),
		"dependencies", len(fileEdges),
	)

	return response, nil
}

// scanAllImports scans all imports from the repository or a target path
func (g *ArchitectureGenerator) scanAllImports(_ context.Context, targetPath string) ([]*modules.ImportEdge, error) {
	scanPath := g.repoRoot
	if targetPath != "" {
		scanPath = filepath.Join(g.repoRoot, targetPath)
	}

	imports, err := g.importScanner.ScanDirectory(
		scanPath,
		g.repoRoot,
		g.config.Modules.Ignore,
	)
	if err != nil {
		return nil, err
	}

	return imports, nil
}

// buildDirectoryEdges aggregates file-level imports into directory-level edges
func (g *ArchitectureGenerator) buildDirectoryEdges(directories []*modules.InferredDirectory, imports []*modules.ImportEdge, opts *GeneratorOptions) []DirectoryDependencyEdge {
	// Build a set of known directory paths for fast lookup
	dirSet := make(map[string]bool)
	for _, dir := range directories {
		dirSet[dir.Path] = true
	}

	// Aggregate edges by (fromDir, toDir)
	edgeMap := make(map[string]*DirectoryDependencyEdge)

	for _, imp := range imports {
		fromDir := extractDirectoryPath(imp.From)
		toDir := extractDirectoryPath(imp.To)

		// Skip self-references
		if fromDir == toDir {
			continue
		}

		// Skip if external and not including external deps
		if imp.Kind == modules.ExternalDependency && !opts.IncludeExternalDeps {
			continue
		}

		// Only include edges to known directories (or external)
		if imp.Kind != modules.ExternalDependency && !dirSet[toDir] {
			continue
		}

		key := fromDir + ":" + toDir
		if edge, exists := edgeMap[key]; exists {
			edge.Strength++
		} else {
			edgeMap[key] = &DirectoryDependencyEdge{
				From:     fromDir,
				To:       toDir,
				Kind:     imp.Kind,
				Strength: 1,
			}
		}
	}

	// Convert map to slice
	edges := make([]DirectoryDependencyEdge, 0, len(edgeMap))
	for _, edge := range edgeMap {
		edges = append(edges, *edge)
	}

	// Sort by strength descending
	sortDirectoryEdges(edges)

	// Apply limit
	maxEdges := 200
	if len(edges) > maxEdges {
		edges = edges[:maxEdges]
	}

	return edges
}

// buildFileLevelData builds file summaries and edges from imports
func (g *ArchitectureGenerator) buildFileLevelData(imports []*modules.ImportEdge, opts *GeneratorOptions) ([]FileSummary, []FileDependencyEdge) {
	// Track unique files and their statistics
	fileStats := make(map[string]*FileSummary)

	// Build edges
	edges := make([]FileDependencyEdge, 0, len(imports))

	for _, imp := range imports {
		// Skip external if not including
		if imp.Kind == modules.ExternalDependency && !opts.IncludeExternalDeps {
			continue
		}

		// Track source file
		if _, exists := fileStats[imp.From]; !exists {
			fileStats[imp.From] = &FileSummary{
				Path:     imp.From,
				Language: detectLanguageFromPath(imp.From),
			}
		}
		fileStats[imp.From].OutgoingEdges++

		// Track target file (if local)
		if imp.Kind == modules.LocalFile || imp.Kind == modules.LocalModule {
			if _, exists := fileStats[imp.To]; !exists {
				fileStats[imp.To] = &FileSummary{
					Path:     imp.To,
					Language: detectLanguageFromPath(imp.To),
				}
			}
			fileStats[imp.To].IncomingEdges++
		}

		// Create edge
		edge := FileDependencyEdge{
			From:     imp.From,
			To:       imp.To,
			Kind:     imp.Kind,
			Line:     imp.Line,
			Resolved: imp.Kind == modules.LocalFile || imp.Kind == modules.LocalModule,
		}
		edges = append(edges, edge)
	}

	// Convert map to slice and sort by edge count
	files := make([]FileSummary, 0, len(fileStats))
	for _, f := range fileStats {
		files = append(files, *f)
	}
	sortFileSummaries(files)

	return files, edges
}

// extractDirectoryPath extracts the directory path from a file path
func extractDirectoryPath(filePath string) string {
	lastSlash := -1
	for i := len(filePath) - 1; i >= 0; i-- {
		if filePath[i] == '/' {
			lastSlash = i
			break
		}
	}
	if lastSlash <= 0 {
		return "."
	}
	return filePath[:lastSlash]
}

// detectLanguageFromPath detects language from file extension
func detectLanguageFromPath(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			ext := path[i:]
			switch ext {
			case ".go":
				return modules.LanguageGo
			case ".ts", ".tsx":
				return modules.LanguageTypeScript
			case ".js", ".jsx", ".mjs":
				return modules.LanguageJavaScript
			case ".py":
				return modules.LanguagePython
			case ".rs":
				return modules.LanguageRust
			case ".dart":
				return modules.LanguageDart
			case ".java":
				return modules.LanguageJava
			case ".kt", ".kts":
				return modules.LanguageKotlin
			}
			break
		}
	}
	return modules.LanguageUnknown
}

// sortDirectoryEdges sorts edges by strength descending
func sortDirectoryEdges(edges []DirectoryDependencyEdge) {
	sort.Slice(edges, func(i, j int) bool {
		return edges[i].Strength > edges[j].Strength
	})
}

// sortFileSummaries sorts files by total edge count descending
func sortFileSummaries(files []FileSummary) {
	sort.Slice(files, func(i, j int) bool {
		totalI := files[i].IncomingEdges + files[i].OutgoingEdges
		totalJ := files[j].IncomingEdges + files[j].OutgoingEdges
		return totalI > totalJ
	})
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
			filepath.Join(g.repoRoot, mod.RootPath),
			g.repoRoot,
			g.config.Modules.Ignore,
		)
		if err != nil {
			g.logger.Warn("Failed to scan imports for module",
				"moduleId", mod.ID,
				"error", err.Error(),
			)
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
