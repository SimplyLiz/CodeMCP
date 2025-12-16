package architecture

import (
	"ckb/internal/modules"
)

// ArchitectureResponse contains the complete architecture view of the repository
// Section 16.5: Architecture response structure
type ArchitectureResponse struct {
	Modules         []ModuleSummary  `json:"modules"`
	DependencyGraph []DependencyEdge `json:"dependencyGraph"`
	Entrypoints     []Entrypoint     `json:"entrypoints"`
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
