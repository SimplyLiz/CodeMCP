package compliance

import (
	"fmt"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/query"
)

// CrossFrameworkMapping maps a single finding category to all applicable framework references.
// This is CKB's key differentiator: a hardcoded credential doesn't just violate one standard,
// it violates PCI DSS 8.3.6, NIST 800-53 IA-5, SOC 2 CC6.1, OWASP ASVS V2.10.4, etc.
type CrossFrameworkMapping struct {
	Category   string               // e.g., "hardcoded-credential"
	CWE        string               // e.g., "CWE-798"
	References []FrameworkReference // All applicable framework articles
}

// FrameworkReference links a finding to a specific regulation clause.
type FrameworkReference struct {
	Framework FrameworkID
	Article   string // e.g., "Req 8.6.2 PCI DSS 4.0"
	Control   string // Short control name for display
}

// crossMappings defines the mapping from finding categories to all applicable frameworks.
// Each category maps to every regulation that cares about that class of issue.
var crossMappings = map[string]CrossFrameworkMapping{
	"hardcoded-credential": {
		Category: "hardcoded-credential",
		CWE:      "CWE-798",
		References: []FrameworkReference{
			{FrameworkPCIDSS, "Req 8.6.2 PCI DSS 4.0", "PCI DSS 8.6.2"},
			{FrameworkNIST80053, "IA-5(1) NIST 800-53", "NIST IA-5"},
			{FrameworkSOC2, "CC6.1 SOC 2", "SOC 2 CC6.1"},
			{FrameworkOWASPASVS, "V2.10.4 ASVS", "ASVS V2.10.4"},
			{FrameworkNIS2, "Art. 21(2)(g) NIS2", "NIS2 Art.21"},
			{FrameworkDORA, "Art. 9(2) DORA", "DORA Art.9"},
			{FrameworkISO27001, "A.8.4 ISO 27001:2022", "ISO 27001 A.8.4"},
			{FrameworkEUCRA, "Art. 13 EU CRA", "EU CRA Art.13"},
			{FrameworkIEC62443, "CR 1.1 IEC 62443-4-2", "IEC 62443 CR1.1"},
		},
	},
	"weak-crypto": {
		Category: "weak-crypto",
		CWE:      "CWE-327",
		References: []FrameworkReference{
			{FrameworkISO27001, "A.8.24 ISO 27001:2022", "ISO 27001 A.8.24"},
			{FrameworkNIST80053, "SC-13 NIST 800-53", "NIST SC-13"},
			{FrameworkPCIDSS, "Req 4.2.1 PCI DSS 4.0", "PCI DSS 4.2.1"},
			{FrameworkOWASPASVS, "V6.2.5 ASVS", "ASVS V6.2.5"},
			{FrameworkNIS2, "Art. 21(2)(j) NIS2", "NIS2 Art.21"},
			{FrameworkGDPR, "Art. 32 GDPR", "GDPR Art.32"},
			{FrameworkHIPAA, "§164.312(a)(2)(iv) HIPAA", "HIPAA §164.312"},
			{FrameworkFDAPart11, "§11.10(a) 21 CFR Part 11", "FDA §11.10"},
		},
	},
	"sql-injection": {
		Category: "sql-injection",
		CWE:      "CWE-89",
		References: []FrameworkReference{
			{FrameworkOWASPASVS, "V5.3.4 ASVS", "ASVS V5.3.4"},
			{FrameworkPCIDSS, "Req 6.2.4 PCI DSS 4.0", "PCI DSS 6.2.4"},
			{FrameworkISO27001, "A.8.28 ISO 27001:2022", "ISO 27001 A.8.28"},
			{FrameworkNIST80053, "SI-10 NIST 800-53", "NIST SI-10"},
			{FrameworkEUCRA, "Annex I, Part I(1) EU CRA", "EU CRA Annex I"},
			{FrameworkIEC62443, "SD-4 IEC 62443-4-1", "IEC 62443 SD-4"},
		},
	},
	"xss": {
		Category: "xss",
		CWE:      "CWE-79",
		References: []FrameworkReference{
			{FrameworkOWASPASVS, "V5.3.3 ASVS", "ASVS V5.3.3"},
			{FrameworkPCIDSS, "Req 6.2.4 PCI DSS 4.0", "PCI DSS 6.2.4"},
			{FrameworkISO27001, "A.8.28 ISO 27001:2022", "ISO 27001 A.8.28"},
			{FrameworkNIST80053, "SI-10 NIST 800-53", "NIST SI-10"},
			{FrameworkEUCRA, "Annex I, Part I(1) EU CRA", "EU CRA Annex I"},
		},
	},
	"pii-in-logs": {
		Category: "pii-in-logs",
		CWE:      "CWE-532",
		References: []FrameworkReference{
			{FrameworkGDPR, "Art. 25, 32 GDPR", "GDPR Art.25/32"},
			{FrameworkISO27001, "A.8.12 ISO 27001:2022", "ISO 27001 A.8.12"},
			{FrameworkHIPAA, "§164.312(b) HIPAA", "HIPAA §164.312"},
			{FrameworkOWASPASVS, "V7.1.1 ASVS", "ASVS V7.1.1"},
			{FrameworkCCPA, "§1798.100 CCPA", "CCPA §1798.100"},
			{FrameworkISO27701, "A.7.4.5 ISO 27701", "ISO 27701 A.7.4.5"},
			{FrameworkNIS2, "Art. 21(2)(g) NIS2", "NIS2 Art.21"},
		},
	},
	"missing-tls": {
		Category: "missing-tls",
		CWE:      "CWE-319",
		References: []FrameworkReference{
			{FrameworkOWASPASVS, "V9.1.1 ASVS", "ASVS V9.1.1"},
			{FrameworkISO27001, "A.8.20 ISO 27001:2022", "ISO 27001 A.8.20"},
			{FrameworkPCIDSS, "Req 4.2.1 PCI DSS 4.0", "PCI DSS 4.2.1"},
			{FrameworkGDPR, "Art. 32 GDPR", "GDPR Art.32"},
			{FrameworkHIPAA, "§164.312(e) HIPAA", "HIPAA §164.312"},
			{FrameworkNIST80053, "SC-8 NIST 800-53", "NIST SC-8"},
			{FrameworkDORA, "Art. 9(2) DORA", "DORA Art.9"},
			{FrameworkSOC2, "CC6.7 SOC 2", "SOC 2 CC6.7"},
		},
	},
	"insecure-random": {
		Category: "insecure-random",
		CWE:      "CWE-338",
		References: []FrameworkReference{
			{FrameworkOWASPASVS, "V6.2.5 ASVS", "ASVS V6.2.5"},
			{FrameworkISO27001, "A.8.24 ISO 27001:2022", "ISO 27001 A.8.24"},
			{FrameworkNIST80053, "SC-13 NIST 800-53", "NIST SC-13"},
			{FrameworkPCIDSS, "Req 6.2.4 PCI DSS 4.0", "PCI DSS 6.2.4"},
		},
	},
	"path-traversal": {
		Category: "path-traversal",
		CWE:      "CWE-22",
		References: []FrameworkReference{
			{FrameworkOWASPASVS, "V12.3.1 ASVS", "ASVS V12.3.1"},
			{FrameworkISO27001, "A.8.28 ISO 27001:2022", "ISO 27001 A.8.28"},
			{FrameworkPCIDSS, "Req 6.2.4 PCI DSS 4.0", "PCI DSS 6.2.4"},
			{FrameworkNIST80053, "SI-10 NIST 800-53", "NIST SI-10"},
		},
	},
	"unsafe-deserialization": {
		Category: "unsafe-deserialization",
		CWE:      "CWE-502",
		References: []FrameworkReference{
			{FrameworkOWASPASVS, "V5.5.1 ASVS", "ASVS V5.5.1"},
			{FrameworkISO27001, "A.8.7 ISO 27001:2022", "ISO 27001 A.8.7"},
			{FrameworkPCIDSS, "Req 6.2.4 PCI DSS 4.0", "PCI DSS 6.2.4"},
			{FrameworkEUCRA, "Annex I, Part I(1) EU CRA", "EU CRA Annex I"},
		},
	},
	"missing-auth": {
		Category: "missing-auth",
		CWE:      "CWE-306",
		References: []FrameworkReference{
			{FrameworkSOC2, "CC6.1 SOC 2", "SOC 2 CC6.1"},
			{FrameworkNIST80053, "AC-3 NIST 800-53", "NIST AC-3"},
			{FrameworkISO27001, "A.8.3 ISO 27001:2022", "ISO 27001 A.8.3"},
			{FrameworkPCIDSS, "Req 7.2.2 PCI DSS 4.0", "PCI DSS 7.2.2"},
			{FrameworkHIPAA, "§164.312(a) HIPAA", "HIPAA §164.312"},
			{FrameworkIEC62443, "CR 1.2 IEC 62443-4-2", "IEC 62443 CR1.2"},
			{FrameworkFDAPart11, "§11.10(d) 21 CFR Part 11", "FDA §11.10"},
		},
	},
	"missing-audit-trail": {
		Category: "missing-audit-trail",
		CWE:      "",
		References: []FrameworkReference{
			{FrameworkHIPAA, "§164.312(b) HIPAA", "HIPAA §164.312"},
			{FrameworkFDAPart11, "§11.10(e) 21 CFR Part 11", "FDA §11.10"},
			{FrameworkSOC2, "CC7.2 SOC 2", "SOC 2 CC7.2"},
			{FrameworkNIST80053, "AU-2 NIST 800-53", "NIST AU-2"},
			{FrameworkGDPR, "Art. 30 GDPR", "GDPR Art.30"},
			{FrameworkDORA, "Art. 10 DORA", "DORA Art.10"},
			{FrameworkEUAIAct, "Art. 12 EU AI Act", "EU AI Act Art.12"},
			{FrameworkPCIDSS, "Req 10.2 PCI DSS 4.0", "PCI DSS 10.2"},
		},
	},
	"missing-deletion": {
		Category: "missing-deletion",
		CWE:      "",
		References: []FrameworkReference{
			{FrameworkGDPR, "Art. 17 GDPR", "GDPR Art.17"},
			{FrameworkCCPA, "§1798.105 CCPA", "CCPA §1798.105"},
			{FrameworkISO27701, "A.7.3.6 ISO 27701", "ISO 27701 A.7.3.6"},
		},
	},
	"missing-consent": {
		Category: "missing-consent",
		CWE:      "",
		References: []FrameworkReference{
			{FrameworkGDPR, "Art. 6, 7 GDPR", "GDPR Art.6/7"},
			{FrameworkCCPA, "§1798.100 CCPA", "CCPA §1798.100"},
			{FrameworkISO27701, "A.7.2.2 ISO 27701", "ISO 27701 A.7.2.2"},
		},
	},
	"goto-usage": {
		Category: "goto-usage",
		CWE:      "",
		References: []FrameworkReference{
			{FrameworkIEC61508, "Table B.1 IEC 61508-3", "IEC 61508 B.1"},
			{FrameworkISO26262, "Part 6, Table 3 ISO 26262", "ISO 26262 Part 6"},
			{FrameworkDO178C, "§6.3.4 DO-178C", "DO-178C §6.3.4"},
			{FrameworkMISRA, "Rule 15.1 MISRA C", "MISRA Rule 15.1"},
		},
	},
	"recursion": {
		Category: "recursion",
		CWE:      "",
		References: []FrameworkReference{
			{FrameworkIEC61508, "Table B.9 IEC 61508-3", "IEC 61508 B.9"},
			{FrameworkISO26262, "Part 6, Table 3 ISO 26262", "ISO 26262 Part 6"},
			{FrameworkDO178C, "§6.3.4 DO-178C", "DO-178C §6.3.4"},
		},
	},
	"complexity-exceeded": {
		Category: "complexity-exceeded",
		CWE:      "",
		References: []FrameworkReference{
			{FrameworkIEC61508, "Table B.9 IEC 61508-3", "IEC 61508 B.9"},
			{FrameworkISO26262, "Part 6, Table 3 ISO 26262", "ISO 26262 Part 6"},
			{FrameworkDO178C, "§6.3.4 DO-178C", "DO-178C §6.3.4"},
		},
	},
}

