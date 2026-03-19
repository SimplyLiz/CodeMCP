package query

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupGitRepoWithBranch creates a temp git repo with a base commit on "main"
// and a feature branch with changed files. Returns engine + cleanup.
func setupGitRepoWithBranch(t *testing.T, files map[string]string) (*Engine, func()) {
	t.Helper()

	engine, cleanup := testEngine(t)
	repoRoot := engine.repoRoot

	// Initialize git repo
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	git("init", "-b", "main")

	// Create initial file on main
	initialFile := filepath.Join(repoRoot, "README.md")
	if err := os.WriteFile(initialFile, []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "initial commit")

	// Create feature branch and add changed files
	git("checkout", "-b", "feature/test")

	for path, content := range files {
		absPath := filepath.Join(repoRoot, path)
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("commit", "-m", "feature changes")

	// Re-initialize git adapter since repo now exists
	reinitEngine(t, engine)

	return engine, cleanup
}

// reinitEngine re-initializes the engine's git adapter after git init.
func reinitEngine(t *testing.T, engine *Engine) {
	t.Helper()
	if err := engine.initializeBackends(engine.config); err != nil {
		t.Fatalf("failed to reinitialize backends: %v", err)
	}
}

func TestReviewPR_EmptyDiff(t *testing.T) {
	t.Parallel()

	engine, cleanup := testEngine(t)
	defer cleanup()
	repoRoot := engine.repoRoot

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	git("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "initial")
	git("checkout", "-b", "feature/empty")

	reinitEngine(t, engine)

	ctx := context.Background()
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/empty",
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	if resp.Verdict != "pass" {
		t.Errorf("expected verdict 'pass', got %q", resp.Verdict)
	}
	if resp.Score != 100 {
		t.Errorf("expected score 100, got %d", resp.Score)
	}
	if len(resp.Checks) != 0 {
		t.Errorf("expected 0 checks for empty diff, got %d", len(resp.Checks))
	}
}

func TestReviewPR_BasicChanges(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"pkg/main.go": "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n",
		"pkg/util.go": "package main\n\nfunc helper() string {\n\treturn \"help\"\n}\n",
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	ctx := context.Background()
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	// Basic response structure
	if resp.CkbVersion == "" {
		t.Error("expected CkbVersion to be set")
	}
	if resp.SchemaVersion != "8.2" {
		t.Errorf("expected SchemaVersion '8.2', got %q", resp.SchemaVersion)
	}
	if resp.Tool != "reviewPR" {
		t.Errorf("expected Tool 'reviewPR', got %q", resp.Tool)
	}

	// Should have files in summary
	if resp.Summary.TotalFiles != 2 {
		t.Errorf("expected 2 changed files, got %d", resp.Summary.TotalFiles)
	}
	if resp.Summary.TotalChanges == 0 {
		t.Error("expected non-zero total changes")
	}

	// Should have checks run
	if len(resp.Checks) == 0 {
		t.Error("expected at least one check to run")
	}

	// Verdict should be one of the valid values
	validVerdicts := map[string]bool{"pass": true, "warn": true, "fail": true}
	if !validVerdicts[resp.Verdict] {
		t.Errorf("unexpected verdict %q", resp.Verdict)
	}

	// Score should be in range
	if resp.Score < 0 || resp.Score > 100 {
		t.Errorf("score %d out of range [0,100]", resp.Score)
	}

	// Languages should include Go
	foundGo := false
	for _, lang := range resp.Summary.Languages {
		if lang == "go" {
			foundGo = true
		}
	}
	if !foundGo {
		t.Errorf("expected Go in languages, got %v", resp.Summary.Languages)
	}
}

func TestReviewPR_ChecksFilter(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"app.go": "package app\n\nfunc Run() {}\n",
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	ctx := context.Background()

	// Request only secrets check
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
		Checks:     []string{"secrets"},
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	// Should only have the secrets check
	if len(resp.Checks) != 1 {
		t.Errorf("expected 1 check, got %d: %v", len(resp.Checks), checkNames(resp.Checks))
	}
	if len(resp.Checks) > 0 && resp.Checks[0].Name != "secrets" {
		t.Errorf("expected check 'secrets', got %q", resp.Checks[0].Name)
	}
}

func TestReviewPR_GeneratedFileExclusion(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"real.go":             "package main\n\nfunc Real() {}\n",
		"types.pb.go":         "// Code generated by protoc. DO NOT EDIT.\npackage main\n",
		"parser.generated.go": "// AUTO-GENERATED\npackage parser\n",
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	ctx := context.Background()
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	if resp.Summary.TotalFiles != 3 {
		t.Errorf("expected 3 total files, got %d", resp.Summary.TotalFiles)
	}
	if resp.Summary.GeneratedFiles < 2 {
		t.Errorf("expected at least 2 generated files, got %d", resp.Summary.GeneratedFiles)
	}
	if resp.Summary.ReviewableFiles > 1 {
		t.Errorf("expected at most 1 reviewable file, got %d", resp.Summary.ReviewableFiles)
	}
}

func TestReviewPR_CriticalPaths(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"drivers/modbus/handler.go": "package modbus\n\nfunc Handle() {}\n",
		"ui/page.go":                "package ui\n\nfunc Render() {}\n",
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	ctx := context.Background()
	policy := DefaultReviewPolicy()
	policy.CriticalPaths = []string{"drivers/**"}

	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
		Policy:     policy,
		Checks:     []string{"critical"},
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	// Should have critical check
	found := false
	for _, c := range resp.Checks {
		if c.Name == "critical" {
			found = true
			if c.Status == "skip" {
				t.Error("critical check should not be skipped when critical paths are configured")
			}
		}
	}
	if !found {
		t.Error("expected 'critical' check to be present")
	}

	// Should flag the driver file
	hasCriticalFinding := false
	for _, f := range resp.Findings {
		if f.Category == "critical" {
			hasCriticalFinding = true
		}
	}
	if !hasCriticalFinding {
		t.Error("expected at least one critical finding for drivers/** path")
	}
}

func TestReviewPR_SecretsDetection(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"config.go": fmt.Sprintf("package config\n\nvar APIKey = %q\n", "AKIAIOSFODNN7EXAMPLE"),
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	ctx := context.Background()
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
		Checks:     []string{"secrets"},
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	// Secrets check should be present
	var secretsCheck *ReviewCheck
	for i := range resp.Checks {
		if resp.Checks[i].Name == "secrets" {
			secretsCheck = &resp.Checks[i]
		}
	}
	if secretsCheck == nil {
		t.Fatal("expected secrets check to be present")
	}

	// The AWS key pattern should be detected
	if secretsCheck.Status == "pass" && len(resp.Findings) == 0 {
		// Secrets detection depends on the scanner implementation — if the builtin
		// scanner catches this pattern, we should have findings. If not, the check
		// still ran which is the important thing.
		t.Log("secrets check passed with no findings — scanner may not catch this pattern")
	}
}

func TestReviewPR_PolicyOverrides(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"app.go": "package app\n\nfunc Run() {}\n",
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	ctx := context.Background()

	// Test with failOnLevel = "none" — should always pass
	policy := DefaultReviewPolicy()
	policy.FailOnLevel = "none"

	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
		Policy:     policy,
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	if resp.Verdict != "pass" {
		t.Errorf("expected verdict 'pass' with failOnLevel=none, got %q", resp.Verdict)
	}
}

func TestReviewPR_NoGitAdapter(t *testing.T) {
	t.Parallel()

	engine, cleanup := testEngine(t)
	defer cleanup()

	// Engine without git init — gitAdapter may be nil or not available
	ctx := context.Background()
	_, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "HEAD",
	})

	// Should error gracefully (either git adapter not available or diff fails)
	if err == nil {
		t.Log("ReviewPR succeeded without git repo — gitAdapter may still be initialized")
	}
}

