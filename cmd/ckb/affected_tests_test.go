package main

import (
	"strings"
	"testing"

	"ckb/internal/query"
)

func TestConvertAffectedTestsResponse(t *testing.T) {
	resp := &query.AffectedTestsResponse{
		Tests: []query.AffectedTest{
			{
				FilePath:   "internal/query/engine_test.go",
				TestNames:  []string{"TestEngine_GetSymbol"},
				Reason:     "direct",
				AffectedBy: []string{"sym1", "sym2"},
				Confidence: 0.9,
			},
			{
				FilePath:   "internal/query/impact_test.go",
				Reason:     "transitive",
				AffectedBy: []string{"sym3"},
				Confidence: 0.7,
			},
		},
		Summary: &query.TestSummary{
			TotalFiles:       2,
			DirectFiles:      1,
			TransitiveFiles:  1,
			CoverageFiles:    0,
			EstimatedRuntime: "15s",
		},
		CoverageUsed: false,
		Confidence:   0.8,
		RunCommand:   "go test ./internal/query/...",
		Provenance: &query.Provenance{
			RepoStateId:     "abc123",
			RepoStateDirty:  false,
			QueryDurationMs: 50,
		},
	}

	result := convertAffectedTestsResponse(resp)

	// Check tests
	if len(result.Tests) != 2 {
		t.Fatalf("expected 2 tests, got %d", len(result.Tests))
	}
	if result.Tests[0].FilePath != "internal/query/engine_test.go" {
		t.Errorf("Tests[0].FilePath: got %q, want %q", result.Tests[0].FilePath, "internal/query/engine_test.go")
	}
	if result.Tests[0].Reason != "direct" {
		t.Errorf("Tests[0].Reason: got %q, want %q", result.Tests[0].Reason, "direct")
	}
	if result.Tests[0].Confidence != 0.9 {
		t.Errorf("Tests[0].Confidence: got %f, want %f", result.Tests[0].Confidence, 0.9)
	}
	if len(result.Tests[0].AffectedBy) != 2 {
		t.Errorf("Tests[0].AffectedBy: got %d, want 2", len(result.Tests[0].AffectedBy))
	}

	// Check summary
	if result.Summary == nil {
		t.Fatal("Summary is nil")
	}
	if result.Summary.TotalFiles != 2 {
		t.Errorf("Summary.TotalFiles: got %d, want 2", result.Summary.TotalFiles)
	}
	if result.Summary.DirectFiles != 1 {
		t.Errorf("Summary.DirectFiles: got %d, want 1", result.Summary.DirectFiles)
	}
	if result.Summary.EstimatedRuntime != "15s" {
		t.Errorf("Summary.EstimatedRuntime: got %q, want %q", result.Summary.EstimatedRuntime, "15s")
	}

	// Check other fields
	if result.CoverageUsed {
		t.Error("CoverageUsed should be false")
	}
	if result.Confidence != 0.8 {
		t.Errorf("Confidence: got %f, want %f", result.Confidence, 0.8)
	}
	if result.RunCommand != "go test ./internal/query/..." {
		t.Errorf("RunCommand: got %q", result.RunCommand)
	}

	// Check provenance
	if result.Provenance == nil {
		t.Fatal("Provenance is nil")
	}
	if result.Provenance.RepoStateId != "abc123" {
		t.Errorf("Provenance.RepoStateId: got %q, want %q", result.Provenance.RepoStateId, "abc123")
	}
}

func TestConvertAffectedTestsResponse_NilFields(t *testing.T) {
	resp := &query.AffectedTestsResponse{
		Tests:      []query.AffectedTest{},
		Summary:    nil,
		Provenance: nil,
	}

	result := convertAffectedTestsResponse(resp)

	if len(result.Tests) != 0 {
		t.Errorf("expected 0 tests, got %d", len(result.Tests))
	}
	if result.Summary != nil {
		t.Error("Summary should be nil")
	}
	if result.Provenance != nil {
		t.Error("Provenance should be nil")
	}
}

