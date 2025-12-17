package backends

import (
	"reflect"

	"ckb/internal/logging"
)

// PreferFirstMerger implements the prefer-first merge strategy
// Uses the highest-preference backend's result, supplementing only metadata
type PreferFirstMerger struct {
	policy *QueryPolicy
	logger *logging.Logger
}

// NewPreferFirstMerger creates a new prefer-first merger
func NewPreferFirstMerger(policy *QueryPolicy, logger *logging.Logger) *PreferFirstMerger {
	return &PreferFirstMerger{
		policy: policy,
		logger: logger,
	}
}

// Allowed supplement fields per Section 6.2
var _ = _allowedSupplementFields // prevent unused error
var _allowedSupplementFields = map[string]bool{
	"Visibility":           true,
	"VisibilityConfidence": true,
	"SignatureNormalized":  true,
	"SignatureFull":        true,
	"Kind":                 true,
	"ContainerName":        true,
	"ModuleID":             true,
}

// MergeSymbolResults merges symbol results using prefer-first strategy
func (m *PreferFirstMerger) MergeSymbolResults(
	results []BackendResult,
) (*SymbolResult, Provenance) {
	if len(results) == 0 {
		return nil, Provenance{MergeMode: MergeModePreferFirst}
	}

	// Find primary result (first non-error result)
	var primaryResult *BackendResult
	var primaryData *SymbolResult
	for i := range results {
		if results[i].Error == nil {
			if data, ok := results[i].Data.(*SymbolResult); ok {
				primaryResult = &results[i]
				primaryData = data
				break
			}
		}
	}

	if primaryData == nil {
		// All results failed
		return nil, Provenance{MergeMode: MergeModePreferFirst}
	}

	// Start with a copy of the primary result
	merged := *primaryData

	// Track provenance
	provenance := Provenance{
		PrimaryBackend:     primaryResult.BackendID,
		SupplementBackends: []BackendID{},
		MergeMode:          MergeModePreferFirst,
		MetadataConflicts:  []MetadataConflict{},
	}

	primaryPriority := m.policy.GetBackendPriority(primaryResult.BackendID)

	// Supplement metadata from equal-or-higher precedence backends
	for i := range results {
		result := &results[i]

		// Skip primary
		if result.BackendID == primaryResult.BackendID {
			continue
		}

		// Skip errors
		if result.Error != nil {
			continue
		}

		// Only supplement from equal-or-higher precedence
		if m.policy.GetBackendPriority(result.BackendID) > primaryPriority {
			continue
		}

		data, ok := result.Data.(*SymbolResult)
		if !ok {
			continue
		}

		// Supplement allowed fields
		supplemented := m.supplementMetadata(&merged, data, result.BackendID, &provenance)
		if supplemented {
			provenance.SupplementBackends = append(provenance.SupplementBackends, result.BackendID)
		}
	}

	return &merged, provenance
}

// supplementMetadata supplements allowed metadata fields from a secondary result
func (m *PreferFirstMerger) supplementMetadata(
	primary *SymbolResult,
	supplement *SymbolResult,
	supplementBackend BackendID,
	provenance *Provenance,
) bool {
	supplemented := false

	// Helper to check if a field should be supplemented
	shouldSupplement := func(primaryVal, supplementVal interface{}) bool {
		// Don't supplement if primary already has a value
		if !isEmptyValue(primaryVal) {
			// Track conflict if values differ
			if !reflect.DeepEqual(primaryVal, supplementVal) && !isEmptyValue(supplementVal) {
				return false // Different value, don't override
			}
			return false
		}
		// Supplement if primary is empty and supplement has value
		return !isEmptyValue(supplementVal)
	}

	// Supplement Visibility
	if shouldSupplement(primary.Visibility, supplement.Visibility) {
		m.logger.Debug("Supplementing Visibility", map[string]interface{}{
			"backend": supplementBackend,
			"value":   supplement.Visibility,
		})
		primary.Visibility = supplement.Visibility
		supplemented = true
	} else if primary.Visibility != supplement.Visibility && supplement.Visibility != "" {
		m.recordConflict(provenance, "Visibility", supplementBackend, primary.Visibility, supplement.Visibility)
	}

	// Supplement VisibilityConfidence
	if shouldSupplement(primary.VisibilityConfidence, supplement.VisibilityConfidence) {
		primary.VisibilityConfidence = supplement.VisibilityConfidence
		supplemented = true
	}

	// Supplement SignatureNormalized
	if shouldSupplement(primary.SignatureNormalized, supplement.SignatureNormalized) {
		m.logger.Debug("Supplementing SignatureNormalized", map[string]interface{}{
			"backend": supplementBackend,
		})
		primary.SignatureNormalized = supplement.SignatureNormalized
		supplemented = true
	} else if primary.SignatureNormalized != supplement.SignatureNormalized && supplement.SignatureNormalized != "" {
		m.recordConflict(provenance, "SignatureNormalized", supplementBackend, primary.SignatureNormalized, supplement.SignatureNormalized)
	}

	// Supplement SignatureFull
	if shouldSupplement(primary.SignatureFull, supplement.SignatureFull) {
		primary.SignatureFull = supplement.SignatureFull
		supplemented = true
	}

	// Supplement Kind
	if shouldSupplement(primary.Kind, supplement.Kind) {
		m.logger.Debug("Supplementing Kind", map[string]interface{}{
			"backend": supplementBackend,
			"value":   supplement.Kind,
		})
		primary.Kind = supplement.Kind
		supplemented = true
	} else if primary.Kind != supplement.Kind && supplement.Kind != "" {
		m.recordConflict(provenance, "Kind", supplementBackend, primary.Kind, supplement.Kind)
	}

	// Supplement ContainerName
	if shouldSupplement(primary.ContainerName, supplement.ContainerName) {
		primary.ContainerName = supplement.ContainerName
		supplemented = true
	}

	// Supplement ModuleID
	if shouldSupplement(primary.ModuleID, supplement.ModuleID) {
		primary.ModuleID = supplement.ModuleID
		supplemented = true
	}

	return supplemented
}