// EnrichWithCrossReferences adds cross-framework references to findings.
// This is what makes CKB unique: a single finding gets annotated with every
// regulation it violates, not just the one it was originally detected under.
func EnrichWithCrossReferences(findings []query.ReviewFinding) []query.ReviewFinding {
	for i := range findings {
		category := findingCategory(findings[i])
		if category == "" {
			continue
		}

		mapping, ok := crossMappings[category]
		if !ok {
			continue
		}

		// Build cross-reference string
		var refs []string
		for _, ref := range mapping.References {
			// Don't duplicate the original framework's reference.
			// Use prefix match on slash-delimited ruleID (e.g., "gdpr/pii-in-logs")
			// to avoid substring collisions (e.g., "nis2" matching "nis").
			rulePrefix := strings.SplitN(findings[i].RuleID, "/", 2)[0]
			if rulePrefix == string(ref.Framework) {
				continue
			}
			refs = append(refs, ref.Control)
		}

		if len(refs) > 0 {
			crossRef := "Also violates: " + strings.Join(refs, ", ")
			if findings[i].Hint == "" {
				findings[i].Hint = crossRef
			} else {
				findings[i].Hint += " | " + crossRef
			}
		}

		// Ensure CWE is set if we have it
		if mapping.CWE != "" && !strings.Contains(findings[i].Detail, "CWE") {
			if findings[i].Detail != "" {
				findings[i].Detail += fmt.Sprintf(" (%s)", mapping.CWE)
			}
		}
	}

	return findings
}

