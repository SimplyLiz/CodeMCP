package diff

import (
	"errors"
	"fmt"
	"math/rand"
	"time"
)

// ValidationMode controls how strict validation is
type ValidationMode string

const (
	// ValidationStrict requires all checks to pass
	ValidationStrict ValidationMode = "strict"
	// ValidationPermissive allows some hash mismatches (for recovery)
	ValidationPermissive ValidationMode = "permissive"
)

// ValidationError represents a validation failure
type ValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s (field: %s)", e.Code, e.Message, e.Field)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Error codes
const (
	ErrCodeUnsupportedVersion = "UNSUPPORTED_VERSION"
	ErrCodeSnapshotMismatch   = "SNAPSHOT_MISMATCH"
	ErrCodeStatsMismatch      = "STATS_MISMATCH"
	ErrCodeHashMismatch       = "HASH_MISMATCH"
	ErrCodeMissingField       = "MISSING_FIELD"
	ErrCodeInvalidFormat      = "INVALID_FORMAT"
)

// Validator validates delta artifacts before ingestion
type Validator struct {
	mode         ValidationMode
	hasher       *Hasher
	spotCheckPct float64 // Percentage of entities to spot-check (0.0-1.0)
	rng          *rand.Rand
}

// ValidatorOption configures the validator
type ValidatorOption func(*Validator)

// WithValidationMode sets the validation mode
func WithValidationMode(mode ValidationMode) ValidatorOption {
	return func(v *Validator) {
		v.mode = mode
	}
}

// WithSpotCheckPercentage sets the percentage of entities to hash-check
func WithSpotCheckPercentage(pct float64) ValidatorOption {
	return func(v *Validator) {
		if pct < 0 {
			pct = 0
		}
		if pct > 1 {
			pct = 1
		}
		v.spotCheckPct = pct
	}
}

// NewValidator creates a new delta validator
func NewValidator(opts ...ValidatorOption) *Validator {
	v := &Validator{
		mode:         ValidationStrict,
		hasher:       NewHasher(),
		spotCheckPct: 0.1, // Default: check 10% of entities
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	for _, opt := range opts {
		opt(v)
	}

	return v
}

// ValidationResult contains the outcome of validation
type ValidationResult struct {
	Valid           bool               `json:"valid"`
	Errors          []ValidationError  `json:"errors,omitempty"`
	Warnings        []ValidationError  `json:"warnings,omitempty"`
	SpotChecked     int                `json:"spotChecked"`
	SpotCheckPassed int                `json:"spotCheckPassed"`
}

// Validate performs full validation of a delta artifact
func (v *Validator) Validate(delta *Delta, currentSnapshotID string) *ValidationResult {
	result := &ValidationResult{Valid: true}

	// 1. Verify schema version is supported
	if err := v.validateVersion(delta); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, *err)
		return result // Can't proceed with unsupported version
	}

	// 2. Verify base_snapshot_id matches current state
	if err := v.validateBaseSnapshot(delta, currentSnapshotID); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, *err)
		// Continue to collect more errors
	}

	// 3. Verify counts match stats
	if errs := v.validateStats(delta); len(errs) > 0 {
		result.Valid = false
		result.Errors = append(result.Errors, errs...)
	}

	// 4. Validate required fields
	if errs := v.validateRequiredFields(delta); len(errs) > 0 {
		result.Valid = false
		result.Errors = append(result.Errors, errs...)
	}

	// 5. Spot-check hashes for modified/added entities
	spotResult := v.spotCheckHashes(delta)
	result.SpotChecked = spotResult.checked
	result.SpotCheckPassed = spotResult.passed

	if spotResult.failed > 0 {
		if v.mode == ValidationStrict {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Code:    ErrCodeHashMismatch,
				Message: fmt.Sprintf("%d of %d spot-checked entities failed hash validation", spotResult.failed, spotResult.checked),
			})
		} else {
			result.Warnings = append(result.Warnings, ValidationError{
				Code:    ErrCodeHashMismatch,
				Message: fmt.Sprintf("%d of %d spot-checked entities failed hash validation (permissive mode)", spotResult.failed, spotResult.checked),
			})
		}
	}

	return result
}

// ValidateForIngestion is a convenience method that returns an error if invalid
func (v *Validator) ValidateForIngestion(delta *Delta, currentSnapshotID string) error {
	result := v.Validate(delta, currentSnapshotID)
	if !result.Valid {
		if len(result.Errors) > 0 {
			return &result.Errors[0]
		}
		return errors.New("validation failed")
	}
	return nil
}

func (v *Validator) validateVersion(delta *Delta) *ValidationError {
	if delta.SchemaVersion < 1 || delta.SchemaVersion > SchemaVersion {
		return &ValidationError{
			Code:    ErrCodeUnsupportedVersion,
			Message: fmt.Sprintf("schema version %d not supported (max: %d)", delta.SchemaVersion, SchemaVersion),
			Field:   "delta_schema_version",
		}
	}
	return nil
}

func (v *Validator) validateBaseSnapshot(delta *Delta, currentSnapshotID string) *ValidationError {
	// Empty base snapshot is valid for initial import
	if delta.BaseSnapshotID == "" && currentSnapshotID == "" {
		return nil
	}

	// If we have a current snapshot, delta must reference it
	if currentSnapshotID != "" && delta.BaseSnapshotID != currentSnapshotID {
		return &ValidationError{
			Code:    ErrCodeSnapshotMismatch,
			Message: fmt.Sprintf("base_snapshot_id '%s' does not match current '%s'", delta.BaseSnapshotID, currentSnapshotID),
			Field:   "base_snapshot_id",
		}
	}

	return nil
}

