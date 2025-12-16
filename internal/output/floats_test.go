package output

import (
	"testing"
)

func TestRoundFloat(t *testing.T) {
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
			name:  "negative round up",
			input: -0.123456789,
			want:  -0.123457,
		},
		{
			name:  "negative round down",
			input: -0.1234564,
			want:  -0.123456,
		},
		{
			name:  "large number",
			input: 1234567.123456789,
			want:  1234567.123457,
		},
		{
			name:  "very small number",
			input: 0.000001234567,
			want:  0.000001,
		},
		{
			name:  "trailing zeros preserved in calculation",
			input: 0.100000,
			want:  0.1,
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

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  string
	}{
		{
			name:  "remove trailing zeros",
			input: 0.100000,
			want:  "0.1",
		},
		{
			name:  "remove trailing zeros after rounding",
			input: 0.123000,
			want:  "0.123",
		},
		{
			name:  "no trailing zeros",
			input: 0.123456,
			want:  "0.123456",
		},
		{
			name:  "zero",
			input: 0.0,
			want:  "0",
		},
		{
			name:  "integer",
			input: 42.0,
			want:  "42",
		},
		{
			name:  "negative",
			input: -0.123000,
			want:  "-0.123",
		},
		{
			name:  "large number",
			input: 1234567.123,
			want:  "1234567.123",
		},
		{
			name:  "round and format",
			input: 0.123456789,
			want:  "0.123457",
		},
		{
			name:  "very small",
			input: 0.000001,
			want:  "0.000001",
		},
		{
			name:  "all zeros after decimal",
			input: 100.000000,
			want:  "100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatFloat(tt.input)
			if got != tt.want {
				t.Errorf("FormatFloat(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeFloat(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  float64
	}{
		{
			name:  "normalize confidence score",
			input: 0.987654321,
			want:  0.987654,
		},
		{
			name:  "normalize relevance score",
			input: 0.123456789,
			want:  0.123457,
		},
		{
			name:  "already normalized",
			input: 0.5,
			want:  0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeFloat(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeFloat(%v) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFloatRoundingEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input float64
	}{
		{name: "0.5 rounding", input: 0.5},
		{name: "0.4999995", input: 0.4999995},
		{name: "0.5000005", input: 0.5000005},
		{name: "negative 0.5", input: -0.5},
		{name: "very large", input: 9999999.123456789},
		{name: "very small", input: 0.0000001234567},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that rounding is consistent
			result1 := RoundFloat(tt.input)
			result2 := RoundFloat(tt.input)

			if result1 != result2 {
				t.Errorf("RoundFloat(%v) is not consistent: %v != %v", tt.input, result1, result2)
			}

			// Test that formatting is consistent
			format1 := FormatFloat(tt.input)
			format2 := FormatFloat(tt.input)

			if format1 != format2 {
				t.Errorf("FormatFloat(%v) is not consistent: %v != %v", tt.input, format1, format2)
			}
		})
	}
}

func TestFloatDeterminism(t *testing.T) {
	// Test that float operations are deterministic across multiple runs
	inputs := []float64{
		0.123456789,
		0.987654321,
		0.5,
		0.333333333,
		0.666666666,
		1.0 / 3.0,
		2.0 / 3.0,
	}

	for _, input := range inputs {
		// Run rounding multiple times
		var results []float64
		for i := 0; i < 100; i++ {
			results = append(results, RoundFloat(input))
		}

		// All results should be identical
		for i := 1; i < len(results); i++ {
			if results[0] != results[i] {
				t.Errorf("RoundFloat(%v) is not deterministic: %v != %v", input, results[0], results[i])
			}
		}

		// Run formatting multiple times
		var formats []string
		for i := 0; i < 100; i++ {
			formats = append(formats, FormatFloat(input))
		}

		// All formats should be identical
		for i := 1; i < len(formats); i++ {
			if formats[0] != formats[i] {
				t.Errorf("FormatFloat(%v) is not deterministic: %v != %v", input, formats[0], formats[i])
			}
		}
	}
}
