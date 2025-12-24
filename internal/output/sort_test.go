package output

import (
	"reflect"
	"testing"
)

type testItem struct {
	Name     string
	Priority int
	Score    float64
	Count    uint32
}

func TestMultiFieldSort(t *testing.T) {
	t.Run("single criteria ascending", func(t *testing.T) {
		items := []testItem{
			{Name: "c", Priority: 3},
			{Name: "a", Priority: 1},
			{Name: "b", Priority: 2},
		}

		err := MultiFieldSort(&items, []SortCriteria{{Field: "Priority", Descending: false}})
		if err != nil {
			t.Fatalf("MultiFieldSort() error = %v", err)
		}

		if items[0].Priority != 1 || items[1].Priority != 2 || items[2].Priority != 3 {
			t.Errorf("Items not sorted correctly: %v", items)
		}
	})

	t.Run("single criteria descending", func(t *testing.T) {
		items := []testItem{
			{Name: "a", Priority: 1},
			{Name: "c", Priority: 3},
			{Name: "b", Priority: 2},
		}

		err := MultiFieldSort(&items, []SortCriteria{{Field: "Priority", Descending: true}})
		if err != nil {
			t.Fatalf("MultiFieldSort() error = %v", err)
		}

		if items[0].Priority != 3 || items[1].Priority != 2 || items[2].Priority != 1 {
			t.Errorf("Items not sorted correctly: %v", items)
		}
	})

	t.Run("multiple criteria", func(t *testing.T) {
		items := []testItem{
			{Name: "b", Priority: 1},
			{Name: "a", Priority: 1},
			{Name: "c", Priority: 2},
		}

		err := MultiFieldSort(&items, []SortCriteria{
			{Field: "Priority", Descending: false},
			{Field: "Name", Descending: false},
		})
		if err != nil {
			t.Fatalf("MultiFieldSort() error = %v", err)
		}

		// Should be sorted by Priority first, then by Name
		if items[0].Name != "a" || items[1].Name != "b" || items[2].Name != "c" {
			t.Errorf("Items not sorted correctly: %v", items)
		}
	})

	t.Run("not a pointer", func(t *testing.T) {
		items := []testItem{{Name: "a"}}
		err := MultiFieldSort(items, []SortCriteria{{Field: "Name"}})
		if err == nil {
			t.Error("MultiFieldSort() should error on non-pointer")
		}
	})

	t.Run("not a slice", func(t *testing.T) {
		item := testItem{Name: "a"}
		err := MultiFieldSort(&item, []SortCriteria{{Field: "Name"}})
		if err == nil {
			t.Error("MultiFieldSort() should error on non-slice")
		}
	})

	t.Run("empty criteria", func(t *testing.T) {
		items := []testItem{{Name: "a"}}
		err := MultiFieldSort(&items, []SortCriteria{})
		if err == nil {
			t.Error("MultiFieldSort() should error on empty criteria")
		}
	})
}