func TestFormatAffectedTestsHuman(t *testing.T) {
	t.Run("with tests", func(t *testing.T) {
		resp := &query.AffectedTestsResponse{
			Tests: []query.AffectedTest{
				{FilePath: "foo_test.go", Reason: "direct", Confidence: 0.9},
				{FilePath: "bar_test.go", Reason: "transitive", Confidence: 0.7},
			},
			Summary: &query.TestSummary{
				TotalFiles:      2,
				DirectFiles:     1,
				TransitiveFiles: 1,
			},
			Confidence: 0.8,
			RunCommand: "go test ./...",
		}

		result := formatAffectedTestsHuman(resp)

		if !strings.Contains(result, "Affected Tests") {
			t.Error("missing header")
		}
		if !strings.Contains(result, "Found 2 test files") {
			t.Error("missing summary count")
		}
		if !strings.Contains(result, "1 direct") {
			t.Error("missing direct count")
		}
		if !strings.Contains(result, "1 transitive") {
			t.Error("missing transitive count")
		}
		if !strings.Contains(result, "foo_test.go") {
			t.Error("missing test file")
		}
		if !strings.Contains(result, "● foo_test.go") {
			t.Error("missing direct indicator")
		}
		if !strings.Contains(result, "○ bar_test.go") {
			t.Error("missing transitive indicator")
		}
		if !strings.Contains(result, "go test ./...") {
			t.Error("missing run command")
		}
		if !strings.Contains(result, "Overall confidence: 80%") {
			t.Error("missing confidence")
		}
	})

	t.Run("no tests", func(t *testing.T) {
		resp := &query.AffectedTestsResponse{
			Tests: []query.AffectedTest{},
		}

		result := formatAffectedTestsHuman(resp)

		if !strings.Contains(result, "No affected tests found") {
			t.Error("missing 'no tests' message")
		}
	})

	t.Run("truncation", func(t *testing.T) {
		tests := make([]query.AffectedTest, 25)
		for i := range tests {
			tests[i] = query.AffectedTest{
				FilePath:   "test.go",
				Reason:     "direct",
				Confidence: 0.9,
			}
		}

		resp := &query.AffectedTestsResponse{
			Tests:      tests,
			Confidence: 0.9,
		}

		result := formatAffectedTestsHuman(resp)

		if !strings.Contains(result, "... and 5 more") {
			t.Error("missing truncation message")
		}
	})

	t.Run("coverage files", func(t *testing.T) {
		resp := &query.AffectedTestsResponse{
			Tests: []query.AffectedTest{
				{FilePath: "foo_test.go", Reason: "coverage", Confidence: 0.6},
			},
			Summary: &query.TestSummary{
				TotalFiles:    1,
				CoverageFiles: 1,
			},
			Confidence: 0.6,
		}

		result := formatAffectedTestsHuman(resp)

		if !strings.Contains(result, "1 from coverage data") {
			t.Error("missing coverage file count")
		}
	})
}

func TestAffectedTestCLI(t *testing.T) {
	test := AffectedTestCLI{
		FilePath:   "internal/query/engine_test.go",
		TestNames:  []string{"TestEngine_GetSymbol", "TestEngine_Search"},
		Reason:     "direct",
		AffectedBy: []string{"sym1"},
		Confidence: 0.95,
	}

	if test.FilePath != "internal/query/engine_test.go" {
		t.Errorf("FilePath: got %q", test.FilePath)
	}
	if len(test.TestNames) != 2 {
		t.Errorf("TestNames: got %d, want 2", len(test.TestNames))
	}
	if test.Reason != "direct" {
		t.Errorf("Reason: got %q, want %q", test.Reason, "direct")
	}
	if test.Confidence != 0.95 {
		t.Errorf("Confidence: got %f, want %f", test.Confidence, 0.95)
	}
}

func TestTestSummaryCLI(t *testing.T) {
	summary := TestSummaryCLI{
		TotalFiles:       10,
		DirectFiles:      5,
		TransitiveFiles:  3,
		CoverageFiles:    2,
		EstimatedRuntime: "30s",
	}

	if summary.TotalFiles != 10 {
		t.Errorf("TotalFiles: got %d, want 10", summary.TotalFiles)
	}
	if summary.DirectFiles != 5 {
		t.Errorf("DirectFiles: got %d, want 5", summary.DirectFiles)
	}
	if summary.TransitiveFiles != 3 {
		t.Errorf("TransitiveFiles: got %d, want 3", summary.TransitiveFiles)
	}
	if summary.CoverageFiles != 2 {
		t.Errorf("CoverageFiles: got %d, want 2", summary.CoverageFiles)
	}
	if summary.EstimatedRuntime != "30s" {
		t.Errorf("EstimatedRuntime: got %q, want %q", summary.EstimatedRuntime, "30s")
	}
}
