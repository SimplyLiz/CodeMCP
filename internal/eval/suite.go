// Package eval provides retrieval quality evaluation for CKB.
// It measures recall@K, precision@K, and MRR for symbol search.
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ckb/internal/logging"
	"ckb/internal/query"
)

// TestCase represents a single evaluation test case.
type TestCase struct {
	// ID is a unique identifier for this test case.
	ID string `json:"id"`

	// Type is the test type: "needle", "expansion", or "ranking".
	Type string `json:"type"`

	// Description is a natural language description of what to find.
	Description string `json:"description"`

	// Query is the search query to execute.
	Query string `json:"query"`

	// ExpectedSymbols are the symbol IDs or name patterns that should be found.
	// For needle tests: at least one must be in results.
	// For expansion tests: all must be reachable from seed.
	// For ranking tests: first one should be in top-K.
	ExpectedSymbols []string `json:"expectedSymbols"`

	// Scope optionally limits the search to a module.
	Scope string `json:"scope,omitempty"`

	// Kinds optionally filters to specific symbol kinds.
	Kinds []string `json:"kinds,omitempty"`

	// TopK is the number of results to consider (default: 10).
	TopK int `json:"topK,omitempty"`
}

// TestResult captures the outcome of a single test case.
type TestResult struct {
	TestCase   TestCase      `json:"testCase"`
	Passed     bool          `json:"passed"`
	FoundAt    int           `json:"foundAt,omitempty"`    // Position where expected was found (1-indexed)
	TotalFound int           `json:"totalFound,omitempty"` // How many expected symbols were found
	Duration   time.Duration `json:"duration"`
	Error      string        `json:"error,omitempty"`
	TopResults []string      `json:"topResults,omitempty"` // Top-K result names for debugging
}

// SuiteResult aggregates results across all test cases.
type SuiteResult struct {
	// Metrics
	TotalTests  int     `json:"totalTests"`
	PassedTests int     `json:"passedTests"`
	FailedTests int     `json:"failedTests"`
	RecallAtK   float64 `json:"recallAtK"`   // % of tests where expected was in top-K
	MRR         float64 `json:"mrr"`         // Mean Reciprocal Rank
	AvgLatency  float64 `json:"avgLatencyMs"`

	// Breakdown by type
	NeedleRecall    float64 `json:"needleRecall,omitempty"`
	ExpansionRecall float64 `json:"expansionRecall,omitempty"`
	RankingRecall   float64 `json:"rankingRecall,omitempty"`

	// Individual results
	Results []TestResult `json:"results"`

	// Timing
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
}

// Suite runs evaluation tests against a query engine.
type Suite struct {
	engine   *query.Engine
	logger   *logging.Logger
	fixtures []TestCase
}

// NewSuite creates a new evaluation suite.
func NewSuite(engine *query.Engine, logger *logging.Logger) *Suite {
	return &Suite{
		engine:   engine,
		logger:   logger,
		fixtures: make([]TestCase, 0),
	}
}

// LoadFixtures loads test cases from a JSON file.
func (s *Suite) LoadFixtures(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read fixtures: %w", err)
	}

	var fixtures []TestCase
	if err := json.Unmarshal(data, &fixtures); err != nil {
		return fmt.Errorf("failed to parse fixtures: %w", err)
	}

	// Validate and set defaults
	for i := range fixtures {
		if fixtures[i].ID == "" {
			fixtures[i].ID = fmt.Sprintf("test-%d", i+1)
		}
		if fixtures[i].Type == "" {
			fixtures[i].Type = "needle"
		}
		if fixtures[i].TopK <= 0 {
			fixtures[i].TopK = 10
		}
	}

	s.fixtures = append(s.fixtures, fixtures...)
	return nil
}

// LoadFixturesDir loads all JSON fixtures from a directory.
func (s *Suite) LoadFixturesDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read fixtures directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if err := s.LoadFixtures(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}

	return nil
}

// AddFixture adds a single test case programmatically.
func (s *Suite) AddFixture(tc TestCase) {
	if tc.TopK <= 0 {
		tc.TopK = 10
	}
	s.fixtures = append(s.fixtures, tc)
}

