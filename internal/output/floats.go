package output

import (
	"math"
	"strconv"
	"strings"
)

// RoundFloat rounds a float to max 6 decimal places and removes trailing zeros
func RoundFloat(f float64) float64 {
	// Round to 6 decimal places
	multiplier := math.Pow(10, 6)
	return math.Round(f*multiplier) / multiplier
}

// FormatFloat formats a float with no trailing zeros
func FormatFloat(f float64) string {
	// Round to 6 decimal places first
	rounded := RoundFloat(f)

	// Format with 6 decimal places
	str := strconv.FormatFloat(rounded, 'f', 6, 64)

	// Remove trailing zeros
	str = strings.TrimRight(str, "0")

	// Remove trailing decimal point if no decimals remain
	str = strings.TrimRight(str, ".")

	return str
}

// NormalizeFloat normalizes a float for deterministic output
// Returns the rounded value suitable for JSON encoding
func NormalizeFloat(f float64) float64 {
	return RoundFloat(f)
}
