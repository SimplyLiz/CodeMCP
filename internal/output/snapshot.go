package output

import (
	"bytes"
	"encoding/json"
	"reflect"
)

// SnapshotExcludeFields lists fields to exclude when comparing responses for tests
var SnapshotExcludeFields = []string{
	"provenance.cachedAt",
	"provenance.queryDurationMs",
	"provenance.computedAt",
}

// NormalizeForSnapshot removes time-varying fields for comparison
func NormalizeForSnapshot(data []byte) ([]byte, error) {
	// Parse JSON into a map
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}

	// Remove excluded fields
	for _, field := range SnapshotExcludeFields {
		removeNestedField(parsed, field)
	}

	// Re-encode deterministically
	return DeterministicEncode(parsed)
}

// CompareSnapshots returns true if two responses are identical
// (ignoring time-varying fields)
func CompareSnapshots(a, b []byte) (bool, string) {
	// Normalize both snapshots
	normalizedA, err := NormalizeForSnapshot(a)
	if err != nil {
		return false, "failed to normalize snapshot A: " + err.Error()
	}

	normalizedB, err := NormalizeForSnapshot(b)
	if err != nil {
		return false, "failed to normalize snapshot B: " + err.Error()
	}

	// Compare byte-for-byte
	if !bytes.Equal(normalizedA, normalizedB) {
		return false, "snapshots differ"
	}

	return true, ""
}

// removeNestedField removes a nested field from a map using dot notation
// e.g., "provenance.cachedAt" removes the "cachedAt" field from the "provenance" object
func removeNestedField(data map[string]interface{}, path string) {
	parts := splitPath(path)
	if len(parts) == 0 {
		return
	}

	// Navigate to the parent object
	current := data
	for i := 0; i < len(parts)-1; i++ {
		next, ok := current[parts[i]]
		if !ok {
			return
		}

		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return
		}

		current = nextMap
	}

	// Remove the final field
	delete(current, parts[len(parts)-1])
}

// splitPath splits a dot-separated path into parts
func splitPath(path string) []string {
	if path == "" {
		return nil
	}

	parts := []string{}
	current := ""
	for _, ch := range path {
		if ch == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// SnapshotEqual compares two values for equality, ignoring time-varying fields
func SnapshotEqual(a, b interface{}) bool {
	// Convert both to JSON
	aJSON, err := json.Marshal(a)
	if err != nil {
		return false
	}

	bJSON, err := json.Marshal(b)
	if err != nil {
		return false
	}

	// Compare using CompareSnapshots
	equal, _ := CompareSnapshots(aJSON, bJSON)
	return equal
}

// DeepEqual performs a deep equality check on two values
func DeepEqual(a, b interface{}) bool {
	return reflect.DeepEqual(a, b)
}
