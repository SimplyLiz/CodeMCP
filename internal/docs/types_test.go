package docs

import (
	"testing"
	"time"
)

func TestDocTypeConstants(t *testing.T) {
	if DocTypeMarkdown != "markdown" {
		t.Errorf("DocTypeMarkdown = %q, want %q", DocTypeMarkdown, "markdown")
	}
	if DocTypeADR != "adr" {
		t.Errorf("DocTypeADR = %q, want %q", DocTypeADR, "adr")
	}
}

func TestDetectionMethodConstants(t *testing.T) {
	if DetectBacktick != "backtick" {
		t.Errorf("DetectBacktick = %q, want %q", DetectBacktick, "backtick")
	}
	if DetectDirective != "directive" {
		t.Errorf("DetectDirective = %q, want %q", DetectDirective, "directive")
	}
	if DetectFence != "fence" {
		t.Errorf("DetectFence = %q, want %q", DetectFence, "fence")
	}
}

func TestResolutionStatusConstants(t *testing.T) {
	statuses := []ResolutionStatus{
		ResolutionExact,
		ResolutionSuffix,
		ResolutionAmbiguous,
		ResolutionMissing,
		ResolutionIneligible,
	}

	for _, s := range statuses {
		if string(s) == "" {
			t.Errorf("ResolutionStatus should not be empty: %v", s)
		}
	}

	// Verify expected values
	if ResolutionExact != "exact" {
		t.Errorf("ResolutionExact = %q, want %q", ResolutionExact, "exact")
	}
	if ResolutionSuffix != "suffix" {
		t.Errorf("ResolutionSuffix = %q, want %q", ResolutionSuffix, "suffix")
	}
	if ResolutionAmbiguous != "ambiguous" {
		t.Errorf("ResolutionAmbiguous = %q, want %q", ResolutionAmbiguous, "ambiguous")
	}
}

func TestStalenessReasonConstants(t *testing.T) {
	reasons := []StalenessReason{
		StalenessMissing,
		StalenessAmbiguous,
		StalenessIndexGap,
		StalenessRenamed,
	}

	for _, r := range reasons {
		if string(r) == "" {
			t.Errorf("StalenessReason should not be empty: %v", r)
		}
	}

	// Verify expected values
	if StalenessMissing != "missing_symbol" {
		t.Errorf("StalenessMissing = %q, want %q", StalenessMissing, "missing_symbol")
	}
	if StalenessRenamed != "symbol_renamed" {
		t.Errorf("StalenessRenamed = %q, want %q", StalenessRenamed, "symbol_renamed")
	}
}

func TestDocumentStruct(t *testing.T) {
	now := time.Now()
	symID := "ckb:repo:sym:abc123"

	doc := Document{
		Path:        "docs/auth.md",
		Type:        DocTypeMarkdown,
		Title:       "Authentication Guide",
		Hash:        "abc123def456",
		LastIndexed: now,
		References: []DocReference{
			{
				ID:              1,
				DocPath:         "docs/auth.md",
				RawText:         "`UserService.Auth`",
				NormalizedText:  "UserService.Auth",
				SymbolID:        &symID,
				SymbolName:      "Auth",
				Line:            10,
				Column:          5,
				DetectionMethod: DetectBacktick,
				Resolution:      ResolutionExact,
				Confidence:      1.0,
				LastResolved:    now,
			},
		},
		Modules: []string{"internal/auth"},
	}

	if doc.Path != "docs/auth.md" {
		t.Errorf("Path = %q, want %q", doc.Path, "docs/auth.md")
	}
	if doc.Type != DocTypeMarkdown {
		t.Errorf("Type = %v, want %v", doc.Type, DocTypeMarkdown)
	}
	if len(doc.References) != 1 {
		t.Errorf("len(References) = %d, want %d", len(doc.References), 1)
	}
	if doc.References[0].Resolution != ResolutionExact {
		t.Errorf("References[0].Resolution = %v, want %v", doc.References[0].Resolution, ResolutionExact)
	}
}

func TestDocReferenceStruct(t *testing.T) {
	now := time.Now()
	symID := "ckb:repo:sym:abc123"

	ref := DocReference{
		ID:              42,
		DocPath:         "docs/api.md",
		RawText:         "`Handler.Serve`",
		NormalizedText:  "Handler.Serve",
		SymbolID:        &symID,
		SymbolName:      "Serve",
		Line:            25,
		Column:          10,
		Context:         "The Handler.Serve method handles...",
		DetectionMethod: DetectBacktick,
		Resolution:      ResolutionSuffix,
		Candidates:      nil,
		Confidence:      0.9,
		LastResolved:    now,
	}

	if ref.ID != 42 {
		t.Errorf("ID = %d, want %d", ref.ID, 42)
	}
	if ref.Line != 25 {
		t.Errorf("Line = %d, want %d", ref.Line, 25)
	}
	if ref.Confidence != 0.9 {
		t.Errorf("Confidence = %f, want %f", ref.Confidence, 0.9)
	}
	if ref.SymbolID == nil || *ref.SymbolID != symID {
		t.Error("SymbolID not set correctly")
	}
}

func TestMentionStruct(t *testing.T) {
	mention := Mention{
		RawText: "`Foo.Bar`",
		Line:    15,
		Column:  20,
		Context: "See `Foo.Bar` for more details",
		Method:  DetectBacktick,
	}

	if mention.RawText != "`Foo.Bar`" {
		t.Errorf("RawText = %q, want %q", mention.RawText, "`Foo.Bar`")
	}
	if mention.Method != DetectBacktick {
		t.Errorf("Method = %v, want %v", mention.Method, DetectBacktick)
	}
}

