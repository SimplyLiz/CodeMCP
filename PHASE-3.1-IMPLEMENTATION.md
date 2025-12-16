# Phase 3.1 Implementation: Deterministic Output for CKB

## Overview

Implemented Phase 3.1 of the CKB design document, creating the `internal/output/` package with deterministic sorting and encoding capabilities. This ensures that identical queries produce byte-identical JSON outputs, enabling reliable caching, snapshot testing, and reproducible results.

## Implementation Summary

### Package Structure

Created `/Users/lisa/Work/Ideas/CodeMCP/internal/output/` with the following components:

#### Core Files

1. **types.go** (1,476 bytes)
   - Common data structures: Module, Symbol, Reference, ImpactItem, Drilldown, Warning
   - All types include proper JSON tags

2. **priorities.go** (1,094 bytes)
   - ImpactKindPriority mapping (direct-caller → transitive-caller → type-dependency → test-dependency → unknown)
   - WarningSeverity mapping (error → warning → info)
   - Helper functions GetImpactKindPriority() and GetWarningSeverity()

3. **floats.go** (874 bytes)
   - RoundFloat(): Rounds to max 6 decimal places
   - FormatFloat(): Formats with no trailing zeros
   - NormalizeFloat(): Wrapper for deterministic encoding

4. **ordering.go** (3,048 bytes)
   - SortModules(): impactCount DESC → symbolCount DESC → moduleId ASC
   - SortSymbols(): confidence DESC → refCount DESC → stableId ASC
   - SortReferences(): fileId ASC → startLine ASC → startColumn ASC
   - SortImpactItems(): kind priority → confidence DESC → stableId ASC
   - SortDrilldowns(): relevanceScore DESC → label ASC
   - SortWarnings(): severity DESC → text ASC
   - All use stable sorting algorithms

5. **encoding.go** (5,549 bytes)
   - DeterministicEncode(): Produces byte-identical JSON output
   - DeterministicEncodeIndented(): With indentation support
   - normalizeValue(): Recursive normalization for all types
   - Stable key ordering (sorted alphabetically)
   - Float formatting with 6 decimal places
   - Omits nil/undefined fields entirely
   - DeterministicMap type with custom MarshalJSON

6. **snapshot.go** (2,802 bytes)
   - SnapshotExcludeFields: List of time-varying fields to ignore
   - NormalizeForSnapshot(): Removes time-varying fields
   - CompareSnapshots(): Compares two responses ignoring timestamps
   - SnapshotEqual(): High-level equality check
   - removeNestedField(): Handles dot-notation field paths
   - splitPath(): Parses field paths

7. **sort.go** (3,246 bytes)
   - SortCriteria struct: Defines multi-field sort criteria
   - MultiFieldSort(): Generic multi-field sorting using reflection
   - getFieldValue(): Extracts struct field values
   - compareValues(): Compares values of different types

#### Test Files

8. **ordering_test.go** (11,162 bytes)
   - Tests for all sorting functions
   - Edge cases: empty slices, equal values, complex sorting
   - Stability tests: Ensures equal elements maintain order
   - Determinism tests: Multiple runs produce identical results

9. **encoding_test.go** (7,100 bytes)
   - Tests for deterministic encoding
   - Float rounding tests
   - Nil/empty field handling
   - Key ordering verification
   - Consistency tests: Multiple encodings produce identical bytes
   - Complex nested structure tests

10. **floats_test.go** (4,731 bytes)
    - RoundFloat tests: Various decimal precisions
    - FormatFloat tests: Trailing zero removal
    - Edge cases: Zero, negative, very large/small numbers
    - Determinism tests: Consistent results across runs

11. **snapshot_test.go** (8,787 bytes)
    - NormalizeForSnapshot tests
    - CompareSnapshots tests with various scenarios
    - removeNestedField tests for path handling
    - splitPath tests
    - Determinism tests for normalization

12. **example_test.go** (8,569 bytes)
    - Comprehensive examples for all public APIs
    - Real-world usage scenarios
    - Complete CKB response workflow example
    - Snapshot testing example

#### Documentation

13. **doc.go** (3,134 bytes)
    - Package-level documentation
    - Usage examples
    - Design principles
    - Integration guidance

