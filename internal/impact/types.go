package impact

// SymbolKind represents the type of symbol
type SymbolKind string

const (
	KindClass     SymbolKind = "class"
	KindInterface SymbolKind = "interface"
	KindFunction  SymbolKind = "function"
	KindMethod    SymbolKind = "method"
	KindProperty  SymbolKind = "property"
	KindVariable  SymbolKind = "variable"
	KindConstant  SymbolKind = "constant"
	KindType      SymbolKind = "type"
)

// Symbol represents a code symbol with metadata
type Symbol struct {
	StableId            string     // Unique identifier for the symbol
	Name                string     // Symbol name
	Kind                SymbolKind // Symbol kind (class, function, etc.)
	Signature           string     // Full signature
	SignatureNormalized string     // Normalized signature for comparison
	ModuleId            string     // Module identifier
	ModuleName          string     // Module name
	ContainerName       string     // Container name (class, namespace, etc.)
	Location            *Location  // Location in source code
	Modifiers           []string   // Modifiers from SCIP (public, private, static, etc.)
}

// Location represents a position in source code
type Location struct {
	FileId      string // File identifier
	StartLine   int    // Starting line number (1-indexed)
	StartColumn int    // Starting column number (1-indexed)
	EndLine     int    // Ending line number (1-indexed)
	EndColumn   int    // Ending column number (1-indexed)
}

// ReferenceKind represents the type of reference
type ReferenceKind string

const (
	RefCall       ReferenceKind = "call"       // Function/method call
	RefRead       ReferenceKind = "read"       // Read access
	RefWrite      ReferenceKind = "write"      // Write access
	RefType       ReferenceKind = "type"       // Type reference
	RefImplements ReferenceKind = "implements" // Interface implementation
	RefExtends    ReferenceKind = "extends"    // Class extension
)

// Reference represents a reference to a symbol
type Reference struct {
	Location   *Location     // Location of the reference
	Kind       ReferenceKind // Kind of reference
	FromSymbol string        // StableId of the referencing symbol
	FromModule string        // ModuleId of the referencing module
	IsTest     bool          // Whether this reference is from a test
}
