package identity

// SymbolIdentity contains all ID-related fields for a symbol
// Section 4.1: Stable ID + Definition Version ID
type SymbolIdentity struct {
	StableId                   string            `json:"stableId"`                    // ckb:<repo>:sym:<fingerprint>
	DefinitionVersionId        string            `json:"definitionVersionId,omitempty"`
	DefinitionVersionSemantics VersionSemantics  `json:"definitionVersionSemantics"`
	Fingerprint                SymbolFingerprint `json:"fingerprint"`
	Location                   *Location         `json:"location"`
	LocationFreshness          LocationFreshness `json:"locationFreshness"`
	LastVerifiedAt             string            `json:"lastVerifiedAt"`
	LastVerifiedStateId        string            `json:"lastVerifiedStateId"`
}

// VersionSemantics describes what changes trigger a new definition version
type VersionSemantics string

const (
	// BackendDefinitionHash means the backend provided a definition hash that changes with signature
	BackendDefinitionHash VersionSemantics = "backend-definition-hash"
	// StructuralSignatureHash means we computed a hash from the normalized signature
	StructuralSignatureHash VersionSemantics = "structural-signature-hash"
	// UnknownSemantics means we couldn't determine versioning semantics
	UnknownSemantics VersionSemantics = "unknown"
)

// LocationFreshness indicates whether the location is current or may be stale
type LocationFreshness string

const (
	// Fresh means the location has been verified against the current working tree
	Fresh LocationFreshness = "fresh"
	// MayBeStale means there are uncommitted changes and the location may be outdated
	MayBeStale LocationFreshness = "may-be-stale"
)

// Location represents a position in source code
type Location struct {
	Path      string `json:"path"`                // Repo-relative path
	Line      int    `json:"line"`                // 1-indexed
	Column    int    `json:"column"`              // 1-indexed
	EndLine   int    `json:"endLine,omitempty"`   // For ranges
	EndColumn int    `json:"endColumn,omitempty"` // For ranges
}

// SymbolKind represents the kind of symbol
type SymbolKind string

const (
	KindFunction   SymbolKind = "function"
	KindMethod     SymbolKind = "method"
	KindClass      SymbolKind = "class"
	KindInterface  SymbolKind = "interface"
	KindStruct     SymbolKind = "struct"
	KindEnum       SymbolKind = "enum"
	KindVariable   SymbolKind = "variable"
	KindConstant   SymbolKind = "constant"
	KindField      SymbolKind = "field"
	KindProperty   SymbolKind = "property"
	KindNamespace  SymbolKind = "namespace"
	KindModule     SymbolKind = "module"
	KindPackage    SymbolKind = "package"
	KindType       SymbolKind = "type"
	KindParameter  SymbolKind = "parameter"
	KindUnknown    SymbolKind = "unknown"
)
