package diff

import (
	"fmt"
	"sort"
	"strings"

	"ckb/internal/impact"
)

// SymbolIndex is an interface for querying symbols by file and line
// This abstracts the SCIP index to allow for testing and flexibility
type SymbolIndex interface {
	// GetDocument returns the document for a file path, or nil if not found
	GetDocument(filePath string) *DocumentInfo

	// GetSymbolInfo returns symbol information for a symbol ID
	GetSymbolInfo(symbolID string) *SymbolInfo
}

// DocumentInfo contains document data needed for symbol mapping
type DocumentInfo struct {
	RelativePath string
	Language     string
	Occurrences  []OccurrenceInfo
	Symbols      []SymbolDefInfo
}

// OccurrenceInfo contains occurrence data
type OccurrenceInfo struct {
	StartLine    int
	EndLine      int
	StartCol     int
	EndCol       int
	Symbol       string
	IsDefinition bool
}

// SymbolDefInfo contains symbol definition data
type SymbolDefInfo struct {
	Symbol    string
	Name      string
	Kind      string
	StartLine int
	EndLine   int
}

// SymbolInfo contains symbol metadata
type SymbolInfo struct {
	Symbol    string
	Name      string
	Kind      string
	Signature string
}

// DiffSymbolMapper maps diff changes to SCIP symbols
type DiffSymbolMapper struct {
	index SymbolIndex
}

// NewDiffSymbolMapper creates a new DiffSymbolMapper
func NewDiffSymbolMapper(index SymbolIndex) *DiffSymbolMapper {
	return &DiffSymbolMapper{index: index}
}