14. **README.md** (9,012 bytes)
    - Comprehensive package documentation
    - Component descriptions
    - Usage examples
    - Integration patterns
    - Testing guidance
    - Definition of Done checklist

## Key Features

### 1. Deterministic Sorting

All sorting functions implement the exact ordering contract from Section 12.1:

```go
// Modules sorted by impact, then symbols, then ID
SortModules(modules)

// Symbols sorted by confidence, then refs, then ID
SortSymbols(symbols)

// References sorted by file, line, column
SortReferences(refs)

// Impact items sorted by kind priority, confidence, ID
SortImpactItems(items)

// Drilldowns sorted by relevance, then label
SortDrilldowns(drilldowns)

// Warnings sorted by severity, then text
SortWarnings(warnings)
```

### 2. Deterministic Encoding

Ensures byte-identical JSON for identical inputs:

```go
data := map[string]interface{}{
    "modules": modules,
    "confidence": 0.987654321,  // Rounded to 0.987654
}

jsonBytes, _ := DeterministicEncode(data)
// Keys sorted, floats rounded, nil fields omitted
```

### 3. Snapshot Testing Support

Allows reliable comparison while ignoring time-varying fields:

```go
response1, _ := executeQuery("getSymbol", "sym1")
response2, _ := executeQuery("getSymbol", "sym1")

equal, _ := CompareSnapshots(response1, response2)
// true, even if timestamps differ
```

## Test Coverage

All components have comprehensive tests covering:

- ✅ Basic functionality
- ✅ Edge cases (empty slices, nil values, zero values)
- ✅ Determinism (multiple runs produce identical results)
- ✅ Stability (equal elements maintain relative order)
- ✅ Float rounding precision
- ✅ Key ordering
- ✅ Snapshot comparison

## Definition of Done

✅ **Same query twice = identical bytes**
   - DeterministicEncode produces byte-identical output for same input
   - Verified with consistency tests running encodings 10+ times

✅ **Snapshot tests can ignore time fields**
   - CompareSnapshots excludes provenance.cachedAt, queryDurationMs, computedAt
   - NormalizeForSnapshot removes time-varying fields before comparison

✅ **All sorting is stable and deterministic**
   - Uses sort.SliceStable throughout
   - Stability tests verify equal elements maintain order
   - Determinism tests verify identical results across runs

✅ **Ordering contract per Section 12.1**
   - Modules: impactCount DESC → symbolCount DESC → moduleId ASC
   - Symbols: confidence DESC → refCount DESC → stableId ASC
   - References: fileId ASC → startLine ASC → startColumn ASC
   - ImpactItems: kind priority → confidence DESC → stableId ASC
   - Drilldowns: relevanceScore DESC → label ASC
   - Warnings: severity DESC → text ASC

✅ **JSON encoding rules per Section 12.2**
   - Stable key ordering (sorted alphabetically)
   - Float formatting: max 6 decimal places, no trailing zeros
   - Null/undefined fields omitted entirely
   - Timestamps only in provenance block

✅ **Impact kind priorities implemented**
   - direct-caller: 1 (highest)
   - transitive-caller: 2
   - type-dependency: 3
   - test-dependency: 4
   - unknown: 5 (lowest)

✅ **Warning severity priorities implemented**
   - error: 1 (highest)
   - warning: 2
   - info: 3 (lowest)

✅ **Generic sorting utilities**
   - MultiFieldSort for complex sorting scenarios
   - Reflection-based field access
   - Support for multiple sort criteria

✅ **Comprehensive documentation**
   - Package-level doc.go
   - Detailed README with examples
   - Example tests demonstrating all features
   - Integration guidance

## Integration Points

### With CKB Tools

Tools should use the package as follows:

```go
import "github.com/ckb/ckb/internal/output"

func handleGetSymbol(symbolId string) ([]byte, error) {
    // 1. Fetch data
    symbol, references := fetchSymbolData(symbolId)

    // 2. Sort arrays
    output.SortReferences(references)

    // 3. Build response
    response := map[string]interface{}{
        "symbol": symbol,
        "references": references,
        "provenance": buildProvenance(),
    }

    // 4. Encode deterministically
    return output.DeterministicEncode(response)
}
```

### With Cache System

Enables content-based caching:

```go
// Response bytes are always identical for same query
responseBytes, _ := handleGetSymbol(symbolId)

// Can safely cache by content hash
contentHash := sha256.Sum256(responseBytes)
cache.Set(cacheKey, responseBytes, contentHash)
```

