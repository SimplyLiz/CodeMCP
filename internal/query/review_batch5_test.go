package query

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/SimplyLiz/CodeMCP/internal/config"
	"github.com/SimplyLiz/CodeMCP/internal/storage"
)

// newTestEngineWithGit creates a full engine with git adapter for a given repo dir.
func newTestEngineWithGit(t *testing.T, dir string) *Engine {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ckbDir := filepath.Join(dir, ".ckb")
	os.MkdirAll(ckbDir, 0755)

	db, err := storage.Open(dir, logger)
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := config.DefaultConfig()
	cfg.RepoRoot = dir

	engine, err := NewEngine(dir, db, logger, cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return engine
}

// --- Traceability Tests ---

func TestCheckTraceability_NoPatterns(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	e := &Engine{repoRoot: t.TempDir(), logger: logger}
	ctx := context.Background()

	opts := ReviewPROptions{
		Policy: &ReviewPolicy{
			RequireTraceability: true,
		},
	}

	check, _ := e.checkTraceability(ctx, nil, opts)
	if check.Status != "skip" {
		t.Errorf("check.Status = %q, want %q", check.Status, "skip")
	}
}

func TestCheckTraceability_WithPatterns_NoMatch(t *testing.T) {
	dir := setupGitRepoForTraceability(t, "feature/no-ticket", "no ticket here")
	e := newTestEngineWithGit(t, dir)
	ctx := context.Background()

	opts := ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/no-ticket",
		Policy: &ReviewPolicy{
			RequireTraceability:  true,
			TraceabilityPatterns: []string{`JIRA-\d+`},
			TraceabilitySources:  []string{"commit-message", "branch-name"},
		},
	}

	check, findings := e.checkTraceability(ctx, nil, opts)
	if check.Status != "warn" {
		t.Errorf("check.Status = %q, want %q", check.Status, "warn")
	}
	if len(findings) == 0 {
		t.Error("expected findings for missing traceability")
	}
}

func TestCheckTraceability_MatchInCommit(t *testing.T) {
	dir := setupGitRepoForTraceability(t, "feature/stuff", "JIRA-1234 fix the bug")
	e := newTestEngineWithGit(t, dir)
	ctx := context.Background()

	opts := ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/stuff",
		Policy: &ReviewPolicy{
			RequireTraceability:  true,
			TraceabilityPatterns: []string{`JIRA-\d+`},
			TraceabilitySources:  []string{"commit-message"},
		},
	}

	check, findings := e.checkTraceability(ctx, nil, opts)
	if check.Status != "pass" {
		t.Errorf("check.Status = %q, want %q (summary: %s)", check.Status, "pass", check.Summary)
	}
	warnCount := 0
	for _, f := range findings {
		if f.Severity == "warning" || f.Severity == "error" {
			warnCount++
		}
	}
	if warnCount > 0 {
		t.Errorf("expected 0 warn/error findings, got %d", warnCount)
	}
}

func TestCheckTraceability_MatchInBranch(t *testing.T) {
	dir := setupGitRepoForTraceability(t, "feature/JIRA-5678-fix", "some commit")
	e := newTestEngineWithGit(t, dir)
	ctx := context.Background()

	opts := ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/JIRA-5678-fix",
		Policy: &ReviewPolicy{
			RequireTraceability:  true,
			TraceabilityPatterns: []string{`JIRA-\d+`},
			TraceabilitySources:  []string{"branch-name"},
		},
	}

	check, _ := e.checkTraceability(ctx, nil, opts)
	if check.Status != "pass" {
		t.Errorf("check.Status = %q, want %q (summary: %s)", check.Status, "pass", check.Summary)
	}
}

func TestCheckTraceability_CriticalOrphan(t *testing.T) {
	dir := setupGitRepoForTraceability(t, "feature/no-ticket", "no ticket here")
	e := newTestEngineWithGit(t, dir)
	ctx := context.Background()

	files := []string{"drivers/hw/plc.go"}

	opts := ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/no-ticket",
		Policy: &ReviewPolicy{
			RequireTraceForCriticalPaths: true,
			TraceabilityPatterns:         []string{`JIRA-\d+`},
			TraceabilitySources:          []string{"commit-message", "branch-name"},
			CriticalPaths:               []string{"drivers/**"},
		},
	}

	check, findings := e.checkTraceability(ctx, files, opts)
	if check.Status != "fail" {
		t.Errorf("check.Status = %q, want %q", check.Status, "fail")
	}

	hasOrphan := false
	for _, f := range findings {
		if f.RuleID == "ckb/traceability/critical-orphan" {
			hasOrphan = true
		}
	}
	if !hasOrphan {
		t.Error("expected critical-orphan finding")
	}
}

