// Package export provides LLM-friendly codebase export functionality.
// It outputs codebase structure in a format optimized for LLM context windows.
package export

// LLMExport is the main export structure
type LLMExport struct {
	Metadata ExportMetadata  `json:"metadata"`
	Modules  []ExportModule  `json:"modules"`
}

// ExportMetadata contains metadata about the export
type ExportMetadata struct {
	Repo         string `json:"repo"`
	Generated    string `json:"generated"` // ISO 8601 timestamp
	SymbolCount  int    `json:"symbolCount"`
	FileCount    int    `json:"fileCount"`
	ModuleCount  int    `json:"moduleCount"`
}

// ExportModule represents a module in the export
type ExportModule struct {
	Path   string       `json:"path"`
	Owner  string       `json:"owner,omitempty"`
	Files  []ExportFile `json:"files"`
}

// ExportFile represents a file in the export
type ExportFile struct {
	Name    string         `json:"name"`
	Symbols []ExportSymbol `json:"symbols"`
}

// ExportSymbol represents a symbol in the export
type ExportSymbol struct {
	Type        string   `json:"type"`                  // "class" | "function" | "interface" | "constant"
	Name        string   `json:"name"`
	Complexity  int      `json:"complexity,omitempty"`
	CallsPerDay int      `json:"callsPerDay,omitempty"`
	Importance  int      `json:"importance,omitempty"` // 1-3 stars
	Contracts   []string `json:"contracts,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
	IsInterface bool     `json:"isInterface,omitempty"`
	IsExported  bool     `json:"isExported,omitempty"`
	Line        int      `json:"line,omitempty"`
}

// ExportOptions configures the export
type ExportOptions struct {
	RepoRoot          string // Repository root path
	Federation        string // Federation name (optional)
	IncludeUsage      bool   // Include telemetry data (default: true)
	IncludeOwnership  bool   // Include owner annotations (default: true)
	IncludeContracts  bool   // Include contract indicators (default: true)
	IncludeComplexity bool   // Include complexity scores (default: true)
	MinComplexity     int    // Only include symbols with complexity >= N
	MinCalls          int    // Only include symbols with calls/day >= N
	MaxSymbols        int    // Limit total symbols (default: unlimited)
	Format            string // Output format: "text" | "json" | "markdown"
}

// SymbolType constants
const (
	SymbolTypeClass     = "class"
	SymbolTypeFunction  = "function"
	SymbolTypeInterface = "interface"
	SymbolTypeConstant  = "constant"
	SymbolTypeVariable  = "variable"
	SymbolTypeMethod    = "method"
)

// ImportanceLevel represents the importance of a symbol
type ImportanceLevel int

const (
	ImportanceLow    ImportanceLevel = 1
	ImportanceMedium ImportanceLevel = 2
	ImportanceHigh   ImportanceLevel = 3
)

// CalculateImportance calculates importance based on usage and complexity
func CalculateImportance(callsPerDay, complexity int) ImportanceLevel {
	// Score based on usage and complexity
	score := 0

	// Usage component
	if callsPerDay >= 10000 {
		score += 3
	} else if callsPerDay >= 1000 {
		score += 2
	} else if callsPerDay >= 100 {
		score += 1
	}

	// Complexity component
	if complexity >= 30 {
		score += 2
	} else if complexity >= 15 {
		score += 1
	}

	// Map to importance level
	if score >= 4 {
		return ImportanceHigh
	} else if score >= 2 {
		return ImportanceMedium
	}
	return ImportanceLow
}
