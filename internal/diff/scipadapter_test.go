package diff

import (
	"testing"

	"ckb/internal/backends/scip"
)

func TestConvertOccurrence(t *testing.T) {
	tests := []struct {
		name     string
		occ      *scip.Occurrence
		expected *OccurrenceInfo
	}{
		{
			name:     "nil occurrence",
			occ:      nil,
			expected: nil,
		},
		{
			name:     "empty range",
			occ:      &scip.Occurrence{Range: []int32{}},
			expected: nil,
		},
		{
			name:     "single element range",
			occ:      &scip.Occurrence{Range: []int32{5}},
			expected: nil,
		},
		{
			name: "two element range (line, col)",
			occ: &scip.Occurrence{
				Range:  []int32{10, 5},
				Symbol: "test#symbol",
			},
			expected: &OccurrenceInfo{
				StartLine:    11, // 0-indexed to 1-indexed
				EndLine:      11,
				StartCol:     5,
				EndCol:       0,
				Symbol:       "test#symbol",
				IsDefinition: false,
			},
		},
		{
			name: "three element range (line, startCol, endCol)",
			occ: &scip.Occurrence{
				Range:  []int32{10, 5, 20},
				Symbol: "test#symbol",
			},
			expected: &OccurrenceInfo{
				StartLine:    11,
				EndLine:      11,
				StartCol:     5,
				EndCol:       20,
				Symbol:       "test#symbol",
				IsDefinition: false,
			},
		},
		{
			name: "four element range (startLine, startCol, endLine, endCol)",
			occ: &scip.Occurrence{
				Range:  []int32{10, 5, 15, 25},
				Symbol: "test#symbol",
			},
			expected: &OccurrenceInfo{
				StartLine:    11,
				EndLine:      16,
				StartCol:     5,
				EndCol:       25,
				Symbol:       "test#symbol",
				IsDefinition: false,
			},
		},
		{
			name: "definition occurrence",
			occ: &scip.Occurrence{
				Range:       []int32{10, 5},
				Symbol:      "test#symbol",
				SymbolRoles: scip.SymbolRoleDefinition,
			},
			expected: &OccurrenceInfo{
				StartLine:    11,
				EndLine:      11,
				StartCol:     5,
				EndCol:       0,
				Symbol:       "test#symbol",
				IsDefinition: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertOccurrence(tt.occ)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatalf("expected %+v, got nil", tt.expected)
			}
			if result.StartLine != tt.expected.StartLine {
				t.Errorf("StartLine: got %d, want %d", result.StartLine, tt.expected.StartLine)
			}
			if result.EndLine != tt.expected.EndLine {
				t.Errorf("EndLine: got %d, want %d", result.EndLine, tt.expected.EndLine)
			}
			if result.StartCol != tt.expected.StartCol {
				t.Errorf("StartCol: got %d, want %d", result.StartCol, tt.expected.StartCol)
			}
			if result.EndCol != tt.expected.EndCol {
				t.Errorf("EndCol: got %d, want %d", result.EndCol, tt.expected.EndCol)
			}
			if result.Symbol != tt.expected.Symbol {
				t.Errorf("Symbol: got %q, want %q", result.Symbol, tt.expected.Symbol)
			}
			if result.IsDefinition != tt.expected.IsDefinition {
				t.Errorf("IsDefinition: got %v, want %v", result.IsDefinition, tt.expected.IsDefinition)
			}
		})
	}
}

