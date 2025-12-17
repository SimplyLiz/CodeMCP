package query

import "testing"

func TestComputeJustifyVerdict(t *testing.T) {
	t.Run("keeps when callers exist", func(t *testing.T) {
		facts := ExplainSymbolFacts{Usage: &ExplainUsage{CallerCount: 2}}
		verdict, confidence, reasoning := computeJustifyVerdict(facts)
		if verdict != "keep" || confidence != 0.9 || reasoning == "" {
			t.Fatalf("unexpected verdict: %s %f %s", verdict, confidence, reasoning)
		}
	})

	t.Run("investigate public api with no callers", func(t *testing.T) {
		facts := ExplainSymbolFacts{Usage: &ExplainUsage{}, Flags: &ExplainSymbolFlags{IsPublicApi: true}}
		verdict, confidence, reasoning := computeJustifyVerdict(facts)
		if verdict != "investigate" || confidence != 0.6 || reasoning == "" {
			t.Fatalf("unexpected verdict: %s %f %s", verdict, confidence, reasoning)
		}
	})

	t.Run("remove candidate when unused and private", func(t *testing.T) {
		facts := ExplainSymbolFacts{}
		verdict, confidence, reasoning := computeJustifyVerdict(facts)
		if verdict != "remove-candidate" || confidence != 0.7 || reasoning == "" {
			t.Fatalf("unexpected verdict: %s %f %s", verdict, confidence, reasoning)
		}
	})
}
