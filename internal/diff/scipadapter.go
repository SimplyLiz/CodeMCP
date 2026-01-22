package diff

import (
	"ckb/internal/backends/scip"
)

// SCIPSymbolIndex adapts a SCIP index to the SymbolIndex interface
type SCIPSymbolIndex struct {
	index *scip.SCIPIndex
}

// NewSCIPSymbolIndex creates a new SCIPSymbolIndex from a SCIP index
func NewSCIPSymbolIndex(index *scip.SCIPIndex) *SCIPSymbolIndex {
	if index == nil {
		return nil
	}
	return &SCIPSymbolIndex{index: index}
}

// GetDocument returns the document for a file path, or nil if not found
func (s *SCIPSymbolIndex) GetDocument(filePath string) *DocumentInfo {
	doc := s.index.GetDocument(filePath)
	if doc == nil {
		return nil
	}

	// Convert occurrences
	occurrences := make([]OccurrenceInfo, 0, len(doc.Occurrences))
	for _, occ := range doc.Occurrences {
		occInfo := convertOccurrence(occ)
		if occInfo != nil {
			occurrences = append(occurrences, *occInfo)
		}
	}

	// Convert symbol definitions
	symbols := make([]SymbolDefInfo, 0, len(doc.Symbols))
	for _, sym := range doc.Symbols {
		symDef := convertSymbolDef(sym, doc)
		if symDef != nil {
			symbols = append(symbols, *symDef)
		}
	}

	return &DocumentInfo{
		RelativePath: doc.RelativePath,
		Language:     doc.Language,
		Occurrences:  occurrences,
		Symbols:      symbols,
	}
}

// GetSymbolInfo returns symbol information for a symbol ID
func (s *SCIPSymbolIndex) GetSymbolInfo(symbolID string) *SymbolInfo {
	sym := s.index.GetSymbol(symbolID)
	if sym == nil {
		return nil
	}

	// Get the pre-converted symbol for additional info
	var name, kind, signature string
	if converted, exists := s.index.ConvertedSymbols[symbolID]; exists {
		name = converted.Name
		kind = string(converted.Kind)
		signature = converted.SignatureNormalized
	} else {
		// Fall back to extracting name from display name
		name = sym.DisplayName
		if name == "" {
			name = extractSymbolName(sym.Symbol)
		}
		kind = scipKindToString(sym.Kind)
	}

	return &SymbolInfo{
		Symbol:    sym.Symbol,
		Name:      name,
		Kind:      kind,
		Signature: signature,
	}
}

// convertOccurrence converts a SCIP occurrence to OccurrenceInfo
func convertOccurrence(occ *scip.Occurrence) *OccurrenceInfo {
	if occ == nil || len(occ.Range) < 2 {
		return nil
	}

	startLine := int(occ.Range[0]) + 1 // Convert to 1-indexed
	endLine := startLine
	startCol := 0
	endCol := 0

	if len(occ.Range) >= 2 {
		startCol = int(occ.Range[1])
	}
	if len(occ.Range) >= 3 {
		endCol = int(occ.Range[2])
		// If only 3 elements, end is on same line
	}
	if len(occ.Range) >= 4 {
		endLine = int(occ.Range[2]) + 1 // Convert to 1-indexed
		endCol = int(occ.Range[3])
	}

	isDefinition := (occ.SymbolRoles & scip.SymbolRoleDefinition) != 0

	return &OccurrenceInfo{
		StartLine:    startLine,
		EndLine:      endLine,
		StartCol:     startCol,
		EndCol:       endCol,
		Symbol:       occ.Symbol,
		IsDefinition: isDefinition,
	}
}

// convertSymbolDef converts a SCIP symbol to SymbolDefInfo
func convertSymbolDef(sym *scip.SymbolInformation, doc *scip.Document) *SymbolDefInfo {
	if sym == nil {
		return nil
	}

	// Find the definition occurrence to get location
	var startLine, endLine int
	for _, occ := range doc.Occurrences {
		if occ.Symbol == sym.Symbol && (occ.SymbolRoles&scip.SymbolRoleDefinition) != 0 {
			if len(occ.Range) >= 1 {
				startLine = int(occ.Range[0]) + 1 // Convert to 1-indexed
			}
			// Use enclosing range for end line if available
			if len(occ.EnclosingRange) >= 3 {
				endLine = int(occ.EnclosingRange[2]) + 1 // Convert to 1-indexed
			} else if len(occ.Range) >= 3 {
				endLine = int(occ.Range[2]) + 1
			} else {
				endLine = startLine + 10 // Default assumption for body
			}
			break
		}
	}

	name := sym.DisplayName
	if name == "" {
		name = extractSymbolName(sym.Symbol)
	}

	return &SymbolDefInfo{
		Symbol:    sym.Symbol,
		Name:      name,
		Kind:      scipKindToString(sym.Kind),
		StartLine: startLine,
		EndLine:   endLine,
	}
}

// scipKindToString converts SCIP kind int to string
func scipKindToString(kind int32) string {
	// SCIP kind enum values (from protocol)
	switch kind {
	case 1:
		return "file"
	case 2:
		return "module"
	case 3:
		return "namespace"
	case 4:
		return "package"
	case 5:
		return "class"
	case 6:
		return "method"
	case 7:
		return "property"
	case 8:
		return "field"
	case 9:
		return "constructor"
	case 10:
		return "enum"
	case 11:
		return "interface"
	case 12:
		return "function"
	case 13:
		return "variable"
	case 14:
		return "constant"
	case 15:
		return "string"
	case 16:
		return "number"
	case 17:
		return "boolean"
	case 18:
		return "array"
	case 19:
		return "object"
	case 20:
		return "key"
	case 21:
		return "null"
	case 22:
		return "enum_member"
	case 23:
		return "struct"
	case 24:
		return "event"
	case 25:
		return "operator"
	case 26:
		return "type_parameter"
	default:
		return "unknown"
	}
}
