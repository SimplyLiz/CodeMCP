# Phase 3.4: Architecture Generator Implementation

## Status: ✅ Complete

Implementation of the Architecture Generator for CKB (Codebase Knowledge Backend) per Phase 3.4 requirements.

## What Was Implemented

### 1. Core Types (`types.go`)
- ✅ `ArchitectureResponse` with modules, dependency graph, and entrypoints
- ✅ `ModuleSummary` with statistics (fileCount, LOC, symbolCount, language)
- ✅ `DependencyEdge` with import classification and strength
- ✅ `Entrypoint` with kind detection (main, cli, server, test)
- ✅ `ExternalDependency` tracking (filtered by default)
- ✅ `ImportEdgeKind` enum (local-file, local-module, workspace-package, external-dependency, stdlib, unknown)

### 2. Architecture Generator (`generator.go`)
- ✅ `ArchitectureGenerator` struct with scanner, logger, limits, cache
- ✅ `GeneratorOptions` with depth, includeExternalDeps, refresh, maxFilesScanned
- ✅ `NewArchitectureGenerator()` factory with config integration
- ✅ `Generate()` orchestration method with:
  - Module detection via `modules.DetectModules()`
  - Module aggregation for statistics
  - Import scanning per module
  - Dependency graph building
  - Entrypoint detection
  - Caching with repoStateId
- ✅ `scanImportsForModules()` helper
- ✅ Cache management methods (GetCached, InvalidateCache, ClearCache)

### 3. Module Aggregation (`aggregation.go`)
- ✅ `AggregateModules()` collects statistics for all modules
- ✅ `CountFiles()` counts source files by language
- ✅ `CountLOC()` counts total lines of code
- ✅ `isSourceFile()` language-aware file filtering
- ✅ `countFileLines()` line counting utility
- ✅ `shouldIgnoreDir()` respects ignore patterns and hidden directories

### 4. Dependency Graph (`graph.go`)
- ✅ `BuildDependencyGraph()` creates module-to-module edges
- ✅ `classifyImport()` resolves imports to target modules
- ✅ `isStdlibImport()` detects standard library imports per language
- ✅ `findModuleForPath()` finds containing module for a path
- ✅ `extractPackageName()` handles scoped and regular packages
- ✅ `FilterExternalDeps()` removes external dependencies from graph
- ✅ `ComputeStrength()` calculates edge weight from import count
- ✅ Edge aggregation with strength counting

### 5. Entrypoint Detection (`entrypoints.go`)
- ✅ `DetectEntrypoints()` finds entry points in all modules
- ✅ `detectByFilename()` pattern-based detection for:
  - Go: main.go, cmd/*/main.go
  - TypeScript: main.ts, index.ts, cli.ts, server.ts
  - Dart: main.dart, lib/main.dart, bin/*.dart
  - Python: __main__.py, main.py, cli.py
  - Rust: src/main.rs, src/bin/*.rs
  - Java: src/main/java/**/Main.java
- ✅ `detectFromManifest()` manifest-based detection for:
  - package.json (main, bin fields)
  - pubspec.yaml (convention-based)
  - Cargo.toml (binary targets)
- ✅ Test entrypoint detection (*_test.go, *.test.ts, etc.)
- ✅ `inferEntrypointKind()` classifies entrypoint type

### 6. External Dependencies (`external.go`)
- ✅ `GetExternalDeps()` extracts external dependencies per module
- ✅ `getExternalDepsFromPackageJSON()` parses npm dependencies
- ✅ `getExternalDepsFromPubspec()` parses Dart pub dependencies
- ✅ `getExternalDepsFromGoMod()` parses Go module dependencies
- ✅ `getExternalDepsFromCargoToml()` parses Rust crate dependencies
- ✅ `getExternalDepsFromPyproject()` parses Python package dependencies
- ✅ Version extraction from manifest files

### 7. Caching (`cache.go`)
- ✅ `CachedArchitecture` with response, repoStateId, computedAt
- ✅ `ArchitectureCache` with thread-safe operations
- ✅ `Get()` retrieves cached architecture
- ✅ `Set()` stores architecture with timestamp
- ✅ `Invalidate()` removes specific cached entry
- ✅ `Clear()` removes all cached entries
- ✅ `Size()` returns cache entry count

### 8. Performance Limits (`limits.go`)
- ✅ `ArchitectureLimits` struct with configurable limits
- ✅ `DefaultLimits()` with sensible defaults:
  - MaxFilesScanned: 5000
  - MaxModules: 100
  - ScanTimeout: 30s
- ✅ `checkLimits()` validates file count
- ✅ `checkModuleCount()` validates module count
- ✅ Integration with config.BackendLimits and config.Budget

### 9. Testing (`generator_test.go`)
- ✅ `TestArchitectureGenerator` validates end-to-end generation
- ✅ `TestArchitectureCache` tests caching functionality
- ✅ `TestFilterExternalDeps` tests dependency filtering
- ✅ `TestComputeStrength` tests edge strength computation
- ✅ Uses temporary directory for isolated testing

### 10. Documentation
- ✅ `README.md` with comprehensive usage guide
- ✅ `example_integration.go` with integration examples
- ✅ `IMPLEMENTATION.md` (this file) with implementation checklist

## Integration Points

### Module Detection
- Uses `internal/modules.DetectModules()` for cascading resolution
- Respects config.Modules (detection, roots, ignore)
- Supports manifest-based, convention-based, and fallback detection

### Import Scanning
- Uses `internal/modules.ImportScanner` for import extraction
- Language-specific regex patterns for TS, JS, Dart, Go, Python, Rust, Java, Kotlin
- Respects config.ImportScan (maxFileSizeBytes, scanTimeoutMs)

