package impact

// TypeContextLevel represents the level of type context available in analysis
type TypeContextLevel string

const (
	TypeContextFull    TypeContextLevel = "full"    // Complete type information available
	TypeContextPartial TypeContextLevel = "partial" // Some type information available
	TypeContextNone    TypeContextLevel = "none"    // No type information available
)

// AnalysisLimits describes the limitations of the impact analysis
type AnalysisLimits struct {
	TypeContext TypeContextLevel // Level of type context available
	Notes       []string         // Additional notes about limitations
}

// NewAnalysisLimits creates a new AnalysisLimits with default values
func NewAnalysisLimits() *AnalysisLimits {
	return &AnalysisLimits{
		TypeContext: TypeContextNone,
		Notes:       make([]string, 0),
	}
}

// AddNote adds a limitation note to the analysis
func (al *AnalysisLimits) AddNote(note string) {
	al.Notes = append(al.Notes, note)
}

// HasLimitations returns true if there are any limitations
func (al *AnalysisLimits) HasLimitations() bool {
	return al.TypeContext != TypeContextFull || len(al.Notes) > 0
}

// DetermineTypeContext analyzes the available data to determine type context level
func DetermineTypeContext(symbol *Symbol, refs []Reference) TypeContextLevel {
	// Check if we have type information in the symbol
	hasTypeInfo := symbol.Signature != "" || symbol.SignatureNormalized != ""

	// Check if references have type information
	hasTypedRefs := false
	for _, ref := range refs {
		if ref.Kind == RefType || ref.Kind == RefImplements || ref.Kind == RefExtends {
			hasTypedRefs = true
			break
		}
	}

	// Determine context level based on available information
	if hasTypeInfo && hasTypedRefs {
		return TypeContextFull
	}
	if hasTypeInfo || hasTypedRefs {
		return TypeContextPartial
	}
	return TypeContextNone
}
