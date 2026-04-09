// Package perf detects performance and structural health issues in a codebase.
// Currently focuses on hidden coupling: file pairs that co-change frequently
// in git but have no static import edge between them, indicating implicit
// shared state or behavioral coupling that the static graph cannot see.
package perf

import "time"

// ScanOptions configures a performance scan.
type ScanOptions struct {
	// RepoRoot is the root of the repository.
	RepoRoot string

	// Scope limits analysis to these paths (relative to RepoRoot).
	// Empty means whole repo.
	Scope []string

	// MinCorrelation is the minimum co-change correlation to report (0–1).
	// Default: 0.3
	MinCorrelation float64

	// MinCoChanges is the minimum absolute number of shared commits.
	// Filters out spurious pairs from low-activity files. Default: 3
	MinCoChanges int

	// WindowDays is the git history window to consider. Default: 365
	WindowDays int

	// Limit caps the number of hidden-coupling pairs returned. Default: 50
	Limit int
}

// HiddenCouplingPair represents two files that co-change without any static
// import edge between them. This is the primary signal: implicit coupling that
// the dependency graph cannot explain.
type HiddenCouplingPair struct {
	// FileA and FileB are repo-relative paths of the coupled files.
	FileA string `json:"fileA"`
	FileB string `json:"fileB"`

	// Correlation is the co-change ratio: sharedCommits / min(totalA, totalB).
	Correlation float64 `json:"correlation"`

	// CoChangeCount is the raw number of commits that touched both files.
	CoChangeCount int `json:"coChangeCount"`

	// Level is "high" (≥0.8), "medium" (≥0.5), or "low".
	Level string `json:"level"`

	// Explanation is a human-readable description of why this is notable.
	Explanation string `json:"explanation"`
}

// PerfScanSummary aggregates the scan results.
type PerfScanSummary struct {
	FilesObserved    int       `json:"filesObserved"`
	PairsChecked     int       `json:"pairsChecked"`
	HiddenPairsFound int       `json:"hiddenPairsFound"`
	AnalysisFrom     time.Time `json:"analysisFrom"`
	AnalysisTo       time.Time `json:"analysisTo"`
}

// PerfScanResult is the output of a perf scan.
type PerfScanResult struct {
	HiddenCoupling []HiddenCouplingPair `json:"hiddenCoupling"`
	Summary        PerfScanSummary      `json:"summary"`
}
