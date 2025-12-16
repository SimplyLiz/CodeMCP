package output

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
)

// DeterministicEncode produces byte-identical JSON output
// - Stable key ordering (sorted alphabetically)
// - Float formatting: max 6 decimal places, no trailing zeros
// - Null/undefined fields omitted entirely
func DeterministicEncode(v interface{}) ([]byte, error) {
	// Normalize the value first
	normalized := normalizeValue(v)

	// Use json.Marshal with sorted keys
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(normalized); err != nil {
		return nil, err
	}

	// Remove the trailing newline added by Encode
	result := buf.Bytes()
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}

	return result, nil
}

// DeterministicEncodeIndented produces indented byte-identical JSON output
func DeterministicEncodeIndented(v interface{}, indent string) ([]byte, error) {
	// Normalize the value first
	normalized := normalizeValue(v)

	// Marshal with indentation
	result, err := json.MarshalIndent(normalized, "", indent)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// normalizeValue recursively normalizes a value for deterministic encoding
func normalizeValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}

	val := reflect.ValueOf(v)

	// Dereference pointers
	for val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil
		}
		val = val.Elem()
	}

	switch val.Kind() {
	case reflect.Map:
		return normalizeMap(val)
	case reflect.Slice, reflect.Array:
		return normalizeSlice(val)
	case reflect.Struct:
		return normalizeStruct(val)
	case reflect.Float32, reflect.Float64:
		return RoundFloat(val.Float())
	case reflect.Interface:
		if val.IsNil() {
			return nil
		}
		return normalizeValue(val.Interface())
	default:
		return v
	}
}

// normalizeMap converts a map to an ordered map for deterministic JSON output
func normalizeMap(val reflect.Value) map[string]interface{} {
	if val.IsNil() {
		return nil
	}

	result := make(map[string]interface{})
	iter := val.MapRange()
	for iter.Next() {
		key := iter.Key().String()
		value := normalizeValue(iter.Value().Interface())
		// Only include non-nil values
		if value != nil {
			result[key] = value
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

// normalizeSlice normalizes a slice or array
func normalizeSlice(val reflect.Value) interface{} {
	if val.Kind() == reflect.Slice && val.IsNil() {
		return nil
	}

	length := val.Len()
	if length == 0 {
		return nil
	}

	result := make([]interface{}, length)
	for i := 0; i < length; i++ {
		result[i] = normalizeValue(val.Index(i).Interface())
	}

	return result
}

// normalizeStruct converts a struct to a map for deterministic JSON output
func normalizeStruct(val reflect.Value) map[string]interface{} {
	result := make(map[string]interface{})
	typ := val.Type()

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		fieldVal := val.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get JSON tag
		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}

		// Parse tag options
		tagName, omitEmpty := parseJSONTag(jsonTag)
		if tagName == "" {
			tagName = field.Name
		}

		// Normalize the field value
		normalized := normalizeValue(fieldVal.Interface())

		// Skip if omitempty and value is zero/nil
		if omitEmpty && isZeroValue(normalized) {
			continue
		}

		// Skip nil values
		if normalized != nil {
			result[tagName] = normalized
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

// parseJSONTag parses a JSON struct tag
func parseJSONTag(tag string) (name string, omitEmpty bool) {
	if tag == "" {
		return "", false
	}

	// Split by comma
	parts := []string{}
	current := ""
	for _, ch := range tag {
		if ch == ',' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}

	name = parts[0]
	for i := 1; i < len(parts); i++ {
		if parts[i] == "omitempty" {
			omitEmpty = true
		}
	}

	return name, omitEmpty
}

// isZeroValue checks if a value is zero/empty
func isZeroValue(v interface{}) bool {
	if v == nil {
		return true
	}

	switch val := v.(type) {
	case bool:
		return !val
	case int, int8, int16, int32, int64:
		return val == 0
	case uint, uint8, uint16, uint32, uint64:
		return val == 0
	case float32, float64:
		return val == 0
	case string:
		return val == ""
	case []interface{}:
		return len(val) == 0
	case map[string]interface{}:
		return len(val) == 0
	default:
		return false
	}
}

// MarshalJSON implements a custom JSON marshaler that ensures deterministic output
type DeterministicMap map[string]interface{}

// MarshalJSON implements json.Marshaler
func (m DeterministicMap) MarshalJSON() ([]byte, error) {
	// Sort keys
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build JSON manually to ensure key order
	var buf bytes.Buffer
	buf.WriteByte('{')

	first := true
	for _, k := range keys {
		v := m[k]
		// Skip nil values
		if v == nil {
			continue
		}

		if !first {
			buf.WriteByte(',')
		}
		first = false

		// Encode key
		keyJSON, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(keyJSON)
		buf.WriteByte(':')

		// Encode value
		valJSON, err := json.Marshal(normalizeValue(v))
		if err != nil {
			return nil, err
		}
		buf.Write(valJSON)
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}
