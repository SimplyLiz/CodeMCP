package breaking

// ChangeKind represents the type of API change
type ChangeKind string

const (
	ChangeRemoved           ChangeKind = "removed"            // Symbol was deleted
	ChangeSignatureChanged  ChangeKind = "signature_changed"  // Function/method signature changed
	ChangeTypeChanged       ChangeKind = "type_changed"       // Type definition changed
	ChangeVisibilityChanged ChangeKind = "visibility_changed" // Export status changed
	ChangeRenamed           ChangeKind = "renamed"            // Symbol was renamed
	ChangeAdded             ChangeKind = "added"              // New symbol added (non-breaking)
	ChangeDeprecated        ChangeKind = "deprecated"         // Symbol marked as deprecated
)

// Severity indicates how breaking a change is
type Severity string

const (
	SeverityBreaking    Severity = "breaking"     // Will cause compile/runtime errors
	SeverityWarning     Severity = "warning"      // May cause issues (deprecated, behavior change)
	SeverityNonBreaking Severity = "non_breaking" // Safe change (additions)
)

// APISymbol represents a public API symbol for comparison
type APISymbol struct {
	Name          string            `json:"name"`
	Kind          string            `json:"kind"` // function, type, method, const, var
	Package       string            `json:"package"`
	FilePath      string            `json:"filePath"`
	Signature     string            `json:"signature,omitempty"`     // For functions/methods
	TypeSignature string            `json:"typeSignature,omitempty"` // For types
	Exported      bool              `json:"exported"`
	Documentation string            `json:"documentation,omitempty"`
	Deprecated    bool              `json:"deprecated"`
	LineNumber    int               `json:"lineNumber,omitempty"`
	Attributes    map[string]string `json:"attributes,omitempty"` // Language-specific metadata
}

// APIChange represents a single change between two API versions
type APIChange struct {
	Kind         ChangeKind `json:"kind"`
	Severity     Severity   `json:"severity"`
	SymbolName   string     `json:"symbolName"`
	SymbolKind   string     `json:"symbolKind"`
	Package      string     `json:"package"`
	FilePath     string     `json:"filePath"`
	LineNumber   int        `json:"lineNumber,omitempty"`
	Description  string     `json:"description"`
	OldValue     string     `json:"oldValue,omitempty"`   // Previous signature/definition
	NewValue     string     `json:"newValue,omitempty"`   // New signature/definition
	Suggestion   string     `json:"suggestion,omitempty"` // Recommended migration
	AffectsUsers bool       `json:"affectsUsers"`         // True if external consumers are affected
}

// CompareOptions configures the comparison behavior
type CompareOptions struct {
	BaseRef       string   // Git ref for base version (e.g., "v1.0.0", "main")
	TargetRef     string   // Git ref for target version (e.g., "HEAD", "v2.0.0")
	Scope         []string // Limit to specific packages/paths
	IncludeMinor  bool     // Include non-breaking changes in output
	IgnorePrivate bool     // Only compare exported symbols (default: true)
}

// DefaultCompareOptions returns sensible defaults
func DefaultCompareOptions() CompareOptions {
	return CompareOptions{
		BaseRef:       "HEAD~1",
		TargetRef:     "HEAD",
		IncludeMinor:  false,
		IgnorePrivate: true,
	}
}

// CompareResult contains the result of comparing two API versions
type CompareResult struct {
	BaseRef            string      `json:"baseRef"`
	TargetRef          string      `json:"targetRef"`
	Changes            []APIChange `json:"changes"`
	Summary            *Summary    `json:"summary"`
	SemverAdvice       string      `json:"semverAdvice,omitempty"` // "major", "minor", "patch"
	TotalBaseSymbols   int         `json:"totalBaseSymbols"`
	TotalTargetSymbols int         `json:"totalTargetSymbols"`
}

// Summary provides an overview of the changes
type Summary struct {
	TotalChanges    int            `json:"totalChanges"`
	BreakingChanges int            `json:"breakingChanges"`
	Warnings        int            `json:"warnings"`
	Additions       int            `json:"additions"`
	ByKind          map[string]int `json:"byKind"`
	ByPackage       map[string]int `json:"byPackage,omitempty"`
}

// HasBreakingChanges returns true if there are any breaking changes
func (r *CompareResult) HasBreakingChanges() bool {
	return r.Summary != nil && r.Summary.BreakingChanges > 0
}