// Run executes all test cases and returns aggregated results.
func (s *Suite) Run(ctx context.Context) (*SuiteResult, error) {
	result := &SuiteResult{
		StartTime:  time.Now(),
		TotalTests: len(s.fixtures),
		Results:    make([]TestResult, 0, len(s.fixtures)),
	}

	if len(s.fixtures) == 0 {
		return nil, fmt.Errorf("no test fixtures loaded")
	}

	var totalLatency time.Duration
	var reciprocalRankSum float64
	var needleTotal, needlePassed int
	var expansionTotal, expansionPassed int
	var rankingTotal, rankingPassed int

	for _, tc := range s.fixtures {
		tr := s.runTestCase(ctx, tc)
		result.Results = append(result.Results, tr)
		totalLatency += tr.Duration

		if tr.Passed {
			result.PassedTests++
		} else {
			result.FailedTests++
		}

		// Calculate reciprocal rank
		if tr.FoundAt > 0 {
			reciprocalRankSum += 1.0 / float64(tr.FoundAt)
		}

		// Track by type
		switch tc.Type {
		case "needle":
			needleTotal++
			if tr.Passed {
				needlePassed++
			}
		case "expansion":
			expansionTotal++
			if tr.Passed {
				expansionPassed++
			}
		case "ranking":
			rankingTotal++
			if tr.Passed {
				rankingPassed++
			}
		}
	}

	result.EndTime = time.Now()

	// Calculate metrics
	if result.TotalTests > 0 {
		result.RecallAtK = float64(result.PassedTests) / float64(result.TotalTests) * 100
		result.MRR = reciprocalRankSum / float64(result.TotalTests)
		result.AvgLatency = float64(totalLatency.Milliseconds()) / float64(result.TotalTests)
	}

	if needleTotal > 0 {
		result.NeedleRecall = float64(needlePassed) / float64(needleTotal) * 100
	}
	if expansionTotal > 0 {
		result.ExpansionRecall = float64(expansionPassed) / float64(expansionTotal) * 100
	}
	if rankingTotal > 0 {
		result.RankingRecall = float64(rankingPassed) / float64(rankingTotal) * 100
	}

	return result, nil
}

// runTestCase executes a single test case.
func (s *Suite) runTestCase(ctx context.Context, tc TestCase) TestResult {
	start := time.Now()
	result := TestResult{
		TestCase: tc,
	}

	// Execute search
	searchOpts := query.SearchSymbolsOptions{
		Query: tc.Query,
		Scope: tc.Scope,
		Kinds: tc.Kinds,
		Limit: tc.TopK * 2, // Fetch extra for margin
	}

	resp, err := s.engine.SearchSymbols(ctx, searchOpts)
	result.Duration = time.Since(start)

	if err != nil {
		result.Error = err.Error()
		return result
	}

	// Extract result names/IDs for matching
	topK := min(tc.TopK, len(resp.Symbols))

	result.TopResults = make([]string, topK)
	for i := 0; i < topK; i++ {
		result.TopResults[i] = resp.Symbols[i].Name
	}

	// Evaluate based on test type
	switch tc.Type {
	case "needle":
		result.Passed, result.FoundAt = s.evaluateNeedle(resp.Symbols, tc.ExpectedSymbols, tc.TopK)
	case "expansion":
		result.Passed, result.TotalFound = s.evaluateExpansion(ctx, resp.Symbols, tc.ExpectedSymbols, tc.TopK)
	case "ranking":
		result.Passed, result.FoundAt = s.evaluateRanking(resp.Symbols, tc.ExpectedSymbols, tc.TopK)
	default:
		result.Passed, result.FoundAt = s.evaluateNeedle(resp.Symbols, tc.ExpectedSymbols, tc.TopK)
	}

	return result
}

// evaluateNeedle checks if at least one expected symbol is in top-K results.
func (s *Suite) evaluateNeedle(symbols []query.SearchResultItem, expected []string, topK int) (bool, int) {
	if len(expected) == 0 {
		return false, 0
	}

	limit := min(topK, len(symbols))

	for i := 0; i < limit; i++ {
		sym := symbols[i]
		for _, exp := range expected {
			if matchSymbol(sym, exp) {
				return true, i + 1
			}
		}
	}

	return false, 0
}

// evaluateRanking checks if the first expected symbol is in top-K (stricter than needle).
func (s *Suite) evaluateRanking(symbols []query.SearchResultItem, expected []string, topK int) (bool, int) {
	if len(expected) == 0 {
		return false, 0
	}

	// Only check the primary expected symbol
	primary := expected[0]
	limit := min(topK, len(symbols))

	for i := 0; i < limit; i++ {
		if matchSymbol(symbols[i], primary) {
			return true, i + 1
		}
	}

	return false, 0
}

