package docs

import (
	"fmt"
	"time"

	"ckb/internal/identity"
	"ckb/internal/logging"
	"ckb/internal/storage"
)

// StalenessChecker checks documents for stale symbol references.
type StalenessChecker struct {
	symbolIndex      SymbolIndex
	store            *Store
	identityResolver *identity.IdentityResolver // v1.1: For rename detection
}

// NewStalenessChecker creates a new staleness checker.
func NewStalenessChecker(symbolIndex SymbolIndex, store *Store) *StalenessChecker {
	return &StalenessChecker{
		symbolIndex: symbolIndex,
		store:       store,
	}
}

// NewStalenessCheckerWithIdentity creates a staleness checker with rename detection support.
func NewStalenessCheckerWithIdentity(symbolIndex SymbolIndex, store *Store, db *storage.DB, logger *logging.Logger) *StalenessChecker {
	return &StalenessChecker{
		symbolIndex:      symbolIndex,
		store:            store,
		identityResolver: identity.NewIdentityResolver(db, logger),
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

	// v1.1: Check for rename/move via alias chain
	if ref.SymbolID != nil && *ref.SymbolID != "" {
		if renameInfo := c.checkForRename(*ref.SymbolID); renameInfo != nil {
			stale.Reason = StalenessRenamed
			stale.Message = fmt.Sprintf("Symbol was %s", renameInfo.Reason)
			stale.Suggestions = []string{renameInfo.NewName}
			stale.NewSymbolID = &renameInfo.NewSymbolID
			return stale
		}
	}

	// Default: symbol is missing, offer suggestions
	stale.Reason = StalenessMissing
	stale.Message = "Symbol not found in index (may have been deleted or renamed)"
	stale.Suggestions = c.findSuggestions(ref.NormalizedText)

	return stale
}

// renameInfo holds information about a renamed symbol.
type renameInfo struct {
	NewSymbolID string
	NewName     string
	Reason      string
	Confidence  float64
}

// checkForRename checks if a symbol was renamed/moved using the alias chain.
func (c *StalenessChecker) checkForRename(oldSymbolID string) *renameInfo {
	if c.identityResolver == nil {
		return nil
	}

	resolved, err := c.identityResolver.ResolveSymbolId(oldSymbolID)
	if err != nil {
		return nil
	}

	// Check if we followed a redirect (alias chain)
	if resolved.Redirected && resolved.Symbol != nil {
		newName := getSymbolDisplayName(resolved.Symbol)
		return &renameInfo{
			NewSymbolID: resolved.Symbol.StableId,
			NewName:     newName,
			Reason:      string(resolved.RedirectReason),
			Confidence:  resolved.RedirectConfidence,
		}
	}

	return nil
}

// getSymbolDisplayName extracts a human-readable name from a SymbolMapping.
func getSymbolDisplayName(sym *identity.SymbolMapping) string {
	if sym.Fingerprint != nil {
		if sym.Fingerprint.QualifiedContainer != "" && sym.Fingerprint.Name != "" {
			return sym.Fingerprint.QualifiedContainer + "." + sym.Fingerprint.Name
		}
		if sym.Fingerprint.Name != "" {
			return sym.Fingerprint.Name
		}
	}
	// Fallback to StableId
	return sym.StableId
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
