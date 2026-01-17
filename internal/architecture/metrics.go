package architecture

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ckb/internal/backends/git"
	"ckb/internal/complexity"
)

// MetricsCalculator computes aggregate metrics for directories
type MetricsCalculator struct {
	repoRoot   string
	gitAdapter *git.GitAdapter
	analyzer   *complexity.Analyzer
}

// NewMetricsCalculator creates a metrics calculator
func NewMetricsCalculator(repoRoot string, gitAdapter *git.GitAdapter) *MetricsCalculator {
	return &MetricsCalculator{
		repoRoot:   repoRoot,
		gitAdapter: gitAdapter,
		analyzer:   complexity.NewAnalyzer(),
	}
}

// ComputeDirectoryMetrics computes aggregate metrics for directories
// Modifies the DirectorySummary slice in place to add Metrics
func (m *MetricsCalculator) ComputeDirectoryMetrics(ctx context.Context, directories []DirectorySummary) error {
	if len(directories) == 0 {
		return nil
	}

	// Build a map for quick lookup
	dirMap := make(map[string]*DirectorySummary)
	for i := range directories {
		dirMap[directories[i].Path] = &directories[i]
	}

	// Collect all source files and their directories
	type fileInfo struct {
		path   string
		dir    string
		relDir string
	}
	var files []fileInfo

	for _, dir := range directories {
		if dir.IsIntermediate {
			// Skip intermediate directories - they have no direct files
			continue
		}

		absDir := filepath.Join(m.repoRoot, dir.Path)
		entries, err := os.ReadDir(absDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if isMetricsSourceFile(entry.Name()) {
				files = append(files, fileInfo{
					path:   filepath.Join(absDir, entry.Name()),
					dir:    absDir,
					relDir: dir.Path,
				})
			}
		}
	}

	// Compute complexity for all files (batch)
	fileComplexity := make(map[string]*complexity.FileComplexity)
	if complexity.IsAvailable() {
		for _, f := range files {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			fc, err := m.analyzer.AnalyzeFile(ctx, f.path)
			if err == nil && fc != nil {
				fileComplexity[f.path] = fc
			}
		}
	}

	// Get git churn data
	var gitChurn map[string]*git.ChurnMetrics
	if m.gitAdapter != nil && m.gitAdapter.IsAvailable() {
		// Get hotspots for last 30 days
		since := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
		hotspots, err := m.gitAdapter.GetHotspots(500, since) // Get more to cover all files
		if err == nil {
			gitChurn = make(map[string]*git.ChurnMetrics)
			for i := range hotspots {
				gitChurn[hotspots[i].FilePath] = &hotspots[i]
			}
		}
	}

	// Aggregate metrics per directory
	for i := range directories {
		dir := &directories[i]
		metrics := &DirectoryMetrics{
			LOC: dir.LOC, // Use existing LOC if already computed
		}

		var totalComplexity int
		var functionCount int
		var maxComplexity int
		var latestModified time.Time
		var totalChurn int

		absDir := filepath.Join(m.repoRoot, dir.Path)

		for _, f := range files {
			if f.dir != absDir {
				continue
			}

			// Aggregate complexity
			if fc, ok := fileComplexity[f.path]; ok {
				totalComplexity += fc.TotalCyclomatic
				functionCount += fc.FunctionCount
				if fc.MaxCyclomatic > maxComplexity {
					maxComplexity = fc.MaxCyclomatic
				}
			}

			// Aggregate git metrics
			relPath := strings.TrimPrefix(f.path, m.repoRoot+"/")
			if churn, ok := gitChurn[relPath]; ok {
				totalChurn += churn.ChangeCount
				if churn.LastModified != "" {
					t, err := time.Parse(time.RFC3339, churn.LastModified)
					if err == nil && t.After(latestModified) {
						latestModified = t
					}
				}
			}
		}

		// Set computed metrics
		if functionCount > 0 {
			metrics.AvgComplexity = float64(totalComplexity) / float64(functionCount)
		}
		if maxComplexity > 0 {
			metrics.MaxComplexity = maxComplexity
		}
		if !latestModified.IsZero() {
			metrics.LastModified = latestModified.Format(time.RFC3339)
		}
		if totalChurn > 0 {
			metrics.Churn30d = totalChurn
		}

		// Only set metrics if we computed something meaningful
		if metrics.LOC > 0 || metrics.AvgComplexity > 0 || metrics.MaxComplexity > 0 ||
			metrics.LastModified != "" || metrics.Churn30d > 0 {
			dir.Metrics = metrics
		}
	}

	return nil
}

// isMetricsSourceFile checks if a filename is a source file for metrics computation
func isMetricsSourceFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".mjs",
		".py", ".rs", ".java", ".kt", ".kts", ".dart",
		".c", ".cpp", ".h", ".hpp", ".cs", ".rb":
		return true
	}
	return false
}