func TestModuleLinkStruct(t *testing.T) {
	link := ModuleLink{
		ModuleID: "internal/auth",
		Line:     5,
	}

	if link.ModuleID != "internal/auth" {
		t.Errorf("ModuleID = %q, want %q", link.ModuleID, "internal/auth")
	}
	if link.Line != 5 {
		t.Errorf("Line = %d, want %d", link.Line, 5)
	}
}

func TestDocModuleLinkStruct(t *testing.T) {
	link := DocModuleLink{
		DocPath:  "docs/auth.md",
		ModuleID: "internal/auth",
		Line:     3,
	}

	if link.DocPath != "docs/auth.md" {
		t.Errorf("DocPath = %q, want %q", link.DocPath, "docs/auth.md")
	}
	if link.ModuleID != "internal/auth" {
		t.Errorf("ModuleID = %q, want %q", link.ModuleID, "internal/auth")
	}
}

func TestScanResultStruct(t *testing.T) {
	result := ScanResult{
		Doc: Document{
			Path:  "docs/test.md",
			Type:  DocTypeMarkdown,
			Title: "Test Doc",
		},
		Mentions: []Mention{
			{RawText: "`Foo.Bar`", Line: 10},
		},
		Modules: []ModuleLink{
			{ModuleID: "internal/foo", Line: 5},
		},
		KnownSymbols: []string{"Engine", "Start"},
		Error:        nil,
	}

	if result.Doc.Path != "docs/test.md" {
		t.Errorf("Doc.Path = %q, want %q", result.Doc.Path, "docs/test.md")
	}
	if len(result.Mentions) != 1 {
		t.Errorf("len(Mentions) = %d, want %d", len(result.Mentions), 1)
	}
	if len(result.Modules) != 1 {
		t.Errorf("len(Modules) = %d, want %d", len(result.Modules), 1)
	}
	if len(result.KnownSymbols) != 2 {
		t.Errorf("len(KnownSymbols) = %d, want %d", len(result.KnownSymbols), 2)
	}
}

func TestResolutionResultStruct(t *testing.T) {
	result := ResolutionResult{
		Status:     ResolutionAmbiguous,
		SymbolID:   "",
		SymbolName: "",
		Candidates: []string{"pkg.Foo", "other.Foo"},
		Confidence: 0.5,
		Message:    "Multiple candidates found",
	}

	if result.Status != ResolutionAmbiguous {
		t.Errorf("Status = %v, want %v", result.Status, ResolutionAmbiguous)
	}
	if len(result.Candidates) != 2 {
		t.Errorf("len(Candidates) = %d, want %d", len(result.Candidates), 2)
	}
}

func TestIndexStatsStruct(t *testing.T) {
	stats := IndexStats{
		DocsIndexed:     10,
		DocsSkipped:     5,
		ReferencesFound: 50,
		Resolved:        40,
		Ambiguous:       3,
		Missing:         5,
		Ineligible:      2,
	}

	if stats.DocsIndexed != 10 {
		t.Errorf("DocsIndexed = %d, want %d", stats.DocsIndexed, 10)
	}
	total := stats.Resolved + stats.Ambiguous + stats.Missing + stats.Ineligible
	if total != 50 {
		t.Errorf("total should equal ReferencesFound, got %d", total)
	}
}

func TestCoverageReportStruct(t *testing.T) {
	report := CoverageReport{
		TotalSymbols:    100,
		Documented:      75,
		Undocumented:    25,
		CoveragePercent: 75.0,
		TopUndocumented: []UndocSymbol{
			{
				SymbolID:   "ckb:repo:sym:abc",
				Name:       "ImportantFunc",
				Kind:       "function",
				File:       "src/main.go",
				Centrality: 0.9,
			},
		},
		ByModule: []ModuleCoverage{
			{
				ModuleID:        "internal/auth",
				TotalSymbols:    20,
				Documented:      15,
				CoveragePercent: 75.0,
			},
		},
	}

	if report.CoveragePercent != 75.0 {
		t.Errorf("CoveragePercent = %f, want %f", report.CoveragePercent, 75.0)
	}
	if len(report.TopUndocumented) != 1 {
		t.Errorf("len(TopUndocumented) = %d, want %d", len(report.TopUndocumented), 1)
	}
	if report.TopUndocumented[0].Centrality != 0.9 {
		t.Errorf("TopUndocumented[0].Centrality = %f, want %f", report.TopUndocumented[0].Centrality, 0.9)
	}
}

func TestUndocSymbolStruct(t *testing.T) {
	sym := UndocSymbol{
		SymbolID:   "ckb:repo:sym:abc123",
		Name:       "Handler",
		Kind:       "struct",
		File:       "internal/api/handler.go",
		Centrality: 0.85,
	}

	if sym.SymbolID != "ckb:repo:sym:abc123" {
		t.Errorf("SymbolID = %q, want %q", sym.SymbolID, "ckb:repo:sym:abc123")
	}
	if sym.Centrality != 0.85 {
		t.Errorf("Centrality = %f, want %f", sym.Centrality, 0.85)
	}
}

func TestModuleCoverageStruct(t *testing.T) {
	mc := ModuleCoverage{
		ModuleID:        "internal/query",
		TotalSymbols:    50,
		Documented:      40,
		CoveragePercent: 80.0,
	}

	if mc.ModuleID != "internal/query" {
		t.Errorf("ModuleID = %q, want %q", mc.ModuleID, "internal/query")
	}
	if mc.CoveragePercent != 80.0 {
		t.Errorf("CoveragePercent = %f, want %f", mc.CoveragePercent, 80.0)
	}
}