### Configuration
- Reads from `internal/config.Config`:
  - modules.roots, modules.ignore
  - importScan.enabled, importScan.maxFileSizeBytes
  - backendLimits.maxFilesScanned
  - budget.maxModules

### Logging
- Uses `internal/logging.Logger` for structured logging
- Logs at appropriate levels (debug, info, warn, error)
- Includes contextual fields (moduleId, fileCount, duration, etc.)

## Design Decisions

### 1. External Dependencies (Default: Excluded)
Per Section 16.5, external dependencies are **excluded by default** (`includeExternalDeps=false`). They're classified internally during import scanning but filtered from the output dependency graph. This keeps the architecture view focused on internal structure.

### 2. In-Memory Caching
Architecture views are cached in memory with `repoStateId` as the key. This provides fast lookups without database overhead. For persistent caching, consider extending to use SQLite storage.

### 3. Module-Level Granularity
Dependencies are aggregated at the module level, not the file level. This provides a high-level view suitable for architecture visualization while keeping the response size manageable.

### 4. Import Classification
Import paths are classified using a multi-stage approach:
1. Stdlib detection (language-specific patterns)
2. Relative path resolution (./,../)
3. Workspace package matching (module names)
4. Default to external dependency

### 5. Entrypoint Patterns
Uses both filename patterns and manifest inspection to detect entrypoints. This hybrid approach works across different project structures and conventions.

### 6. Performance Targets
Adheres to Section 15.3 targets:
- Architecture (1000 files) < 30s
- Import scan (1000 files) < 10s

Limits are configurable and enforced during generation.

## DoD Checklist

- ✅ `types.go` implements ArchitectureResponse per Section 16.5
- ✅ `generator.go` orchestrates architecture generation
- ✅ `aggregation.go` computes module statistics (fileCount, LOC)
- ✅ `graph.go` builds dependency graph with classification
- ✅ `entrypoints.go` detects entry points via patterns and manifests
- ✅ `external.go` extracts external dependencies (filtered by default)
- ✅ `cache.go` provides caching with repoStateId
- ✅ `limits.go` enforces performance limits
- ✅ Integration with internal/modules, internal/config, internal/logging
- ✅ Tests validate core functionality
- ✅ Documentation complete (README, examples, this file)
- ✅ Package builds without errors

## Usage Example

```go
import (
    "context"
    "github.com/ckb/ckb/internal/architecture"
    "github.com/ckb/ckb/internal/config"
    "github.com/ckb/ckb/internal/logging"
    "github.com/ckb/ckb/internal/modules"
)

// Setup
cfg := config.DefaultConfig()
logger := logging.NewLogger(logging.Config{
    Format: logging.HumanFormat,
    Level:  logging.InfoLevel,
})
importScanner := modules.NewImportScanner(&cfg.ImportScan, logger)

generator := architecture.NewArchitectureGenerator(
    "/path/to/repo",
    cfg,
    importScanner,
    logger,
)

// Generate
ctx := context.Background()
opts := architecture.DefaultGeneratorOptions()
response, err := generator.Generate(ctx, "state-abc123", opts)

// Use
fmt.Printf("Modules: %d\n", len(response.Modules))
fmt.Printf("Dependencies: %d\n", len(response.DependencyGraph))
fmt.Printf("Entrypoints: %d\n", len(response.Entrypoints))
```

## Next Steps

To use this in a `getArchitecture` MCP handler:

1. Wire up the generator in your MCP server initialization
2. Create an MCP handler that calls `generator.Generate()`
3. Map request parameters to `GeneratorOptions`
4. Return the `ArchitectureResponse` as JSON

See `example_integration.go` for detailed integration examples.

## Files Created

```
internal/architecture/
├── types.go                   # Core types (ArchitectureResponse, etc.)
├── generator.go               # Main generator orchestration
├── aggregation.go             # Module statistics (fileCount, LOC)
├── graph.go                   # Dependency graph building
├── entrypoints.go             # Entry point detection
├── external.go                # External dependency extraction
├── cache.go                   # In-memory caching
├── limits.go                  # Performance limits
├── generator_test.go          # Tests
├── example_integration.go     # Integration examples
├── README.md                  # Usage documentation
└── IMPLEMENTATION.md          # This file
```

## Performance Characteristics

- **Module Detection**: O(n) where n = number of directories
- **File Counting**: O(m) where m = number of files
- **LOC Counting**: O(m × l) where l = average lines per file
- **Import Scanning**: O(m × l) with regex matching
- **Dependency Classification**: O(i) where i = number of imports
- **Cache Lookup**: O(1) hash map lookup

Total complexity is approximately O(m × l) dominated by file I/O and line counting.

## Known Limitations

1. **Symbol Count**: Currently always 0, needs integration with symbol backend
2. **Memory Cache**: Not persistent across restarts
3. **Binary Detection**: Simple extension-based, may miss some binaries
4. **Manifest Parsing**: Simple line-based, doesn't handle all edge cases
5. **Cyclic Dependencies**: Not explicitly detected or flagged

## Future Enhancements

1. Integrate with symbol backend for accurate symbolCount
2. Add persistent caching to SQLite
3. Detect and flag cyclic dependencies
4. Add complexity metrics (fan-in, fan-out, cyclomatic complexity)
5. Export to visualization formats (DOT, GraphML)
6. Support incremental updates on repo state changes
7. Add more sophisticated manifest parsing (full YAML/TOML/JSON parsers)
8. Add API endpoints for sub-queries (get module by ID, get dependencies for module, etc.)
