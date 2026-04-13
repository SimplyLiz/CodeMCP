//go:build !cgo

package perf

import "context"

// AnalyzeStructural is a stub for non-CGO builds.
// Tree-sitter loop-call-site detection requires CGO.
func (a *Analyzer) AnalyzeStructural(_ context.Context, opts StructuralPerfOptions) (*StructuralPerfResult, error) {
	return &StructuralPerfResult{
		NoCGO: true,
		Summary: StructuralPerfSummary{
			FilesScanned:   0,
			HotFilesFound:  0,
			CallSitesFound: 0,
		},
	}, nil
}