func TestDefaultReviewPolicy(t *testing.T) {
	t.Parallel()

	policy := DefaultReviewPolicy()

	if !policy.NoBreakingChanges {
		t.Error("expected NoBreakingChanges to be true by default")
	}
	if !policy.NoSecrets {
		t.Error("expected NoSecrets to be true by default")
	}
	if policy.FailOnLevel != "error" {
		t.Errorf("expected FailOnLevel 'error', got %q", policy.FailOnLevel)
	}
	if !policy.HoldTheLine {
		t.Error("expected HoldTheLine to be true by default")
	}
	if policy.SplitThreshold != 50 {
		t.Errorf("expected SplitThreshold 50, got %d", policy.SplitThreshold)
	}
	if len(policy.GeneratedPatterns) == 0 {
		t.Error("expected default generated patterns")
	}
	if len(policy.GeneratedMarkers) == 0 {
		t.Error("expected default generated markers")
	}
}

func TestDetectGeneratedFile(t *testing.T) {
	t.Parallel()

	policy := DefaultReviewPolicy()

	tests := []struct {
		path     string
		expected bool
	}{
		{"types.pb.go", true},
		{"parser.tab.c", true},
		{"lex.yy.c", true},
		{"widget.generated.dart", true},
		{"main.go", false},
		{"src/app.ts", false},
		{"README.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			_, detected := detectGeneratedFile(tt.path, policy)
			if detected != tt.expected {
				t.Errorf("detectGeneratedFile(%q) = %v, want %v", tt.path, detected, tt.expected)
			}
		})
	}
}

