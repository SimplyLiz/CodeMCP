package explain

import (
	"testing"
	"time"
)

func TestParseSymbolQuery(t *testing.T) {
	explainer := &Explainer{}

	tests := []struct {
		query    string
		wantFile string
		wantLine int
	}{
		{"src/main.go", "src/main.go", 0},
		{"src/main.go:42", "src/main.go", 42},
		{"src/main.go:10", "src/main.go", 10},
		{"path/to/file.ts:100", "path/to/file.ts", 100},
		{"path/to/file.ts", "path/to/file.ts", 0},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			gotFile, gotLine := explainer.parseSymbolQuery(tt.query)
			if gotFile != tt.wantFile {
				t.Errorf("parseSymbolQuery(%q) file = %q, want %q", tt.query, gotFile, tt.wantFile)
			}
			if gotLine != tt.wantLine {
				t.Errorf("parseSymbolQuery(%q) line = %d, want %d", tt.query, gotLine, tt.wantLine)
			}
		})
	}
}

func TestWarningSeverityConstants(t *testing.T) {
	// Ensure severity constants are defined
	if SeverityInfo == "" {
		t.Error("SeverityInfo should not be empty")
	}
	if SeverityWarning == "" {
		t.Error("SeverityWarning should not be empty")
	}
	if SeverityCritical == "" {
		t.Error("SeverityCritical should not be empty")
	}
}

func TestWarningTypeConstants(t *testing.T) {
	tests := []struct {
		warningType string
	}{
		{WarningTemporaryCode},
		{WarningBusFactor},
		{WarningHighCoupling},
		{WarningStale},
		{WarningComplex},
	}

	for _, tt := range tests {
		t.Run(tt.warningType, func(t *testing.T) {
			if tt.warningType == "" {
				t.Error("Warning type should not be empty")
			}
		})
	}
}

func TestOriginStructure(t *testing.T) {
	// Test Origin struct can be created properly
	origin := Origin{
		CommitSha:     "abc123",
		Author:        "Test User",
		Date:          time.Now(),
		CommitMessage: "Initial commit",
	}

	if origin.CommitSha != "abc123" {
		t.Errorf("Origin.CommitSha = %q, want %q", origin.CommitSha, "abc123")
	}
	if origin.Author != "Test User" {
		t.Errorf("Origin.Author = %q, want %q", origin.Author, "Test User")
	}
}

func TestEvolutionCalculation(t *testing.T) {
	// Test evolution structure
	evolution := Evolution{
		TotalCommits: 10,
		Timeline:     make([]TimelineEntry, 0),
	}

	if evolution.TotalCommits != 10 {
		t.Errorf("Evolution.TotalCommits = %d, want %d", evolution.TotalCommits, 10)
	}
}

func TestWarningStructure(t *testing.T) {
	warning := Warning{
		Type:     WarningBusFactor,
		Message:  "Only one contributor",
		Severity: SeverityWarning,
	}

	if warning.Type != WarningBusFactor {
		t.Errorf("Warning.Type = %q, want %q", warning.Type, WarningBusFactor)
	}
	if warning.Severity != SeverityWarning {
		t.Errorf("Warning.Severity = %q, want %q", warning.Severity, SeverityWarning)
	}
}

func TestReferencesStructure(t *testing.T) {
	refs := References{
		Issues:      []string{"#123", "#456"},
		PRs:         []string{"#789"},
		JiraTickets: []string{"PROJ-123"},
	}

	if len(refs.Issues) != 2 {
		t.Errorf("len(Issues) = %d, want %d", len(refs.Issues), 2)
	}
	if len(refs.PRs) != 1 {
		t.Errorf("len(PRs) = %d, want %d", len(refs.PRs), 1)
	}
}

func TestSymbolExplanationStructure(t *testing.T) {
	exp := SymbolExplanation{
		Symbol: "TestFunc",
		File:   "test.go",
		Origin: Origin{
			Author:        "Test Author",
			CommitMessage: "Add test function",
		},
		Warnings: []Warning{
			{Type: WarningStale, Message: "Not touched in 12 months", Severity: SeverityWarning},
		},
	}

	if exp.Symbol != "TestFunc" {
		t.Errorf("SymbolExplanation.Symbol = %q, want %q", exp.Symbol, "TestFunc")
	}
	if len(exp.Warnings) != 1 {
		t.Errorf("len(Warnings) = %d, want %d", len(exp.Warnings), 1)
	}
}
