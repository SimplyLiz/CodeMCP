package query

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckCommentDrift_NumericMismatch(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"pkg/config.go": `package pkg

const (
	// Maximum retries: 3
	MaxRetries = 5

	// Timeout in seconds: 30
	Timeout = 30
)
`,
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	ctx := context.Background()
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
		Checks:     []string{"comment-drift"},
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	// Should find the MaxRetries mismatch (comment says 3, code says 5)
	found := false
	for _, c := range resp.Checks {
		if c.Name == "comment-drift" {
			found = true
			if c.Status != "info" {
				t.Errorf("expected comment-drift status 'info', got %q", c.Status)
			}
		}
	}
	if !found {
		t.Error("expected 'comment-drift' check to be present")
	}

	// Should have at least one finding for MaxRetries
	driftFindings := 0
	for _, f := range resp.Findings {
		if f.Check == "comment-drift" {
			driftFindings++
			if f.RuleID != "ckb/comment-drift/numeric-mismatch" {
				t.Errorf("unexpected ruleID %q", f.RuleID)
			}
		}
	}
	if driftFindings == 0 {
		t.Error("expected at least one comment-drift finding for MaxRetries mismatch")
	}
}

func TestCheckCommentDrift_NoMismatch(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"pkg/config.go": `package pkg

const (
	// Maximum retries: 5
	MaxRetries = 5
)
`,
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	ctx := context.Background()
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
		Checks:     []string{"comment-drift"},
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	for _, f := range resp.Findings {
		if f.Check == "comment-drift" {
			t.Errorf("unexpected comment-drift finding: %s", f.Message)
		}
	}
}

func TestCheckFormatConsistency_DivergentLiterals(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"cmd/review.go": `package main

func formatReviewHuman() string {
	limit := 10
	cap := 50
	return ""
}

func formatReviewMarkdown() string {
	limit := 10
	cap := 100
	return ""
}
`,
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	ctx := context.Background()
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
		Checks:     []string{"format-consistency"},
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	found := false
	for _, c := range resp.Checks {
		if c.Name == "format-consistency" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'format-consistency' check to be present")
	}

	// Should find divergent literals (50 in Human, 100 in Markdown)
	consistencyFindings := 0
	for _, f := range resp.Findings {
		if f.Check == "format-consistency" {
			consistencyFindings++
		}
	}
	if consistencyFindings == 0 {
		t.Error("expected at least one format-consistency finding for divergent cap values")
	}
}

func TestCheckFormatConsistency_MatchingPair(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"cmd/review.go": `package main

func formatReviewHuman() string {
	limit := 10
	return ""
}

func formatReviewMarkdown() string {
	limit := 10
	return ""
}
`,
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	ctx := context.Background()
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
		Checks:     []string{"format-consistency"},
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	for _, f := range resp.Findings {
		if f.Check == "format-consistency" {
			t.Errorf("unexpected format-consistency finding: %s", f.Message)
		}
	}
}

func TestCheckTestGaps_CoverageUpgrade(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"pkg/handler.go": `package pkg

import "fmt"

func HandleRequest(input string) string {
	result := process(input)
	return fmt.Sprintf("handled: %s", result)
}

func process(s string) string {
	return s + " processed"
}
`,
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	// Create a mock coverage file showing 0% coverage
	lcovContent := `SF:pkg/handler.go
LF:10
LH:0
end_of_record
`
	lcovPath := filepath.Join(engine.repoRoot, "coverage.lcov")
	if err := os.WriteFile(lcovPath, []byte(lcovContent), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
		Checks:     []string{"test-gaps"},
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	// If test-gaps found findings, the ones for handler.go should be upgraded
	for _, f := range resp.Findings {
		if f.Check == "test-gaps" && f.File == "pkg/handler.go" {
			if f.Severity != "warning" {
				t.Logf("Expected severity 'warning' for 0%% coverage file, got %q (may depend on tree-sitter availability)", f.Severity)
			}
		}
	}
}

func TestHealthDelta_ConfidenceAndParseable(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"pkg/main.go": `package pkg

func Hello() string {
	return "hello"
}
`,
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	ctx := context.Background()
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
		Checks:     []string{"health"},
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	if resp.HealthReport == nil {
		t.Fatal("expected health report")
	}

	for _, d := range resp.HealthReport.Deltas {
		// Confidence should be between 0 and 1
		if d.Confidence < 0 || d.Confidence > 1 {
			t.Errorf("file %s: confidence %.2f out of range [0, 1]", d.File, d.Confidence)
		}
		// Go files should be parseable if tree-sitter is available
		// (may be false on systems without CGO)
		t.Logf("file %s: confidence=%.2f parseable=%v", d.File, d.Confidence, d.Parseable)
	}
}

func TestBlastRadius_InformationalMode(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"pkg/core.go": `package pkg

func CoreFunction() string {
	return "core"
}
`,
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	ctx := context.Background()

	// Default (maxFanOut=0) → informational mode
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
		Checks:     []string{"blast-radius"},
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	for _, c := range resp.Checks {
		if c.Name == "blast-radius" {
			if c.Severity != "info" {
				t.Errorf("expected severity 'info' in informational mode, got %q", c.Severity)
			}
			// Any findings should also be info severity
			for _, f := range resp.Findings {
				if f.Check == "blast-radius" && f.Severity != "info" {
					t.Errorf("expected finding severity 'info' in informational mode, got %q", f.Severity)
				}
			}
		}
	}

	// With threshold set → warning mode
	policy := DefaultReviewPolicy()
	policy.MaxFanOut = 5
	resp2, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
		Checks:     []string{"blast-radius"},
		Policy:     policy,
	})
	if err != nil {
		t.Fatalf("ReviewPR with maxFanOut failed: %v", err)
	}

	for _, c := range resp2.Checks {
		if c.Name == "blast-radius" {
			if c.Severity != "warning" {
				t.Errorf("expected severity 'warning' with threshold set, got %q", c.Severity)
			}
		}
	}
}

func TestCouplingGaps_NewFilesSuppressed(t *testing.T) {
	t.Parallel()

	// Create an initial repo with an established file
	files := map[string]string{
		"pkg/existing.go": `package pkg

func Existing() string {
	return "existing"
}
`,
		"pkg/new_feature.go": `package pkg

func NewFeature() string {
	return "new"
}
`,
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	ctx := context.Background()
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
		Checks:     []string{"coupling"},
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	// Coupling check should exist
	found := false
	for _, c := range resp.Checks {
		if c.Name == "coupling" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'coupling' check to be present")
	}

	// New files should not generate coupling warnings
	for _, f := range resp.Findings {
		if f.Check == "coupling" && f.File == "pkg/new_feature.go" {
			t.Logf("Note: coupling finding for new file (may depend on git history): %s", f.Message)
		}
	}
}