// findingCategory extracts the cross-mapping category from a ReviewFinding.
func findingCategory(f query.ReviewFinding) string {
	// Map RuleIDs to categories
	ruleID := strings.ToLower(f.RuleID)

	switch {
	case strings.Contains(ruleID, "hardcoded-secret") || strings.Contains(ruleID, "hardcoded-credential") || strings.Contains(ruleID, "default-credentials"):
		return "hardcoded-credential"
	case strings.Contains(ruleID, "weak-crypto") || strings.Contains(ruleID, "weak-pii-crypto") || strings.Contains(ruleID, "non-fips") || strings.Contains(ruleID, "deprecated-crypto") || strings.Contains(ruleID, "weak-algorithm"):
		return "weak-crypto"
	case strings.Contains(ruleID, "sql-injection"):
		return "sql-injection"
	case strings.Contains(ruleID, "xss"):
		return "xss"
	case strings.Contains(ruleID, "pii-in-logs") || strings.Contains(ruleID, "phi-in-logs") || strings.Contains(ruleID, "pan-in-logs"):
		return "pii-in-logs"
	case strings.Contains(ruleID, "missing-tls") || strings.Contains(ruleID, "unencrypted-transport"):
		return "missing-tls"
	case strings.Contains(ruleID, "insecure-random"):
		return "insecure-random"
	case strings.Contains(ruleID, "path-traversal"):
		return "path-traversal"
	case strings.Contains(ruleID, "unsafe-deserialization"):
		return "unsafe-deserialization"
	case strings.Contains(ruleID, "missing-auth"):
		return "missing-auth"
	case strings.Contains(ruleID, "missing-audit"):
		return "missing-audit-trail"
	case strings.Contains(ruleID, "no-deletion") || strings.Contains(ruleID, "missing-deletion"):
		return "missing-deletion"
	case strings.Contains(ruleID, "missing-consent") || strings.Contains(ruleID, "no-consent"):
		return "missing-consent"
	case strings.Contains(ruleID, "goto"):
		return "goto-usage"
	case strings.Contains(ruleID, "recursion"):
		return "recursion"
	case strings.Contains(ruleID, "complexity-exceeded"):
		return "complexity-exceeded"
	}

	return ""
}

// GetCrossReferences returns all framework references for a finding category.
func GetCrossReferences(category string) []FrameworkReference {
	if mapping, ok := crossMappings[category]; ok {
		return mapping.References
	}
	return nil
}

// ListMappedCategories returns all categories that have cross-framework mappings.
func ListMappedCategories() []string {
	categories := make([]string, 0, len(crossMappings))
	for cat := range crossMappings {
		categories = append(categories, cat)
	}
	return categories
}
