package output

import (
	"fmt"
	"reflect"
	"sort"
)

// SortCriteria defines a sort criterion with field name and direction
type SortCriteria struct {
	Field      string // Field name to sort by
	Descending bool   // If true, sort descending; otherwise ascending
}

// MultiFieldSort sorts a slice by multiple criteria
// The slice parameter must be a pointer to a slice
func MultiFieldSort(slice interface{}, criteria []SortCriteria) error {
	sliceVal := reflect.ValueOf(slice)
	if sliceVal.Kind() != reflect.Ptr {
		return fmt.Errorf("slice must be a pointer to a slice")
	}

	sliceVal = sliceVal.Elem()
	if sliceVal.Kind() != reflect.Slice {
		return fmt.Errorf("slice must be a pointer to a slice")
	}

	if len(criteria) == 0 {
		return fmt.Errorf("at least one sort criteria must be provided")
	}

	// Perform stable sort
	sort.SliceStable(sliceVal.Interface(), func(i, j int) bool {
		iVal := sliceVal.Index(i)
		jVal := sliceVal.Index(j)

		for _, criterion := range criteria {
			// Get field values
			iField, err := getFieldValue(iVal, criterion.Field)
			if err != nil {
				return false
			}

			jField, err := getFieldValue(jVal, criterion.Field)
			if err != nil {
				return false
			}

			// Compare
			cmp := compareValues(iField, jField)
			if cmp != 0 {
				if criterion.Descending {
					return cmp > 0
				}
				return cmp < 0
			}
		}

		return false
	})

	return nil
}

// getFieldValue gets a field value from a struct using reflection
func getFieldValue(val reflect.Value, fieldName string) (interface{}, error) {
	// Dereference pointers
	for val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return nil, fmt.Errorf("nil pointer encountered")
		}
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return nil, fmt.Errorf("value is not a struct")
	}

	field := val.FieldByName(fieldName)
	if !field.IsValid() {
		return nil, fmt.Errorf("field %s not found", fieldName)
	}

	return field.Interface(), nil
}

// compareValues compares two values and returns:
// -1 if a < b
// 0 if a == b
// 1 if a > b
func compareValues(a, b interface{}) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	aVal := reflect.ValueOf(a)
	bVal := reflect.ValueOf(b)

	// Handle different types
	switch aVal.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		aInt := aVal.Int()
		bInt := bVal.Int()
		if aInt < bInt {
			return -1
		}
		if aInt > bInt {
			return 1
		}
		return 0

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		aUint := aVal.Uint()
		bUint := bVal.Uint()
		if aUint < bUint {
			return -1
		}
		if aUint > bUint {
			return 1
		}
		return 0

	case reflect.Float32, reflect.Float64:
		aFloat := aVal.Float()
		bFloat := bVal.Float()
		if aFloat < bFloat {
			return -1
		}
		if aFloat > bFloat {
			return 1
		}
		return 0

	case reflect.String:
		aStr := aVal.String()
		bStr := bVal.String()
		if aStr < bStr {
			return -1
		}
		if aStr > bStr {
			return 1
		}
		return 0

	default:
		// For other types, use string representation
		aStr := fmt.Sprintf("%v", a)
		bStr := fmt.Sprintf("%v", b)
		if aStr < bStr {
			return -1
		}
		if aStr > bStr {
			return 1
		}
		return 0
	}
}
