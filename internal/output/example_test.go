package output_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"ckb/internal/output"
)

// Example demonstrates basic sorting and encoding
func Example() {
	// Create some modules
	modules := []output.Module{
		{ModuleId: "mod3", Name: "third", ImpactCount: 5, SymbolCount: 10},
		{ModuleId: "mod1", Name: "first", ImpactCount: 10, SymbolCount: 5},
		{ModuleId: "mod2", Name: "second", ImpactCount: 10, SymbolCount: 10},
	}

	// Sort them deterministically
	output.SortModules(modules)

	// Create a response
	response := map[string]interface{}{
		"modules": modules,
		"metadata": map[string]interface{}{
			"total": len(modules),
			"score": 0.987654321,
		},
	}

	// Encode deterministically
	jsonBytes, _ := output.DeterministicEncode(response)
	fmt.Println(string(jsonBytes))

	// Output will be identical every time this runs
}

// ExampleSortModules demonstrates module sorting
func ExampleSortModules() {
	modules := []output.Module{
		{ModuleId: "mod3", ImpactCount: 5, SymbolCount: 10},
		{ModuleId: "mod1", ImpactCount: 10, SymbolCount: 5},
		{ModuleId: "mod2", ImpactCount: 10, SymbolCount: 10},
	}

	output.SortModules(modules)

	for _, m := range modules {
		fmt.Printf("%s: impact=%d symbols=%d\n", m.ModuleId, m.ImpactCount, m.SymbolCount)
	}

	// Output:
	// mod2: impact=10 symbols=10
	// mod1: impact=10 symbols=5
	// mod3: impact=5 symbols=10
}

// ExampleSortSymbols demonstrates symbol sorting
func ExampleSortSymbols() {
	symbols := []output.Symbol{
		{StableId: "sym3", Confidence: 0.5, RefCount: 10},
		{StableId: "sym1", Confidence: 0.9, RefCount: 5},
		{StableId: "sym2", Confidence: 0.9, RefCount: 10},
	}

	output.SortSymbols(symbols)

	for _, s := range symbols {
		fmt.Printf("%s: confidence=%.1f refs=%d\n", s.StableId, s.Confidence, s.RefCount)
	}

	// Output:
	// sym2: confidence=0.9 refs=10
	// sym1: confidence=0.9 refs=5
	// sym3: confidence=0.5 refs=10
}

// ExampleSortReferences demonstrates reference sorting
func ExampleSortReferences() {
	refs := []output.Reference{
		{FileId: "file2.go", StartLine: 10, StartColumn: 5},
		{FileId: "file1.go", StartLine: 20, StartColumn: 3},
		{FileId: "file1.go", StartLine: 10, StartColumn: 8},
	}

	output.SortReferences(refs)

	for _, r := range refs {
		fmt.Printf("%s:%d:%d\n", r.FileId, r.StartLine, r.StartColumn)
	}

	// Output:
	// file1.go:10:8
	// file1.go:20:3
	// file2.go:10:5
}

// ExampleSortImpactItems demonstrates impact item sorting
func ExampleSortImpactItems() {
	items := []output.ImpactItem{
		{StableId: "item3", Kind: "test-dependency", Confidence: 0.9},
		{StableId: "item1", Kind: "direct-caller", Confidence: 0.8},
		{StableId: "item2", Kind: "direct-caller", Confidence: 0.9},
	}

	output.SortImpactItems(items)

	for _, item := range items {
		fmt.Printf("%s: kind=%s confidence=%.1f\n", item.StableId, item.Kind, item.Confidence)
	}

	// Output:
	// item2: kind=direct-caller confidence=0.9
	// item1: kind=direct-caller confidence=0.8
	// item3: kind=test-dependency confidence=0.9
}

// ExampleRoundFloat demonstrates float rounding
func ExampleRoundFloat() {
	values := []float64{0.123456789, 0.987654321, 0.5, 1.0 / 3.0}

	for _, v := range values {
		rounded := output.RoundFloat(v)
		fmt.Printf("%.10f -> %.6f\n", v, rounded)
	}

	// Output:
	// 0.1234567890 -> 0.123457
	// 0.9876543210 -> 0.987654
	// 0.5000000000 -> 0.500000
	// 0.3333333333 -> 0.333333
}

// ExampleFormatFloat demonstrates float formatting
func ExampleFormatFloat() {
	values := []float64{0.100000, 0.123000, 42.0, 0.123456}

	for _, v := range values {
		formatted := output.FormatFloat(v)
		fmt.Printf("%.6f -> %s\n", v, formatted)
	}

	// Output:
	// 0.100000 -> 0.1
	// 0.123000 -> 0.123
	// 42.000000 -> 42
	// 0.123456 -> 0.123456
}

// ExampleDeterministicEncode demonstrates deterministic encoding
func ExampleDeterministicEncode() {
	data := map[string]interface{}{
		"zebra": "last",
		"alpha": "first",
		"beta":  "second",
		"score": 0.123456789,
	}

	// Encode twice
	json1, _ := output.DeterministicEncode(data)
	json2, _ := output.DeterministicEncode(data)

	// Results are byte-identical
	fmt.Printf("Identical: %v\n", bytes.Equal(json1, json2))
	fmt.Printf("JSON: %s\n", string(json1))

	// Output:
	// Identical: true
	// JSON: {"alpha":"first","beta":"second","score":0.123457,"zebra":"last"}
}

