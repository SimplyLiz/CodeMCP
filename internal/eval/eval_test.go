package eval

import (
	"context"
	"testing"
	"time"

	"ckb/internal/query"
)

func TestMatchSymbol(t *testing.T) {
	tests := []struct {
		name    string
		sym     query.SearchResultItem
		pattern string
		want    bool
	}{
		{
			name:    "exact name match",
			sym:     query.SearchResultItem{Name: "Engine"},
			pattern: "Engine",
			want:    true,
		},
		{
			name:    "case insensitive name match",
			sym:     query.SearchResultItem{Name: "Engine"},
			pattern: "engine",
			want:    true,
		},
		{
			name:    "suffix match",
			sym:     query.SearchResultItem{Name: "query.Engine"},
			pattern: "Engine",
			want:    true,
		},
		{
			name:    "case insensitive suffix match",
			sym:     query.SearchResultItem{Name: "query.Engine"},
			pattern: "engine",
			want:    true,
		},
		{
			name:    "stable id contains match",
			sym:     query.SearchResultItem{Name: "foo", StableId: "ckb:repo:sym:Orchestrator"},
			pattern: "orchestrator",
			want:    true,
		},
		{
			name:    "no match",
			sym:     query.SearchResultItem{Name: "Foo", StableId: "ckb:repo:sym:bar"},
			pattern: "baz",
			want:    false,
		},
		{
			name:    "empty pattern",
			sym:     query.SearchResultItem{Name: "Engine"},
			pattern: "",
			want:    true, // empty pattern matches as suffix
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchSymbol(tt.sym, tt.pattern)
			if got != tt.want {
				t.Errorf("matchSymbol() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSuiteResultFormatReport(t *testing.T) {
	result := &SuiteResult{
		TotalTests:  10,
		PassedTests: 8,
		FailedTests: 2,
		RecallAtK:   80.0,
		MRR:         0.75,
		AvgLatency:  50.5,
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(time.Second),
		Results: []TestResult{
			{
				TestCase: TestCase{ID: "test-1", Description: "Find Engine", Query: "Engine", ExpectedSymbols: []string{"Engine"}},
				Passed:   true,
				FoundAt:  1,
			},
			{
				TestCase:   TestCase{ID: "test-2", Description: "Find Missing", Query: "missing", ExpectedSymbols: []string{"Missing"}},
				Passed:     false,
				TopResults: []string{"Other", "Stuff"},
			},
		},
	}

	report := result.FormatReport()

	// Check essential parts of the report
	if !containsStr(report, "Total Tests: 10") {
		t.Errorf("Report should contain total tests")
	}
	if !containsStr(report, "Passed:") {
		t.Errorf("Report should contain passed count")
	}
	if !containsStr(report, "MRR:") {
		t.Errorf("Report should contain MRR")
	}
	if !containsStr(report, "Failed Tests:") {
		t.Errorf("Report should contain failed tests section")
	}
	if !containsStr(report, "test-2") {
		t.Errorf("Report should contain failed test ID")
	}
}

func TestSuiteResultFormatReportWithTypeBreakdown(t *testing.T) {
	result := &SuiteResult{
		TotalTests:      3,
		PassedTests:     2,
		FailedTests:     1,
		RecallAtK:       66.7,
		NeedleRecall:    100.0,
		ExpansionRecall: 50.0,
		RankingRecall:   0.0,
		StartTime:       time.Now(),
		EndTime:         time.Now().Add(time.Second),
		Results:         []TestResult{},
	}

	report := result.FormatReport()

	if !containsStr(report, "Needle:") {
		t.Errorf("Report should contain needle recall")
	}
	if !containsStr(report, "Expansion:") {
		t.Errorf("Report should contain expansion recall")
	}
}

func TestSuiteResultJSON(t *testing.T) {
	result := &SuiteResult{
		TotalTests:  5,
		PassedTests: 4,
		FailedTests: 1,
		RecallAtK:   80.0,
		StartTime:   time.Now(),
		EndTime:     time.Now().Add(time.Second),
	}

	data, err := result.JSON()
	if err != nil {
		t.Fatalf("JSON() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("JSON() returned empty data")
	}

	if !containsStr(string(data), `"totalTests"`) {
		t.Errorf("JSON should contain totalTests field")
	}
}

func TestSuiteResultSortResultsByDuration(t *testing.T) {
	result := &SuiteResult{
		Results: []TestResult{
			{TestCase: TestCase{ID: "fast"}, Duration: 10 * time.Millisecond},
			{TestCase: TestCase{ID: "slow"}, Duration: 100 * time.Millisecond},
			{TestCase: TestCase{ID: "medium"}, Duration: 50 * time.Millisecond},
		},
	}

	result.SortResultsByDuration()

	// Should be sorted descending by duration
	if result.Results[0].TestCase.ID != "slow" {
		t.Errorf("First result should be 'slow', got %s", result.Results[0].TestCase.ID)
	}
	if result.Results[1].TestCase.ID != "medium" {
		t.Errorf("Second result should be 'medium', got %s", result.Results[1].TestCase.ID)
	}
	if result.Results[2].TestCase.ID != "fast" {
		t.Errorf("Third result should be 'fast', got %s", result.Results[2].TestCase.ID)
	}
}

func TestSuiteResultSortResultsByID(t *testing.T) {
	result := &SuiteResult{
		Results: []TestResult{
			{TestCase: TestCase{ID: "c-test"}},
			{TestCase: TestCase{ID: "a-test"}},
			{TestCase: TestCase{ID: "b-test"}},
		},
	}

	result.SortResultsByID()

	// Should be sorted ascending by ID
	if result.Results[0].TestCase.ID != "a-test" {
		t.Errorf("First result should be 'a-test', got %s", result.Results[0].TestCase.ID)
	}
	if result.Results[1].TestCase.ID != "b-test" {
		t.Errorf("Second result should be 'b-test', got %s", result.Results[1].TestCase.ID)
	}
	if result.Results[2].TestCase.ID != "c-test" {
		t.Errorf("Third result should be 'c-test', got %s", result.Results[2].TestCase.ID)
	}
}

func TestNewSuite(t *testing.T) {
	suite := NewSuite(nil, nil)
	if suite == nil {
		t.Fatal("NewSuite returned nil")
	}
	if suite.fixtures == nil {
		t.Error("fixtures should be initialized")
	}
}

func TestAddFixture(t *testing.T) {
	suite := NewSuite(nil, nil)

	tc := TestCase{
		ID:              "test-1",
		Query:           "Engine",
		ExpectedSymbols: []string{"Engine"},
	}

	suite.AddFixture(tc)

	if len(suite.fixtures) != 1 {
		t.Errorf("fixtures count = %d, want 1", len(suite.fixtures))
	}

	// Check default TopK was set
	if suite.fixtures[0].TopK != 10 {
		t.Errorf("TopK = %d, want 10", suite.fixtures[0].TopK)
	}
}

func TestAddFixtureWithCustomTopK(t *testing.T) {
	suite := NewSuite(nil, nil)

	tc := TestCase{
		ID:    "test-1",
		Query: "Engine",
		TopK:  5,
	}

	suite.AddFixture(tc)

	// Custom TopK should be preserved
	if suite.fixtures[0].TopK != 5 {
		t.Errorf("TopK = %d, want 5", suite.fixtures[0].TopK)
	}
}

func TestTestCaseStructure(t *testing.T) {
	tc := TestCase{
		ID:              "test-1",
		Type:            "needle",
		Description:     "Find the Engine class",
		Query:           "Engine",
		ExpectedSymbols: []string{"Engine", "engine"},
		Scope:           "internal/query",
		Kinds:           []string{"class", "struct"},
		TopK:            20,
	}

	if tc.ID != "test-1" {
		t.Errorf("ID = %q, want 'test-1'", tc.ID)
	}
	if tc.Type != "needle" {
		t.Errorf("Type = %q, want 'needle'", tc.Type)
	}
	if len(tc.ExpectedSymbols) != 2 {
		t.Errorf("ExpectedSymbols len = %d, want 2", len(tc.ExpectedSymbols))
	}
}

func TestTestResultStructure(t *testing.T) {
	now := time.Now()
	tr := TestResult{
		TestCase: TestCase{ID: "test-1"},
		Passed:   true,
		FoundAt:  3,
		Duration: 50 * time.Millisecond,
	}

	if !tr.Passed {
		t.Error("Passed should be true")
	}
	if tr.FoundAt != 3 {
		t.Errorf("FoundAt = %d, want 3", tr.FoundAt)
	}
	if tr.Duration != 50*time.Millisecond {
		t.Errorf("Duration = %v, want 50ms", tr.Duration)
	}
	_ = now
}

func TestEvaluateNeedleNoExpected(t *testing.T) {
	suite := NewSuite(nil, nil)
	symbols := []query.SearchResultItem{{Name: "Engine"}}

	passed, foundAt := suite.evaluateNeedle(symbols, []string{}, 10)
	if passed {
		t.Error("Should not pass with empty expected")
	}
	if foundAt != 0 {
		t.Errorf("foundAt = %d, want 0", foundAt)
	}
}

func TestEvaluateNeedleFound(t *testing.T) {
	suite := NewSuite(nil, nil)
	symbols := []query.SearchResultItem{
		{Name: "Foo"},
		{Name: "Bar"},
		{Name: "Engine"},
	}

	passed, foundAt := suite.evaluateNeedle(symbols, []string{"Engine"}, 10)
	if !passed {
		t.Error("Should pass when expected is found")
	}
	if foundAt != 3 {
		t.Errorf("foundAt = %d, want 3", foundAt)
	}
}

func TestEvaluateNeedleNotInTopK(t *testing.T) {
	suite := NewSuite(nil, nil)
	symbols := []query.SearchResultItem{
		{Name: "Foo"},
		{Name: "Bar"},
		{Name: "Baz"},
		{Name: "Engine"}, // At position 4
	}

	// TopK = 3 means we only check first 3
	passed, foundAt := suite.evaluateNeedle(symbols, []string{"Engine"}, 3)
	if passed {
		t.Error("Should not pass when expected is outside top-K")
	}
	if foundAt != 0 {
		t.Errorf("foundAt = %d, want 0", foundAt)
	}
}

func TestEvaluateRankingNoExpected(t *testing.T) {
	suite := NewSuite(nil, nil)
	symbols := []query.SearchResultItem{{Name: "Engine"}}

	passed, foundAt := suite.evaluateRanking(symbols, []string{}, 10)
	if passed {
		t.Error("Should not pass with empty expected")
	}
	if foundAt != 0 {
		t.Errorf("foundAt = %d, want 0", foundAt)
	}
}

func TestEvaluateRankingOnlyChecksPrimary(t *testing.T) {
	suite := NewSuite(nil, nil)
	symbols := []query.SearchResultItem{
		{Name: "Secondary"}, // Second expected is at position 1
		{Name: "Primary"},   // Primary expected is at position 2
	}

	// Should only check primary (first expected symbol)
	passed, foundAt := suite.evaluateRanking(symbols, []string{"Primary", "Secondary"}, 10)
	if !passed {
		t.Error("Should pass when primary is found")
	}
	if foundAt != 2 {
		t.Errorf("foundAt = %d, want 2", foundAt)
	}
}

func TestEvaluateExpansion(t *testing.T) {
	suite := NewSuite(nil, nil)
	symbols := []query.SearchResultItem{
		{Name: "A"},
		{Name: "B"},
		{Name: "C"},
	}

	// Should pass if 80%+ are found
	passed, found := suite.evaluateExpansion(context.Background(), symbols, []string{"A", "B", "C"}, 10)
	if !passed {
		t.Error("Should pass when all expected are found")
	}
	if found != 3 {
		t.Errorf("found = %d, want 3", found)
	}
}

func TestEvaluateExpansionPartial(t *testing.T) {
	suite := NewSuite(nil, nil)
	symbols := []query.SearchResultItem{
		{Name: "A"},
		{Name: "B"},
	}

	// 2/5 = 40%, below 80% threshold
	passed, found := suite.evaluateExpansion(context.Background(), symbols, []string{"A", "B", "C", "D", "E"}, 10)
	if passed {
		t.Error("Should not pass when less than 80% found")
	}
	if found != 2 {
		t.Errorf("found = %d, want 2", found)
	}
}

// Helper function
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
