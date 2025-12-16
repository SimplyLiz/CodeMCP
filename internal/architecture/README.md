# Architecture Generator Package

This package implements Phase 3.4 of the CKB (Codebase Knowledge Backend) design, providing comprehensive architecture views of repositories.

## Overview

The Architecture Generator analyzes a repository's structure and generates:
- **Module Summaries**: Aggregated statistics for each detected module (file count, LOC, language, etc.)
- **Dependency Graph**: Inter-module dependencies with classified edge types and reference counts
- **Entrypoints**: Detected entry points (main files, CLIs, servers, tests)

## Architecture Response Structure

```go
type ArchitectureResponse struct {
    Modules         []ModuleSummary   // Module-level statistics
    DependencyGraph []DependencyEdge  // Module dependencies
    Entrypoints     []Entrypoint      // Entry points per module
}
```

### Module Summary
Each module includes:
- `moduleId`: Unique identifier
- `name`: Human-readable name (from manifest or directory)
- `rootPath`: Repo-relative path
- `language`: Detected primary language
- `fileCount`: Number of source files
- `symbolCount`: Count of symbols (TODO: integrate with symbol backend)
- `loc`: Total lines of code

### Dependency Edge
Dependencies are classified per Section 5.2:
- `local-file`: Relative import to same module
- `local-module`: Import to another module in workspace
- `workspace-package`: Import to sibling package in monorepo
- `external-dependency`: Import to external package (npm/pub/cargo/etc)
- `stdlib`: Standard library import
- `unknown`: Unclassified import

Each edge includes:
- `from`, `to`: Module IDs
- `kind`: Import classification
- `strength`: Reference count (number of import statements)

### Entrypoint
Detected entry points with:
- `fileId`: Repo-relative path
- `name`: File name
- `kind`: "main", "cli", "server", "test"
- `moduleId`: Parent module ID

## Usage

```go
import (
    "context"
    "github.com/ckb/ckb/internal/architecture"
    "github.com/ckb/ckb/internal/config"
    "github.com/ckb/ckb/internal/logging"
    "github.com/ckb/ckb/internal/modules"
)

// Create generator
cfg := config.DefaultConfig()
logger := logging.NewLogger(logging.Config{
    Format: logging.HumanFormat,
    Level:  logging.InfoLevel,
})
importScanner := modules.NewImportScanner(&cfg.ImportScan, logger)

generator := architecture.NewArchitectureGenerator(
    repoRoot,
    cfg,
    importScanner,
    logger,
)

// Generate architecture view
ctx := context.Background()
opts := architecture.DefaultGeneratorOptions()
opts.IncludeExternalDeps = false  // Default: exclude external deps
opts.Depth = 2                     // Dependency analysis depth

response, err := generator.Generate(ctx, repoStateId, opts)
if err != nil {
    log.Fatal(err)
}

// Use the response
fmt.Printf("Found %d modules\n", len(response.Modules))
fmt.Printf("Dependency edges: %d\n", len(response.DependencyGraph))
fmt.Printf("Entry points: %d\n", len(response.Entrypoints))
```

## Generator Options

```go
type GeneratorOptions struct {
    Depth               int  // Depth of dependency analysis (default 2)
    IncludeExternalDeps bool // Include external dependencies (default false)
    Refresh             bool // Force refresh, bypass cache
    MaxFilesScanned     int  // Override max files limit
}
```

### External Dependencies

By default, external dependencies are **excluded** from the dependency graph (`includeExternalDeps=false`). They are classified internally during import scanning but filtered from the output.

To include external dependencies in the graph:
```go
opts.IncludeExternalDeps = true
```

## Performance Limits

Per Section 15.3 performance targets:
- Architecture generation (1000 files): < 30s
- Import scanning (1000 files): < 10s

Configurable limits:
- `MaxFilesScanned`: 5000 (default)
- `MaxModules`: 100 (from budget config)
- `ScanTimeout`: 30s

## Caching

Architecture views are cached with the full `repoStateId`. Cache operations:

