package query

import (
	"testing"
)

func TestComputeJustifyVerdictWithADR(t *testing.T) {
	tests := []struct {
		name             string
		facts            ExplainSymbolFacts
		expectedVerdict  string
		expectADRMention bool
	}{
		{
			name: "active callers should return keep regardless of ADR",
			facts: ExplainSymbolFacts{
				Usage: &ExplainUsage{CallerCount: 5},
				Annotations: &AnnotationContext{
					RelatedDecisions: []RelatedDecision{
						{ID: "ADR-001", Title: "Test decision", Status: "accepted"},
					},
				},
			},
			expectedVerdict:  "keep",
			expectADRMention: false,
		},
		{
			name: "no callers with accepted ADR should return investigate",
			facts: ExplainSymbolFacts{
				Usage: &ExplainUsage{CallerCount: 0},
				Annotations: &AnnotationContext{
					RelatedDecisions: []RelatedDecision{
						{ID: "ADR-001", Title: "Intentional design", Status: "accepted"},
					},
				},
			},
			expectedVerdict:  "investigate",
			expectADRMention: true,
		},
		{
			name: "no callers with proposed ADR should return investigate",
			facts: ExplainSymbolFacts{
				Usage: &ExplainUsage{CallerCount: 0},
				Annotations: &AnnotationContext{
					RelatedDecisions: []RelatedDecision{
						{ID: "ADR-002", Title: "Proposed feature", Status: "proposed"},
					},
				},
			},
			expectedVerdict:  "investigate",
			expectADRMention: true,
		},
		{
			name: "no callers with deprecated ADR should return remove-candidate",
			facts: ExplainSymbolFacts{
				Usage: &ExplainUsage{CallerCount: 0},
				Annotations: &AnnotationContext{
					RelatedDecisions: []RelatedDecision{
						{ID: "ADR-003", Title: "Old decision", Status: "deprecated"},
					},
				},
			},
			expectedVerdict:  "remove-candidate",
			expectADRMention: false,
		},
		{
			name: "no callers, no ADR, public API should return investigate",
			facts: ExplainSymbolFacts{
				Usage: &ExplainUsage{CallerCount: 0},
				Flags: &ExplainSymbolFlags{IsPublicApi: true},
			},
			expectedVerdict:  "investigate",
			expectADRMention: false,
		},
		{
			name: "no callers, no ADR, not public should return remove-candidate",
			facts: ExplainSymbolFacts{
				Usage: &ExplainUsage{CallerCount: 0},
			},
			expectedVerdict:  "remove-candidate",
			expectADRMention: false,
		},
		{
			name: "nil usage with ADR should return investigate",
			facts: ExplainSymbolFacts{
				Annotations: &AnnotationContext{
					RelatedDecisions: []RelatedDecision{
						{ID: "ADR-004", Title: "Extension point", Status: "accepted"},
					},
				},
			},
			expectedVerdict:  "investigate",
			expectADRMention: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			verdict, _, reasoning := computeJustifyVerdict(tt.facts)

			if verdict != tt.expectedVerdict {
				t.Errorf("expected verdict %q, got %q", tt.expectedVerdict, verdict)
			}

			hasADRMention := len(reasoning) > 0 && (contains(reasoning, "ADR-") ||
				contains(reasoning, "related to"))

			if tt.expectADRMention && !hasADRMention {
				t.Errorf("expected ADR mention in reasoning, got: %q", reasoning)
			}
		})
	}
}

func TestRelatedDecisionStruct(t *testing.T) {
	rd := RelatedDecision{
		ID:              "ADR-001",
		Title:           "Use Redis for caching",
		Status:          "accepted",
		AffectedModules: []string{"internal/cache", "internal/api"},
		FilePath:        "docs/decisions/adr-001-use-redis.md",
	}

	if rd.ID != "ADR-001" {
		t.Errorf("expected ID ADR-001, got %s", rd.ID)
	}
	if rd.Status != "accepted" {
		t.Errorf("expected status accepted, got %s", rd.Status)
	}
	if len(rd.AffectedModules) != 2 {
		t.Errorf("expected 2 affected modules, got %d", len(rd.AffectedModules))
	}
	if rd.Title != "Use Redis for caching" {
		t.Errorf("expected title 'Use Redis for caching', got %s", rd.Title)
	}
	if rd.FilePath != "docs/decisions/adr-001-use-redis.md" {
		t.Errorf("expected filePath, got %s", rd.FilePath)
	}
}

func TestModuleAnnotationsStruct(t *testing.T) {
	ma := ModuleAnnotations{
		Responsibility: "HTTP API handlers",
		Capabilities:   []string{"REST", "WebSocket"},
		Tags:           []string{"core", "infrastructure"},
		Source:         "declared",
		Confidence:     0.95,
	}

	if ma.Responsibility != "HTTP API handlers" {
		t.Errorf("expected responsibility 'HTTP API handlers', got %s", ma.Responsibility)
	}
	if ma.Source != "declared" {
		t.Errorf("expected source 'declared', got %s", ma.Source)
	}
	if len(ma.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(ma.Capabilities))
	}
	if len(ma.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(ma.Tags))
	}
	if ma.Confidence != 0.95 {
		t.Errorf("expected confidence 0.95, got %f", ma.Confidence)
	}
}

func TestAnnotationContextStruct(t *testing.T) {
	ac := AnnotationContext{
		RelatedDecisions: []RelatedDecision{
			{ID: "ADR-001", Title: "Test", Status: "accepted"},
		},
		ModuleAnnotations: &ModuleAnnotations{
			Responsibility: "Test module",
			Source:         "declared",
			Confidence:     1.0,
		},
	}

	if len(ac.RelatedDecisions) != 1 {
		t.Errorf("expected 1 related decision, got %d", len(ac.RelatedDecisions))
	}
	if ac.ModuleAnnotations == nil {
		t.Error("expected module annotations to be set")
	}
	if ac.ModuleAnnotations.Responsibility != "Test module" {
		t.Errorf("expected responsibility 'Test module', got %s", ac.ModuleAnnotations.Responsibility)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
