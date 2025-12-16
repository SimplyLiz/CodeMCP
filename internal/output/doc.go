// Package output provides deterministic sorting and encoding for CKB responses.
//
// This package implements Phase 3.1 of the CKB design document, ensuring that
// identical queries produce byte-identical JSON outputs. This enables:
//   - Reliable caching with content-based keys
//   - Snapshot testing without false positives
//   - Reproducible results for debugging
//
// # Ordering Contract
//
// All arrays are deterministically sorted according to Section 12.1 of the
// design document:
//
//   - modules: impactCount DESC → symbolCount DESC → moduleId ASC
//   - symbols: confidence DESC → refCount DESC → stableId ASC
//   - references: fileId ASC → startLine ASC → startColumn ASC
//   - impactItems: kind priority → confidence DESC → stableId ASC
//   - drilldowns: relevanceScore DESC → label ASC
//   - warnings: severity DESC → text ASC
//
// # JSON Encoding Rules
//
// The DeterministicEncode function produces byte-identical outputs by:
//
//  1. Stable key ordering: Object keys are sorted alphabetically
//  2. Float formatting: Rounded to max 6 decimal places, no trailing zeros
//  3. Null handling: Nil/undefined fields are omitted entirely
//  4. Timestamps: Only in provenance block, excluded from snapshot tests
//
// # Snapshot Testing
//
// The package provides tools for comparing responses in tests while excluding
// time-varying fields:
//
//   - provenance.cachedAt
//   - provenance.queryDurationMs
//   - provenance.computedAt
//
// # Usage Example
//
//	modules := []output.Module{
//	    {ModuleId: "mod1", ImpactCount: 10, SymbolCount: 5},
//	    {ModuleId: "mod2", ImpactCount: 5, SymbolCount: 10},
//	}
//
//	// Sort deterministically
//	output.SortModules(modules)
//
//	// Build response
//	response := map[string]interface{}{
//	    "modules": modules,
//	    "provenance": map[string]interface{}{
//	        "backend": "scip",
//	    },
//	}
//
//	// Encode deterministically
//	jsonBytes, err := output.DeterministicEncode(response)
//
//	// Same input will always produce identical bytes
//	jsonBytes2, _ := output.DeterministicEncode(response)
//	// bytes.Equal(jsonBytes, jsonBytes2) == true
//
// # Snapshot Comparison
//
//	// Two responses with different timestamps
//	response1 := executeQuery("getSymbol", "sym1")
//	response2 := executeQuery("getSymbol", "sym1")
//
//	json1, _ := json.Marshal(response1)
//	json2, _ := json.Marshal(response2)
//
//	// Compare ignoring time-varying fields
//	equal, msg := output.CompareSnapshots(json1, json2)
//	if !equal {
//	    t.Errorf("Responses differ: %s", msg)
//	}
//
// # Design Principles
//
// Determinism: The primary goal is byte-identical outputs for identical inputs.
// All sorting uses stable algorithms, JSON keys are always ordered, floats are
// consistently rounded, and nil values are omitted.
//
// Performance: Sorting is O(n log n) with stable algorithms. Encoding uses
// reflection for flexibility. Float rounding is a simple multiplication/division.
//
// Testing: All functions have comprehensive tests covering basic functionality,
// edge cases, determinism, and stability.
package output