```go
// Check cache
if cached, found := generator.GetCached(repoStateId); found {
    return cached.Response
}

// Invalidate specific state
generator.InvalidateCache(repoStateId)

// Clear all cache
generator.ClearCache()

// Force refresh (bypass cache)
opts := architecture.DefaultGeneratorOptions()
opts.Refresh = true
response, _ := generator.Generate(ctx, repoStateId, opts)
```

## Entrypoint Detection

Detects entry points using multiple strategies:

### Filename Patterns
- **Go**: `main.go`, `cmd/*/main.go`
- **TypeScript**: `main.ts`, `index.ts`, `cli.ts`, `server.ts`
- **Dart**: `main.dart`, `lib/main.dart`, `bin/*.dart`
- **Python**: `__main__.py`, `main.py`, `cli.py`
- **Rust**: `src/main.rs`, `src/bin/*.rs`

### Manifest-based
- **package.json**: `main` and `bin` fields
- **pubspec.yaml**: Convention-based detection
- **Cargo.toml**: Binary targets

### Test Entrypoints
- **Go**: `*_test.go`
- **TypeScript**: `*.test.ts`, `*.spec.ts`
- **Dart**: `*_test.dart`
- **Python**: `test_*.py`, `*_test.py`
- **Rust**: `tests/*.rs`

## Module Aggregation

For each module, the generator computes:

### File Count
Counts all source files matching the module's language extensions, excluding:
- Hidden directories (`.git`, `.cache`, etc.)
- Ignored directories (from config)
- Binary files

### Lines of Code (LOC)
Counts all lines in source files, including:
- Code
- Comments
- Blank lines

This provides a rough estimate of module size. For more precise metrics, integrate with a language-specific analyzer.

## Dependency Classification

Import paths are resolved and classified:

1. **Relative imports** (`./`, `../`): Resolved to target module
2. **Workspace packages**: Matched against module names
3. **Standard library**: Language-specific patterns
4. **External dependencies**: Default classification

Example classifications:

```typescript
import './utils'              // local-file (same module)
import '../other-module'      // local-module (different module)
import '@myorg/shared'        // workspace-package (monorepo)
import 'lodash'               // external-dependency
import 'node:fs'              // stdlib
```

## File Structure

- `types.go`: Core data types (ArchitectureResponse, ModuleSummary, DependencyEdge, Entrypoint)
- `generator.go`: Main generator orchestration
- `aggregation.go`: Module statistics computation (file count, LOC)
- `graph.go`: Dependency graph building and import classification
- `entrypoints.go`: Entry point detection (filename patterns, manifest-based)
- `external.go`: External dependency extraction from manifests
- `cache.go`: In-memory architecture caching
- `limits.go`: Performance limits and resource constraints

## Integration Points

### Module Detection
Uses `internal/modules.DetectModules()` for module discovery with cascading resolution:
1. Explicit config (`modules.roots`)
2. Manifest-based (package.json, go.mod, etc.)
3. Language conventions (src/, lib/, internal/, pkg/)
4. Fallback to top-level directories

### Import Scanning
Uses `internal/modules.ImportScanner` for extracting imports with:
- Language-specific regex patterns
- File size limits
- Scan timeouts
- Binary file filtering

### Configuration
Respects configuration from `internal/config`:
- `modules.detection`, `modules.roots`, `modules.ignore`
- `importScan.enabled`, `importScan.maxFileSizeBytes`
- `backendLimits.maxFilesScanned`
- `budget.maxModules`

## Future Enhancements

1. **Symbol Integration**: Populate `symbolCount` from symbol backend
2. **Cyclic Dependency Detection**: Flag circular dependencies
3. **Complexity Metrics**: Cyclomatic complexity, fan-in/fan-out
4. **Visualization**: Export to DOT/GraphML for rendering
5. **Incremental Updates**: Delta computation on repo state changes
6. **Persistent Cache**: Store in SQLite instead of memory

## References

- Section 16.5: Architecture Response Structure
- Section 5.2: Import Classification
- Section 15.3: Performance Targets
- Section 9: Cache Tiers
