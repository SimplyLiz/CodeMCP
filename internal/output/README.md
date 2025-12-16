# Output Package

The `output` package provides deterministic sorting and encoding for CKB responses, implementing Phase 3.1 of the CKB design.

## Overview

This package ensures that identical queries produce byte-identical JSON outputs, enabling:
- Reliable caching with content-based keys
- Snapshot testing without false positives
- Reproducible results for debugging

## Components

### Types (`types.go`)

Common data structures used across CKB responses:
- `Module`: Module with impact and symbol counts
- `Symbol`: Symbol with confidence and reference count
- `Reference`: Code reference location
- `ImpactItem`: Item affected by a change
- `Drilldown`: Suggested follow-up query
- `Warning`: Warning message

### Ordering Contract (`ordering.go`)

Deterministic sorting functions per Section 12.1 of the design document:

```go
// Sort modules by: impactCount DESC → symbolCount DESC → moduleId ASC
SortModules(modules []Module)

// Sort symbols by: confidence DESC → refCount DESC → stableId ASC
SortSymbols(symbols []Symbol)

// Sort references by: fileId ASC → startLine ASC → startColumn ASC
SortReferences(refs []Reference)

// Sort impact items by: kind priority → confidence DESC → stableId ASC
SortImpactItems(items []ImpactItem)

// Sort drilldowns by: relevanceScore DESC → label ASC
SortDrilldowns(drilldowns []Drilldown)

// Sort warnings by: severity DESC → text ASC
SortWarnings(warnings []Warning)
```

All sorting functions use stable sort to maintain relative order of equal elements.

### Priorities (`priorities.go`)

Priority mappings for impact kinds and warning severities:

```go
ImpactKindPriority = map[string]int{
    "direct-caller":     1,  // Highest priority
    "transitive-caller": 2,
    "type-dependency":   3,
    "test-dependency":   4,
    "unknown":           5,  // Lowest priority
}

WarningSeverity = map[string]int{
    "error":   1,  // Highest priority
    "warning": 2,
    "info":    3,  // Lowest priority
}
```

### Float Handling (`floats.go`)

Float normalization for deterministic output:

```go
// Round to max 6 decimal places
RoundFloat(f float64) float64

// Format with no trailing zeros
FormatFloat(f float64) string

// Normalize for JSON encoding
NormalizeFloat(f float64) float64
```

### Encoding (`encoding.go`)

Deterministic JSON encoding per Section 12.2:

```go
// Produces byte-identical JSON output
DeterministicEncode(v interface{}) ([]byte, error)

// With indentation
DeterministicEncodeIndented(v interface{}, indent string) ([]byte, error)
```

Features:
- Stable key ordering (sorted alphabetically)
- Float formatting: max 6 decimal places, no trailing zeros
- Null/undefined fields omitted entirely
- Respects JSON struct tags including `omitempty`

### Snapshot Testing (`snapshot.go`)

Tools for comparing responses in tests:

```go
// Fields excluded from snapshot comparison
SnapshotExcludeFields = []string{
    "provenance.cachedAt",
    "provenance.queryDurationMs",
    "provenance.computedAt",
}

// Remove time-varying fields for comparison
NormalizeForSnapshot(data []byte) ([]byte, error)

// Compare two responses (ignoring time-varying fields)
CompareSnapshots(a, b []byte) (bool, string)

// High-level equality check
SnapshotEqual(a, b interface{}) bool
```

### Generic Sorting (`sort.go`)

Multi-field sorting for complex data structures:

```go
type SortCriteria struct {
    Field      string  // Field name to sort by
    Descending bool    // Sort direction
}

// Sort by multiple criteria
MultiFieldSort(slice interface{}, criteria []SortCriteria) error
```

Example:
```go
modules := []Module{...}
err := MultiFieldSort(&modules, []SortCriteria{
    {Field: "ImpactCount", Descending: true},
    {Field: "SymbolCount", Descending: true},
    {Field: "ModuleId", Descending: false},
})
```

## Usage Examples

### Sorting Response Data

```go
import "github.com/ckb/ckb/internal/output"

// Sort modules before returning
modules := []output.Module{
    {ModuleId: "mod1", ImpactCount: 5, SymbolCount: 10},
    {ModuleId: "mod2", ImpactCount: 10, SymbolCount: 5},
}
output.SortModules(modules)

// Sort symbols
symbols := []output.Symbol{
    {StableId: "sym1", Confidence: 0.9, RefCount: 5},
    {StableId: "sym2", Confidence: 0.7, RefCount: 10},
}
output.SortSymbols(symbols)
```

### Encoding Responses

