package backends

import (
	"fmt"

	"ckb/internal/logging"
)

// UnionMerger implements the union merge strategy
// Queries all backends and merges all results, resolving conflicts by precedence
type UnionMerger struct {
	policy *QueryPolicy
	logger *logging.Logger
}

// NewUnionMerger creates a new union merger
func NewUnionMerger(policy *QueryPolicy, logger *logging.Logger) *UnionMerger {
	return &UnionMerger{
		policy: policy,
		logger: logger,
	}
}

// MergeSymbolResults merges symbol results from all backends
func (m *UnionMerger) MergeSymbolResults(
	results []BackendResult,
) (*SymbolResult, Provenance) {
	if len(results) == 0 {
		return nil, Provenance{MergeMode: MergeModeUnion}
	}

	// Collect all successful results
	successfulResults := []struct {
		backend BackendID
		data    *SymbolResult
	}{}

	for i := range results {
		if results[i].Error == nil {
			if data, ok := results[i].Data.(*SymbolResult); ok {
				successfulResults = append(successfulResults, struct {
					backend BackendID
					data    *SymbolResult
				}{results[i].BackendID, data})
			}
		}
	}

	if len(successfulResults) == 0 {
		return nil, Provenance{MergeMode: MergeModeUnion}
	}

	// Sort by backend precedence (already in order from orchestrator)
	// The first result becomes the base
	merged := *successfulResults[0].data

	provenance := Provenance{
		PrimaryBackend:     successfulResults[0].backend,
		SupplementBackends: []BackendID{},
		MergeMode:          MergeModeUnion,
		UnionConflicts:     []UnionConflict{},
	}

	// Merge data from other backends
	for i := 1; i < len(successfulResults); i++ {
		m.mergeSymbolData(&merged, successfulResults[i].data, successfulResults[i].backend, &provenance)
		provenance.SupplementBackends = append(provenance.SupplementBackends, successfulResults[i].backend)
	}

	return &merged, provenance
}

// mergeSymbolData merges data from a secondary symbol result into the primary
func (m *UnionMerger) mergeSymbolData(
	primary *SymbolResult,
	secondary *SymbolResult,
	secondaryBackend BackendID,
	provenance *Provenance,
) {
	// For each field, if primary is empty, use secondary
	// If both have values and they differ, record a conflict

	m.mergeField(&primary.Name, secondary.Name, "Name", primary.StableID, secondaryBackend, provenance)
	m.mergeField(&primary.Kind, secondary.Kind, "Kind", primary.StableID, secondaryBackend, provenance)
	m.mergeField(&primary.SignatureNormalized, secondary.SignatureNormalized, "SignatureNormalized", primary.StableID, secondaryBackend, provenance)
	m.mergeField(&primary.SignatureFull, secondary.SignatureFull, "SignatureFull", primary.StableID, secondaryBackend, provenance)
	m.mergeField(&primary.Visibility, secondary.Visibility, "Visibility", primary.StableID, secondaryBackend, provenance)
	m.mergeField(&primary.ContainerName, secondary.ContainerName, "ContainerName", primary.StableID, secondaryBackend, provenance)
	m.mergeField(&primary.ModuleID, secondary.ModuleID, "ModuleID", primary.StableID, secondaryBackend, provenance)
	m.mergeField(&primary.Documentation, secondary.Documentation, "Documentation", primary.StableID, secondaryBackend, provenance)

	// For numeric fields, prefer non-zero values
	if primary.VisibilityConfidence == 0 && secondary.VisibilityConfidence > 0 {
		primary.VisibilityConfidence = secondary.VisibilityConfidence
	} else if primary.VisibilityConfidence != secondary.VisibilityConfidence && secondary.VisibilityConfidence > 0 {
		m.recordUnionConflict(provenance, "VisibilityConfidence", primary.StableID, secondaryBackend,
			primary.VisibilityConfidence, secondary.VisibilityConfidence)
	}

	// Location: prefer primary, but record conflict if different
	if !locationsEqual(primary.Location, secondary.Location) {
		m.recordUnionConflict(provenance, "Location", primary.StableID, secondaryBackend,
			primary.Location, secondary.Location)
	}
}

// mergeField merges a string field, recording conflicts
func (m *UnionMerger) mergeField(
	primary *string,
	secondary string,
	fieldName string,
	itemID string,
	secondaryBackend BackendID,
	provenance *Provenance,
) {
	if *primary == "" && secondary != "" {
		*primary = secondary
	} else if *primary != secondary && secondary != "" {
		m.recordUnionConflict(provenance, fieldName, itemID, secondaryBackend, *primary, secondary)
	}
}

