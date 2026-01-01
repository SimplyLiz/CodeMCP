package main

import "fmt"

// Add returns the sum of two integers.
func Add(a, b int) int {
	return a + b
}

// Subtract returns the difference of two integers.
func Subtract(a, b int) int {
	return a - b
}

// formatGreeting formats a greeting message.
func formatGreeting(name string) string {
	return fmt.Sprintf("Hello, %s!", name)
}

// ValidateName checks if a name is valid.
func ValidateName(name string) bool {
	return len(name) > 0
}
