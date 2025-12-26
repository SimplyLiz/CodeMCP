package testutil

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Normalizer defines the interface for normalizing golden test data.
type Normalizer interface {
	// Normalize processes the data for stable comparison.
	Normalize(t *testing.T, fixture *FixtureContext, data any) any
}

// DefaultNormalizer provides standard normalization for most test cases.
type DefaultNormalizer struct{}

// Normalize applies all normalization rules for stable golden comparison.
// This is called before both compare AND update operations.
func (n *DefaultNormalizer) Normalize(t *testing.T, fixture *FixtureContext, data any) any {
	t.Helper()

	// Deep copy via JSON round-trip to avoid modifying original
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Failed to marshal data for normalization: %v", err)
	}

	var normalized any
	if err := json.Unmarshal(jsonBytes, &normalized); err != nil {
		t.Fatalf("Failed to unmarshal data for normalization: %v", err)
	}

	// Apply normalizations recursively
	normalized = n.normalizeValue(normalized, fixture.Root)

	return normalized
}

func (n *DefaultNormalizer) normalizeValue(v any, fixtureRoot string) any {
	switch val := v.(type) {
	case map[string]any:
		return n.normalizeMap(val, fixtureRoot)
	case []any:
		return n.normalizeSlice(val, fixtureRoot)
	case string:
		return n.normalizeString(val, fixtureRoot)
	default:
		return v
	}
}

func (n *DefaultNormalizer) normalizeMap(m map[string]any, fixtureRoot string) map[string]any {
	result := make(map[string]any)

	for k, v := range m {
		// Skip volatile fields entirely
		if n.isVolatileField(k) {
			continue
		}

		result[k] = n.normalizeValue(v, fixtureRoot)
	}

	return result
}

func (n *DefaultNormalizer) normalizeSlice(s []any, fixtureRoot string) []any {
	result := make([]any, len(s))
	for i, v := range s {
		result[i] = n.normalizeValue(v, fixtureRoot)
	}

	// Sort slices of maps by a stable key
	if len(result) > 0 {
		if _, ok := result[0].(map[string]any); ok {
			sort.SliceStable(result, func(i, j int) bool {
				mi, oki := result[i].(map[string]any)
				mj, okj := result[j].(map[string]any)
				if !oki || !okj {
					return false
				}
				return n.compareMapKeys(mi, mj)
			})
		}
	}

	return result
}

func (n *DefaultNormalizer) normalizeString(s, fixtureRoot string) string {
	// Replace absolute paths with relative paths
	s = n.normalizePath(s, fixtureRoot)

	// Normalize path separators to forward slashes
	s = strings.ReplaceAll(s, "\\", "/")

	return s
}

func (n *DefaultNormalizer) normalizePath(s, fixtureRoot string) string {
	// Replace fixture root path with placeholder
	if fixtureRoot != "" {
		s = strings.ReplaceAll(s, fixtureRoot, "<fixture>")
	}

	// Replace common temp directory patterns
	tempPatterns := []string{
		"/tmp/",
		"/var/folders/",
		"C:\\Users\\",
		"C:/Users/",
	}
	for _, pattern := range tempPatterns {
		if strings.Contains(s, pattern) {
			// Extract just the filename
			s = regexp.MustCompile(`(?:/tmp/|/var/folders/[^/]+/[^/]+/[^/]+/|C:\\Users\\[^\\]+\\|C:/Users/[^/]+/)[^/\\]+`).ReplaceAllString(s, "<tempdir>")
		}
	}

	return s
}

func (n *DefaultNormalizer) isVolatileField(name string) bool {
	volatileFields := map[string]bool{
		"timestamp":    true,
		"generatedAt":  true,
		"duration":     true,
		"elapsed":      true,
		"createdAt":    true,
		"updatedAt":    true,
		"lastModified": true,
		"buildTime":    true,
	}
	return volatileFields[name]
}

func (n *DefaultNormalizer) compareMapKeys(a, b map[string]any) bool {
	// Sort by these keys in order of priority
	keyPriority := []string{"kind", "name", "symbol", "file", "path", "startLine", "line", "id"}

	for _, key := range keyPriority {
		va, oka := a[key]
		vb, okb := b[key]

		if oka && okb {
			cmp := n.compareValues(va, vb)
			if cmp != 0 {
				return cmp < 0
			}
		} else if oka {
			return true
		} else if okb {
			return false
		}
	}

	return false
}

