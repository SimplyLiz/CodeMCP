package identity

// AliasReason describes why a symbol alias was created
// Section 4.4: Alias/Redirect mechanism
type AliasReason string

const (
	// ReasonRenamed means the symbol was renamed (same location, different name)
	ReasonRenamed AliasReason = "renamed"
	// ReasonMoved means the symbol was moved (same name, different location)
	ReasonMoved AliasReason = "moved"
	// ReasonMerged means multiple symbols were merged into one
	ReasonMerged AliasReason = "merged"
	// ReasonFuzzyMatch means we matched based on heuristics (lower confidence)
	ReasonFuzzyMatch AliasReason = "fuzzy-match"
)

// SymbolAlias represents a redirect from an old stable ID to a new one
// This allows tracking symbols across renames, moves, and refactorings
type SymbolAlias struct {
	OldStableId    string      `json:"oldStableId"`    // The old/previous stable ID
	NewStableId    string      `json:"newStableId"`    // The new/current stable ID
	Reason         AliasReason `json:"reason"`         // Why this alias was created
	Confidence     float64     `json:"confidence"`     // Confidence in the mapping (0.0-1.0)
	CreatedAt      string      `json:"createdAt"`      // ISO 8601 timestamp
	CreatedStateId string      `json:"createdStateId"` // RepoStateId when alias was created
}

// Validate checks if the alias is valid
func (a *SymbolAlias) Validate() error {
	if a.OldStableId == "" {
		return &ValidationError{Field: "OldStableId", Message: "old stable ID cannot be empty"}
	}

	if a.NewStableId == "" {
		return &ValidationError{Field: "NewStableId", Message: "new stable ID cannot be empty"}
	}

	if a.OldStableId == a.NewStableId {
		return &ValidationError{Field: "OldStableId", Message: "old and new stable IDs cannot be the same"}
	}

	if a.Confidence < 0.0 || a.Confidence > 1.0 {
		return &ValidationError{Field: "Confidence", Message: "confidence must be between 0.0 and 1.0"}
	}

	if a.Reason == "" {
		return &ValidationError{Field: "Reason", Message: "reason cannot be empty"}
	}

	if a.CreatedAt == "" {
		return &ValidationError{Field: "CreatedAt", Message: "createdAt cannot be empty"}
	}

	if a.CreatedStateId == "" {
		return &ValidationError{Field: "CreatedStateId", Message: "createdStateId cannot be empty"}
	}

	return nil
}

// IsHighConfidence returns true if the alias has high confidence (>= 0.8)
func (a *SymbolAlias) IsHighConfidence() bool {
	return a.Confidence >= 0.8
}

// IsLowConfidence returns true if the alias has low confidence (< 0.6)
func (a *SymbolAlias) IsLowConfidence() bool {
	return a.Confidence < 0.6
}
