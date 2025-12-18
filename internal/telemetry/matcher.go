package telemetry

import (
	"strings"
)

// SymbolIndex provides symbol lookup capabilities for matching
type SymbolIndex interface {
	// FindByLocation finds a symbol at a specific file:line location
	FindByLocation(filePath string, line int) *IndexedSymbol
	// FindByFile returns all symbols in a file
	FindByFile(filePath string) []*IndexedSymbol
	// FindByNamespace returns all symbols in a namespace/package
	FindByNamespace(namespace string) []*IndexedSymbol
	// FindByName returns all symbols with the given name
	FindByName(name string) []*IndexedSymbol
}

// IndexedSymbol represents a symbol in the index
type IndexedSymbol struct {
	ID        string
	Name      string
	File      string
	Line      int
	Namespace string
	Kind      string // "function", "method", etc.
}

// Matcher matches telemetry calls to SCIP symbols
type Matcher struct {
	index SymbolIndex
}

// NewMatcher creates a new symbol matcher
func NewMatcher(index SymbolIndex) *Matcher {
	return &Matcher{index: index}
}

// Match attempts to match a call aggregate to a symbol
func (m *Matcher) Match(call *CallAggregate) SymbolMatch {
	// 1. Try exact match (file + function + line)
	if call.FilePath != "" && call.LineNumber > 0 {
		if symbol := m.index.FindByLocation(call.FilePath, call.LineNumber); symbol != nil {
			if namesMatch(symbol.Name, call.FunctionName) {
				return SymbolMatch{
					SymbolID:   symbol.ID,
					Quality:    MatchExact,
					Confidence: MatchExact.Confidence(),
					MatchBasis: []string{"file_path", "function_name", "line_number"},
				}
			}
		}
	}

	// 2. Try strong match (file + function)
	if call.FilePath != "" {
		symbols := m.index.FindByFile(call.FilePath)
		match := findUniqueByName(symbols, call.FunctionName)
		if match != nil {
			return SymbolMatch{
				SymbolID:   match.ID,
				Quality:    MatchStrong,
				Confidence: MatchStrong.Confidence(),
				MatchBasis: []string{"file_path", "function_name"},
			}
		}
		// Multiple matches in file = ambiguous, fall through to weak
	}

	// 3. Try weak match (namespace + function)
	if call.Namespace != "" {
		symbols := m.index.FindByNamespace(call.Namespace)
		matches := filterByName(symbols, call.FunctionName)

		if len(matches) == 1 {
			return SymbolMatch{
				SymbolID:   matches[0].ID,
				Quality:    MatchWeak,
				Confidence: MatchWeak.Confidence(),
				MatchBasis: []string{"namespace", "function_name"},
			}
		}

		if len(matches) > 1 {
			// Ambiguous - multiple symbols with same name in namespace
			return SymbolMatch{
				Quality:    MatchUnmatched,
				Confidence: 0,
				MatchBasis: []string{"ambiguous_function_name"},
			}
		}
	}

	// 4. Last resort: global name search (very weak)
	if call.FunctionName != "" {
		symbols := m.index.FindByName(call.FunctionName)
		if len(symbols) == 1 {
			return SymbolMatch{
				SymbolID:   symbols[0].ID,
				Quality:    MatchWeak,
				Confidence: 0.50, // Even lower confidence for global name match
				MatchBasis: []string{"function_name_global"},
			}
		}
	}

	// 5. Unmatched
	return SymbolMatch{
		Quality:    MatchUnmatched,
		Confidence: 0,
		MatchBasis: []string{"no_match"},
	}
}

// MatchBatch matches multiple calls efficiently
func (m *Matcher) MatchBatch(calls []*CallAggregate) map[*CallAggregate]SymbolMatch {
	results := make(map[*CallAggregate]SymbolMatch, len(calls))
	for _, call := range calls {
		results[call] = m.Match(call)
	}
	return results
}