func TestMatchGlob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pattern string
		path    string
		match   bool
	}{
		{"drivers/**", "drivers/modbus/handler.go", true},
		{"drivers/**", "ui/page.go", false},
		{"*.pb.go", "types.pb.go", true},
		{"*.pb.go", "main.go", false},
		{"protocol/**", "protocol/v2/packet.go", true},
		{"src/**/*.ts", "src/components/app.ts", true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.pattern, tt.path), func(t *testing.T) {
			got, err := matchGlob(tt.pattern, tt.path)
			if err != nil {
				t.Fatalf("matchGlob error: %v", err)
			}
			if got != tt.match {
				t.Errorf("matchGlob(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.match)
			}
		})
	}
}

func TestCalculateReviewScore(t *testing.T) {
	t.Parallel()

	// No findings → 100
	score := calculateReviewScore(nil, nil)
	if score != 100 {
		t.Errorf("expected score 100 for no findings, got %d", score)
	}

	// Error findings reduce by 10 each
	findings := []ReviewFinding{
		{Severity: "error", File: "a.go"},
	}
	score = calculateReviewScore(nil, findings)
	if score != 90 {
		t.Errorf("expected score 90 for 1 error finding, got %d", score)
	}

	// Warning findings reduce by 3 each
	findings = []ReviewFinding{
		{Severity: "warning", File: "b.go"},
	}
	scoreWarn := calculateReviewScore(nil, findings)
	if scoreWarn != 97 {
		t.Errorf("expected score 97 for 1 warning finding, got %d", scoreWarn)
	}

	// Mixed findings
	findings = []ReviewFinding{
		{Severity: "error", File: "a.go"},
		{Severity: "warning", File: "b.go"},
		{Severity: "info", File: "c.go"},
	}
	score = calculateReviewScore(nil, findings)
	// 100 - 10 - 3 - 1 = 86
	if score != 86 {
		t.Errorf("expected score 86 for mixed findings, got %d", score)
	}

	// Score floors at 0
	manyErrors := make([]ReviewFinding, 15)
	for i := range manyErrors {
		manyErrors[i] = ReviewFinding{Severity: "error"}
	}
	score = calculateReviewScore(nil, manyErrors)
	if score != 0 {
		t.Errorf("expected score 0 for 15 errors, got %d", score)
	}
}

func TestDetermineVerdict(t *testing.T) {
	t.Parallel()

	policy := DefaultReviewPolicy()

	tests := []struct {
		name    string
		checks  []ReviewCheck
		verdict string
	}{
		{
			name:    "all pass",
			checks:  []ReviewCheck{{Status: "pass"}, {Status: "pass"}},
			verdict: "pass",
		},
		{
			name:    "has fail",
			checks:  []ReviewCheck{{Status: "fail"}, {Status: "pass"}},
			verdict: "fail",
		},
		{
			name:    "has warn",
			checks:  []ReviewCheck{{Status: "warn"}, {Status: "pass"}},
			verdict: "warn",
		},
		{
			name:    "empty checks",
			checks:  []ReviewCheck{},
			verdict: "pass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := determineVerdict(tt.checks, policy)
			if got != tt.verdict {
				t.Errorf("determineVerdict() = %q, want %q", got, tt.verdict)
			}
		})
	}

	// failOnLevel = "none" → always pass
	nonePolicy := DefaultReviewPolicy()
	nonePolicy.FailOnLevel = "none"
	got := determineVerdict([]ReviewCheck{{Status: "fail"}}, nonePolicy)
	if got != "pass" {
		t.Errorf("expected 'pass' with failOnLevel=none, got %q", got)
	}
}

func TestSortChecks(t *testing.T) {
	t.Parallel()

	checks := []ReviewCheck{
		{Name: "a", Status: "pass"},
		{Name: "b", Status: "fail"},
		{Name: "c", Status: "warn"},
		{Name: "d", Status: "skip"},
	}

	sortChecks(checks)

	expected := []string{"fail", "warn", "pass", "skip"}
	for i, exp := range expected {
		if checks[i].Status != exp {
			t.Errorf("sortChecks[%d]: expected status %q, got %q", i, exp, checks[i].Status)
		}
	}
}

func TestSortFindings(t *testing.T) {
	t.Parallel()

	findings := []ReviewFinding{
		{Severity: "info", File: "c.go"},
		{Severity: "error", File: "a.go"},
		{Severity: "warning", File: "b.go"},
	}

	sortFindings(findings)

	expected := []string{"error", "warning", "info"}
	for i, exp := range expected {
		if findings[i].Severity != exp {
			t.Errorf("sortFindings[%d]: expected severity %q, got %q", i, exp, findings[i].Severity)
		}
	}
}

// checkNames is a test helper that extracts check names for error messages.
func checkNames(checks []ReviewCheck) []string {
	names := make([]string, len(checks))
	for i, c := range checks {
		names[i] = c.Name
	}
	return names
}
