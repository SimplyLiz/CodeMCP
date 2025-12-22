package docs

import (
	"time"
)

// StalenessChecker checks documents for stale symbol references.
type StalenessChecker struct {
	symbolIndex SymbolIndex
	store       *Store
}

// NewStalenessChecker creates a new staleness checker.
func NewStalenessChecker(symbolIndex SymbolIndex, store *Store) *StalenessChecker {
	return &StalenessChecker{
		symbolIndex: symbolIndex,
		store:       store,
	}
}

// CheckDocument checks a document for stale references.
func (c *StalenessChecker) CheckDocument(doc Document) StalenessReport {
	report := StalenessReport{
		DocPath:         doc.Path,
		TotalReferences: len(doc.References),
		CheckedAt:       time.Now(),
	}

	// Get current symbol index version
	version, _ := c.store.GetSymbolIndexVersion()
	report.SymbolIndexVersion = version

	for _, ref := range doc.References {
		// Skip ineligible references (single-segment, etc.)
		if ref.Resolution == ResolutionIneligible {
			continue
		}

		if ref.SymbolID != nil && *ref.SymbolID != "" {
			// Previously resolved - check if still valid
			if c.symbolIndex.Exists(*ref.SymbolID) {
				report.Valid++
				continue
			}

			// Symbol no longer exists
			stale := c.diagnoseStale(ref)
			report.Stale = append(report.Stale, stale)
		} else {
			// Never resolved
			switch ref.Resolution {
			case ResolutionMissing:
				report.Stale = append(report.Stale, StaleReference{
					RawText:     ref.RawText,
					Line:        ref.Line,
					Reason:      StalenessMissing,
					Message:     "Symbol not found in index",
					Suggestions: c.findSuggestions(ref.NormalizedText),
				})
			case ResolutionAmbiguous:
				report.Stale = append(report.Stale, StaleReference{
					RawText:     ref.RawText,
					Line:        ref.Line,
					Reason:      StalenessAmbiguous,
					Message:     "Multiple symbols match - use directive to disambiguate",
					Suggestions: ref.Candidates,
				})
			default:
				// Resolved references (exact, suffix) that have a valid symbolID
				report.Valid++
			}
		}
	}

	return report
}

// diagnoseStale determines why a previously-resolved reference is now stale.
func (c *StalenessChecker) diagnoseStale(ref DocReference) StaleReference {
	stale := StaleReference{
		RawText: ref.RawText,
		Line:    ref.Line,
	}

	// Check if language might not be indexed
	if !c.symbolIndex.IsLanguageIndexed(ref.NormalizedText) {
		stale.Reason = StalenessIndexGap
		stale.Message = "Language may not be indexed at current tier"
		return stale
	}

	// Default: symbol is missing, offer suggestions
	stale.Reason = StalenessMissing
	stale.Message = "Symbol not found in index (may have been deleted or renamed)"
	stale.Suggestions = c.findSuggestions(ref.NormalizedText)

	return stale
}

// findSuggestions looks for symbols with similar suffix.
// This is NOT rename detection - just helpful suggestions.
func (c *StalenessChecker) findSuggestions(normalized string) []string {
	parts := splitNormalized(normalized)
	if len(parts) < 2 {
		return nil
	}

	// Try to find symbols ending with last 2 segments
	lastTwo := parts[len(parts)-2] + "." + parts[len(parts)-1]
	candidates, err := c.store.SuffixMatch(lastTwo)
	if err != nil {
		return nil
	}

	// Limit suggestions
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}

	return candidates
}

// splitNormalized splits a normalized name into segments.
func splitNormalized(normalized string) []string {
	if normalized == "" {
		return nil
	}

	var parts []string
	current := ""
	for _, r := range normalized {
		if r == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(r)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// CheckAllDocuments checks all indexed documents for staleness.
func (c *StalenessChecker) CheckAllDocuments() ([]StalenessReport, error) {
	docs, err := c.store.GetAllDocuments()
	if err != nil {
		return nil, err
	}

	var reports []StalenessReport
	for _, doc := range docs {
		// Load full document with references
		fullDoc, err := c.store.GetDocument(doc.Path)
		if err != nil {
			continue
		}
		if fullDoc == nil {
			continue
		}

		report := c.CheckDocument(*fullDoc)
		if len(report.Stale) > 0 {
			reports = append(reports, report)
		}
	}

	return reports, nil
}