func TestConvertSymbolDef(t *testing.T) {
	tests := []struct {
		name     string
		sym      *scip.SymbolInformation
		doc      *scip.Document
		expected *SymbolDefInfo
	}{
		{
			name:     "nil symbol",
			sym:      nil,
			doc:      &scip.Document{},
			expected: nil,
		},
		{
			name: "basic symbol with display name",
			sym: &scip.SymbolInformation{
				Symbol:      "test#MyFunc",
				DisplayName: "MyFunc",
				Kind:        12, // function
			},
			doc: &scip.Document{
				Occurrences: []*scip.Occurrence{
					{
						Symbol:      "test#MyFunc",
						SymbolRoles: scip.SymbolRoleDefinition,
						Range:       []int32{10, 5},
					},
				},
			},
			expected: &SymbolDefInfo{
				Symbol:    "test#MyFunc",
				Name:      "MyFunc",
				Kind:      "function",
				StartLine: 11,
				EndLine:   21, // Default +10
			},
		},
		{
			name: "symbol with enclosing range",
			sym: &scip.SymbolInformation{
				Symbol:      "test#BigFunc",
				DisplayName: "BigFunc",
				Kind:        6, // method
			},
			doc: &scip.Document{
				Occurrences: []*scip.Occurrence{
					{
						Symbol:         "test#BigFunc",
						SymbolRoles:    scip.SymbolRoleDefinition,
						Range:          []int32{20, 0},
						EnclosingRange: []int32{20, 0, 50, 0},
					},
				},
			},
			expected: &SymbolDefInfo{
				Symbol:    "test#BigFunc",
				Name:      "BigFunc",
				Kind:      "method",
				StartLine: 21,
				EndLine:   51,
			},
		},
		{
			name: "symbol without definition occurrence",
			sym: &scip.SymbolInformation{
				Symbol:      "test#Orphan",
				DisplayName: "Orphan",
				Kind:        13, // variable
			},
			doc: &scip.Document{
				Occurrences: []*scip.Occurrence{}, // No occurrences
			},
			expected: &SymbolDefInfo{
				Symbol:    "test#Orphan",
				Name:      "Orphan",
				Kind:      "variable",
				StartLine: 0,
				EndLine:   0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertSymbolDef(tt.sym, tt.doc)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatalf("expected %+v, got nil", tt.expected)
			}
			if result.Symbol != tt.expected.Symbol {
				t.Errorf("Symbol: got %q, want %q", result.Symbol, tt.expected.Symbol)
			}
			if result.Name != tt.expected.Name {
				t.Errorf("Name: got %q, want %q", result.Name, tt.expected.Name)
			}
			if result.Kind != tt.expected.Kind {
				t.Errorf("Kind: got %q, want %q", result.Kind, tt.expected.Kind)
			}
			if result.StartLine != tt.expected.StartLine {
				t.Errorf("StartLine: got %d, want %d", result.StartLine, tt.expected.StartLine)
			}
			if result.EndLine != tt.expected.EndLine {
				t.Errorf("EndLine: got %d, want %d", result.EndLine, tt.expected.EndLine)
			}
		})
	}
}

func TestScipKindToString(t *testing.T) {
	tests := []struct {
		kind     int32
		expected string
	}{
		{1, "file"},
		{2, "module"},
		{3, "namespace"},
		{4, "package"},
		{5, "class"},
		{6, "method"},
		{7, "property"},
		{8, "field"},
		{9, "constructor"},
		{10, "enum"},
		{11, "interface"},
		{12, "function"},
		{13, "variable"},
		{14, "constant"},
		{15, "string"},
		{16, "number"},
		{17, "boolean"},
		{18, "array"},
		{19, "object"},
		{20, "key"},
		{21, "null"},
		{22, "enum_member"},
		{23, "struct"},
		{24, "event"},
		{25, "operator"},
		{26, "type_parameter"},
		{0, "unknown"},
		{99, "unknown"},
		{-1, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := scipKindToString(tt.kind)
			if result != tt.expected {
				t.Errorf("scipKindToString(%d) = %q, want %q", tt.kind, result, tt.expected)
			}
		})
	}
}

