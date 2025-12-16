# Compression Package

The `compression` package implements Phase 3.2 of the CKB (Codebase Knowledge Base) system, providing response compression, budget enforcement, and contextual drilldown generation.

## Overview

This package ensures that CKB responses stay within manageable token limits while providing smart follow-up suggestions when data is truncated. It implements the design specified in Sections 10 (Compression) and 11 (Drilldowns) of the CKB specification.

## Components

### 1. ResponseBudget (`budget.go`)

Defines limits for response sizes to keep token counts manageable.

```go
budget := compression.DefaultBudget()
// MaxModules: 10
// MaxSymbolsPerModule: 5
// MaxImpactItems: 20
// MaxDrilldowns: 5
// EstimatedMaxTokens: 4000

// Load from config
budget = compression.NewBudgetFromConfig(cfg)
```

### 2. BackendLimits (`limits.go`)

Defines hard limits for backend operations to prevent resource exhaustion.

```go
limits := compression.DefaultLimits()
// MaxRefsPerQuery: 10,000
// MaxSymbolsPerSearch: 1,000
// MaxFilesScanned: 5,000
// MaxFileSizeBytes: 1MB
// MaxUnionModeTimeMs: 60s
// MaxScipIndexSizeMb: 500MB (warning threshold)
```

### 3. Truncation Tracking (`truncation.go`)

Tracks why and how much data was truncated.

```go
type TruncationReason string
const (
    TruncMaxModules  // "max-modules"
    TruncMaxSymbols  // "max-symbols"
    TruncMaxItems    // "max-items"
    TruncMaxRefs     // "max-refs"
    TruncTimeout     // "timeout"
    TruncBudget      // "budget-exceeded"
)

truncInfo := compression.NewTruncationInfo(
    compression.TruncMaxModules,
    originalCount,
    returnedCount,
)
```

### 4. Compressor (`compressor.go`)

Main compression engine that applies budgets and limits.

```go
compressor := compression.NewCompressor(budget, limits)

// Compress various data types
modules, truncInfo := compressor.CompressModules(modules)
symbols, truncInfo := compressor.CompressSymbols(symbols)
items, truncInfo := compressor.CompressImpactItems(items)
refs, truncInfo := compressor.CompressReferences(refs)
```

### 5. Deduplication (`dedup.go`)

Removes duplicate entries before compression.

```go
// Deduplicate by location
refs = compression.DeduplicateReferences(refs)

// Deduplicate by stableId
symbols = compression.DeduplicateSymbols(symbols)
modules = compression.DeduplicateModules(modules)
items = compression.DeduplicateImpactItems(items)
```

### 6. Smart Drilldowns (`drilldowns.go`)

Generates contextual follow-up suggestions based on truncation and completeness.

```go
ctx := &compression.DrilldownContext{
    TruncationReason: compression.TruncMaxModules,
    Completeness: compression.CompletenessInfo{
        Score: 0.7,
        IsWorkspaceReady: false,
        IsBestEffort: true,
    },
    IndexFreshness: &compression.IndexFreshness{
        StaleAgainstHead: true,
    },
    SymbolId: "sym123",
    TopModule: topModule,
    Budget: budget,
}

drilldowns := compression.GenerateDrilldowns(ctx)
```

#### Drilldown Generation Rules

| Condition | Suggested Drilldown |
|-----------|-------------------|
| `truncation=max-modules` | "Explore top module: {name}" → `getModuleOverview {id}` |
| `truncation=max-items` | "Scope to specific module" → `findReferences {symbolId} --scope={moduleId}` |
| `completeness=best-effort-lsp` | "Check workspace status" → `getStatus` |
| `completeness.workspaceNotReady` | "Retry after warmup" → `findReferences {symbolId} --wait-for-ready` |
| `completeness.score < 0.8` | "Get maximum results (slower)" → `findReferences {symbolId} --merge=union` |
| `indexFreshness.staleAgainstHead` | "Regenerate SCIP index" → `doctor --check=scip` |

### 7. Compression Metrics (`metrics.go`)

Tracks compression statistics for observability.

```go
metrics := compression.NewMetrics(inputCount, outputCount)

// Add truncation info
metrics.AddTruncation(truncInfo)

// Query metrics
fmt.Printf("Compression ratio: %.2f\n", metrics.CompressionRatio)
fmt.Printf("Dropped: %d (%.1f%%)\n",
    metrics.TotalDropped(),
    metrics.CompressionPercentage())
```

## Usage Patterns

### Basic Compression Pipeline

```go
// 1. Create compressor
budget := compression.DefaultBudget()
limits := compression.DefaultLimits()
compressor := compression.NewCompressor(budget, limits)

// 2. Deduplicate input data
refs = compression.DeduplicateReferences(refs)

// 3. Compress to fit budget
refs, truncInfo := compressor.CompressReferences(refs)

// 4. Track metrics
metrics := compression.NewMetrics(originalCount, len(refs))
metrics.AddTruncation(truncInfo)

// 5. Generate drilldowns if truncated
if truncInfo != nil && truncInfo.WasTruncated() {
    ctx := &compression.DrilldownContext{
        TruncationReason: truncInfo.Reason,
        Budget: budget,
        // ... set other context fields
    }
    drilldowns := compression.GenerateDrilldowns(ctx)
}
```

### Integration with Config

```go
// Load budget and limits from config
cfg, _ := config.LoadConfig(repoRoot)
budget := compression.NewBudgetFromConfig(cfg)
limits := compression.NewLimitsFromConfig(cfg)

compressor := compression.NewCompressor(budget, limits)
```

### Checking SCIP Index Size

```go
limits := compression.DefaultLimits()

indexSize := getScipIndexSize() // in bytes
if limits.IsScipIndexTooLarge(indexSize) {
    log.Warn("SCIP index exceeds recommended size",
        map[string]interface{}{
            "size_mb": indexSize / (1024 * 1024),
            "max_mb": limits.MaxScipIndexSizeMb,
        })
}
```

## Design Principles

1. **Budget-First**: All compression operations respect configured budgets
2. **Context-Aware**: Drilldowns are generated based on the specific reason for truncation
3. **Transparency**: Truncation information is always tracked and returned
4. **Deduplication**: Data is deduplicated before compression to maximize information density
5. **Observability**: Comprehensive metrics track compression effectiveness

## Performance Considerations

- Deduplication uses hash maps with O(n) time complexity
- Compression is O(1) as it only takes the first N items (assumes pre-sorted input)
- Drilldown generation is O(1) with a maximum of 5 drilldowns per context

## Configuration

Budget and limits can be configured in `.ckb/config.json`:

```json
{
  "budget": {
    "maxModules": 10,
    "maxSymbolsPerModule": 5,
    "maxImpactItems": 20,
    "maxDrilldowns": 5,
    "estimatedMaxTokens": 4000
  },
  "backendLimits": {
    "maxRefsPerQuery": 10000,
    "maxFilesScanned": 5000,
    "maxUnionModeTimeMs": 60000
  }
}
```

## Testing

See `example_test.go` for comprehensive usage examples.

## Dependencies

- `internal/config`: Configuration management
- `internal/output`: Output type definitions (Module, Symbol, Reference, etc.)

## Future Enhancements

- Adaptive compression based on token estimation
- Priority-based compression (keep high-confidence items)
- Smart pagination for large result sets
- A/B testing different compression strategies