func TestCompareValues(t *testing.T) {
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want int
	}{
		// nil cases
		{"nil vs nil", nil, nil, 0},
		{"nil vs value", nil, 1, -1},
		{"value vs nil", 1, nil, 1},

		// int cases
		{"int equal", 5, 5, 0},
		{"int less", 3, 5, -1},
		{"int greater", 7, 5, 1},
		{"int64 equal", int64(100), int64(100), 0},
		{"int64 less", int64(50), int64(100), -1},
		{"int64 greater", int64(150), int64(100), 1},

		// uint cases
		{"uint equal", uint(10), uint(10), 0},
		{"uint less", uint(5), uint(10), -1},
		{"uint greater", uint(15), uint(10), 1},
		{"uint32 equal", uint32(100), uint32(100), 0},
		{"uint32 less", uint32(50), uint32(100), -1},
		{"uint32 greater", uint32(150), uint32(100), 1},

		// float cases
		{"float equal", 3.14, 3.14, 0},
		{"float less", 2.71, 3.14, -1},
		{"float greater", 3.14, 2.71, 1},
		{"float32 equal", float32(1.5), float32(1.5), 0},
		{"float32 less", float32(1.0), float32(2.0), -1},
		{"float32 greater", float32(3.0), float32(2.0), 1},

		// string cases
		{"string equal", "abc", "abc", 0},
		{"string less", "abc", "xyz", -1},
		{"string greater", "xyz", "abc", 1},
		{"string empty vs value", "", "a", -1},
		{"string value vs empty", "a", "", 1},

		// bool cases (uses fmt.Sprintf)
		{"bool equal true", true, true, 0},
		{"bool equal false", false, false, 0},
		{"bool false < true", false, true, -1}, // "false" < "true"
		{"bool true > false", true, false, 1},  // "true" > "false"
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareValues(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareValues(%v, %v) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestGetFieldValue(t *testing.T) {
	type nested struct {
		Value string
	}

	type testStruct struct {
		Name   string
		Count  int
		Nested *nested
	}

	t.Run("get string field", func(t *testing.T) {
		s := testStruct{Name: "test", Count: 10}
		val, err := getFieldValue(reflect.ValueOf(s), "Name")
		if err != nil {
			t.Fatalf("getFieldValue() error = %v", err)
		}
		if val.(string) != "test" {
			t.Errorf("getFieldValue() = %v, want 'test'", val)
		}
	})

	t.Run("get int field", func(t *testing.T) {
		s := testStruct{Name: "test", Count: 42}
		val, err := getFieldValue(reflect.ValueOf(s), "Count")
		if err != nil {
			t.Fatalf("getFieldValue() error = %v", err)
		}
		if val.(int) != 42 {
			t.Errorf("getFieldValue() = %v, want 42", val)
		}
	})

	t.Run("get field from pointer", func(t *testing.T) {
		s := &testStruct{Name: "pointer-test", Count: 99}
		val, err := getFieldValue(reflect.ValueOf(s), "Name")
		if err != nil {
			t.Fatalf("getFieldValue() error = %v", err)
		}
		if val.(string) != "pointer-test" {
			t.Errorf("getFieldValue() = %v, want 'pointer-test'", val)
		}
	})

	t.Run("field not found", func(t *testing.T) {
		s := testStruct{Name: "test"}
		_, err := getFieldValue(reflect.ValueOf(s), "NonExistent")
		if err == nil {
			t.Error("getFieldValue() should error on non-existent field")
		}
	})

	t.Run("not a struct", func(t *testing.T) {
		val := 42
		_, err := getFieldValue(reflect.ValueOf(val), "Name")
		if err == nil {
			t.Error("getFieldValue() should error on non-struct")
		}
	})
}

func TestMultiFieldSortWithFloats(t *testing.T) {
	items := []testItem{
		{Name: "a", Score: 0.9},
		{Name: "b", Score: 0.5},
		{Name: "c", Score: 0.7},
	}

	err := MultiFieldSort(&items, []SortCriteria{{Field: "Score", Descending: true}})
	if err != nil {
		t.Fatalf("MultiFieldSort() error = %v", err)
	}

	if items[0].Score != 0.9 || items[1].Score != 0.7 || items[2].Score != 0.5 {
		t.Errorf("Items not sorted correctly by float: %v", items)
	}
}

func TestMultiFieldSortWithUint(t *testing.T) {
	items := []testItem{
		{Name: "a", Count: 100},
		{Name: "b", Count: 50},
		{Name: "c", Count: 200},
	}

	err := MultiFieldSort(&items, []SortCriteria{{Field: "Count", Descending: true}})
	if err != nil {
		t.Fatalf("MultiFieldSort() error = %v", err)
	}

	if items[0].Count != 200 || items[1].Count != 100 || items[2].Count != 50 {
		t.Errorf("Items not sorted correctly by uint: %v", items)
	}
}