func TestNewSCIPSymbolIndex(t *testing.T) {
	t.Run("nil index returns nil", func(t *testing.T) {
		result := NewSCIPSymbolIndex(nil)
		if result != nil {
			t.Errorf("expected nil for nil input, got %+v", result)
		}
	})

	t.Run("valid index returns wrapper", func(t *testing.T) {
		idx := &scip.SCIPIndex{}
		result := NewSCIPSymbolIndex(idx)
		if result == nil {
			t.Fatal("expected non-nil wrapper for valid index")
		}
		if result.index != idx {
			t.Error("wrapper should contain the same index")
		}
	})
}

func TestSCIPSymbolIndex_GetDocument(t *testing.T) {
	// Create a minimal SCIP index with test data
	idx := &scip.SCIPIndex{
		Documents: []*scip.Document{
			{
				RelativePath: "internal/foo.go",
				Language:     "go",
				Occurrences: []*scip.Occurrence{
					{
						Symbol:      "test#Foo",
						SymbolRoles: scip.SymbolRoleDefinition,
						Range:       []int32{10, 5, 20},
					},
				},
				Symbols: []*scip.SymbolInformation{
					{
						Symbol:      "test#Foo",
						DisplayName: "Foo",
						Kind:        12,
					},
				},
			},
		},
	}

	wrapper := NewSCIPSymbolIndex(idx)

	t.Run("existing document", func(t *testing.T) {
		doc := wrapper.GetDocument("internal/foo.go")
		if doc == nil {
			t.Fatal("expected document, got nil")
		}
		if doc.RelativePath != "internal/foo.go" {
			t.Errorf("RelativePath: got %q, want %q", doc.RelativePath, "internal/foo.go")
		}
		if doc.Language != "go" {
			t.Errorf("Language: got %q, want %q", doc.Language, "go")
		}
		if len(doc.Occurrences) != 1 {
			t.Errorf("Occurrences: got %d, want 1", len(doc.Occurrences))
		}
		if len(doc.Symbols) != 1 {
			t.Errorf("Symbols: got %d, want 1", len(doc.Symbols))
		}
	})

	t.Run("non-existing document", func(t *testing.T) {
		doc := wrapper.GetDocument("nonexistent.go")
		if doc != nil {
			t.Errorf("expected nil for nonexistent document, got %+v", doc)
		}
	})
}

func TestSCIPSymbolIndex_GetSymbolInfo(t *testing.T) {
	// Create a minimal SCIP index with test data
	idx := &scip.SCIPIndex{
		Symbols: map[string]*scip.SymbolInformation{
			"test#MyFunc": {
				Symbol:      "test#MyFunc",
				DisplayName: "MyFunc",
				Kind:        12,
			},
		},
		ConvertedSymbols: map[string]*scip.SCIPSymbol{
			"test#MyFunc": {
				Name:                "MyFunc",
				Kind:                "function",
				SignatureNormalized: "func MyFunc(x int) error",
			},
		},
	}

	wrapper := NewSCIPSymbolIndex(idx)

	t.Run("symbol with converted info", func(t *testing.T) {
		info := wrapper.GetSymbolInfo("test#MyFunc")
		if info == nil {
			t.Fatal("expected symbol info, got nil")
		}
		if info.Symbol != "test#MyFunc" {
			t.Errorf("Symbol: got %q, want %q", info.Symbol, "test#MyFunc")
		}
		if info.Name != "MyFunc" {
			t.Errorf("Name: got %q, want %q", info.Name, "MyFunc")
		}
		if info.Kind != "function" {
			t.Errorf("Kind: got %q, want %q", info.Kind, "function")
		}
		if info.Signature != "func MyFunc(x int) error" {
			t.Errorf("Signature: got %q, want %q", info.Signature, "func MyFunc(x int) error")
		}
	})

	t.Run("non-existing symbol", func(t *testing.T) {
		info := wrapper.GetSymbolInfo("nonexistent")
		if info != nil {
			t.Errorf("expected nil for nonexistent symbol, got %+v", info)
		}
	})
}
