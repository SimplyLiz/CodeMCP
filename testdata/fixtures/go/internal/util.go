// Package internal provides internal utilities (not exported from module).
package internal

import (
	"errors"
	"strings"
)

// ErrEmptyInput is returned when input is empty.
var ErrEmptyInput = errors.New("input cannot be empty")

// FormatOutput formats the output string.
// This is called from multiple places to test reverse references.
func FormatOutput(s string) string {
	return "[output] " + strings.TrimSpace(s)
}

// ValidateInput checks if input is valid.
func ValidateInput(input string) error {
	if input == "" {
		return ErrEmptyInput
	}
	if len(input) > 1000 {
		return errors.New("input too long")
	}
	return nil
}

// SanitizeInput cleans the input string.
func SanitizeInput(input string) string {
	return strings.TrimSpace(strings.ToLower(input))
}
