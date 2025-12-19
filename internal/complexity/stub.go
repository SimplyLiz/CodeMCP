//go:build !cgo

package complexity

import (
	"context"
	"errors"
)

// ErrNoCGO is returned when complexity analysis is unavailable due to missing CGO.
var ErrNoCGO = errors.New("complexity analysis requires CGO (tree-sitter)")

// Analyzer computes complexity metrics for source files.
// This is a stub implementation for non-CGO builds.
type Analyzer struct{}

// NewAnalyzer creates a new complexity analyzer.
// Returns nil when CGO is disabled.
func NewAnalyzer() *Analyzer {
	return nil
}

// AnalyzeFile analyzes a single file and returns complexity metrics.
// Stub implementation returns an error.
func (a *Analyzer) AnalyzeFile(ctx context.Context, path string) (*FileComplexity, error) {
	return nil, ErrNoCGO
}

// AnalyzeSource analyzes source code bytes.
// Stub implementation returns an error.
func (a *Analyzer) AnalyzeSource(ctx context.Context, source []byte, lang Language) (*FileComplexity, error) {
	return nil, ErrNoCGO
}

// Parser wraps tree-sitter parsing functionality.
// This is a stub implementation for non-CGO builds.
type Parser struct{}

// NewParser creates a new tree-sitter parser.
// Returns nil when CGO is disabled.
func NewParser() *Parser {
	return nil
}

// IsAvailable returns whether complexity analysis is available.
// Returns false when CGO is disabled.
func IsAvailable() bool {
	return false
}
