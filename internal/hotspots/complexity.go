package hotspots

import (
	"context"

	"ckb/internal/complexity"
)

// ComplexityAnalyzer provides complexity metrics for hotspot analysis.
type ComplexityAnalyzer struct {
	analyzer *complexity.Analyzer
}

// NewComplexityAnalyzer creates a new complexity analyzer for hotspots.
func NewComplexityAnalyzer() *ComplexityAnalyzer {
	return &ComplexityAnalyzer{
		analyzer: complexity.NewAnalyzer(),
	}
}

// GetFileComplexity returns complexity metrics for a file.
// Returns cyclomatic and cognitive complexity, or (0, 0) for unsupported files.
func (ca *ComplexityAnalyzer) GetFileComplexity(ctx context.Context, path string) (cyclomatic, cognitive float64, err error) {
	fc, err := ca.analyzer.AnalyzeFile(ctx, path)
	if err != nil {
		return 0, 0, err
	}

	if fc.Error != "" {
		// Unsupported file type or read error - return 0 without error
		return 0, 0, nil
	}

	return float64(fc.MaxCyclomatic), float64(fc.MaxCognitive), nil
}

// GetFileComplexityFull returns the full complexity analysis for a file.
func (ca *ComplexityAnalyzer) GetFileComplexityFull(ctx context.Context, path string) (*complexity.FileComplexity, error) {
	return ca.analyzer.AnalyzeFile(ctx, path)
}

// IsSupported returns true if complexity analysis is available for the file extension.
func IsSupported(path string) bool {
	ext := ""
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			ext = path[i:]
			break
		}
	}
	if ext == "" {
		return false
	}
	_, ok := complexity.LanguageFromExtension(ext)
	return ok
}

// SupportedExtensions returns all file extensions that support complexity analysis.
func SupportedExtensions() []string {
	return []string{
		".go",
		".js", ".mjs", ".cjs", ".jsx",
		".ts", ".mts", ".cts", ".tsx",
		".py", ".pyw",
		".rs",
		".java",
		".kt", ".kts",
	}
}