// MapToSymbols maps changed lines in a ParsedDiff to symbols
func (m *DiffSymbolMapper) MapToSymbols(diff *impact.ParsedDiff) ([]impact.ChangedSymbol, error) {
	if diff == nil {
		return nil, nil
	}

	var result []impact.ChangedSymbol
	seen := make(map[string]bool) // Dedupe by symbol ID

	for _, file := range diff.Files {
		symbols, err := m.mapFileToSymbols(&file)
		if err != nil {
			// Log but continue with other files
			continue
		}

		for _, sym := range symbols {
			if !seen[sym.SymbolID] {
				seen[sym.SymbolID] = true
				result = append(result, sym)
			} else {
				// Update existing with merged info if needed
				for i := range result {
					if result[i].SymbolID == sym.SymbolID {
						result[i].Lines = mergeLines(result[i].Lines, sym.Lines)
						// Keep higher confidence
						if sym.Confidence > result[i].Confidence {
							result[i].Confidence = sym.Confidence
						}
						break
					}
				}
			}
		}
	}

	// Sort by confidence descending, then by name
	sort.Slice(result, func(i, j int) bool {
		if result[i].Confidence != result[j].Confidence {
			return result[i].Confidence > result[j].Confidence
		}
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// mapFileToSymbols maps changes in a single file to symbols
func (m *DiffSymbolMapper) mapFileToSymbols(file *impact.ChangedFile) ([]impact.ChangedSymbol, error) {
	// Determine the file path to look up
	filePath := file.NewPath
	if file.Deleted {
		filePath = file.OldPath
	}
	if filePath == "" {
		return nil, nil
	}

	// Get document from index
	doc := m.index.GetDocument(filePath)
	if doc == nil {
		// Try without leading slash or with different path formats
		doc = m.index.GetDocument(strings.TrimPrefix(filePath, "/"))
	}
	if doc == nil {
		// File not in index - return with low confidence
		return m.mapFileWithoutIndex(file)
	}

	// Determine change type
	changeType := impact.ChangeModified
	if file.IsNew {
		changeType = impact.ChangeAdded
	} else if file.Deleted {
		changeType = impact.ChangeDeleted
	}

	var result []impact.ChangedSymbol

	// Get all changed lines
	changedLines := make(map[int]int) // line -> hunk index
	for hunkIdx, hunk := range file.Hunks {
		for _, line := range hunk.Added {
			changedLines[line] = hunkIdx
		}
		// For deletions, we need to find symbols that were on removed lines
		// This requires looking at the old file, which we may not have
		// For now, focus on added/modified lines
	}

	// Find symbols that overlap with changed lines
	symbolLines := make(map[string][]int) // symbol -> lines
	symbolHunks := make(map[string]int)   // symbol -> first hunk index
	symbolConfidence := make(map[string]float64)

	// First pass: find definitions that span changed lines
	for _, symDef := range doc.Symbols {
		for line, hunkIdx := range changedLines {
			if line >= symDef.StartLine && line <= symDef.EndLine {
				symbolLines[symDef.Symbol] = append(symbolLines[symDef.Symbol], line)
				if _, exists := symbolHunks[symDef.Symbol]; !exists {
					symbolHunks[symDef.Symbol] = hunkIdx
				}
				// High confidence for definition match
				if symDef.StartLine == line {
					symbolConfidence[symDef.Symbol] = 1.0 // Exact definition line
				} else if symbolConfidence[symDef.Symbol] < 0.8 {
					symbolConfidence[symDef.Symbol] = 0.8 // Symbol body contains change
				}
			}
		}
	}

	// Second pass: find occurrences (references) on changed lines
	for _, occ := range doc.Occurrences {
		for line, hunkIdx := range changedLines {
			if occ.StartLine == line || (line >= occ.StartLine && line <= occ.EndLine) {
				if _, exists := symbolLines[occ.Symbol]; !exists {
					symbolLines[occ.Symbol] = []int{}
					symbolHunks[occ.Symbol] = hunkIdx
				}
				symbolLines[occ.Symbol] = append(symbolLines[occ.Symbol], line)

				// Confidence based on occurrence type
				if occ.IsDefinition {
					symbolConfidence[occ.Symbol] = 1.0
				} else if symbolConfidence[occ.Symbol] < 0.7 {
					symbolConfidence[occ.Symbol] = 0.7 // Reference on changed line
				}
			}
		}
	}

	// Build result
	for symbolID, lines := range symbolLines {
		// Get symbol info
		symInfo := m.index.GetSymbolInfo(symbolID)
		name := extractSymbolName(symbolID)
		if symInfo != nil && symInfo.Name != "" {
			name = symInfo.Name
		}

		confidence := symbolConfidence[symbolID]
		if confidence == 0 {
			confidence = 0.5 // Default confidence
		}

		// Lower confidence for whitespace-only or comment changes
		// (This would require parsing the actual content, simplified here)

		result = append(result, impact.ChangedSymbol{
			SymbolID:   symbolID,
			Name:       name,
			File:       filePath,
			ChangeType: changeType,
			Lines:      uniqueLines(lines),
			Confidence: confidence,
			HunkIndex:  symbolHunks[symbolID],
		})
	}

	return result, nil
}

// mapFileWithoutIndex creates low-confidence entries for files not in the index
func (m *DiffSymbolMapper) mapFileWithoutIndex(file *impact.ChangedFile) ([]impact.ChangedSymbol, error) {
	filePath := file.NewPath
	if file.Deleted {
		filePath = file.OldPath
	}
	if filePath == "" {
		return nil, nil
	}

	changeType := impact.ChangeModified
	if file.IsNew {
		changeType = impact.ChangeAdded
	} else if file.Deleted {
		changeType = impact.ChangeDeleted
	}

	// Collect all changed lines (added for new/modified, removed for deleted)
	var allLines []int
	for _, hunk := range file.Hunks {
		if file.Deleted {
			allLines = append(allLines, hunk.Removed...)
		} else {
			allLines = append(allLines, hunk.Added...)
		}
	}

	if len(allLines) == 0 && len(file.Hunks) == 0 {
		return nil, nil
	}

	// Create a single entry for the file with low confidence
	return []impact.ChangedSymbol{{
		SymbolID:   fmt.Sprintf("file:%s", filePath),
		Name:       filePath,
		File:       filePath,
		ChangeType: changeType,
		Lines:      uniqueLines(allLines),
		Confidence: 0.3, // Low confidence - no index data
		HunkIndex:  0,
	}}, nil
}

// extractSymbolName extracts a readable name from a SCIP symbol ID
func extractSymbolName(symbolID string) string {
	// SCIP symbol format: scheme manager package descriptor name
	// Example: scip-go gomod github.com/foo/bar pkg.Function().
	parts := strings.Split(symbolID, " ")
	if len(parts) > 0 {
		// Get the last part which is usually the name
		last := parts[len(parts)-1]
		// Remove trailing (). for function symbols
		last = strings.TrimSuffix(last, "().")
		// Remove trailing () for functions without trailing dot
		last = strings.TrimSuffix(last, "()")
		// Remove trailing . for types
		last = strings.TrimSuffix(last, ".")
		return last
	}
	return symbolID
}

// mergeLines merges two line slices and returns unique sorted lines
func mergeLines(a, b []int) []int {
	seen := make(map[int]bool)
	for _, l := range a {
		seen[l] = true
	}
	for _, l := range b {
		seen[l] = true
	}
	result := make([]int, 0, len(seen))
	for l := range seen {
		result = append(result, l)
	}
	sort.Ints(result)
	return result
}

// uniqueLines returns unique sorted lines
func uniqueLines(lines []int) []int {
	if len(lines) == 0 {
		return lines
	}
	seen := make(map[int]bool)
	result := make([]int, 0, len(lines))
	for _, l := range lines {
		if !seen[l] {
			seen[l] = true
			result = append(result, l)
		}
	}
	sort.Ints(result)
	return result
}
