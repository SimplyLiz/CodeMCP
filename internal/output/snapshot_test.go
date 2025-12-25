package output

import (
	"testing"
)

func TestNormalizeForSnapshot(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name: "remove cachedAt",
			input: `{
				"modules": ["module1"],
				"provenance": {
					"cachedAt": "2024-01-01T00:00:00Z",
					"backend": "scip"
				}
			}`,
			want: `{"modules":["module1"],"provenance":{"backend":"scip"}}`,
		},
		{
			name: "remove queryDurationMs",
			input: `{
				"result": "success",
				"provenance": {
					"queryDurationMs": 123,
					"backend": "scip"
				}
			}`,
			want: `{"provenance":{"backend":"scip"},"result":"success"}`,
		},
		{
			name: "remove computedAt",
			input: `{
				"data": "test",
				"provenance": {
					"computedAt": "2024-01-01T00:00:00Z",
					"backend": "scip"
				}
			}`,
			want: `{"data":"test","provenance":{"backend":"scip"}}`,
		},
		{
			name: "remove all time-varying fields",
			input: `{
				"data": "test",
				"provenance": {
					"cachedAt": "2024-01-01T00:00:00Z",
					"queryDurationMs": 123,
					"computedAt": "2024-01-01T00:00:00Z",
					"backend": "scip"
				}
			}`,
			want: `{"data":"test","provenance":{"backend":"scip"}}`,
		},
		{
			name: "no provenance block",
			input: `{
				"data": "test",
				"result": "success"
			}`,
			want: `{"data":"test","result":"success"}`,
		},
		{
			name:    "invalid JSON",
			input:   `{invalid json}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeForSnapshot([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeForSnapshot() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && string(got) != tt.want {
				t.Errorf("NormalizeForSnapshot() = %s, want %s", string(got), tt.want)
			}
		})
	}
}

func TestCompareSnapshots(t *testing.T) {
	tests := []struct {
		name      string
		a         string
		b         string
		wantEqual bool
		wantMsg   string
	}{
		{
			name: "identical after normalization",
			a: `{
				"data": "test",
				"provenance": {
					"cachedAt": "2024-01-01T00:00:00Z",
					"backend": "scip"
				}
			}`,
			b: `{
				"data": "test",
				"provenance": {
					"cachedAt": "2024-01-02T00:00:00Z",
					"backend": "scip"
				}
			}`,
			wantEqual: true,
		},
		{
			name: "different data",
			a: `{
				"data": "test1",
				"provenance": {
					"backend": "scip"
				}
			}`,
			b: `{
				"data": "test2",
				"provenance": {
					"backend": "scip"
				}
			}`,
			wantEqual: false,
			wantMsg:   "snapshots differ",
		},
		{
			name: "different backend",
			a: `{
				"data": "test",
				"provenance": {
					"backend": "scip"
				}
			}`,
			b: `{
				"data": "test",
				"provenance": {
					"backend": "lsp"
				}
			}`,
			wantEqual: false,
			wantMsg:   "snapshots differ",
		},
		{
			name: "same data different timestamps",
			a: `{
				"modules": [{"moduleId": "mod1"}],
				"provenance": {
					"cachedAt": "2024-01-01T00:00:00Z",
					"queryDurationMs": 100,
					"computedAt": "2024-01-01T00:00:00Z",
					"backend": "scip"
				}
			}`,
			b: `{
				"modules": [{"moduleId": "mod1"}],
				"provenance": {
					"cachedAt": "2024-01-02T00:00:00Z",
					"queryDurationMs": 200,
					"computedAt": "2024-01-02T00:00:00Z",
					"backend": "scip"
				}
			}`,
			wantEqual: true,
		},
		{
			name:      "invalid JSON in a",
			a:         `{invalid}`,
			b:         `{"data": "test"}`,
			wantEqual: false,
		},
		{
			name:      "invalid JSON in b",
			a:         `{"data": "test"}`,
			b:         `{invalid}`,
			wantEqual: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotEqual, gotMsg := CompareSnapshots([]byte(tt.a), []byte(tt.b))
			if gotEqual != tt.wantEqual {
				t.Errorf("CompareSnapshots() equal = %v, want %v", gotEqual, tt.wantEqual)
			}
			if !tt.wantEqual && tt.wantMsg != "" && gotMsg != tt.wantMsg {
				t.Logf("CompareSnapshots() msg = %v, want %v", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestSnapshotEqual(t *testing.T) {
	type TestResponse struct {
		Data       string                 `json:"data"`
		Provenance map[string]interface{} `json:"provenance"`
	}

	response1 := TestResponse{
		Data: "test",
		Provenance: map[string]interface{}{
			"cachedAt":        "2024-01-01T00:00:00Z",
			"queryDurationMs": 100,
			"backend":         "scip",
		},
	}

	response2 := TestResponse{
		Data: "test",
		Provenance: map[string]interface{}{
			"cachedAt":        "2024-01-02T00:00:00Z",
			"queryDurationMs": 200,
			"backend":         "scip",
		},
	}

	response3 := TestResponse{
		Data: "different",
		Provenance: map[string]interface{}{
			"cachedAt": "2024-01-01T00:00:00Z",
			"backend":  "scip",
		},
	}

	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		{
			name: "same data different timestamps",
			a:    response1,
			b:    response2,
			want: true,
		},
		{
			name: "different data",
			a:    response1,
			b:    response3,
			want: false,
		},
		{
			name: "identical",
			a:    response1,
			b:    response1,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SnapshotEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("SnapshotEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRemoveNestedField(t *testing.T) {
	tests := []struct {
		name  string
		data  map[string]interface{}
		path  string
		check func(map[string]interface{}) bool
	}{
		{
			name: "remove top-level field",
			data: map[string]interface{}{
				"field1": "value1",
				"field2": "value2",
			},
			path: "field1",
			check: func(m map[string]interface{}) bool {
				_, exists := m["field1"]
				return !exists
			},
		},
		{
			name: "remove nested field",
			data: map[string]interface{}{
				"parent": map[string]interface{}{
					"child1": "value1",
					"child2": "value2",
				},
			},
			path: "parent.child1",
			check: func(m map[string]interface{}) bool {
				parent, ok := m["parent"].(map[string]interface{})
				if !ok {
					return false
				}
				_, exists := parent["child1"]
				return !exists && parent["child2"] == "value2"
			},
		},
		{
			name: "remove deeply nested field",
			data: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": "value",
					},
				},
			},
			path: "level1.level2.level3",
			check: func(m map[string]interface{}) bool {
				level1, ok := m["level1"].(map[string]interface{})
				if !ok {
					return false
				}
				level2, ok := level1["level2"].(map[string]interface{})
				if !ok {
					return false
				}
				_, exists := level2["level3"]
				return !exists
			},
		},
		{
			name: "path does not exist",
			data: map[string]interface{}{
				"field": "value",
			},
			path: "nonexistent.field",
			check: func(m map[string]interface{}) bool {
				// Should not crash, data unchanged
				return m["field"] == "value"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			removeNestedField(tt.data, tt.path)
			if !tt.check(tt.data) {
				t.Errorf("removeNestedField() failed check for path %s", tt.path)
			}
		})
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "simple path",
			path: "field",
			want: []string{"field"},
		},
		{
			name: "nested path",
			path: "parent.child",
			want: []string{"parent", "child"},
		},
		{
			name: "deeply nested path",
			path: "level1.level2.level3",
			want: []string{"level1", "level2", "level3"},
		},
		{
			name: "empty path",
			path: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitPath(tt.path)
			if len(got) != len(tt.want) {
				t.Errorf("splitPath() length = %v, want %v", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitPath()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestSnapshotDeterminism(t *testing.T) {
	// Test that snapshot comparison is deterministic
	data := `{
		"modules": [
			{"moduleId": "mod1", "name": "first"},
			{"moduleId": "mod2", "name": "second"}
		],
		"provenance": {
			"cachedAt": "2024-01-01T00:00:00Z",
			"queryDurationMs": 123,
			"backend": "scip"
		}
	}`

	// Normalize multiple times
	var results [][]byte
	for i := 0; i < 10; i++ {
		normalized, err := NormalizeForSnapshot([]byte(data))
		if err != nil {
			t.Fatalf("NormalizeForSnapshot() error = %v", err)
		}
		results = append(results, normalized)
	}

	// All results should be byte-identical
	for i := 1; i < len(results); i++ {
		equal, msg := CompareSnapshots(results[0], results[i])
		if !equal {
			t.Errorf("Snapshot normalization is not deterministic: %s", msg)
		}
	}
}

func TestDeepEqual(t *testing.T) {
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		{
			name: "equal strings",
			a:    "hello",
			b:    "hello",
			want: true,
		},
		{
			name: "different strings",
			a:    "hello",
			b:    "world",
			want: false,
		},
		{
			name: "equal ints",
			a:    42,
			b:    42,
			want: true,
		},
		{
			name: "different ints",
			a:    42,
			b:    43,
			want: false,
		},
		{
			name: "equal slices",
			a:    []int{1, 2, 3},
			b:    []int{1, 2, 3},
			want: true,
		},
		{
			name: "different slices",
			a:    []int{1, 2, 3},
			b:    []int{1, 2, 4},
			want: false,
		},
		{
			name: "equal maps",
			a:    map[string]int{"a": 1, "b": 2},
			b:    map[string]int{"a": 1, "b": 2},
			want: true,
		},
		{
			name: "different maps",
			a:    map[string]int{"a": 1, "b": 2},
			b:    map[string]int{"a": 1, "b": 3},
			want: false,
		},
		{
			name: "nil vs nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "nil vs empty slice",
			a:    []int(nil),
			b:    []int{},
			want: false,
		},
		{
			name: "equal structs",
			a:    struct{ Name string }{Name: "test"},
			b:    struct{ Name string }{Name: "test"},
			want: true,
		},
		{
			name: "different structs",
			a:    struct{ Name string }{Name: "test1"},
			b:    struct{ Name string }{Name: "test2"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DeepEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("DeepEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
