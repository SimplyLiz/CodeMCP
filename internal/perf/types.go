// Package perf detects performance and structural health issues in a codebase.
// Currently focuses on hidden coupling: file pairs that co-change frequently
// in git but have no static import edge between them, indicating implicit
// shared state or behavioral coupling that the static graph cannot see.
package perf

import "time"

// ScanOptions configures a performance scan.
type ScanOptions struct {
	// Scope limits analysis to these paths (relative to repo root).
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

	// MaxCommitFiles skips commits that touch more than this many files.
	// Mass renames and formatting sweeps produce O(files²) pairs that dominate
	// the pairCounts map without contributing useful coupling signal.
	// 0 means unlimited (no commits are skipped).
	MaxCommitFiles int
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

// StructuralPerfOptions configures a structural performance scan.
type StructuralPerfOptions struct {
	// Scope limits analysis to these paths (relative to RepoRoot). Empty = whole repo.
	Scope []string
	// Limit caps the number of loop call sites returned. Default: 100.
	Limit int
	// WindowDays is the git history window to consider. Default: 90.
	WindowDays int
	// MinChurnCount is the minimum commit count for a file to be considered hot. Default: 3.
	MinChurnCount int
	// EntrypointFiles are repo-relative paths of known system entrypoints.
	// Loop call sites in these files are marked NearEntrypoint and ranked higher.
	EntrypointFiles []string
}

// LoopCallSite represents a function call expression found inside a loop body
// in a high-churn file. These are the primary structural signal for O(n) or
// O(n²) hidden costs that do not appear in profiling until production load.
type LoopCallSite struct {
	File           string `json:"file"`           // repo-relative path
	Line           int    `json:"line"`            // 1-indexed line of the call expression
	FunctionName   string `json:"functionName"`   // enclosing function/method name
	CallText       string `json:"callText"`       // call expression text (truncated to 120 chars)
	LoopType       string `json:"loopType"`       // "for", "range", "while", "do-while", "loop"
	ChurnCount     int    `json:"churnCount"`     // commits touching this file in the window
	NearEntrypoint bool   `json:"nearEntrypoint"` // true if file is a known system entrypoint
	Severity       string `json:"severity"`       // "high", "medium", "low"
	Explanation    string `json:"explanation"`    // human-readable description
}

// StructuralPerfSummary aggregates the structural scan results.
type StructuralPerfSummary struct {
	FilesScanned   int `json:"filesScanned"`
	HotFilesFound  int `json:"hotFilesFound"`
	CallSitesFound int `json:"callSitesFound"`
}

// StructuralPerfResult is the output of a structural performance scan.
type StructuralPerfResult struct {
	LoopCallSites []LoopCallSite        `json:"loopCallSites"`
	Summary       StructuralPerfSummary `json:"summary"`
	// NoCGO is true when tree-sitter analysis was unavailable (non-CGO build).
	NoCGO bool `json:"noCGO,omitempty"`
}