// recordUnionConflict records a conflict in union merge mode
func (m *UnionMerger) recordUnionConflict(
	provenance *Provenance,
	field string,
	itemID string,
	backendID BackendID,
	primaryVal interface{},
	secondaryVal interface{},
) {
	conflict := UnionConflict{
		Field:  field,
		ItemID: itemID,
		BackendValues: map[BackendID]interface{}{
			provenance.PrimaryBackend: primaryVal,
			backendID:                 secondaryVal,
		},
		Resolution: fmt.Sprintf("Used value from %s (higher precedence)", provenance.PrimaryBackend),
	}

	provenance.UnionConflicts = append(provenance.UnionConflicts, conflict)

	m.logger.Debug("Union conflict detected", map[string]interface{}{
		"field":        field,
		"itemID":       itemID,
		"primaryValue": primaryVal,
		"backendValue": secondaryVal,
		"backend":      backendID,
	})
}

// locationsEqual checks if two locations are equal
func locationsEqual(a, b Location) bool {
	return a.Path == b.Path &&
		a.Line == b.Line &&
		a.Column == b.Column &&
		a.EndLine == b.EndLine &&
		a.EndColumn == b.EndColumn
}

// MergeSearchResults merges search results from all backends
func (m *UnionMerger) MergeSearchResults(
	results []BackendResult,
) (*SearchResult, Provenance) {
	if len(results) == 0 {
		return nil, Provenance{MergeMode: MergeModeUnion}
	}

	merged := &SearchResult{
		Symbols:      []SymbolResult{},
		TotalMatches: 0,
	}

	provenance := Provenance{
		MergeMode:          MergeModeUnion,
		SupplementBackends: []BackendID{},
	}

	// Collect symbols from all backends
	symbolsByID := make(map[string]*SymbolResult)
	var backends []BackendID

	for i := range results {
		if results[i].Error != nil {
			continue
		}

		data, ok := results[i].Data.(*SearchResult)
		if !ok {
			continue
		}

		backends = append(backends, results[i].BackendID)

		for j := range data.Symbols {
			symbol := data.Symbols[j]
			if existing, ok := symbolsByID[symbol.StableID]; ok {
				// Merge with existing symbol
				m.mergeSymbolData(existing, &symbol, results[i].BackendID, &provenance)
			} else {
				// Add new symbol
				symbolCopy := symbol
				symbolsByID[symbol.StableID] = &symbolCopy
			}
		}

		merged.TotalMatches += data.TotalMatches
	}

	// Convert map to slice
	for _, symbol := range symbolsByID {
		merged.Symbols = append(merged.Symbols, *symbol)
	}

	if len(backends) > 0 {
		provenance.PrimaryBackend = backends[0]
		if len(backends) > 1 {
			provenance.SupplementBackends = backends[1:]
		}
	}

	// Merge completeness
	var completenessInfos []CompletenessInfo
	for i := range results {
		if results[i].Error == nil {
			if data, ok := results[i].Data.(*SearchResult); ok {
				completenessInfos = append(completenessInfos, data.Completeness)
			}
		}
	}
	merged.Completeness = MergeCompleteness(completenessInfos)

	return merged, provenance
}

// MergeReferencesResults merges references results from all backends
func (m *UnionMerger) MergeReferencesResults(
	results []BackendResult,
) (*ReferencesResult, Provenance) {
	if len(results) == 0 {
		return nil, Provenance{MergeMode: MergeModeUnion}
	}

	merged := &ReferencesResult{
		References:      []Reference{},
		TotalReferences: 0,
	}

	provenance := Provenance{
		MergeMode:          MergeModeUnion,
		SupplementBackends: []BackendID{},
	}

	// Collect references from all backends, deduplicating by location
	referencesByLocation := make(map[string]*Reference)
	var backends []BackendID

	for i := range results {
		if results[i].Error != nil {
			continue
		}

		data, ok := results[i].Data.(*ReferencesResult)
		if !ok {
			continue
		}

		backends = append(backends, results[i].BackendID)

		for j := range data.References {
			ref := data.References[j]
			key := locationKey(ref.Location)
			if _, ok := referencesByLocation[key]; !ok {
				// Add new reference
				refCopy := ref
				referencesByLocation[key] = &refCopy
			}
			// If duplicate, we could merge metadata here
		}

		merged.TotalReferences += data.TotalReferences
	}

	// Convert map to slice
	for _, ref := range referencesByLocation {
		merged.References = append(merged.References, *ref)
	}

	if len(backends) > 0 {
		provenance.PrimaryBackend = backends[0]
		if len(backends) > 1 {
			provenance.SupplementBackends = backends[1:]
		}
	}

	// Merge completeness
	var completenessInfos []CompletenessInfo
	for i := range results {
		if results[i].Error == nil {
			if data, ok := results[i].Data.(*ReferencesResult); ok {
				completenessInfos = append(completenessInfos, data.Completeness)
			}
		}
	}
	merged.Completeness = MergeCompleteness(completenessInfos)

	return merged, provenance
}

// locationKey generates a unique key for a location
func locationKey(loc Location) string {
	return fmt.Sprintf("%s:%d:%d", loc.Path, loc.Line, loc.Column)
}
