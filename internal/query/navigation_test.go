package query

import "testing"

func TestComputeJustifyVerdict(t *testing.T) {
	t.Run("keeps symbol with active callers", func(t *testing.T) {
		facts := ExplainSymbolFacts{
			Usage: &ExplainUsage{CallerCount: 5},
		}
		verdict, confidence, reasoning := computeJustifyVerdict(facts)

		if verdict != "keep" {
			t.Errorf("expected verdict 'keep', got %q", verdict)
		}
		if confidence != 0.9 {
			t.Errorf("expected confidence 0.9, got %f", confidence)
		}
		if reasoning == "" {
			t.Error("expected non-empty reasoning")
		}
	})

	t.Run("investigates public API with no callers", func(t *testing.T) {
		facts := ExplainSymbolFacts{
			Usage: &ExplainUsage{CallerCount: 0},
			Flags: &ExplainSymbolFlags{IsPublicApi: true},
		}
		verdict, confidence, reasoning := computeJustifyVerdict(facts)

		if verdict != "investigate" {
			t.Errorf("expected verdict 'investigate', got %q", verdict)
		}
		if confidence != 0.6 {
			t.Errorf("expected confidence 0.6, got %f", confidence)
		}
		if reasoning == "" {
			t.Error("expected non-empty reasoning")
		}
	})

	t.Run("removes private symbol with no callers", func(t *testing.T) {
		facts := ExplainSymbolFacts{
			Usage: &ExplainUsage{CallerCount: 0},
			Flags: &ExplainSymbolFlags{IsPublicApi: false},
		}
		verdict, confidence, reasoning := computeJustifyVerdict(facts)

		if verdict != "remove-candidate" {
			t.Errorf("expected verdict 'remove-candidate', got %q", verdict)
		}
		if confidence != 0.7 {
			t.Errorf("expected confidence 0.7, got %f", confidence)
		}
		if reasoning == "" {
			t.Error("expected non-empty reasoning")
		}
	})

	t.Run("removes when no usage info available", func(t *testing.T) {
		facts := ExplainSymbolFacts{}
		verdict, confidence, reasoning := computeJustifyVerdict(facts)

		if verdict != "remove-candidate" {
			t.Errorf("expected verdict 'remove-candidate', got %q", verdict)
		}
		if confidence != 0.7 {
			t.Errorf("expected confidence 0.7, got %f", confidence)
		}
		if reasoning == "" {
			t.Error("expected non-empty reasoning")
		}
	})
}

func TestClassifyCommitFrequency(t *testing.T) {
	tests := []struct {
		count    int
		expected string
	}{
		{0, "unknown"},
		{1, "stable"},
		{10, "stable"},
		{11, "moderate"},
		{50, "moderate"},
		{51, "volatile"},
		{100, "volatile"},
	}

	for _, tc := range tests {
		result := classifyCommitFrequency(tc.count)
		if result != tc.expected {
			t.Errorf("classifyCommitFrequency(%d) = %q, expected %q", tc.count, result, tc.expected)
		}
	}
}

func TestTopLevelModule(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"internal/query/engine.go", "internal"},
		{"./internal/query/engine.go", "internal"},
		{"cmd/ckb/main.go", "cmd"},
		{"main.go", "main.go"},
		{"", ""},
	}

	for _, tc := range tests {
		result := topLevelModule(tc.path)
		if result != tc.expected {
			t.Errorf("topLevelModule(%q) = %q, expected %q", tc.path, result, tc.expected)
		}
	}
}

func TestBuildExplainSummary(t *testing.T) {
	t.Run("builds complete summary", func(t *testing.T) {
		facts := ExplainSymbolFacts{
			Symbol: &SymbolInfo{
				Name:     "MyFunction",
				Kind:     "function",
				ModuleId: "internal/query",
			},
			Usage: &ExplainUsage{
				CallerCount:    5,
				ReferenceCount: 10,
				ModuleCount:    3,
			},
			History: &ExplainHistory{
				CommitCount:    15,
				LastModifiedAt: "2024-01-15",
			},
		}

		summary := buildExplainSummary(facts)

		if summary.Identity == "" {
			t.Error("expected non-empty identity")
		}
		if summary.Usage == "" {
			t.Error("expected non-empty usage")
		}
		if summary.History == "" {
			t.Error("expected non-empty history")
		}
		if summary.Tldr == "" {
			t.Error("expected non-empty tldr")
		}
	})

	t.Run("handles empty facts", func(t *testing.T) {
		facts := ExplainSymbolFacts{}
		summary := buildExplainSummary(facts)

		if summary.Tldr != "" {
			t.Errorf("expected empty tldr for empty facts, got %q", summary.Tldr)
		}
	})
}
