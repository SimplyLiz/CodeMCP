# Output Package Integration Guide

This guide shows how to integrate the `output` package with CKB tools and services.

## Quick Start

```go
import "github.com/ckb/ckb/internal/output"

// 1. Sort your data
output.SortModules(modules)
output.SortSymbols(symbols)
output.SortReferences(references)

// 2. Build response
response := map[string]interface{}{
    "modules": modules,
    "symbols": symbols,
    "references": references,
}

// 3. Encode deterministically
jsonBytes, err := output.DeterministicEncode(response)
```

## Integration Patterns

### Pattern 1: CKB Tool Handler

Each CKB tool should follow this pattern:

```go
package tools

import (
    "github.com/ckb/ckb/internal/output"
    "github.com/ckb/ckb/internal/logging"
)

func HandleGetSymbol(logger *logging.Logger, symbolId string) ([]byte, error) {
    // 1. Fetch data from backends
    symbol, err := fetchSymbol(symbolId)
    if err != nil {
        return nil, err
    }

    references, err := fetchReferences(symbolId)
    if err != nil {
        return nil, err
    }

    // 2. Sort arrays deterministically
    output.SortReferences(references)

    // 3. Build response structure
    response := map[string]interface{}{
        "symbol": symbol,
        "references": references,
        "provenance": map[string]interface{}{
            "backend": "scip",
            "cachedAt": time.Now().Format(time.RFC3339),
            "queryDurationMs": 42,
        },
    }

    // 4. Encode deterministically
    jsonBytes, err := output.DeterministicEncode(response)
    if err != nil {
        logger.Error("Failed to encode response", map[string]interface{}{
            "symbolId": symbolId,
            "error": err.Error(),
        })
        return nil, err
    }

    return jsonBytes, nil
}
```

### Pattern 2: Multi-Array Response

When returning multiple arrays:

```go
func HandleSearchSymbols(query string) ([]byte, error) {
    // Fetch data
    symbols := fetchSymbols(query)
    modules := fetchAffectedModules(query)
    drilldowns := suggestDrilldowns(query)
    warnings := collectWarnings()

    // Sort ALL arrays
    output.SortSymbols(symbols)
    output.SortModules(modules)
    output.SortDrilldowns(drilldowns)
    output.SortWarnings(warnings)

    // Build and encode
    response := map[string]interface{}{
        "symbols": symbols,
        "modules": modules,
        "drilldowns": drilldowns,
        "warnings": warnings,
        "provenance": buildProvenance(),
    }

    return output.DeterministicEncode(response)
}
```

### Pattern 3: Nested Structures

For complex nested responses:

```go
func HandleAnalyzeImpact(symbolId string) ([]byte, error) {
    // Fetch impact data
    impactItems := fetchImpactItems(symbolId)

    // Sort top-level array
    output.SortImpactItems(impactItems)

    // For each impact item with nested references, sort those too
    for i := range impactItems {
        if impactItems[i].References != nil {
            refs := impactItems[i].References.([]output.Reference)
            output.SortReferences(refs)
            impactItems[i].References = refs
        }
    }

    response := map[string]interface{}{
        "impact": impactItems,
        "provenance": buildProvenance(),
    }

    return output.DeterministicEncode(response)
}
```

### Pattern 4: Conditional Data

Handle optional fields properly:

```go
func HandleGetArchitecture(scope string) ([]byte, error) {
    modules := fetchModules(scope)
    output.SortModules(modules)

    response := map[string]interface{}{
        "modules": modules,
        "provenance": buildProvenance(),
    }

    // Add optional fields only if they have data
    if edges := fetchImportEdges(); len(edges) > 0 {
        // Sort edges if needed
        response["edges"] = edges
    }

    // Empty slices will be omitted during encoding
    return output.DeterministicEncode(response)
}
```

## Cache Integration

### Content-Based Caching

Use deterministic encoding for reliable cache keys:

