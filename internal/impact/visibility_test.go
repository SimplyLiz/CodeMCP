package impact

import (
	"testing"
)

func TestDeriveVisibility(t *testing.T) {
	tests := []struct {
		name           string
		symbol         *Symbol
		refs           []Reference
		expectedVis    Visibility
		expectedSource string
	}{
		{
			name: "public from modifiers",
			symbol: &Symbol{
				Name:      "MyFunction",
				ModuleId:  "module1",
				Modifiers: []string{"public"},
			},
			refs:           []Reference{},
			expectedVis:    VisibilityPublic,
			expectedSource: "scip-modifiers",
		},
		{
			name: "private from modifiers",
			symbol: &Symbol{
				Name:      "myFunction",
				ModuleId:  "module1",
				Modifiers: []string{"private"},
			},
			refs:           []Reference{},
			expectedVis:    VisibilityPrivate,
			expectedSource: "scip-modifiers",
		},
		{
			name: "internal from modifiers",
			symbol: &Symbol{
				Name:      "myFunction",
				ModuleId:  "module1",
				Modifiers: []string{"internal"},
			},
			refs:           []Reference{},
			expectedVis:    VisibilityInternal,
			expectedSource: "scip-modifiers",
		},
		{
			name: "public from external references",
			symbol: &Symbol{
				Name:      "MyFunction",
				ModuleId:  "module1",
				Modifiers: []string{},
			},
			refs: []Reference{
				{FromModule: "module2"},
				{FromModule: "module3"},
			},
			expectedVis:    VisibilityPublic,
			expectedSource: "ref-analysis",
		},
		{
			name: "internal from same module references",
			symbol: &Symbol{
				Name:      "myFunction",
				ModuleId:  "module1",
				Modifiers: []string{},
			},
			refs: []Reference{
				{FromModule: "module1"},
				{FromModule: "module1"},
			},
			expectedVis:    VisibilityInternal,
			expectedSource: "ref-analysis",
		},
		{
			name: "private from underscore naming",
			symbol: &Symbol{
				Name:      "_privateFunction",
				ModuleId:  "module1",
				Modifiers: []string{},
			},
			refs:           []Reference{},
			expectedVis:    VisibilityPrivate,
			expectedSource: "naming-convention",
		},
		{
			name: "private from hash naming",
			symbol: &Symbol{
				Name:      "#privateFunction",
				ModuleId:  "module1",
				Modifiers: []string{},
			},
			refs:           []Reference{},
			expectedVis:    VisibilityPrivate,
			expectedSource: "naming-convention",
		},
		{
			name: "private from double underscore",
			symbol: &Symbol{
				Name:      "__privateFunction",
				ModuleId:  "module1",
				Modifiers: []string{},
			},
			refs:           []Reference{},
			expectedVis:    VisibilityPrivate,
			expectedSource: "naming-convention",
		},
		{
			name: "public from uppercase (Go convention)",
			symbol: &Symbol{
				Name:      "PublicFunction",
				Kind:      KindFunction,
				ModuleId:  "module1",
				Modifiers: []string{},
			},
			refs:           []Reference{},
			expectedVis:    VisibilityPublic,
			expectedSource: "naming-convention",
		},
		{
			name: "internal from lowercase (Go convention)",
			symbol: &Symbol{
				Name:      "internalFunction",
				Kind:      KindFunction,
				ModuleId:  "module1",
				Modifiers: []string{},
			},
			refs:           []Reference{},
			expectedVis:    VisibilityInternal,
			expectedSource: "naming-convention",
		},
		{
			name: "unknown when no information",
			symbol: &Symbol{
				Name:      "someFunction",
				ModuleId:  "module1",
				Modifiers: []string{},
			},
			refs:           []Reference{},
			expectedVis:    VisibilityUnknown,
			expectedSource: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DeriveVisibility(tt.symbol, tt.refs)

			if result.Visibility != tt.expectedVis {
				t.Errorf("expected visibility %s, got %s", tt.expectedVis, result.Visibility)
			}

			if result.Source != tt.expectedSource {
				t.Errorf("expected source %s, got %s", tt.expectedSource, result.Source)
			}

			// Check confidence is in valid range
			if result.Confidence < 0.0 || result.Confidence > 1.0 {
				t.Errorf("confidence out of range: %f", result.Confidence)
			}
		})
	}
}

func TestDeriveFromModifiers(t *testing.T) {
	tests := []struct {
		name        string
		modifiers   []string
		expectedVis *Visibility
	}{
		{"public modifier", []string{"public"}, ptr(VisibilityPublic)},
		{"private modifier", []string{"private"}, ptr(VisibilityPrivate)},
		{"internal modifier", []string{"internal"}, ptr(VisibilityInternal)},
		{"protected modifier", []string{"protected"}, ptr(VisibilityInternal)},
		{"package modifier", []string{"package"}, ptr(VisibilityInternal)},
		{"no modifiers", []string{}, nil},
		{"nil modifiers", nil, nil},
		{"unrecognized modifier", []string{"static"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			symbol := &Symbol{
				Name:      "testSymbol",
				Modifiers: tt.modifiers,
			}

			result := deriveFromModifiers(symbol)

			if tt.expectedVis == nil {
				if result != nil {
					t.Errorf("expected nil result, got %v", result)
				}
			} else {
				if result == nil {
					t.Errorf("expected non-nil result")
				} else if result.Visibility != *tt.expectedVis {
					t.Errorf("expected visibility %s, got %s", *tt.expectedVis, result.Visibility)
				}
			}
		})
	}
}

func TestDeriveFromReferences(t *testing.T) {
	tests := []struct {
		name        string
		moduleId    string
		refs        []Reference
		expectedVis *Visibility
	}{
		{
			name:     "external references",
			moduleId: "module1",
			refs: []Reference{
				{FromModule: "module2"},
				{FromModule: "module3"},
			},
			expectedVis: ptr(VisibilityPublic),
		},
		{
			name:     "internal references only",
			moduleId: "module1",
			refs: []Reference{
				{FromModule: "module1"},
				{FromModule: "module1"},
			},
			expectedVis: ptr(VisibilityInternal),
		},
		{
			name:        "no references",
			moduleId:    "module1",
			refs:        []Reference{},
			expectedVis: nil,
		},
		{
			name:     "mixed references",
			moduleId: "module1",
			refs: []Reference{
				{FromModule: "module1"},
				{FromModule: "module2"},
			},
			expectedVis: ptr(VisibilityPublic),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			symbol := &Symbol{
				ModuleId: tt.moduleId,
			}

			result := deriveFromReferences(symbol, tt.refs)

			if tt.expectedVis == nil {
				if result != nil {
					t.Errorf("expected nil result, got %v", result)
				}
			} else {
				if result == nil {
					t.Errorf("expected non-nil result")
				} else if result.Visibility != *tt.expectedVis {
					t.Errorf("expected visibility %s, got %s", *tt.expectedVis, result.Visibility)
				}
			}
		})
	}
}

// Helper function to create pointer to Visibility
func ptr(v Visibility) *Visibility {
	return &v
}
