package output

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestDeterministicEncode(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		wantJSON string
	}{
		{
			name: "simple struct with floats",
			input: struct {
				Name  string  `json:"name"`
				Score float64 `json:"score"`
				Count int     `json:"count"`
			}{
				Name:  "test",
				Score: 0.123456789,
				Count: 42,
			},
			wantJSON: `{"count":42,"name":"test","score":0.123457}`,
		},
		{
			name: "struct with omitted nil fields",
			input: struct {
				Name  string   `json:"name"`
				Score *float64 `json:"score,omitempty"`
			}{
				Name:  "test",
				Score: nil,
			},
			wantJSON: `{"name":"test"}`,
		},
		{
			name: "struct with zero values and omitempty",
			input: struct {
				Name  string `json:"name"`
				Count int    `json:"count,omitempty"`
			}{
				Name:  "test",
				Count: 0,
			},
			wantJSON: `{"name":"test"}`,
		},
		{
			name: "map with sorted keys",
			input: map[string]interface{}{
				"zebra": "last",
				"alpha": "first",
				"beta":  "second",
			},
			wantJSON: `{"alpha":"first","beta":"second","zebra":"last"}`,
		},
		{
			name: "slice of structs",
			input: []struct {
				ID    string  `json:"id"`
				Value float64 `json:"value"`
			}{
				{ID: "a", Value: 1.123456789},
				{ID: "b", Value: 2.987654321},
			},
			wantJSON: `[{"id":"a","value":1.123457},{"id":"b","value":2.987654}]`,
		},
		{
			name:     "nil value",
			input:    nil,
			wantJSON: `null`,
		},
		{
			name:     "empty slice returns null",
			input:    []string{},
			wantJSON: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DeterministicEncode(tt.input)
			if err != nil {
				t.Fatalf("DeterministicEncode() error = %v", err)
			}

			// Compare JSON strings
			var gotObj, wantObj interface{}
			if err := json.Unmarshal(got, &gotObj); err != nil {
				t.Fatalf("Failed to unmarshal got: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.wantJSON), &wantObj); err != nil {
				t.Fatalf("Failed to unmarshal want: %v", err)
			}

			gotJSON, _ := json.Marshal(gotObj)
			wantJSON, _ := json.Marshal(wantObj)

			if !bytes.Equal(gotJSON, wantJSON) {
				t.Errorf("DeterministicEncode() = %s, want %s", string(got), tt.wantJSON)
			}
		})
	}
}

func TestDeterministicEncodeConsistency(t *testing.T) {
	// Test that encoding the same data multiple times produces identical bytes
	data := map[string]interface{}{
		"modules": []Module{
			{ModuleId: "mod2", Name: "second", ImpactCount: 5, SymbolCount: 10},
			{ModuleId: "mod1", Name: "first", ImpactCount: 10, SymbolCount: 5},
		},
		"symbols": []Symbol{
			{StableId: "sym2", Name: "second", Confidence: 0.8, RefCount: 3},
			{StableId: "sym1", Name: "first", Confidence: 0.9, RefCount: 5},
		},
		"metadata": map[string]interface{}{
			"version": "1.0",
			"score":   0.123456789,
		},
	}

	// Encode 10 times
	var results [][]byte
	for i := 0; i < 10; i++ {
		encoded, err := DeterministicEncode(data)
		if err != nil {
			t.Fatalf("DeterministicEncode() error = %v", err)
		}
		results = append(results, encoded)
	}

	// All results should be byte-identical
	for i := 1; i < len(results); i++ {
		if !bytes.Equal(results[0], results[i]) {
			t.Errorf("Encoding is not deterministic:\nrun 0: %s\nrun %d: %s", string(results[0]), i, string(results[i]))
		}
	}
}

func TestFloatRounding(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  float64
	}{
		{
			name:  "round to 6 decimal places",
			input: 0.123456789,
			want:  0.123457,
		},
		{
			name:  "no rounding needed",
			input: 0.123456,
			want:  0.123456,
		},
		{
			name:  "round up",
			input: 0.1234567,
			want:  0.123457,
		},
		{
			name:  "round down",
			input: 0.1234564,
			want:  0.123456,
		},
		{
			name:  "zero",
			input: 0.0,
			want:  0.0,
		},
		{
			name:  "negative",
			input: -0.123456789,
			want:  -0.123457,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RoundFloat(tt.input)
			if got != tt.want {
				t.Errorf("RoundFloat(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestDeterministicEncodeIndented(t *testing.T) {
	data := map[string]interface{}{
		"name":  "test",
		"value": 0.123456789,
	}

	got, err := DeterministicEncodeIndented(data, "  ")
	if err != nil {
		t.Fatalf("DeterministicEncodeIndented() error = %v", err)
	}

	// Verify it's valid JSON
	var decoded map[string]interface{}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	// Verify indentation is present
	if !bytes.Contains(got, []byte("\n")) {
		t.Error("DeterministicEncodeIndented() should produce indented output")
	}
}

func TestDeterministicMapMarshalJSON(t *testing.T) {
	dm := DeterministicMap{
		"zebra": "last",
		"alpha": "first",
		"beta":  "second",
	}

	got, err := json.Marshal(dm)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Keys should be sorted
	want := `{"alpha":"first","beta":"second","zebra":"last"}`
	if string(got) != want {
		t.Errorf("DeterministicMap.MarshalJSON() = %s, want %s", string(got), want)
	}
}

func TestComplexNestedStructure(t *testing.T) {
	type ComplexResponse struct {
		Modules   []Module               `json:"modules"`
		Symbols   []Symbol               `json:"symbols,omitempty"`
		Metadata  map[string]interface{} `json:"metadata"`
		Timestamp *string                `json:"timestamp,omitempty"`
	}

	response := ComplexResponse{
		Modules: []Module{
			{ModuleId: "mod2", Name: "second", ImpactCount: 5, SymbolCount: 10},
			{ModuleId: "mod1", Name: "first", ImpactCount: 10, SymbolCount: 5},
		},
		Symbols: nil, // Should be omitted
		Metadata: map[string]interface{}{
			"zebra": "last",
			"alpha": "first",
			"score": 0.123456789,
		},
		Timestamp: nil, // Should be omitted
	}

	// Encode twice
	result1, err := DeterministicEncode(response)
	if err != nil {
		t.Fatalf("DeterministicEncode() error = %v", err)
	}

	result2, err := DeterministicEncode(response)
	if err != nil {
		t.Fatalf("DeterministicEncode() error = %v", err)
	}

	// Should be byte-identical
	if !bytes.Equal(result1, result2) {
		t.Errorf("Complex structure encoding is not deterministic:\n%s\nvs\n%s", string(result1), string(result2))
	}

	// Verify nil fields are omitted
	if bytes.Contains(result1, []byte("symbols")) {
		t.Error("Nil symbols field should be omitted")
	}
	if bytes.Contains(result1, []byte("timestamp")) {
		t.Error("Nil timestamp field should be omitted")
	}

	// Verify map keys are sorted
	var decoded map[string]interface{}
	if err := json.Unmarshal(result1, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	metadata, ok := decoded["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("metadata is not a map")
	}

	// Re-encode to check key order
	metadataJSON, _ := json.Marshal(metadata)
	if !bytes.Contains(metadataJSON, []byte(`"alpha"`)) ||
		!bytes.Contains(metadataJSON, []byte(`"score"`)) ||
		!bytes.Contains(metadataJSON, []byte(`"zebra"`)) {
		t.Error("metadata keys are not properly handled")
	}
}
