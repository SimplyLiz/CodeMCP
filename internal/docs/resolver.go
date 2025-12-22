package docs

import (
	"strings"
)

// SymbolIndex is the interface for accessing symbols.
type SymbolIndex interface {
	// ExactMatch finds a symbol by exact canonical name.
	ExactMatch(canonicalName string) (symbolID string, found bool)

	// GetDisplayName returns the human-friendly name for a symbol ID.
	GetDisplayName(symbolID string) string

	// Exists checks if a symbol ID is still valid in the index.
	Exists(symbolID string) bool

	// IsLanguageIndexed checks if a language is indexed at current tier.
	IsLanguageIndexed(hint string) bool
}

// ResolverConfig contains configuration for the resolver.
type ResolverConfig struct {
	MinSegments        int  // Minimum segments for suffix match (default: 2)
	AllowSingleSegment bool // Allow single segment like "Authenticate" (default: false)
}

// DefaultResolverConfig returns the default resolver configuration.
func DefaultResolverConfig() ResolverConfig {
	return ResolverConfig{
		MinSegments:        2,
		AllowSingleSegment: false,
	}
}

// Resolver resolves raw symbol mentions to SCIP symbol IDs.
type Resolver struct {
	symbolIndex SymbolIndex
	store       *Store
	config      ResolverConfig
}

// NewResolver creates a new resolver.
func NewResolver(symbolIndex SymbolIndex, store *Store, config ResolverConfig) *Resolver {
	return &Resolver{
		symbolIndex: symbolIndex,
		store:       store,
		config:      config,
	}
}

// Resolve attempts to resolve a raw mention to a symbol ID.
func (r *Resolver) Resolve(rawText string) ResolutionResult {
	normalized := Normalize(rawText)
	segments := CountSegments(normalized)

	// Enforce minimum segment requirement
	// This is NOT ambiguous - it's ineligible (never attempted matching)
	if segments < r.config.MinSegments && !r.config.AllowSingleSegment {
		return ResolutionResult{
			Status:     ResolutionIneligible,
			Confidence: 0.0,
			Message:    "Single-segment names require directive. Use <!-- ckb:symbol full.path -->",
		}
	}

	// Step 1: Try exact match (fully qualified)
	if symbolID, found := r.symbolIndex.ExactMatch(normalized); found {
		return ResolutionResult{
			Status:     ResolutionExact,
			SymbolID:   symbolID,
			SymbolName: r.symbolIndex.GetDisplayName(symbolID),
			Confidence: 1.0,
		}
	}

	// Step 2: Try suffix match (must be unique)
	candidates, err := r.store.SuffixMatch(normalized)
	if err != nil {
		return ResolutionResult{
			Status:  ResolutionMissing,
			Message: "Failed to query suffix index",
		}
	}

	switch len(candidates) {
	case 0:
		return ResolutionResult{
			Status:  ResolutionMissing,
			Message: "Symbol not found in index",
		}
	case 1:
		return ResolutionResult{
			Status:     ResolutionSuffix,
			SymbolID:   candidates[0],
			SymbolName: r.symbolIndex.GetDisplayName(candidates[0]),
			Confidence: 0.95,
		}
	default:
		return ResolutionResult{
			Status:     ResolutionAmbiguous,
			Candidates: candidates,
			Message:    "Multiple matches. Add <!-- ckb:symbol full.path --> to disambiguate.",
		}
	}
}

// SuffixIndex manages the suffix lookup table.
type SuffixIndex struct {
	store *Store
}

// NewSuffixIndex creates a new suffix index.
func NewSuffixIndex(store *Store) *SuffixIndex {
	return &SuffixIndex{store: store}
}

// Symbol represents a symbol for suffix index building.
type Symbol struct {
	ID            string // SCIP symbol ID (unique)
	CanonicalName string // Fully qualified: "internal/auth.UserService.Authenticate"
	DisplayName   string // For UI: "UserService.Authenticate"
}

// Build builds the suffix index from a list of symbols.
func (s *SuffixIndex) Build(symbols []Symbol, version string) error {
	// Clear existing suffixes
	if err := s.store.ClearSuffixIndex(); err != nil {
		return err
	}

	// Build suffix entries for each symbol
	for _, sym := range symbols {
		suffixes := GenerateSuffixes(sym.CanonicalName)
		if err := s.store.SaveSuffixes(sym.ID, suffixes); err != nil {
			return err
		}
	}

	// Store the symbol index version
	return s.store.SetSymbolIndexVersion(version)
}

// GenerateSuffixes creates all valid suffix forms of a canonical name.
// For "internal/auth.UserService.Authenticate":
//   - "UserService.Authenticate" (2 segments)
//   - "auth.UserService.Authenticate" (3 segments)
//   - "internal.auth.UserService.Authenticate" (4 segments, full - normalized)
func GenerateSuffixes(canonicalName string) []string {
	// Normalize path separators to dots for consistent suffix matching
	normalized := strings.ReplaceAll(canonicalName, "/", ".")

	parts := strings.Split(normalized, ".")
	if len(parts) == 0 {
		return nil
	}

	var suffixes []string

	// Generate suffixes from 2 segments up to full name
	for i := len(parts) - 2; i >= 0; i-- {
		suffix := strings.Join(parts[i:], ".")
		suffixes = append(suffixes, suffix)
	}

	return suffixes
}

// ParseCanonicalName extracts the canonical name from a SCIP symbol ID.
// SCIP format: "scip-{lang} {manager} {name} {version} {descriptor}"
// Example: "scip-go gomod github.com/foo/ckb 1.0.0 internal/auth.UserService.Authenticate()."
// Returns: "internal/auth.UserService.Authenticate"
func ParseCanonicalName(scipSymbol string) string {
	parts := strings.Split(scipSymbol, " ")
	if len(parts) < 5 {
		return scipSymbol // fallback
	}

	descriptor := parts[4]

	// Strip trailing () and . (method signatures, package markers)
	descriptor = strings.TrimSuffix(descriptor, "().")
	descriptor = strings.TrimSuffix(descriptor, "()")
	descriptor = strings.TrimSuffix(descriptor, ".")

	return descriptor
}

// ExtractDisplayName extracts a human-friendly display name from a SCIP symbol ID.
// Returns the last 2-3 segments (Class.Method or Package.Class.Method)
func ExtractDisplayName(scipSymbol string) string {
	canonical := ParseCanonicalName(scipSymbol)

	// Normalize to dots
	normalized := strings.ReplaceAll(canonical, "/", ".")

	parts := strings.Split(normalized, ".")
	if len(parts) <= 2 {
		return canonical
	}

	// Take last 2 segments for method (Class.Method)
	// or last 3 for nested (Outer.Inner.Method)
	start := len(parts) - 2
	if start < 0 {
		start = 0
	}

	return strings.Join(parts[start:], ".")
}