func (v *Validator) validateStats(delta *Delta) []ValidationError {
	var errs []ValidationError

	computed := delta.ComputeStats()

	if computed.TotalAdded != delta.Stats.TotalAdded {
		errs = append(errs, ValidationError{
			Code:    ErrCodeStatsMismatch,
			Message: fmt.Sprintf("total_added mismatch: declared %d, actual %d", delta.Stats.TotalAdded, computed.TotalAdded),
			Field:   "stats.total_added",
		})
	}

	if computed.TotalModified != delta.Stats.TotalModified {
		errs = append(errs, ValidationError{
			Code:    ErrCodeStatsMismatch,
			Message: fmt.Sprintf("total_modified mismatch: declared %d, actual %d", delta.Stats.TotalModified, computed.TotalModified),
			Field:   "stats.total_modified",
		})
	}

	if computed.TotalDeleted != delta.Stats.TotalDeleted {
		errs = append(errs, ValidationError{
			Code:    ErrCodeStatsMismatch,
			Message: fmt.Sprintf("total_deleted mismatch: declared %d, actual %d", delta.Stats.TotalDeleted, computed.TotalDeleted),
			Field:   "stats.total_deleted",
		})
	}

	return errs
}

func (v *Validator) validateRequiredFields(delta *Delta) []ValidationError {
	var errs []ValidationError

	if delta.NewSnapshotID == "" {
		errs = append(errs, ValidationError{
			Code:    ErrCodeMissingField,
			Message: "new_snapshot_id is required",
			Field:   "new_snapshot_id",
		})
	}

	if delta.Commit == "" {
		errs = append(errs, ValidationError{
			Code:    ErrCodeMissingField,
			Message: "commit is required",
			Field:   "commit",
		})
	}

	if delta.Timestamp <= 0 {
		errs = append(errs, ValidationError{
			Code:    ErrCodeInvalidFormat,
			Message: "timestamp must be positive",
			Field:   "timestamp",
		})
	}

	// Validate symbol records
	for i, s := range delta.Deltas.Symbols.Added {
		if s.ID == "" {
			errs = append(errs, ValidationError{
				Code:    ErrCodeMissingField,
				Message: fmt.Sprintf("symbols.added[%d].id is required", i),
				Field:   fmt.Sprintf("deltas.symbols.added[%d].id", i),
			})
		}
	}

	for i, s := range delta.Deltas.Symbols.Modified {
		if s.ID == "" {
			errs = append(errs, ValidationError{
				Code:    ErrCodeMissingField,
				Message: fmt.Sprintf("symbols.modified[%d].id is required", i),
				Field:   fmt.Sprintf("deltas.symbols.modified[%d].id", i),
			})
		}
	}

	return errs
}

type spotCheckResult struct {
	checked int
	passed  int
	failed  int
}

func (v *Validator) spotCheckHashes(delta *Delta) spotCheckResult {
	result := spotCheckResult{}

	// Check symbols
	for i := range delta.Deltas.Symbols.Added {
		if v.shouldCheck() {
			result.checked++
			s := &delta.Deltas.Symbols.Added[i]
			if v.hasher.VerifySymbolHash(s) {
				result.passed++
			} else {
				result.failed++
			}
		}
	}

	for i := range delta.Deltas.Symbols.Modified {
		if v.shouldCheck() {
			result.checked++
			s := &delta.Deltas.Symbols.Modified[i]
			if v.hasher.VerifySymbolHash(s) {
				result.passed++
			} else {
				result.failed++
			}
		}
	}

	// Check refs
	for i := range delta.Deltas.Refs.Added {
		if v.shouldCheck() {
			result.checked++
			r := &delta.Deltas.Refs.Added[i]
			if v.hasher.VerifyRefHash(r) {
				result.passed++
			} else {
				result.failed++
			}
		}
	}

	// Check call edges
	for i := range delta.Deltas.CallGraph.Added {
		if v.shouldCheck() {
			result.checked++
			c := &delta.Deltas.CallGraph.Added[i]
			if v.hasher.VerifyCallEdgeHash(c) {
				result.passed++
			} else {
				result.failed++
			}
		}
	}

	return result
}

func (v *Validator) shouldCheck() bool {
	return v.rng.Float64() < v.spotCheckPct
}

// QuickValidate performs basic validation without spot-checking
func (v *Validator) QuickValidate(delta *Delta) error {
	// Version check
	if delta.SchemaVersion < 1 || delta.SchemaVersion > SchemaVersion {
		return &ValidationError{
			Code:    ErrCodeUnsupportedVersion,
			Message: fmt.Sprintf("unsupported schema version %d", delta.SchemaVersion),
		}
	}

	// Stats check
	if !delta.ValidateStats() {
		return &ValidationError{
			Code:    ErrCodeStatsMismatch,
			Message: "stats do not match actual counts",
		}
	}

	// Required fields
	if delta.NewSnapshotID == "" {
		return &ValidationError{
			Code:    ErrCodeMissingField,
			Message: "new_snapshot_id is required",
		}
	}

	return nil
}