// ExampleCompareSnapshots demonstrates snapshot comparison
func ExampleCompareSnapshots() {
	// Two responses with different timestamps
	response1 := `{
		"data": "test",
		"provenance": {
			"cachedAt": "2024-01-01T00:00:00Z",
			"backend": "scip"
		}
	}`

	response2 := `{
		"data": "test",
		"provenance": {
			"cachedAt": "2024-01-02T00:00:00Z",
			"backend": "scip"
		}
	}`

	equal, msg := output.CompareSnapshots([]byte(response1), []byte(response2))
	fmt.Printf("Equal: %v\n", equal)
	if msg != "" {
		fmt.Printf("Message: %s\n", msg)
	}

	// Output:
	// Equal: true
}

// ExampleSnapshotEqual demonstrates high-level snapshot comparison
func ExampleSnapshotEqual() {
	type Response struct {
		Data       string                 `json:"data"`
		Provenance map[string]interface{} `json:"provenance"`
	}

	r1 := Response{
		Data: "test",
		Provenance: map[string]interface{}{
			"cachedAt": "2024-01-01T00:00:00Z",
			"backend":  "scip",
		},
	}

	r2 := Response{
		Data: "test",
		Provenance: map[string]interface{}{
			"cachedAt": "2024-01-02T00:00:00Z",
			"backend":  "scip",
		},
	}

	fmt.Printf("Equal: %v\n", output.SnapshotEqual(r1, r2))

	// Output:
	// Equal: true
}

// Example_complexResponse demonstrates a complete CKB response workflow
func Example_complexResponse() {
	// Simulate building a CKB response
	modules := []output.Module{
		{ModuleId: "mod2", Name: "auth", ImpactCount: 15, SymbolCount: 25},
		{ModuleId: "mod1", Name: "core", ImpactCount: 20, SymbolCount: 30},
	}

	symbols := []output.Symbol{
		{StableId: "sym2", Name: "login", Confidence: 0.95, RefCount: 10},
		{StableId: "sym1", Name: "authenticate", Confidence: 0.98, RefCount: 15},
	}

	warnings := []output.Warning{
		{Severity: "info", Text: "Using LSP fallback"},
		{Severity: "error", Text: "SCIP index not found"},
	}

	// Sort all arrays
	output.SortModules(modules)
	output.SortSymbols(symbols)
	output.SortWarnings(warnings)

	// Build response
	response := map[string]interface{}{
		"modules":  modules,
		"symbols":  symbols,
		"warnings": warnings,
		"provenance": map[string]interface{}{
			"backend":         "lsp",
			"cachedAt":        time.Now().Format(time.RFC3339),
			"queryDurationMs": 123,
		},
	}

	// Encode deterministically
	jsonBytes, _ := output.DeterministicEncodeIndented(response, "  ")

	// Parse to verify structure (omitting provenance for example output)
	var parsed map[string]interface{}
	json.Unmarshal(jsonBytes, &parsed)
	delete(parsed, "provenance")

	output, _ := json.MarshalIndent(parsed, "", "  ")
	fmt.Println(string(output))

	// Output will show sorted, deterministic structure
}

// Example_snapshotTesting demonstrates how to use snapshots in tests
func Example_snapshotTesting() {
	// This would typically be in a test file

	// Simulate executing a query
	executeQuery := func() ([]byte, error) {
		response := map[string]interface{}{
			"result": "success",
			"data":   "test",
			"provenance": map[string]interface{}{
				"cachedAt":        time.Now().Format(time.RFC3339),
				"queryDurationMs": 42,
				"backend":         "scip",
			},
		}
		return output.DeterministicEncode(response)
	}

	// Execute twice
	response1, _ := executeQuery()
	response2, _ := executeQuery()

	// Compare (ignoring time fields)
	equal, msg := output.CompareSnapshots(response1, response2)

	fmt.Printf("Responses are equal: %v\n", equal)
	if msg != "" {
		fmt.Printf("Difference: %s\n", msg)
	}

	// Output:
	// Responses are equal: true
}

// Example_multiFieldSort demonstrates generic multi-field sorting
func Example_multiFieldSort() {
	modules := []output.Module{
		{ModuleId: "mod3", ImpactCount: 10, SymbolCount: 5},
		{ModuleId: "mod1", ImpactCount: 10, SymbolCount: 10},
		{ModuleId: "mod2", ImpactCount: 10, SymbolCount: 10},
	}

	// Sort using multiple criteria
	criteria := []output.SortCriteria{
		{Field: "ImpactCount", Descending: true},
		{Field: "SymbolCount", Descending: true},
		{Field: "ModuleId", Descending: false},
	}

	output.MultiFieldSort(&modules, criteria)

	for _, m := range modules {
		fmt.Printf("%s: impact=%d symbols=%d\n", m.ModuleId, m.ImpactCount, m.SymbolCount)
	}

	// Output:
	// mod1: impact=10 symbols=10
	// mod2: impact=10 symbols=10
	// mod3: impact=10 symbols=5
}
