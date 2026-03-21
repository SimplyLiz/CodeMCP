package query

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBuildChangedLinesMap(t *testing.T) {
	t.Parallel()

	rawDiff := `diff --git a/foo.go b/foo.go
index 1234567..abcdef0 100644
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,4 @@
 package foo

+func newFunc() {}
 func oldFunc() {}
diff --git a/bar.go b/bar.go
new file mode 100644
index 0000000..1234567
--- /dev/null
+++ b/bar.go
@@ -0,0 +1,3 @@
+package bar
+
+func barFunc() {}
`

	result := buildChangedLinesMap(rawDiff)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// foo.go: line 3 is the added line
	fooLines, ok := result["foo.go"]
	if !ok {
		t.Fatal("expected foo.go in result")
	}
	if !fooLines[3] {
		t.Error("expected line 3 to be changed in foo.go")
	}
	if fooLines[1] {
		t.Error("line 1 should not be marked as changed")
	}

	// bar.go: lines 1-3 are all new
	barLines, ok := result["bar.go"]
	if !ok {
		t.Fatal("expected bar.go in result")
	}
	if !barLines[1] || !barLines[2] || !barLines[3] {
		t.Error("expected lines 1-3 to be changed in bar.go")
	}
}

func TestFilterByChangedLines(t *testing.T) {
	t.Parallel()

	changedLines := map[string]map[int]bool{
		"foo.go": {10: true, 20: true},
		"bar.go": {5: true},
	}

	findings := []ReviewFinding{
		{File: "foo.go", StartLine: 10, Message: "on changed line"},
		{File: "foo.go", StartLine: 15, Message: "off changed line"},
		{File: "foo.go", StartLine: 0, Message: "file-level finding"},
		{File: "baz.go", StartLine: 5, Message: "file not in diff"},
		{File: "bar.go", StartLine: 5, Message: "on changed line"},
		{File: "bar.go", StartLine: 99, Message: "off changed line"},
		{File: "", StartLine: 0, Message: "global finding"},
	}

	filtered := filterByChangedLines(findings, changedLines)

	expected := 5 // on-changed(foo:10), file-level(foo:0), not-in-diff(baz:5), on-changed(bar:5), global
	if len(filtered) != expected {
		t.Errorf("expected %d findings after filter, got %d", expected, len(filtered))
		for _, f := range filtered {
			t.Logf("  kept: %s:%d %s", f.File, f.StartLine, f.Message)
		}
	}
}

func TestReviewPR_HoldTheLine(t *testing.T) {
	t.Parallel()

	// Create a file with a pre-existing "issue" on line 2,
	// then on the feature branch only modify line 5.
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

	// Base: file with content on lines 1-5
	mainContent := "package main\n\nvar secret = \"AKIAIOSFODNN7EXAMPLE\"\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte(mainContent), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "initial")

	// Feature branch: add a new line at the end, don't touch the secret line
	git("checkout", "-b", "feature/holdtheline")
	featureContent := mainContent + "\nfunc newFunc() {}\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte(featureContent), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "add new func")

	reinitEngine(t, engine)

	ctx := context.Background()

	// With HoldTheLine enabled (default), pre-existing secret on line 3 should be filtered
	resp, err := engine.ReviewPR(ctx, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/holdtheline",
		Checks:     []string{"secrets"},
	})
	if err != nil {
		t.Fatalf("ReviewPR failed: %v", err)
	}

	// The secret on line 3 was already in main, so HoldTheLine should filter it out
	for _, f := range resp.Findings {
		if f.Check == "secrets" && f.StartLine == 3 {
			t.Errorf("HoldTheLine should have filtered pre-existing secret on line 3, but finding was kept: %+v", f)
		}
	}
}