func (n *DefaultNormalizer) compareValues(a, b any) int {
	// Compare strings
	if sa, ok := a.(string); ok {
		if sb, ok := b.(string); ok {
			return strings.Compare(sa, sb)
		}
	}

	// Compare numbers
	if na, ok := a.(float64); ok {
		if nb, ok := b.(float64); ok {
			if na < nb {
				return -1
			} else if na > nb {
				return 1
			}
			return 0
		}
	}

	// Fallback to string comparison of JSON representation
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return strings.Compare(string(ja), string(jb))
}

// MarshalNormalized normalizes data and marshals it to stable JSON bytes.
// Uses canonical key ordering and 2-space indentation with trailing newline.
func MarshalNormalized(t *testing.T, fixture *FixtureContext, data any) []byte {
	t.Helper()

	normalizer := &DefaultNormalizer{}
	normalized := normalizer.Normalize(t, fixture, data)

	// Marshal with canonical ordering
	bytes, err := marshalCanonical(normalized)
	if err != nil {
		t.Fatalf("Failed to marshal normalized data: %v", err)
	}

	// Ensure trailing newline
	if len(bytes) > 0 && bytes[len(bytes)-1] != '\n' {
		bytes = append(bytes, '\n')
	}

	return bytes
}

// marshalCanonical marshals data with sorted keys and 2-space indentation.
func marshalCanonical(data any) ([]byte, error) {
	// For maps, we need to sort keys
	sortedData := canonicalizeKeys(data)
	return json.MarshalIndent(sortedData, "", "  ")
}

// canonicalizeKeys recursively sorts map keys for deterministic output.
func canonicalizeKeys(data any) any {
	switch v := data.(type) {
	case map[string]any:
		// Sort keys
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// Create ordered map (Go maps preserve insertion order in JSON marshal)
		result := make(map[string]any, len(v))
		for _, k := range keys {
			result[k] = canonicalizeKeys(v[k])
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = canonicalizeKeys(item)
		}
		return result
	default:
		return v
	}
}

// NormalizeSymbolID strips repository-specific prefixes from symbol IDs.
// Converts "ckb:myrepo:sym:abc123" to "ckb:<repo>:sym:abc123"
func NormalizeSymbolID(id string) string {
	parts := strings.SplitN(id, ":", 4)
	if len(parts) >= 4 && parts[0] == "ckb" && parts[2] == "sym" {
		return "ckb:<repo>:sym:" + parts[3]
	}
	return id
}

// StructToMap converts a struct to a map[string]any for normalization.
// This is useful for converting typed API responses before normalization.
func StructToMap(t *testing.T, v any) map[string]any {
	t.Helper()

	bytes, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Failed to marshal struct: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(bytes, &result); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	return result
}

// SliceToMaps converts a slice of structs to []any for normalization.
func SliceToMaps(t *testing.T, v any) []any {
	t.Helper()

	bytes, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Failed to marshal slice: %v", err)
	}

	var result []any
	if err := json.Unmarshal(bytes, &result); err != nil {
		t.Fatalf("Failed to unmarshal to slice: %v", err)
	}

	return result
}

// NormalizeFilePath normalizes a file path for consistent comparison.
// - Converts to forward slashes
// - Makes relative to fixture root
// - Cleans the path
func NormalizeFilePath(path, fixtureRoot string) string {
	// Convert to forward slashes
	path = strings.ReplaceAll(path, "\\", "/")
	fixtureRoot = strings.ReplaceAll(fixtureRoot, "\\", "/")

	// Make relative to fixture root
	rel, err := filepath.Rel(fixtureRoot, path)
	if err == nil {
		path = rel
	} else if strings.HasPrefix(path, fixtureRoot) {
		path = strings.TrimPrefix(path, fixtureRoot)
		path = strings.TrimPrefix(path, "/")
	}

	// Clean the path
	path = filepath.Clean(path)
	path = strings.ReplaceAll(path, "\\", "/")

	return path
}

// DeepEqual compares two values for equality, ignoring volatile fields.
// Returns true if equal, false otherwise.
func DeepEqual(t *testing.T, fixture *FixtureContext, a, b any) bool {
	t.Helper()

	normalizer := &DefaultNormalizer{}
	normA := normalizer.Normalize(t, fixture, a)
	normB := normalizer.Normalize(t, fixture, b)

	return reflect.DeepEqual(normA, normB)
}
