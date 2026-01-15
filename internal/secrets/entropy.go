package secrets

import (
	"math"
	"unicode"
)

// ShannonEntropy calculates the Shannon entropy of a string.
// Higher entropy indicates more randomness, which is characteristic of secrets.
// Typical thresholds:
//   - < 2.0: Very low entropy (likely not a secret)
//   - 2.0-3.0: Low entropy (probably not a secret)
//   - 3.0-4.0: Medium entropy (possible secret, needs verification)
//   - > 4.0: High entropy (likely a secret)
func ShannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}

	// Count character frequencies
	freq := make(map[rune]int)
	for _, r := range s {
		freq[r]++
	}

	// Calculate entropy
	length := float64(len(s))
	var entropy float64

	for _, count := range freq {
		if count > 0 {
			p := float64(count) / length
			entropy -= p * math.Log2(p)
		}
	}

	return entropy
}

// CharacterClassEntropy calculates entropy based on character class distribution.
// This is useful for detecting passwords that may have lower Shannon entropy
// but use multiple character classes.
func CharacterClassEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}

	var (
		hasLower   bool
		hasUpper   bool
		hasDigit   bool
		hasSpecial bool
	)

	for _, r := range s {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}

	// Count character classes used
	classes := 0
	if hasLower {
		classes++
	}
	if hasUpper {
		classes++
	}
	if hasDigit {
		classes++
	}
	if hasSpecial {
		classes++
	}

	// Base entropy on classes and length
	// 4 classes = max bonus of 1.0
	classBonus := float64(classes) * 0.25

	// Length contributes to entropy
	lengthFactor := math.Min(float64(len(s))/20.0, 1.0)

	return ShannonEntropy(s) + (classBonus * lengthFactor)
}

// IsHighEntropy checks if a string has entropy above the threshold.
func IsHighEntropy(s string, threshold float64) bool {
	return ShannonEntropy(s) >= threshold
}

// IsProbablySecret combines multiple heuristics to determine if
// a string is likely to be a secret.
func IsProbablySecret(s string, minEntropy float64) bool {
	if len(s) < 8 {
		return false
	}

	// Check Shannon entropy
	entropy := ShannonEntropy(s)
	if entropy < minEntropy {
		return false
	}

	// Check for common non-secret patterns
	if isLikelyPlaceholder(s) {
		return false
	}

	return true
}

// isLikelyPlaceholder checks for common placeholder patterns.
func isLikelyPlaceholder(s string) bool {
	placeholders := []string{
		"example",
		"placeholder",
		"your_",
		"xxx",
		"changeme",
		"password",
		"secret",
		"dummy",
		"sample",
		"test123",
		"abc123",
		"foobar",
		"<your",
		"${",
		"{{",
		"REPLACE",
		"INSERT",
		"FIXME",
		"TODO",
	}

	lower := toLower(s)
	for _, p := range placeholders {
		if containsIgnoreCase(lower, p) {
			return true
		}
	}

	return false
}

// toLower converts string to lowercase (simple ASCII version).
func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			sc := s[i+j]
			pc := substr[j]
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if pc >= 'A' && pc <= 'Z' {
				pc += 32
			}
			if sc != pc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