```go
package cache

import (
    "crypto/sha256"
    "encoding/hex"
    "github.com/ckb/ckb/internal/output"
)

type ResponseCache struct {
    store map[string]CacheEntry
}

type CacheEntry struct {
    Data        []byte
    ContentHash string
    CachedAt    time.Time
}

func (c *ResponseCache) Get(key string) ([]byte, bool) {
    entry, exists := c.store[key]
    if !exists {
        return nil, false
    }

    // Verify content hash
    hash := sha256.Sum256(entry.Data)
    if hex.EncodeToString(hash[:]) != entry.ContentHash {
        // Cache corruption detected
        delete(c.store, key)
        return nil, false
    }

    return entry.Data, true
}

func (c *ResponseCache) Set(key string, data []byte) {
    hash := sha256.Sum256(data)
    c.store[key] = CacheEntry{
        Data:        data,
        ContentHash: hex.EncodeToString(hash[:]),
        CachedAt:    time.Now(),
    }
}

// Usage in tool
func HandleGetSymbolWithCache(cache *ResponseCache, symbolId string) ([]byte, error) {
    cacheKey := fmt.Sprintf("getSymbol:%s", symbolId)

    // Check cache
    if cached, found := cache.Get(cacheKey); found {
        return cached, nil
    }

    // Execute query
    result, err := HandleGetSymbol(symbolId)
    if err != nil {
        return nil, err
    }

    // Cache result (deterministic encoding ensures same bytes for same data)
    cache.Set(cacheKey, result)

    return result, nil
}
```

### Cache Invalidation

Detect when cache needs invalidation:

```go
func (c *ResponseCache) CompareWithSnapshot(key string, newData []byte) (bool, error) {
    cached, exists := c.Get(key)
    if !exists {
        return false, nil
    }

    // Compare using snapshot comparison (ignores timestamps)
    equal, msg := output.CompareSnapshots(cached, newData)
    if !equal {
        return false, fmt.Errorf("cache mismatch: %s", msg)
    }

    return true, nil
}
```

## Testing Integration

### Unit Tests

Test individual tool handlers:

```go
package tools

import (
    "testing"
    "github.com/ckb/ckb/internal/output"
)

func TestHandleGetSymbol(t *testing.T) {
    // Mock data
    symbolId := "sym1"

    // Execute twice
    result1, err := HandleGetSymbol(logger, symbolId)
    if err != nil {
        t.Fatalf("First call failed: %v", err)
    }

    result2, err := HandleGetSymbol(logger, symbolId)
    if err != nil {
        t.Fatalf("Second call failed: %v", err)
    }

    // Results should be identical (ignoring timestamps)
    equal, msg := output.CompareSnapshots(result1, result2)
    if !equal {
        t.Errorf("Results differ: %s", msg)
    }
}
```

### Golden File Tests

Compare against known-good snapshots:

```go
func TestHandleGetSymbol_Golden(t *testing.T) {
    tests := []struct {
        name     string
        symbolId string
        golden   string
    }{
        {
            name:     "function symbol",
            symbolId: "sym1",
            golden:   "testdata/getSymbol_function.json",
        },
        {
            name:     "class symbol",
            symbolId: "sym2",
            golden:   "testdata/getSymbol_class.json",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Load golden file
            golden, err := os.ReadFile(tt.golden)
            if err != nil {
                t.Fatalf("Failed to read golden file: %v", err)
            }

            // Execute query
            result, err := HandleGetSymbol(logger, tt.symbolId)
            if err != nil {
                t.Fatalf("Query failed: %v", err)
            }

            // Compare (ignoring timestamps)
            equal, msg := output.CompareSnapshots(golden, result)
            if !equal {
                t.Errorf("Result differs from golden: %s", msg)

                // Optionally write actual result for debugging
                os.WriteFile(tt.golden+".actual", result, 0644)
            }
        })
    }
}
```

### Integration Tests

Test end-to-end with real backends:

```go
func TestEndToEnd_GetSymbol(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // Setup real backends
    backend := setupBackend(t)
    defer backend.Close()

    // Execute query multiple times
    var results [][]byte
    for i := 0; i < 5; i++ {
        result, err := HandleGetSymbol(logger, "sym1")
        if err != nil {
            t.Fatalf("Query %d failed: %v", i, err)
        }
        results = append(results, result)
    }

    // All results should be identical (ignoring timestamps)
    for i := 1; i < len(results); i++ {
        equal, msg := output.CompareSnapshots(results[0], results[i])
        if !equal {
            t.Errorf("Result %d differs from result 0: %s", i, msg)
        }
    }
}
```

## Provenance Building

Standard provenance structure:

