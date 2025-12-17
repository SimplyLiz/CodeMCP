package impact

import (
	"strings"
)

// Visibility represents the visibility level of a symbol
type Visibility string

const (
	VisibilityPublic   Visibility = "public"
	VisibilityInternal Visibility = "internal"
	VisibilityPrivate  Visibility = "private"
	VisibilityUnknown  Visibility = "unknown"
)

// VisibilityInfo contains visibility information with confidence score
type VisibilityInfo struct {
	Visibility Visibility // Derived visibility level
	Confidence float64    // Confidence score (0.0 - 1.0)
	Source     string     // Source of the visibility information
}

// DeriveVisibility determines visibility with cascading fallback:
// 1. SCIP/Glean modifiers (confidence 0.95)
// 2. Reference analysis - external refs = public (confidence 0.9)
// 3. Naming conventions - _ or # prefix = private (confidence 0.6)
func DeriveVisibility(symbol *Symbol, refs []Reference) *VisibilityInfo {
	// Strategy 1: Check SCIP/Glean modifiers
	if info := deriveFromModifiers(symbol); info != nil {
		return info
	}

	// Strategy 2: Analyze references
	if info := deriveFromReferences(symbol, refs); info != nil {
		return info
	}

	// Strategy 3: Check naming conventions
	if info := deriveFromNaming(symbol); info != nil {
		return info
	}

	// Fallback: unknown visibility
	return &VisibilityInfo{
		Visibility: VisibilityUnknown,
		Confidence: 0.0,
		Source:     "unknown",
	}
}

// deriveFromModifiers extracts visibility from SCIP modifiers
func deriveFromModifiers(symbol *Symbol) *VisibilityInfo {
	if len(symbol.Modifiers) == 0 {
		return nil
	}

	// Check for explicit visibility modifiers
	for _, modifier := range symbol.Modifiers {
		switch strings.ToLower(modifier) {
		case "public":
			return &VisibilityInfo{
				Visibility: VisibilityPublic,
				Confidence: 0.95,
				Source:     "scip-modifiers",
			}
		case "private":
			return &VisibilityInfo{
				Visibility: VisibilityPrivate,
				Confidence: 0.95,
				Source:     "scip-modifiers",
			}
		case "internal", "package", "protected":
			return &VisibilityInfo{
				Visibility: VisibilityInternal,
				Confidence: 0.95,
				Source:     "scip-modifiers",
			}
		}
	}

	return nil
}

// deriveFromReferences analyzes references to determine visibility
func deriveFromReferences(symbol *Symbol, refs []Reference) *VisibilityInfo {
	if len(refs) == 0 {
		return nil
	}

	hasExternalRefs := false
	hasInternalRefs := false

	// Analyze references from different modules
	for _, ref := range refs {
		if ref.FromModule == "" {
			continue
		}

		if ref.FromModule != symbol.ModuleId {
			hasExternalRefs = true
		} else {
			hasInternalRefs = true
		}
	}

	// If symbol is referenced from external modules, it's likely public
	if hasExternalRefs {
		return &VisibilityInfo{
			Visibility: VisibilityPublic,
			Confidence: 0.9,
			Source:     "ref-analysis",
		}
	}

	// If only internal references exist, it's likely internal or private
	if hasInternalRefs {
		return &VisibilityInfo{
			Visibility: VisibilityInternal,
			Confidence: 0.7,
			Source:     "ref-analysis",
		}
	}

	return nil
}

// deriveFromNaming checks naming conventions for visibility hints
func deriveFromNaming(symbol *Symbol) *VisibilityInfo {
	if symbol.Name == "" {
		return nil
	}

	// Check for common private naming conventions
	// Underscore prefix (Python, TypeScript)
	if strings.HasPrefix(symbol.Name, "_") {
		return &VisibilityInfo{
			Visibility: VisibilityPrivate,
			Confidence: 0.6,
			Source:     "naming-convention",
		}
	}

	// Hash prefix (Ruby, Perl)
	if strings.HasPrefix(symbol.Name, "#") {
		return &VisibilityInfo{
			Visibility: VisibilityPrivate,
			Confidence: 0.6,
			Source:     "naming-convention",
		}
	}

	// Double underscore prefix (Python name mangling)
	if strings.HasPrefix(symbol.Name, "__") && !strings.HasSuffix(symbol.Name, "__") {
		return &VisibilityInfo{
			Visibility: VisibilityPrivate,
			Confidence: 0.7,
			Source:     "naming-convention",
		}
	}

	// Lowercase first letter can indicate package-private in some languages (Go)
	if len(symbol.Name) > 0 && symbol.Name[0] >= 'a' && symbol.Name[0] <= 'z' {
		// Check symbol kind - this is more reliable for certain kinds
		if symbol.Kind == KindFunction || symbol.Kind == KindType || symbol.Kind == KindConstant {
			return &VisibilityInfo{
				Visibility: VisibilityInternal,
				Confidence: 0.5,
				Source:     "naming-convention",
			}
		}
	}

	// Uppercase first letter often indicates public in Go
	if len(symbol.Name) > 0 && symbol.Name[0] >= 'A' && symbol.Name[0] <= 'Z' {
		if symbol.Kind == KindFunction || symbol.Kind == KindType || symbol.Kind == KindConstant {
			return &VisibilityInfo{
				Visibility: VisibilityPublic,
				Confidence: 0.5,
				Source:     "naming-convention",
			}
		}
	}

	return nil
}
