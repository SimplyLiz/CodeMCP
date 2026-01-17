package architecture

import (
	"ckb/internal/modules"
)

// Granularity specifies the level of detail for architecture visualization
type Granularity string

const (
	// GranularityModule provides module-level view (default, existing behavior)
	GranularityModule Granularity = "module"
	// GranularityDirectory provides directory-level aggregation
	GranularityDirectory Granularity = "directory"
	// GranularityFile provides individual file dependencies
	GranularityFile Granularity = "file"
)

// ParseGranularity converts a string to Granularity, defaulting to module
func ParseGranularity(s string) Granularity {
	switch s {
	case "directory":
		return GranularityDirectory
	case "file":
		return GranularityFile
	default:
		return GranularityModule
	}
}

// ArchitectureResponse contains the complete architecture view of the repository
// Section 16.5: Architecture response structure
// Extended in v8.0 to support directory and file-level granularity
type ArchitectureResponse struct {
	// Module-level fields (granularity=module)
	Modules         []ModuleSummary  `json:"modules,omitempty"`
	DependencyGraph []DependencyEdge `json:"dependencyGraph,omitempty"`
	Entrypoints     []Entrypoint     `json:"entrypoints,omitempty"`

	// Directory-level fields (granularity=directory)
	Directories           []DirectorySummary        `json:"directories,omitempty"`
	DirectoryDependencies []DirectoryDependencyEdge `json:"directoryDependencies,omitempty"`

	// File-level fields (granularity=file)
	Files            []FileSummary        `json:"files,omitempty"`
	FileDependencies []FileDependencyEdge `json:"fileDependencies,omitempty"`

	// Metadata (always present)
	Granularity     Granularity `json:"granularity"`
	DetectionMethod string      `json:"detectionMethod"` // "manifest", "convention", "inferred", "fallback"
}

// ModuleSummary provides aggregated statistics for a module
type ModuleSummary struct {
	ModuleId    string `json:"moduleId"`
	Name        string `json:"name"`
	RootPath    string `json:"rootPath"`
	Language    string `json:"language"`
	FileCount   int    `json:"fileCount"`
	SymbolCount int    `json:"symbolCount"`
	LOC         int    `json:"loc"` // Lines of code
}

// DependencyEdge represents a dependency relationship between modules
type DependencyEdge struct {
	From     string                 `json:"from"`     // ModuleId
	To       string                 `json:"to"`       // ModuleId
	Kind     modules.ImportEdgeKind `json:"kind"`     // Classified per Section 5.2
	Strength int                    `json:"strength"` // Reference count
}

// Entrypoint represents an entry point file in a module
type Entrypoint struct {
	FileId   string `json:"fileId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // "main", "cli", "server", "test"
	ModuleId string `json:"moduleId"`
}

// EntrypointKind constants
const (
	EntrypointMain   = "main"
	EntrypointCLI    = "cli"
	EntrypointServer = "server"
	EntrypointTest   = "test"
)

// ExternalDependency represents an external package dependency
// Section 16.5: External dependency tracking (filtered by default)
type ExternalDependency struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Source  string `json:"source"` // "npm", "pub", "cargo", etc.
}

// DirectoryMetrics contains aggregate metrics for visualization
// Added in v8.0 to support metric-based visualization (size = LOC, color = complexity)
type DirectoryMetrics struct {
	LOC           int     `json:"loc"`                      // Total lines of code
	AvgComplexity float64 `json:"avgComplexity,omitempty"`  // Average cyclomatic complexity
	MaxComplexity int     `json:"maxComplexity,omitempty"`  // Highest single-function complexity
	LastModified  string  `json:"lastModified,omitempty"`   // ISO 8601 timestamp of most recent change
	Churn30d      int     `json:"churn30d,omitempty"`       // Commit count in last 30 days
}

// DirectorySummary represents a directory in directory-level architecture views
type DirectorySummary struct {
	Path           string            `json:"path"`                     // Relative path from repo root
	FileCount      int               `json:"fileCount"`                // Number of source files
	SymbolCount    int               `json:"symbolCount"`              // Symbols defined (if SCIP available)
	Language       string            `json:"language"`                 // Dominant language
	LOC            int               `json:"loc"`                      // Lines of code
	Role           string            `json:"role,omitempty"`           // Inferred role: api, ui, data, util, test, config, entrypoint, core
	HasIndexFile   bool              `json:"hasIndexFile"`             // Contains index.ts/js, mod.rs, __init__.py
	IncomingEdges  int               `json:"incomingEdges"`            // Dependencies pointing here
	OutgoingEdges  int               `json:"outgoingEdges"`            // Dependencies from this directory
	IsIntermediate bool              `json:"isIntermediate,omitempty"` // Added to complete hierarchy (no direct files)
	Metrics        *DirectoryMetrics `json:"metrics,omitempty"`        // Aggregate metrics (when includeMetrics=true)
}

// FileSummary represents a file in file-level architecture views
type FileSummary struct {
	Path          string `json:"path"`          // Relative path from repo root
	Language      string `json:"language"`      // Detected language
	SymbolCount   int    `json:"symbolCount"`   // Symbols defined
	LOC           int    `json:"loc"`           // Lines of code
	IncomingEdges int    `json:"incomingEdges"` // Files importing this file
	OutgoingEdges int    `json:"outgoingEdges"` // Files this file imports
}

// FileDependencyEdge represents a dependency between files
type FileDependencyEdge struct {
	From     string                 `json:"from"`           // FileId (relative path)
	To       string                 `json:"to"`             // FileId or external package name
	Kind     modules.ImportEdgeKind `json:"kind"`           // Classification
	Line     int                    `json:"line,omitempty"` // Source line number
	Resolved bool                   `json:"resolved"`       // Whether target was resolved to a file
}

// DirectoryDependencyEdge represents a dependency between directories
type DirectoryDependencyEdge struct {
	From        string                 `json:"from"`                  // Directory path
	To          string                 `json:"to"`                    // Directory path or external package
	Kind        modules.ImportEdgeKind `json:"kind,omitempty"`        // Classification
	ImportCount int                    `json:"importCount"`           // Number of import statements
	Symbols     []string               `json:"symbols,omitempty"`     // Imported symbol names (for tooltip/detail)
	Strength    int                    `json:"strength,omitempty"`    // Deprecated: use importCount instead
}
