package identity

// SymbolState represents the lifecycle state of a symbol
// Section 4.3: Symbol state + Tombstones
type SymbolState string

const (
	// StateActive means the symbol exists and is actively tracked
	StateActive SymbolState = "active"
	// StateDeleted means the symbol has been deleted (tombstone)
	StateDeleted SymbolState = "deleted"
	// StateUnknown means we haven't verified the symbol's state
	StateUnknown SymbolState = "unknown"
)

// SymbolMapping represents a complete symbol record in the database
// This combines identity, state, and tracking information
type SymbolMapping struct {
	// Identity
	StableId        string             `json:"stableId"`                  // Canonical stable ID
	BackendStableId string             `json:"backendStableId,omitempty"` // Backend-provided ID (for anchoring)
	Fingerprint     *SymbolFingerprint `json:"fingerprint"`

	// State
	State SymbolState `json:"state"`

	// Location
	Location          *Location         `json:"location"`
	LocationFreshness LocationFreshness `json:"locationFreshness"`

	// Versioning
	DefinitionVersionId        string           `json:"definitionVersionId,omitempty"`
	DefinitionVersionSemantics VersionSemantics `json:"definitionVersionSemantics"`

	// Tracking
	LastVerifiedAt      string `json:"lastVerifiedAt"`      // ISO 8601 timestamp
	LastVerifiedStateId string `json:"lastVerifiedStateId"` // RepoStateId when last verified

	// Tombstone fields (only set when state=deleted)
	DeletedAt        string `json:"deletedAt,omitempty"`        // ISO 8601 timestamp
	DeletedInStateId string `json:"deletedInStateId,omitempty"` // RepoStateId when deleted
}

// IsActive returns true if the symbol is in active state
func (s *SymbolMapping) IsActive() bool {
	return s.State == StateActive
}

// IsDeleted returns true if the symbol has been deleted
func (s *SymbolMapping) IsDeleted() bool {
	return s.State == StateDeleted
}

// IsUnknown returns true if the symbol state is unknown
func (s *SymbolMapping) IsUnknown() bool {
	return s.State == StateUnknown
}

// Validate checks if the symbol mapping is valid
func (s *SymbolMapping) Validate() error {
	if s.StableId == "" {
		return &ValidationError{Field: "StableId", Message: "stable ID cannot be empty"}
	}

	if s.Fingerprint == nil {
		return &ValidationError{Field: "Fingerprint", Message: "fingerprint cannot be nil"}
	}

	if s.Location == nil {
		return &ValidationError{Field: "Location", Message: "location cannot be nil"}
	}

	// Validate deleted state constraints
	if s.State == StateDeleted {
		if s.DeletedAt == "" {
			return &ValidationError{Field: "DeletedAt", Message: "deletedAt must be set for deleted symbols"}
		}
		if s.DeletedInStateId == "" {
			return &ValidationError{Field: "DeletedInStateId", Message: "deletedInStateId must be set for deleted symbols"}
		}
	} else {
		if s.DeletedAt != "" || s.DeletedInStateId != "" {
			return &ValidationError{Field: "State", Message: "deletedAt/deletedInStateId should only be set for deleted symbols"}
		}
	}

	return nil
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return "validation error on field " + e.Field + ": " + e.Message
}