```go
response := map[string]interface{}{
    "modules": modules,
    "symbols": symbols,
    "provenance": map[string]interface{}{
        "backend": "scip",
        "cachedAt": time.Now().Format(time.RFC3339),
    },
}

// Get deterministic JSON
jsonBytes, err := output.DeterministicEncode(response)
if err != nil {
    return err
}

// Same input will always produce identical bytes
jsonBytes2, _ := output.DeterministicEncode(response)
// bytes.Equal(jsonBytes, jsonBytes2) == true
```

### Snapshot Testing

```go
func TestQueryResponse(t *testing.T) {
    // Execute query twice
    response1 := executeQuery("getSymbol", "sym1")
    response2 := executeQuery("getSymbol", "sym1")

    // Compare ignoring time-varying fields
    json1, _ := json.Marshal(response1)
    json2, _ := json.Marshal(response2)

    equal, msg := output.CompareSnapshots(json1, json2)
    if !equal {
        t.Errorf("Responses differ: %s", msg)
    }
}
```

### Float Normalization

```go
confidence := 0.987654321

// Round to 6 decimals
rounded := output.RoundFloat(confidence)  // 0.987654

// Format without trailing zeros
formatted := output.FormatFloat(confidence)  // "0.987654"

// Use in JSON encoding
jsonBytes, _ := output.DeterministicEncode(map[string]interface{}{
    "confidence": confidence,  // Will be encoded as 0.987654
})
```

## Design Principles

### Determinism

The primary goal is **byte-identical outputs** for identical inputs:

1. **Stable Sorting**: Uses `sort.SliceStable` to maintain order of equal elements
2. **Key Ordering**: JSON object keys are always sorted alphabetically
3. **Float Precision**: Floats are rounded to exactly 6 decimal places
4. **Nil Handling**: Nil/null values are omitted entirely, not encoded as `null`

### Performance

- Sorting is O(n log n) with stable algorithm
- Encoding uses reflection for flexibility but is cached where possible
- Float rounding is a simple multiplication/division operation

### Testing

All functions have comprehensive tests covering:
- Basic functionality
- Edge cases (empty slices, nil values, zero values)
- Determinism (multiple runs produce identical results)
- Stability (equal elements maintain relative order)

Run tests with:
```bash
go test ./internal/output/...
```

Run with coverage:
```bash
go test ./internal/output/... -cover
```

## Integration

### With CKB Tools

Each CKB tool should:
1. Collect response data
2. Sort arrays using appropriate sort functions
3. Encode response using `DeterministicEncode`
4. Return encoded bytes

Example:
```go
func handleGetSymbol(symbolId string) ([]byte, error) {
    // Fetch data
    symbol, references := fetchSymbolData(symbolId)

    // Sort
    output.SortReferences(references)

    // Build response
    response := map[string]interface{}{
        "symbol": symbol,
        "references": references,
        "provenance": buildProvenance(),
    }

    // Encode deterministically
    return output.DeterministicEncode(response)
}
```

### With Cache System

The deterministic encoding enables content-based caching:

```go
// Cache key can be based on query parameters
cacheKey := fmt.Sprintf("getSymbol:%s", symbolId)

// Response bytes are always identical for same query
responseBytes, _ := handleGetSymbol(symbolId)

// Can safely cache by content hash
contentHash := sha256.Sum256(responseBytes)
cache.Set(cacheKey, responseBytes, contentHash)
```

### With Tests

Snapshot tests can reliably compare responses:

```go
func TestGetSymbol_Snapshot(t *testing.T) {
    // Load golden snapshot
    golden, _ := os.ReadFile("testdata/getSymbol_sym1.json")

    // Execute query
    response, _ := handleGetSymbol("sym1")

    // Compare ignoring time fields
    equal, msg := output.CompareSnapshots(golden, response)
    if !equal {
        t.Errorf("Response differs from snapshot: %s", msg)
    }
}
```

## Definition of Done

✅ Same query executed twice produces identical bytes
✅ Snapshot tests can ignore time-varying fields
✅ All sorting is stable and deterministic
✅ Float values are consistently rounded to 6 decimals
✅ JSON keys are always in sorted order
✅ Nil/empty values are omitted from output
✅ Comprehensive tests with 100% coverage goals
✅ Integration examples provided

## Future Enhancements

Potential improvements for future phases:

1. **Binary Encoding**: For even smaller responses, consider protobuf or msgpack
2. **Compression**: Add gzip compression option for large responses
3. **Streaming**: Support streaming JSON encoding for very large responses
4. **Validation**: Add JSON schema validation for response structures
5. **Benchmarks**: Add benchmark tests for performance optimization
