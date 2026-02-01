//go:build !cgo

package extract

import "context"

// Analyzer performs variable flow analysis for extract refactoring.
// This is a stub implementation for non-CGO builds.
type Analyzer struct{}

// NewAnalyzer creates a new extract flow analyzer.
// Returns nil when CGO is disabled.
func NewAnalyzer() *Analyzer {
	return nil
}

// Analyze performs variable flow analysis.
// Stub implementation returns nil (graceful degradation).
func (a *Analyzer) Analyze(ctx context.Context, opts AnalyzeOptions) (*FlowAnalysis, error) {
	return nil, nil
}
