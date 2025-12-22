//go:build !cgo

// Package docs provides documentation ↔ symbol linking.
package docs

// FenceParser extracts identifiers from fenced code blocks.
// This is a stub implementation for non-CGO builds.
type FenceParser struct{}

// NewFenceParser creates a new fence parser.
// Returns nil when CGO is disabled.
func NewFenceParser() *FenceParser {
	return nil
}

// Fence represents a fenced code block in markdown.
type Fence struct {
	Language  string
	StartLine int
	EndLine   int
	Content   string
}

// FenceIdentifier represents an identifier found in a fence.
type FenceIdentifier struct {
	Name       string
	Line       int
	Confidence float64
}

// ExtractIdentifiers is a stub that returns nil when CGO is disabled.
func (fp *FenceParser) ExtractIdentifiers(fence Fence) []FenceIdentifier {
	return nil
}
