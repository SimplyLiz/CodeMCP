package modules

// ImportEdgeKind represents the classification of an import dependency
type ImportEdgeKind string

const (
	// LocalFile represents a relative import to another file in the same module (./foo, ../bar)
	LocalFile ImportEdgeKind = "local-file"

	// LocalModule represents an import to another module in the same workspace
	LocalModule ImportEdgeKind = "local-module"

	// WorkspacePackage represents an import to a sibling package in a monorepo
	WorkspacePackage ImportEdgeKind = "workspace-package"

	// ExternalDependency represents an import to an external package (npm/pub/cargo/etc)
	ExternalDependency ImportEdgeKind = "external-dependency"

	// Stdlib represents an import to a standard library (dart:core, node:*, Go builtins)
	Stdlib ImportEdgeKind = "stdlib"

	// Unknown represents an import that couldn't be classified
	Unknown ImportEdgeKind = "unknown"
)

// ImportEdge represents a dependency edge between files or modules
type ImportEdge struct {
	// From is the FileId (repo-relative path) of the importing file
	From string `json:"from"`

	// To may be a path (for local imports) or a package name (for external imports)
	To string `json:"to"`

	// Kind is the classification of this import
	Kind ImportEdgeKind `json:"kind"`

	// Confidence is a value between 0 and 1 indicating classification confidence
	Confidence float64 `json:"confidence"`

	// RawImport is the original import string as it appears in the source code
	RawImport string `json:"rawImport"`

	// Line is the line number where this import appears (optional)
	Line int `json:"line,omitempty"`
}

// NewImportEdge creates a new ImportEdge with the given parameters
func NewImportEdge(from, to string, kind ImportEdgeKind, confidence float64, rawImport string) *ImportEdge {
	return &ImportEdge{
		From:       from,
		To:         to,
		Kind:       kind,
		Confidence: confidence,
		RawImport:  rawImport,
	}
}

// IsLocal returns true if this is a local import (local-file or local-module)
func (e *ImportEdge) IsLocal() bool {
	return e.Kind == LocalFile || e.Kind == LocalModule
}

// IsExternal returns true if this is an external dependency
func (e *ImportEdge) IsExternal() bool {
	return e.Kind == ExternalDependency
}

// IsStdlib returns true if this is a standard library import
func (e *ImportEdge) IsStdlib() bool {
	return e.Kind == Stdlib
}
