// Package docs provides documentation ↔ symbol linking with staleness detection.
// It enables answering questions like "What docs mention this symbol?",
// "What symbols does this doc reference?", and "Which docs are stale?"
package docs

import "time"

// DocType represents the type of documentation file.
type DocType string

const (
	DocTypeMarkdown DocType = "markdown"
	DocTypeADR      DocType = "adr"
)

// DetectionMethod represents how a symbol reference was found in documentation.
type DetectionMethod string

const (
	DetectBacktick  DetectionMethod = "backtick"  // `Symbol.Name`
	DetectDirective DetectionMethod = "directive" // <!-- ckb:symbol Symbol.Name -->
	DetectFence     DetectionMethod = "fence"     // v1.1: Identifier in fenced code block
)

// ResolutionStatus represents the result of symbol resolution.
type ResolutionStatus string

const (
	ResolutionExact      ResolutionStatus = "exact"      // Fully qualified match
	ResolutionSuffix     ResolutionStatus = "suffix"     // Unique suffix match
	ResolutionAmbiguous  ResolutionStatus = "ambiguous"  // Multiple candidates
	ResolutionMissing    ResolutionStatus = "missing"    // Not found in index
	ResolutionIneligible ResolutionStatus = "ineligible" // Too short / doesn't meet criteria
)

// StalenessReason explains why a reference is stale.
type StalenessReason string

const (
	StalenessMissing   StalenessReason = "missing_symbol"   // Not in index at all
	StalenessAmbiguous StalenessReason = "ambiguous_symbol" // Matches multiple
	StalenessIndexGap  StalenessReason = "index_incomplete" // Language/tier not indexed
	StalenessRenamed   StalenessReason = "symbol_renamed"   // Symbol was renamed/moved (v1.1)
)

// Document represents an indexed documentation file.
type Document struct {
	Path        string         `json:"path"`         // Relative path from repo root
	Type        DocType        `json:"type"`         // markdown, adr
	Title       string         `json:"title"`        // Extracted from first heading or filename
	Hash        string         `json:"hash"`         // SHA256 for change detection
	LastIndexed time.Time      `json:"last_indexed"` // When last indexed
	References  []DocReference `json:"references"`   // Symbol references in this doc
	Modules     []string       `json:"modules"`      // Linked modules via directive
}

// DocReference links a document location to a symbol mention.
type DocReference struct {
	ID              int64            `json:"id"`                    // Database ID
	DocPath         string           `json:"doc_path"`              // Path to document
	RawText         string           `json:"raw_text"`              // Exactly as written: `UserService.Auth`
	NormalizedText  string           `json:"normalized_text"`       // Normalized: UserService.Auth
	SymbolID        *string          `json:"symbol_id,omitempty"`   // Resolved SCIP symbol ID (nil if unresolved)
	SymbolName      string           `json:"symbol_name,omitempty"` // Human-friendly display name
	Line            int              `json:"line"`                  // 1-indexed line number
	Column          int              `json:"column"`                // 1-indexed column
	Context         string           `json:"context,omitempty"`     // Surrounding text snippet (±100 chars)
	DetectionMethod DetectionMethod  `json:"detection_method"`      // How it was detected
	Resolution      ResolutionStatus `json:"resolution"`            // Resolution result
	Candidates      []string         `json:"candidates,omitempty"`  // If ambiguous, list of candidate symbol IDs
	Confidence      float64          `json:"confidence"`            // 0.0-1.0
	LastResolved    time.Time        `json:"last_resolved"`         // When resolution was last attempted
}

// DocModuleLink represents a manual module↔doc link via directive.
type DocModuleLink struct {
	DocPath  string `json:"doc_path"`
	ModuleID string `json:"module_id"` // CKB canonical module ID
	Line     int    `json:"line"`      // Where directive appears
}

// Mention is a raw symbol mention found during scanning (before resolution).
type Mention struct {
	RawText string          `json:"raw_text"`
	Line    int             `json:"line"`
	Column  int             `json:"column"`
	Context string          `json:"context"`
	Method  DetectionMethod `json:"method"`
}

// ModuleLink is a module directive found during scanning.
type ModuleLink struct {
	ModuleID string `json:"module_id"`
	Line     int    `json:"line"`
}

// ScanResult holds the result of scanning a document.
type ScanResult struct {
	Doc          Document     `json:"doc"`
	Mentions     []Mention    `json:"mentions"`
	Modules      []ModuleLink `json:"modules"`
	KnownSymbols []string     `json:"known_symbols,omitempty"` // v1.1: From ckb:known_symbols directive
	Error        error        `json:"-"`
}

// ResolutionResult holds the result of attempting to resolve a mention.
type ResolutionResult struct {
	Status     ResolutionStatus `json:"status"`
	SymbolID   string           `json:"symbol_id,omitempty"`
	SymbolName string           `json:"symbol_name,omitempty"`
	Candidates []string         `json:"candidates,omitempty"`
	Confidence float64          `json:"confidence"`
	Message    string           `json:"message,omitempty"`
}

// IndexStats contains statistics from a doc indexing run.
type IndexStats struct {
	DocsIndexed     int `json:"docs_indexed"`
	DocsSkipped     int `json:"docs_skipped"` // Unchanged (same hash)
	ReferencesFound int `json:"references_found"`
	Resolved        int `json:"resolved"`
	Ambiguous       int `json:"ambiguous"`
	Missing         int `json:"missing"`
	Ineligible      int `json:"ineligible"`
}

// StalenessReport contains the result of checking a document for stale references.
type StalenessReport struct {
	DocPath            string           `json:"doc_path"`
	TotalReferences    int              `json:"total_references"`
	Valid              int              `json:"valid"`
	Stale              []StaleReference `json:"stale,omitempty"`
	CheckedAt          time.Time        `json:"checked_at"`
	SymbolIndexVersion string           `json:"symbol_index_version,omitempty"`
}

// StaleReference describes a single stale symbol reference.
type StaleReference struct {
	RawText     string          `json:"raw_text"`
	Line        int             `json:"line"`
	Reason      StalenessReason `json:"reason"`
	Message     string          `json:"message"`
	Suggestions []string        `json:"suggestions,omitempty"`   // Possible matches
	NewSymbolID *string         `json:"new_symbol_id,omitempty"` // If renamed, the new ID (v1.1)
}

// CoverageReport contains documentation coverage analysis.
type CoverageReport struct {
	TotalSymbols    int              `json:"total_symbols"`
	Documented      int              `json:"documented"`
	Undocumented    int              `json:"undocumented"`
	CoveragePercent float64          `json:"coverage_percent"`
	TopUndocumented []UndocSymbol    `json:"top_undocumented,omitempty"`
	ByModule        []ModuleCoverage `json:"by_module,omitempty"`
}

// UndocSymbol represents an undocumented symbol in coverage reports.
type UndocSymbol struct {
	SymbolID   string  `json:"symbol_id"`
	Name       string  `json:"name"`
	Kind       string  `json:"kind"`
	File       string  `json:"file"`
	Centrality float64 `json:"centrality"` // Higher = more important
}

// ModuleCoverage shows coverage for a single module.
type ModuleCoverage struct {
	ModuleID        string  `json:"module_id"`
	TotalSymbols    int     `json:"total_symbols"`
	Documented      int     `json:"documented"`
	CoveragePercent float64 `json:"coverage_percent"`
}