// evaluateExpansion checks if graph expansion from seed includes required symbols.
// For now, this is a simplified check - full implementation needs call graph traversal.
func (s *Suite) evaluateExpansion(_ context.Context, symbols []query.SearchResultItem, expected []string, _ int) (bool, int) {
	// For expansion tests, we check if all expected symbols appear in results
	// Full implementation will use getCallGraph to verify connectivity
	found := 0
	for _, exp := range expected {
		for _, sym := range symbols {
			if matchSymbol(sym, exp) {
				found++
				break
			}
		}
	}

	// Pass if we found at least 80% of expected
	threshold := max(1, int(float64(len(expected))*0.8))

	return found >= threshold, found
}

// matchSymbol checks if a search result matches an expected pattern.
// Supports exact name match, suffix match, and stableId prefix match.
func matchSymbol(sym query.SearchResultItem, pattern string) bool {
	// Exact name match
	if sym.Name == pattern {
		return true
	}

	// Case-insensitive name match
	if strings.EqualFold(sym.Name, pattern) {
		return true
	}

	// Suffix match (e.g., "Engine" matches "query.Engine")
	if strings.HasSuffix(sym.Name, pattern) {
		return true
	}

	// StableId prefix/contains match
	if strings.Contains(sym.StableId, pattern) {
		return true
	}

	return false
}

// FormatReport generates a human-readable report.
func (r *SuiteResult) FormatReport() string {
	var sb strings.Builder

	sb.WriteString("=== CKB Retrieval Evaluation Report ===\n\n")
	fmt.Fprintf(&sb, "Total Tests: %d\n", r.TotalTests)
	fmt.Fprintf(&sb, "Passed:      %d (%.1f%%)\n", r.PassedTests, r.RecallAtK)
	fmt.Fprintf(&sb, "Failed:      %d\n", r.FailedTests)
	fmt.Fprintf(&sb, "MRR:         %.3f\n", r.MRR)
	fmt.Fprintf(&sb, "Avg Latency: %.1fms\n", r.AvgLatency)
	fmt.Fprintf(&sb, "Duration:    %v\n\n", r.EndTime.Sub(r.StartTime).Round(time.Millisecond))

	// Breakdown by type
	if r.NeedleRecall > 0 || r.ExpansionRecall > 0 || r.RankingRecall > 0 {
		sb.WriteString("By Test Type:\n")
		if r.NeedleRecall > 0 {
			fmt.Fprintf(&sb, "  Needle:    %.1f%% recall\n", r.NeedleRecall)
		}
		if r.ExpansionRecall > 0 {
			fmt.Fprintf(&sb, "  Expansion: %.1f%% recall\n", r.ExpansionRecall)
		}
		if r.RankingRecall > 0 {
			fmt.Fprintf(&sb, "  Ranking:   %.1f%% recall\n", r.RankingRecall)
		}
		sb.WriteString("\n")
	}

	// Failed tests details
	failed := make([]TestResult, 0)
	for _, tr := range r.Results {
		if !tr.Passed {
			failed = append(failed, tr)
		}
	}

	if len(failed) > 0 {
		sb.WriteString("Failed Tests:\n")
		for _, tr := range failed {
			fmt.Fprintf(&sb, "  [%s] %s\n", tr.TestCase.ID, tr.TestCase.Description)
			fmt.Fprintf(&sb, "    Query: %q\n", tr.TestCase.Query)
			fmt.Fprintf(&sb, "    Expected: %v\n", tr.TestCase.ExpectedSymbols)
			if len(tr.TopResults) > 0 {
				fmt.Fprintf(&sb, "    Got Top-3: %v\n", tr.TopResults[:min(3, len(tr.TopResults))])
			}
			if tr.Error != "" {
				fmt.Fprintf(&sb, "    Error: %s\n", tr.Error)
			}
			sb.WriteString("\n")
		}
	}

	// Success criteria
	sb.WriteString("Success Criteria:\n")
	fmt.Fprintf(&sb, "  Recall@10 >= 75%%: %v (current: %.1f%%)\n", r.RecallAtK >= 75, r.RecallAtK)
	fmt.Fprintf(&sb, "  Avg Latency < 100ms: %v (current: %.1fms)\n", r.AvgLatency < 100, r.AvgLatency)

	return sb.String()
}

// JSON returns the result as JSON.
func (r *SuiteResult) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// SortResultsByDuration sorts results by duration descending.
func (r *SuiteResult) SortResultsByDuration() {
	sort.Slice(r.Results, func(i, j int) bool {
		return r.Results[i].Duration > r.Results[j].Duration
	})
}

// SortResultsByID sorts results by test case ID.
func (r *SuiteResult) SortResultsByID() {
	sort.Slice(r.Results, func(i, j int) bool {
		return r.Results[i].TestCase.ID < r.Results[j].TestCase.ID
	})
}