// recordConflict records a metadata conflict in provenance
func (m *PreferFirstMerger) recordConflict(
	provenance *Provenance,
	field string,
	backendID BackendID,
	primaryVal interface{},
	supplementVal interface{},
) {
	// Check if we already have a conflict for this field
	for i := range provenance.MetadataConflicts {
		if provenance.MetadataConflicts[i].Field == field {
			// Add this backend's value to existing conflict
			provenance.MetadataConflicts[i].Values[backendID] = supplementVal
			return
		}
	}

	// Create new conflict record
	conflict := MetadataConflict{
		Field: field,
		Values: map[BackendID]interface{}{
			provenance.PrimaryBackend: primaryVal,
			backendID:                 supplementVal,
		},
		Resolved: primaryVal, // We keep the primary value
	}

	provenance.MetadataConflicts = append(provenance.MetadataConflicts, conflict)

	m.logger.Debug("Metadata conflict detected", map[string]interface{}{
		"field":        field,
		"primaryValue": primaryVal,
		"backendValue": supplementVal,
		"backend":      backendID,
	})
}

// isEmptyValue checks if a value is empty/zero
func isEmptyValue(v interface{}) bool {
	if v == nil {
		return true
	}

	switch val := v.(type) {
	case string:
		return val == ""
	case int, int8, int16, int32, int64:
		return val == 0
	case float32, float64:
		return val == 0.0
	case bool:
		return !val
	default:
		// Use reflection for other types
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Ptr, reflect.Interface:
			return rv.IsNil()
		case reflect.Slice, reflect.Map:
			return rv.Len() == 0
		default:
			return rv.IsZero()
		}
	}
}

// MergeSearchResults merges search results using prefer-first strategy
func (m *PreferFirstMerger) MergeSearchResults(
	results []BackendResult,
) (*SearchResult, Provenance) {
	if len(results) == 0 {
		return nil, Provenance{MergeMode: MergeModePreferFirst}
	}

	// Find primary result
	var primaryResult *BackendResult
	for i := range results {
		if results[i].Error == nil {
			primaryResult = &results[i]
			break
		}
	}

	if primaryResult == nil {
		return nil, Provenance{MergeMode: MergeModePreferFirst}
	}

	data, ok := primaryResult.Data.(*SearchResult)
	if !ok {
		return nil, Provenance{MergeMode: MergeModePreferFirst}
	}

	provenance := Provenance{
		PrimaryBackend: primaryResult.BackendID,
		MergeMode:      MergeModePreferFirst,
	}

	return data, provenance
}

// MergeReferencesResults merges references results using prefer-first strategy
func (m *PreferFirstMerger) MergeReferencesResults(
	results []BackendResult,
) (*ReferencesResult, Provenance) {
	if len(results) == 0 {
		return nil, Provenance{MergeMode: MergeModePreferFirst}
	}

	// Find primary result
	var primaryResult *BackendResult
	for i := range results {
		if results[i].Error == nil {
			primaryResult = &results[i]
			break
		}
	}

	if primaryResult == nil {
		return nil, Provenance{MergeMode: MergeModePreferFirst}
	}

	data, ok := primaryResult.Data.(*ReferencesResult)
	if !ok {
		return nil, Provenance{MergeMode: MergeModePreferFirst}
	}

	provenance := Provenance{
		PrimaryBackend: primaryResult.BackendID,
		MergeMode:      MergeModePreferFirst,
	}

	return data, provenance
}