func TestCheckTraceability_MultiplePatterns(t *testing.T) {
	dir := setupGitRepoForTraceability(t, "feature/stuff", "REQ-42 implement feature")
	e := newTestEngineWithGit(t, dir)
	ctx := context.Background()

	opts := ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/stuff",
		Policy: &ReviewPolicy{
			RequireTraceability:  true,
			TraceabilityPatterns: []string{`JIRA-\d+`, `REQ-\d+`, `#\d+`},
			TraceabilitySources:  []string{"commit-message"},
		},
	}

	check, _ := e.checkTraceability(ctx, nil, opts)
	if check.Status != "pass" {
		t.Errorf("check.Status = %q, want %q", check.Status, "pass")
	}
}

// --- Independence Tests ---

func TestCheckIndependence_NoGitAdapter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	e := &Engine{repoRoot: t.TempDir(), logger: logger}
	ctx := context.Background()

	opts := ReviewPROptions{
		Policy: &ReviewPolicy{RequireIndependentReview: true},
	}

	check, _ := e.checkReviewerIndependence(ctx, opts)
	if check.Status != "skip" {
		t.Errorf("check.Status = %q, want %q", check.Status, "skip")
	}
}

func TestCheckIndependence_WithCommits(t *testing.T) {
	dir := setupGitRepoForTraceability(t, "feature/stuff", "fix something")
	e := newTestEngineWithGit(t, dir)
	ctx := context.Background()

	opts := ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/stuff",
		Policy: &ReviewPolicy{
			RequireIndependentReview: true,
			MinReviewers:             1,
		},
	}

	check, findings := e.checkReviewerIndependence(ctx, opts)
	if check.Status != "warn" {
		t.Errorf("check.Status = %q, want %q", check.Status, "warn")
	}
	if len(findings) == 0 {
		t.Error("expected findings for independence requirement")
	}

	hasIndepFinding := false
	for _, f := range findings {
		if f.RuleID == "ckb/independence/require-independent-reviewer" {
			hasIndepFinding = true
		}
	}
	if !hasIndepFinding {
		t.Error("expected require-independent-reviewer finding")
	}
}

func TestCheckIndependence_WithCriticalPaths(t *testing.T) {
	dir := setupGitRepoForTraceability(t, "feature/critical", "change driver")

	// Create a file that matches the critical path
	driversDir := filepath.Join(dir, "drivers", "hw")
	os.MkdirAll(driversDir, 0755)
	os.WriteFile(filepath.Join(driversDir, "plc.go"), []byte("package hw\n"), 0644)
	runGit(t, dir, "add", "drivers/hw/plc.go")
	runGit(t, dir, "commit", "-m", "add driver")

	e := newTestEngineWithGit(t, dir)
	ctx := context.Background()

	opts := ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/critical",
		Policy: &ReviewPolicy{
			RequireIndependentReview: true,
			CriticalPaths:           []string{"drivers/**"},
		},
	}

	check, findings := e.checkReviewerIndependence(ctx, opts)
	if check.Status != "fail" {
		t.Errorf("check.Status = %q, want %q", check.Status, "fail")
	}

	hasCritical := false
	for _, f := range findings {
		if f.RuleID == "ckb/independence/critical-path-review" {
			hasCritical = true
		}
	}
	if !hasCritical {
		t.Error("expected critical-path-review finding")
	}
}

// --- Helpers ---

func TestContainsSource(t *testing.T) {
	if !containsSource([]string{"commit-message", "branch-name"}, "branch-name") {
		t.Error("expected true for branch-name")
	}
	if containsSource([]string{"commit-message"}, "branch-name") {
		t.Error("expected false for branch-name")
	}
}

// setupGitRepoForTraceability creates a git repo with main branch and a feature branch.
func setupGitRepoForTraceability(t *testing.T, branchName, commitMsg string) string {
	t.Helper()
	dir := t.TempDir()

	runGit(t, dir, "init")
	runGit(t, dir, "checkout", "-b", "main")

	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0644)
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial")

	runGit(t, dir, "checkout", "-b", branchName)

	os.WriteFile(filepath.Join(dir, "change.go"), []byte("package main\n"), 0644)
	runGit(t, dir, "add", "change.go")
	runGit(t, dir, "commit", "-m", commitMsg)

	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}