```go
func buildProvenance(backend string, startTime time.Time) map[string]interface{} {
    duration := time.Since(startTime).Milliseconds()

    return map[string]interface{}{
        "backend":         backend,
        "cachedAt":        time.Now().Format(time.RFC3339),
        "queryDurationMs": duration,
        "computedAt":      time.Now().Format(time.RFC3339),
    }
}

// Usage
func HandleGetSymbol(symbolId string) ([]byte, error) {
    startTime := time.Now()

    // ... fetch data ...

    response := map[string]interface{}{
        "symbol": symbol,
        "provenance": buildProvenance("scip", startTime),
    }

    return output.DeterministicEncode(response)
}
```

## Error Handling

Integrate with error system:

```go
import (
    "github.com/ckb/ckb/internal/errors"
    "github.com/ckb/ckb/internal/output"
)

func HandleGetSymbol(symbolId string) ([]byte, error) {
    symbol, err := fetchSymbol(symbolId)
    if err != nil {
        // Return structured error
        ckbErr := errors.NewCkbError(
            errors.SymbolNotFound,
            fmt.Sprintf("Symbol %s not found", symbolId),
            err,
            errors.GetSuggestedFixes(errors.SymbolNotFound),
            nil,
        )

        // Encode error response deterministically
        errResponse := map[string]interface{}{
            "error": map[string]interface{}{
                "code":           ckbErr.Code,
                "message":        ckbErr.Message,
                "suggestedFixes": ckbErr.SuggestedFixes,
            },
        }

        return output.DeterministicEncode(errResponse)
    }

    // ... normal response ...
}
```

## Performance Considerations

### Sorting Performance

For large arrays, consider:

```go
func HandleLargeResponse(query string) ([]byte, error) {
    references := fetchReferences(query) // Could be 10,000+ items

    // Sorting is O(n log n) but with stable algorithm overhead
    // For 10,000 items, this is ~133,000 comparisons
    // Should complete in milliseconds
    output.SortReferences(references)

    // If sorting becomes a bottleneck, consider:
    // 1. Backend-side sorting (push sorting to SCIP/LSP)
    // 2. Pagination to reduce array sizes
    // 3. Caching sorted results

    // ... build and encode response ...
}
```

### Encoding Performance

For large responses:

```go
func HandleLargeResponse(query string) ([]byte, error) {
    // Build response
    response := buildLargeResponse(query)

    // Encoding uses reflection, which has overhead
    // For typical responses (< 1 MB), this is fast
    // For very large responses (> 10 MB), consider:
    // 1. Streaming encoding
    // 2. Response compression
    // 3. Result pagination

    return output.DeterministicEncode(response)
}
```

## Migration Checklist

When integrating with existing tools:

- [ ] Identify all arrays in responses
- [ ] Add appropriate Sort* calls before encoding
- [ ] Replace `json.Marshal` with `output.DeterministicEncode`
- [ ] Update tests to use `output.CompareSnapshots`
- [ ] Verify provenance structure includes all required fields
- [ ] Test determinism: same query = same bytes
- [ ] Update golden files if format changed
- [ ] Add integration tests for edge cases
- [ ] Update documentation with examples
- [ ] Measure performance impact

## Common Pitfalls

### Pitfall 1: Forgetting to Sort

```go
// BAD: Arrays not sorted
response := map[string]interface{}{
    "modules": modules,  // Unsorted!
    "symbols": symbols,  // Unsorted!
}

// GOOD: All arrays sorted
output.SortModules(modules)
output.SortSymbols(symbols)
response := map[string]interface{}{
    "modules": modules,
    "symbols": symbols,
}
```

### Pitfall 2: Sorting After Building Response

```go
// BAD: Sorting after building response doesn't affect the response
response := map[string]interface{}{
    "modules": modules,
}
output.SortModules(modules)  // Too late!

// GOOD: Sort before building response
output.SortModules(modules)
response := map[string]interface{}{
    "modules": modules,
}
```

### Pitfall 3: Not Using Deterministic Encode

```go
// BAD: Using standard json.Marshal
jsonBytes, _ := json.Marshal(response)  // Non-deterministic!

// GOOD: Using deterministic encoder
jsonBytes, _ := output.DeterministicEncode(response)
```

### Pitfall 4: Comparing Raw Responses

```go
// BAD: Comparing responses with timestamps
equal := bytes.Equal(response1, response2)  // Always false due to timestamps!

// GOOD: Using snapshot comparison
equal, _ := output.CompareSnapshots(response1, response2)
```

## Support

For questions or issues:
1. Check the README.md for API documentation
2. Review example_test.go for usage examples
3. Run tests to verify integration: `go test ./internal/output/...`
4. Check PHASE-3.1-IMPLEMENTATION.md for design decisions
