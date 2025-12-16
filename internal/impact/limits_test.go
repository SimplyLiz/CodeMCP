package impact

import (
	"testing"
)

func TestNewAnalysisLimits(t *testing.T) {
	limits := NewAnalysisLimits()

	if limits.TypeContext != TypeContextNone {
		t.Errorf("expected TypeContextNone, got %s", limits.TypeContext)
	}

	if limits.Notes == nil {
		t.Error("expected non-nil Notes slice")
	}

	if len(limits.Notes) != 0 {
		t.Errorf("expected empty Notes slice, got %d items", len(limits.Notes))
	}
}

func TestAddNote(t *testing.T) {
	limits := NewAnalysisLimits()

	limits.AddNote("First note")
	if len(limits.Notes) != 1 {
		t.Errorf("expected 1 note, got %d", len(limits.Notes))
	}
	if limits.Notes[0] != "First note" {
		t.Errorf("expected 'First note', got '%s'", limits.Notes[0])
	}

	limits.AddNote("Second note")
	if len(limits.Notes) != 2 {
		t.Errorf("expected 2 notes, got %d", len(limits.Notes))
	}
	if limits.Notes[1] != "Second note" {
		t.Errorf("expected 'Second note', got '%s'", limits.Notes[1])
	}
}

func TestHasLimitations(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() *AnalysisLimits
		expected  bool
	}{
		{
			name: "no limitations",
			setupFunc: func() *AnalysisLimits {
				limits := NewAnalysisLimits()
				limits.TypeContext = TypeContextFull
				return limits
			},
			expected: false,
		},
		{
			name: "has note",
			setupFunc: func() *AnalysisLimits {
				limits := NewAnalysisLimits()
				limits.TypeContext = TypeContextFull
				limits.AddNote("Some limitation")
				return limits
			},
			expected: true,
		},
		{
			name: "partial type context",
			setupFunc: func() *AnalysisLimits {
				limits := NewAnalysisLimits()
				limits.TypeContext = TypeContextPartial
				return limits
			},
			expected: true,
		},
		{
			name: "no type context",
			setupFunc: func() *AnalysisLimits {
				limits := NewAnalysisLimits()
				limits.TypeContext = TypeContextNone
				return limits
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits := tt.setupFunc()
			result := limits.HasLimitations()

			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestDetermineTypeContext(t *testing.T) {
	tests := []struct {
		name     string
		symbol   *Symbol
		refs     []Reference
		expected TypeContextLevel
	}{
		{
			name: "full context - has signature and typed refs",
			symbol: &Symbol{
				Signature:           "func test()",
				SignatureNormalized: "func test()",
			},
			refs: []Reference{
				{Kind: RefType},
			},
			expected: TypeContextFull,
		},
		{
			name: "full context - signature and implements",
			symbol: &Symbol{
				Signature: "interface Test",
			},
			refs: []Reference{
				{Kind: RefImplements},
			},
			expected: TypeContextFull,
		},
		{
			name: "partial context - only signature",
			symbol: &Symbol{
				Signature: "func test()",
			},
			refs: []Reference{
				{Kind: RefCall},
			},
			expected: TypeContextPartial,
		},
		{
			name:   "partial context - only typed refs",
			symbol: &Symbol{},
			refs: []Reference{
				{Kind: RefType},
			},
			expected: TypeContextPartial,
		},
		{
			name:   "no context",
			symbol: &Symbol{},
			refs: []Reference{
				{Kind: RefCall},
			},
			expected: TypeContextNone,
		},
		{
			name:     "no context - empty",
			symbol:   &Symbol{},
			refs:     []Reference{},
			expected: TypeContextNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetermineTypeContext(tt.symbol, tt.refs)

			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestTypeContextLevels(t *testing.T) {
	// Verify that the type context levels are defined correctly
	levels := []TypeContextLevel{
		TypeContextFull,
		TypeContextPartial,
		TypeContextNone,
	}

	expectedValues := []string{"full", "partial", "none"}

	for i, level := range levels {
		if string(level) != expectedValues[i] {
			t.Errorf("expected %s, got %s", expectedValues[i], level)
		}
	}
}
