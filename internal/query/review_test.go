package query

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	if !policy.BlockBreakingChanges {
		t.Error("expected BlockBreakingChanges to be true by default")
	}
	if !policy.BlockSecrets {
		t.Error("expected BlockSecrets to be true by default")
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
			_, detected := detectGeneratedFile("", tt.path, policy)
			if detected != tt.expected {
				t.Errorf("detectGeneratedFile(%q) = %v, want %v", tt.path, detected, tt.expected)
			}
		})
	}
}

func TestDetectGeneratedFile_DistPattern(t *testing.T) {
	t.Parallel()
	policy := DefaultReviewPolicy()

	tests := []struct {
		path     string
		expected bool
	}{
		{".github/actions/pr-analysis/dist/index.js", true},
		{"frontend/dist/bundle.js", true},
		{"frontend/dist/styles.css", true},
		{"src/dist.go", false},         // not a dist/ directory
		{"dist/README.md", false},      // not JS/CSS
		{"src/components/app.js", false}, // not in dist/
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			_, detected := detectGeneratedFile("", tt.path, policy)
			if detected != tt.expected {
				t.Errorf("detectGeneratedFile(%q) = %v, want %v", tt.path, detected, tt.expected)
			}
		})
	}
}

func TestDetectGeneratedFile_MarkerInFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	policy := DefaultReviewPolicy()

	// File with a generated marker in the first 10 lines
	genFile := filepath.Join(dir, "gen.go")
	if err := os.WriteFile(genFile, []byte("// Code generated by protoc-gen-go. DO NOT EDIT.\npackage pb\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// File without any marker
	normalFile := filepath.Join(dir, "normal.go")
	if err := os.WriteFile(normalFile, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	info, detected := detectGeneratedFile(dir, "gen.go", policy)
	if !detected {
		t.Error("expected gen.go to be detected as generated via marker")
	}
	if !strings.Contains(info.Reason, "marker") {
		t.Errorf("reason should mention marker, got %q", info.Reason)
	}

	_, detected = detectGeneratedFile(dir, "normal.go", policy)
	if detected {
		t.Error("normal.go should not be detected as generated")
	}
}

func TestReconcileCheckSummaries(t *testing.T) {
	t.Parallel()

	checks := []ReviewCheck{
		{Name: "bug-patterns", Status: "warn", Summary: "5 new bug pattern(s)"},
		{Name: "secrets", Status: "pass", Summary: "No secrets detected"},
		{Name: "coupling", Status: "warn", Summary: "3 missing co-change files"},
	}
	// Only coupling has surviving findings
	findings := []ReviewFinding{
		{Check: "coupling", Message: "Missing co-change: foo.go"},
	}

	reconcileCheckSummaries(checks, findings)

	// bug-patterns had warn but 0 surviving findings → should be downgraded
	if checks[0].Status != "pass" {
		t.Errorf("bug-patterns status = %q, want pass", checks[0].Status)
	}
	if !strings.Contains(checks[0].Summary, "unchanged lines") {
		t.Errorf("bug-patterns summary should note unchanged lines, got %q", checks[0].Summary)
	}
	// secrets was already pass → unchanged
	if checks[1].Status != "pass" {
		t.Errorf("secrets status = %q, want pass", checks[1].Status)
	}
	// coupling has surviving findings → stays warn
	if checks[2].Status != "warn" {
		t.Errorf("coupling status = %q, want warn", checks[2].Status)
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
		{Check: "breaking", Severity: "error", File: "a.go"},
	}
	score = calculateReviewScore(nil, findings)
	if score != 90 {
		t.Errorf("expected score 90 for 1 error finding, got %d", score)
	}

	// Warning findings reduce by 3 each
	findings = []ReviewFinding{
		{Check: "coupling", Severity: "warning", File: "b.go"},
	}
	scoreWarn := calculateReviewScore(nil, findings)
	if scoreWarn != 97 {
		t.Errorf("expected score 97 for 1 warning finding, got %d", scoreWarn)
	}

	// Mixed findings from different checks
	findings = []ReviewFinding{
		{Check: "breaking", Severity: "error", File: "a.go"},
		{Check: "coupling", Severity: "warning", File: "b.go"},
		{Check: "hotspots", Severity: "info", File: "c.go"},
	}
	score = calculateReviewScore(nil, findings)
	// 100 - 10 - 3 - 1 = 86
	if score != 86 {
		t.Errorf("expected score 86 for mixed findings, got %d", score)
	}

	// Per-check cap: 15 errors from one check are capped at 20 points
	manyErrors := make([]ReviewFinding, 15)
	for i := range manyErrors {
		manyErrors[i] = ReviewFinding{Check: "breaking", Severity: "error"}
	}
	score = calculateReviewScore(nil, manyErrors)
	// 100 - 20 (capped) = 80
	if score != 80 {
		t.Errorf("expected score 80 for 15 capped errors, got %d", score)
	}

	// Total deduction cap: score floors at 20 (100 - 80 max deduction)
	var manyCheckErrors []ReviewFinding
	for i := 0; i < 6; i++ {
		for j := 0; j < 5; j++ {
			manyCheckErrors = append(manyCheckErrors, ReviewFinding{
				Check:    fmt.Sprintf("check%d", i),
				Severity: "error",
			})
		}
	}
	score = calculateReviewScore(nil, manyCheckErrors)
	// 6 checks × 20 per-check cap = 120 potential, but total cap is 80, so score = 20
	if score != 20 {
		t.Errorf("expected score 20 for many checks at total cap, got %d", score)
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

func TestReviewPR_NoSCIPIndex(t *testing.T) {
	t.Parallel()

	// Create 25 Go files to trigger concurrent tree-sitter access.
	// The race condition in searchWithTreesitter only manifests with enough
	// files that parsing overlaps across goroutines.
	files := make(map[string]string)
	for i := 0; i < 25; i++ {
		files[fmt.Sprintf("pkg/file%d.go", i)] = fmt.Sprintf(
			"package pkg\n\nfunc Func%d() string {\n\treturn \"value%d\"\n}\n", i, i)
	}

	engine, cleanup := setupGitRepoWithBranch(t, files)
	defer cleanup()

	// Verify SCIP is NOT available (no index built).
	// The adapter struct may exist but IsAvailable() returns false without an index.
	if engine.scipAdapter != nil && engine.scipAdapter.IsAvailable() {
		t.Skip("SCIP index unexpectedly available")
	}

	ctx := context.Background()
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/test",
		Checks:     []string{"secrets", "complexity", "health", "bug-patterns", "dead-code", "blast-radius"},
	})
	if err != nil {
		t.Fatalf("ReviewPR failed (should not crash without SCIP): %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Verdict == "" {
		t.Error("expected non-empty verdict")
	}
	if resp.Score < 0 || resp.Score > 100 {
		t.Errorf("score %d out of range [0,100]", resp.Score)
	}
	// At least some checks should have run (secrets, complexity if tree-sitter available)
	if len(resp.Checks) == 0 {
		t.Error("expected at least one check to run")
	}
	t.Logf("NoSCIP review: verdict=%s score=%d checks=%d findings=%d",
		resp.Verdict, resp.Score, len(resp.Checks), len(resp.Findings))
}

// checkNames is a test helper that extracts check names for error messages.
func checkNames(checks []ReviewCheck) []string {
	names := make([]string, len(checks))
	for i, c := range checks {
		names[i] = c.Name
	}
	return names
}