// namesMatch checks if two function names match (case-sensitive with normalization)
func namesMatch(indexName, telemetryName string) bool {
	// Exact match
	if indexName == telemetryName {
		return true
	}

	// Handle method receivers: "(*Foo).Bar" vs "Bar"
	if idx := strings.LastIndex(indexName, "."); idx >= 0 {
		if indexName[idx+1:] == telemetryName {
			return true
		}
	}

	// Handle package prefix: "pkg.Function" vs "Function"
	if idx := strings.LastIndex(telemetryName, "."); idx >= 0 {
		if telemetryName[idx+1:] == indexName {
			return true
		}
	}

	return false
}

// findUniqueByName finds a unique symbol by name in a list
func findUniqueByName(symbols []*IndexedSymbol, name string) *IndexedSymbol {
	var match *IndexedSymbol
	count := 0

	for _, s := range symbols {
		if namesMatch(s.Name, name) {
			match = s
			count++
		}
	}

	if count == 1 {
		return match
	}
	return nil // No match or ambiguous
}

// filterByName returns all symbols matching the given name
func filterByName(symbols []*IndexedSymbol, name string) []*IndexedSymbol {
	var matches []*IndexedSymbol
	for _, s := range symbols {
		if namesMatch(s.Name, name) {
			matches = append(matches, s)
		}
	}
	return matches
}

// SCIPSymbolIndex implements SymbolIndex using a SCIP index
type SCIPSymbolIndex struct {
	// Maps file path -> symbols in that file
	byFile map[string][]*IndexedSymbol
	// Maps namespace -> symbols in that namespace
	byNamespace map[string][]*IndexedSymbol
	// Maps name -> symbols with that name
	byName map[string][]*IndexedSymbol
	// Maps file:line -> symbol at that location
	byLocation map[string]*IndexedSymbol
}

// NewSCIPSymbolIndex creates a new SCIP-based symbol index
func NewSCIPSymbolIndex() *SCIPSymbolIndex {
	return &SCIPSymbolIndex{
		byFile:      make(map[string][]*IndexedSymbol),
		byNamespace: make(map[string][]*IndexedSymbol),
		byName:      make(map[string][]*IndexedSymbol),
		byLocation:  make(map[string]*IndexedSymbol),
	}
}

// AddSymbol adds a symbol to the index
func (idx *SCIPSymbolIndex) AddSymbol(symbol *IndexedSymbol) {
	// Index by file
	idx.byFile[symbol.File] = append(idx.byFile[symbol.File], symbol)

	// Index by namespace
	if symbol.Namespace != "" {
		idx.byNamespace[symbol.Namespace] = append(idx.byNamespace[symbol.Namespace], symbol)
	}

	// Index by name
	idx.byName[symbol.Name] = append(idx.byName[symbol.Name], symbol)

	// Index by location
	if symbol.Line > 0 {
		key := locationKey(symbol.File, symbol.Line)
		idx.byLocation[key] = symbol
	}
}

func locationKey(file string, line int) string {
	return file + ":" + string(rune(line))
}

// FindByLocation implements SymbolIndex
func (idx *SCIPSymbolIndex) FindByLocation(filePath string, line int) *IndexedSymbol {
	return idx.byLocation[locationKey(filePath, line)]
}

// FindByFile implements SymbolIndex
func (idx *SCIPSymbolIndex) FindByFile(filePath string) []*IndexedSymbol {
	return idx.byFile[filePath]
}

// FindByNamespace implements SymbolIndex
func (idx *SCIPSymbolIndex) FindByNamespace(namespace string) []*IndexedSymbol {
	return idx.byNamespace[namespace]
}

// FindByName implements SymbolIndex
func (idx *SCIPSymbolIndex) FindByName(name string) []*IndexedSymbol {
	return idx.byName[name]
}

// SymbolCount returns the total number of indexed symbols
func (idx *SCIPSymbolIndex) SymbolCount() int {
	count := 0
	for _, symbols := range idx.byFile {
		count += len(symbols)
	}
	return count
}

// Clear removes all symbols from the index
func (idx *SCIPSymbolIndex) Clear() {
	idx.byFile = make(map[string][]*IndexedSymbol)
	idx.byNamespace = make(map[string][]*IndexedSymbol)
	idx.byName = make(map[string][]*IndexedSymbol)
	idx.byLocation = make(map[string]*IndexedSymbol)
}