### With Tests

Enables reliable snapshot testing:

```go
func TestGetSymbol_Snapshot(t *testing.T) {
    golden, _ := os.ReadFile("testdata/getSymbol_sym1.json")
    response, _ := handleGetSymbol("sym1")

    equal, msg := output.CompareSnapshots(golden, response)
    if !equal {
        t.Errorf("Response differs: %s", msg)
    }
}
```

## Usage Examples

### Basic Sorting

```go
modules := []output.Module{
    {ModuleId: "mod2", ImpactCount: 5, SymbolCount: 10},
    {ModuleId: "mod1", ImpactCount: 10, SymbolCount: 5},
}

output.SortModules(modules)
// Result: mod1 (impact=10), mod2 (impact=5)
```

### Deterministic Encoding

```go
response := map[string]interface{}{
    "zebra": "last",
    "alpha": "first",
    "score": 0.123456789,
}

jsonBytes, _ := output.DeterministicEncode(response)
// {"alpha":"first","score":0.123457,"zebra":"last"}
```

### Snapshot Testing

```go
response1 := executeQuery("getSymbol", "sym1")
response2 := executeQuery("getSymbol", "sym1")

equal, _ := output.CompareSnapshots(response1, response2)
// true, timestamps ignored
```

## File Sizes

Total implementation: ~60 KB across 15 files
- Core implementation: ~22 KB (7 files)
- Tests: ~32 KB (4 test files + 1 example file)
- Documentation: ~12 KB (doc.go + README)

## Dependencies

The package uses only standard library:
- `encoding/json`: JSON encoding/decoding
- `reflect`: Generic type handling
- `sort`: Stable sorting algorithms
- `math`: Float rounding
- `strconv`: String formatting
- `bytes`: Byte comparison
- `testing`: Test framework

No external dependencies required.

## Performance Characteristics

- **Sorting**: O(n log n) with stable algorithm overhead
- **Encoding**: O(n) with reflection overhead for normalization
- **Float rounding**: O(1) multiplication/division
- **Snapshot comparison**: O(n) for JSON parsing and normalization

Performance is suitable for typical CKB responses (100s-1000s of items).

## Future Enhancements

Potential improvements for future phases:

1. **Binary Encoding**: Consider protobuf or msgpack for smaller responses
2. **Compression**: Add gzip compression option for large responses
3. **Streaming**: Support streaming JSON for very large responses
4. **Validation**: Add JSON schema validation for response structures
5. **Benchmarks**: Add benchmark tests for performance optimization
6. **Caching**: Pre-compute sorted order for frequently accessed data

## Testing

Run all tests:
```bash
go test ./internal/output/...
```

Run with coverage:
```bash
go test ./internal/output/... -cover
```

Run examples:
```bash
go test ./internal/output/... -run Example
```

Run with verbose output:
```bash
go test ./internal/output/... -v
```

## Files Created

```
/Users/lisa/Work/Ideas/CodeMCP/internal/output/
├── doc.go                 # Package documentation
├── types.go               # Common data structures
├── priorities.go          # Impact/warning priority mappings
├── floats.go              # Float normalization
├── ordering.go            # Deterministic sorting functions
├── encoding.go            # Deterministic JSON encoding
├── snapshot.go            # Snapshot testing utilities
├── sort.go                # Generic multi-field sorting
├── ordering_test.go       # Sorting tests
├── encoding_test.go       # Encoding tests
├── floats_test.go         # Float handling tests
├── snapshot_test.go       # Snapshot testing tests
├── example_test.go        # Usage examples
└── README.md              # Comprehensive documentation
```

## Conclusion

Phase 3.1 is complete and ready for integration. The `internal/output/` package provides all required functionality for deterministic sorting and encoding per the design document, with comprehensive tests and documentation.

The implementation ensures that:
1. Same query twice produces identical bytes
2. Snapshot tests can ignore time-varying fields
3. All sorting is stable and deterministic
4. Float values are consistently rounded to 6 decimals
5. JSON keys are always in sorted order
6. Nil/empty values are omitted from output

Next steps: Integrate with CKB tools (getSymbol, searchSymbols, findReferences, getArchitecture, analyzeImpact) to use deterministic encoding for all responses.
