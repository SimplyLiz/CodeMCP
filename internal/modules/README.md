# Modules Package

The `modules` package implements Phase 1.3 of CKB: Module Detection + Import Classification.

## Overview

This package provides functionality to:
1. **Detect modules** in a repository using cascading resolution strategies
2. **Scan imports** from source files using language-specific patterns
3. **Classify import edges** to understand dependency relationships

## Architecture

### Core Components

1. **Module Detection** (`detection.go`)
   - Manifest-based detection (package.json, go.mod, pubspec.yaml, etc.)
   - Language convention detection (src/, lib/, internal/, pkg/)
   - Directory fallback for unstructured repos
   - Explicit configuration support

2. **Import Scanning** (`import_scan.go`)
   - Built-in patterns for TypeScript, JavaScript, Dart, Go, Python, Rust, Java, Kotlin
   - File size limits and timeout protection
   - Binary file detection and skipping
   - Respects ignore directories

3. **Import Classification** (`classify.go`)
   - Classifies imports into: local-file, local-module, workspace-package, external-dependency, stdlib, unknown
   - Language-specific resolution logic
   - Declared dependency detection from manifests
   - Confidence scoring

4. **Scanner** (`scanner.go`)
   - High-level API for module and import scanning
   - Batch processing support
   - Statistics and filtering utilities

## Module Detection Resolution Order

Per Section 5.1 of the design document, module detection follows this cascading order:

1. **Explicit config** - `.ckb/config.json` `modules.roots`
2. **Manifest roots** - Detect:
   - `package.json` (Node/TypeScript)
   - `pubspec.yaml` (Dart)
   - `go.mod` (Go)
   - `Cargo.toml` (Rust)
   - `pyproject.toml`, `setup.py` (Python)
   - `pom.xml`, `build.gradle` (Java)
3. **Language conventions** - `src/`, `lib/`, `internal/`, `pkg/`
4. **Directory fallback** - Top-level directories

## Import Edge Classification

Per Section 5.2 of the design document, imports are classified as:

### Classification Types

| Type | Description | Example |
|------|-------------|---------|
| `local-file` | Relative import to same module | `./utils`, `../helper` |
| `local-module` | Import to different module in repo | Resolved path within repo |
| `workspace-package` | Monorepo sibling package | `@my-org/shared-lib` |
| `external-dependency` | External package | `react`, `express` |
| `stdlib` | Standard library | `dart:core`, `node:fs`, Go stdlib |
| `unknown` | Cannot be classified | Ambiguous imports |

### Classification Logic

1. Check for relative paths (`./`, `../`) → `local-file`
2. Check for stdlib patterns → `stdlib`
3. Check workspace packages → `workspace-package`
4. Try to resolve within repo → `local-module`
5. Check declared dependencies → `external-dependency`
6. Default → `unknown`

## Data Structures

### Module

```go
type Module struct {
    ID           string  // Unique identifier
    Name         string  // Human-readable name
    RootPath     string  // Repo-relative path
    ManifestType string  // "package.json", "go.mod", etc.
    Language     string  // Detected language
    DetectedAt   string  // ISO8601 timestamp
    StateId      string  // RepoStateId at detection time
}
```

### ImportEdge

```go
type ImportEdge struct {
    From       string         // FileId (repo-relative path)
    To         string         // Path or package name
    Kind       ImportEdgeKind // Classification
    Confidence float64        // 0-1 confidence score
    RawImport  string         // Original import string
    Line       int            // Line number (optional)
}
```

### ModuleContext

```go
type ModuleContext struct {
    RepoRoot             string
    Modules              []*Module
    Language             string
    WorkspacePackages    map[string]bool
    DeclaredDependencies map[string]bool
}
```

## Usage

### Basic Module Detection

```go
scanner := modules.NewScanner(cfg, logger)
modules, err := scanner.ScanModules(repoRoot, stateId)
```

### Import Scanning

```go
edges, err := scanner.ScanImports(repoRoot, module, allModules)
stats := modules.GetImportStatistics(edges)
```

### Import Classification

```go
ctx := modules.BuildModuleContext(repoRoot, modules, language)
classifier := modules.NewImportClassifier(ctx)
kind := classifier.ClassifyImport(importStr, fromFile)
```

### Filtering and Grouping

```go
// Filter by kind
localImports := modules.FilterImportsByKind(edges, modules.LocalFile)

// Group by kind
grouped := modules.GroupImportsByKind(edges)

// Get statistics
stats := modules.GetImportStatistics(edges)
```

## Language Support

### Supported Languages

- **TypeScript/JavaScript** - .ts, .tsx, .js, .jsx, .mjs, .cjs
- **Dart** - .dart
- **Go** - .go
- **Python** - .py, .pyx
- **Rust** - .rs
- **Java** - .java
- **Kotlin** - .kt, .kts

### Import Patterns

Each language has built-in regex patterns to extract imports:

- **TypeScript/JS**: `import from`, `export from`, `require()`, `import()`
- **Dart**: `import`, `export`
- **Go**: `import` statements
- **Python**: `from ... import`, `import`
- **Rust**: `use`, `extern crate`
- **Java**: `import`, `import static`
- **Kotlin**: `import`

### Standard Library Detection

The classifier includes stdlib detection for:
- **Dart**: `dart:*` packages
- **Node.js**: Builtin modules + `node:*` prefix
- **Go**: Stdlib packages (no dots in first component)
- **Python**: Common stdlib modules
- **Rust**: `std`, `core`, `alloc`, `proc_macro`, `test`

## Configuration

### Import Scan Policy (Section 15.1)

```json
{
  "importScan": {
    "enabled": true,
    "maxFileSizeBytes": 1000000,
    "scanTimeoutMs": 30000,
    "skipBinary": true,
    "customPatterns": {}
  }
}
```

### Module Detection

```json
{
  "modules": {
    "detection": "auto",
    "roots": [],
    "ignore": ["node_modules", "build", ".dart_tool", "vendor"]
  }
}
```

## Performance Considerations

- File size limit: 1MB default (configurable)
- Scan timeout: 30s default (configurable)
- Max files per module: 10,000
- Binary files are automatically skipped
- Respects ignore directories

## Integration Points

This package is used by:
- `getArchitecture` tool - Module dependency graph
- `analyzeImpact` tool - Import edge analysis
- Future: Dependency index for caching

## Testing

See `example_usage.go` for comprehensive usage examples covering:
- Basic module detection
- Import scanning
- Manual classification
- Complete workflows
- Custom detection strategies

## Future Enhancements

Planned for later phases:
- Custom pattern support via config
- Performance optimizations for large monorepos
- Incremental scanning
- Cache integration
- Multi-language project detection improvements
