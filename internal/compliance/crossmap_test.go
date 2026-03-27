package compliance

import (
	"strings"
	"testing"

	"github.com/SimplyLiz/CodeMCP/internal/query"
)

func TestEnrichWithCrossReferences_WeakPIICrypto(t *testing.T) {
	findings := []query.ReviewFinding{
		{
			RuleID:   "gdpr/weak-pii-crypto",
			Message:  "Weak crypto used for PII",
			Severity: "error",
			Category: "security",
		},
	}

	enriched := EnrichWithCrossReferences(findings)

	if len(enriched) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(enriched))
	}

	hint := enriched[0].Hint
	if hint == "" {
		t.Fatal("expected Hint to be populated with cross-references")
	}
	if !strings.Contains(hint, "Also violates:") {
		t.Errorf("expected Hint to contain 'Also violates:', got: %s", hint)
	}
	// The ruleID prefix is "gdpr", so GDPR refs should be excluded but ISO 27001 should be included
	if !strings.Contains(hint, "ISO 27001") {
		t.Errorf("expected Hint to reference ISO 27001, got: %s", hint)
	}
	// GDPR should NOT appear since the finding already belongs to GDPR
	if strings.Contains(hint, "GDPR") {
		t.Errorf("GDPR should not appear in cross-references for a gdpr/* rule, got: %s", hint)
	}
}

func TestEnrichWithCrossReferences_NoMapping(t *testing.T) {
	originalHint := "some existing hint"
	findings := []query.ReviewFinding{
		{
			RuleID:   "custom/unknown-check",
			Message:  "Some custom check",
			Severity: "info",
			Hint:     originalHint,
		},
	}

	enriched := EnrichWithCrossReferences(findings)

	if enriched[0].Hint != originalHint {
		t.Errorf("expected Hint to remain unchanged (%q), got: %q", originalHint, enriched[0].Hint)
	}
}

func TestEnrichWithCrossReferences_ExistingHint(t *testing.T) {
	findings := []query.ReviewFinding{
		{
			RuleID:   "iso27001/hardcoded-secret",
			Message:  "Hardcoded secret found",
			Severity: "error",
			Hint:     "existing hint",
		},
	}

	enriched := EnrichWithCrossReferences(findings)

	hint := enriched[0].Hint
	if !strings.HasPrefix(hint, "existing hint") {
		t.Errorf("expected Hint to start with existing hint, got: %s", hint)
	}
	if !strings.Contains(hint, " | Also violates:") {
		t.Errorf("expected ' | Also violates:' separator, got: %s", hint)
	}
}

func TestEnrichWithCrossReferences_CWEAppended(t *testing.T) {
	findings := []query.ReviewFinding{
		{
			RuleID:   "owasp-asvs/sql-injection",
			Message:  "Possible SQL injection",
			Severity: "error",
			Detail:   "Unparameterized query",
		},
	}

	enriched := EnrichWithCrossReferences(findings)

	if !strings.Contains(enriched[0].Detail, "CWE-89") {
		t.Errorf("expected Detail to contain CWE-89, got: %s", enriched[0].Detail)
	}
}

func TestGetCrossReferences_HardcodedCredential(t *testing.T) {
	refs := GetCrossReferences("hardcoded-credential")

	if len(refs) == 0 {
		t.Fatal("expected non-empty references for 'hardcoded-credential'")
	}

	// Should have multiple framework references
	if len(refs) < 5 {
		t.Errorf("expected at least 5 framework references for hardcoded-credential, got %d", len(refs))
	}

	// Check that specific frameworks are included
	frameworks := make(map[FrameworkID]bool)
	for _, ref := range refs {
		frameworks[ref.Framework] = true
	}
	expected := []FrameworkID{FrameworkPCIDSS, FrameworkNIST80053, FrameworkSOC2, FrameworkOWASPASVS, FrameworkISO27001}
	for _, fw := range expected {
		if !frameworks[fw] {
			t.Errorf("expected framework %s in hardcoded-credential references", fw)
		}
	}
}

func TestGetCrossReferences_UnknownCategory(t *testing.T) {
	refs := GetCrossReferences("nonexistent-category")
	if refs != nil {
		t.Errorf("expected nil for unknown category, got %d refs", len(refs))
	}
}

func TestListMappedCategories(t *testing.T) {
	categories := ListMappedCategories()

	if len(categories) == 0 {
		t.Fatal("expected non-empty list of mapped categories")
	}

	// Check for a few expected categories
	catSet := make(map[string]bool)
	for _, c := range categories {
		catSet[c] = true
	}

	expectedCats := []string{
		"hardcoded-credential",
		"weak-crypto",
		"sql-injection",
		"pii-in-logs",
		"missing-tls",
	}
	for _, ec := range expectedCats {
		if !catSet[ec] {
			t.Errorf("expected category %q in ListMappedCategories output", ec)
		}
	}
}
