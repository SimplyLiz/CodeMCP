//go:build cgo

package perf

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/SimplyLiz/CodeMCP/internal/complexity"
)

// ─── Pure function tests ──────────────────────────────────────────────────────

func TestComputeSeverity(t *testing.T) {
	tests := []struct {
		churn  int
		nearEP bool
		want   string
	}{
		{15, true, "high"},
		{10, true, "high"},
		{5, true, "medium"},
		{15, false, "medium"},
		{10, false, "medium"},
		{3, false, "low"},
		{0, false, "low"},
	}
	for _, tt := range tests {
		got := computeSeverity(tt.churn, tt.nearEP)
		if got != tt.want {
			t.Errorf("computeSeverity(%d, %v) = %q, want %q", tt.churn, tt.nearEP, got, tt.want)
		}
	}
}

func TestSeverityRank(t *testing.T) {
	if severityRank("high") <= severityRank("medium") {
		t.Error("high should rank above medium")
	}
	if severityRank("medium") <= severityRank("low") {
		t.Error("medium should rank above low")
	}
	if severityRank("unknown") != severityRank("low") {
		t.Error("unknown severity should rank same as low")
	}
}

func TestBuildExplanation(t *testing.T) {
	t.Run("includes function name and call text", func(t *testing.T) {
		s := buildExplanation("internal/service.go", "processItems", "db.Query()", "for/range", 4, false)
		if s == "" {
			t.Fatal("explanation should not be empty")
		}
		for _, want := range []string{"processItems", "db.Query()", "for/range"} {
			if !containsStr(s, want) {
				t.Errorf("explanation missing %q: %s", want, s)
			}
		}
	})

	t.Run("entrypoint adds request note", func(t *testing.T) {
		s := buildExplanation("cmd/server.go", "handleRequest", "render()", "for", 12, true)
		if !containsStr(s, "entrypoint") {
			t.Errorf("entrypoint explanation should mention entrypoint: %s", s)
		}
	})

	t.Run("very high churn", func(t *testing.T) {
		s := buildExplanation("hot.go", "fn", "call()", "for", 25, false)
		if !containsStr(s, "very frequently changed") {
			t.Errorf("churn=25 should say 'very frequently changed': %s", s)
		}
	})

	t.Run("low churn", func(t *testing.T) {
		s := buildExplanation("new.go", "fn", "call()", "for", 2, false)
		if !containsStr(s, "recently changed") {
			t.Errorf("churn=2 should say 'recently changed': %s", s)
		}
	})
}

func TestFindEnclosingFunction(t *testing.T) {
	fns := []complexity.ComplexityResult{
		{Name: "outer", StartLine: 1, EndLine: 50},
		{Name: "inner", StartLine: 10, EndLine: 20},
		{Name: "other", StartLine: 60, EndLine: 80},
	}

	tests := []struct {
		line int
		want string
	}{
		{5, "outer"},      // inside outer only
		{15, "inner"},     // both match — inner wins (smaller range)
		{55, "<global>"},  // gap between functions
		{70, "other"},
		{100, "<global>"},
	}
	for _, tt := range tests {
		got := findEnclosingFunction(tt.line, fns)
		if got != tt.want {
			t.Errorf("findEnclosingFunction(%d) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestFindEnclosingFunction_Empty(t *testing.T) {
	if got := findEnclosingFunction(10, nil); got != "<global>" {
		t.Errorf("got %q, want <global>", got)
	}
}

func TestHumanLoopType(t *testing.T) {
	tests := []struct {
		nodeType string
		lang     complexity.Language
		want     string
	}{
		{"for_statement", complexity.LangGo, "for/range"},
		{"for_statement", complexity.LangJavaScript, "for"},
		{"enhanced_for_statement", complexity.LangJava, "for-each"},
		{"for_in_statement", complexity.LangJavaScript, "for-in"},
		{"for_of_statement", complexity.LangTypeScript, "for-of"},
		{"while_statement", complexity.LangPython, "while"},
		{"do_statement", complexity.LangJavaScript, "do-while"},
		{"do_while_statement", complexity.LangKotlin, "do-while"},
		{"loop_expression", complexity.LangRust, "loop"},
		{"for_expression", complexity.LangRust, "for"},
		{"mystery_node", complexity.LangGo, "mystery_node"}, // passthrough
	}
	for _, tt := range tests {
		got := humanLoopType(tt.nodeType, tt.lang)
		if got != tt.want {
			t.Errorf("humanLoopType(%q, %v) = %q, want %q", tt.nodeType, tt.lang, got, tt.want)
		}
	}
}

// ─── Integration test with real git repo ─────────────────────────────────────

// TestAnalyzeStructural_FindsLoopCallSite creates a hot Go file containing a
// for loop with a function call inside it, commits it several times so it
// qualifies as a hot file, then verifies the structural scanner surfaces the
// call site.
func TestAnalyzeStructural_FindsLoopCallSite(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := initGitRepoStructural(t)

	// Write a Go file with an obvious loop call site.
	src := `package main

import "fmt"

func processAll(items []string) {
	for _, item := range items {
		fmt.Println(item)
	}
}
`
	// Commit it enough times to exceed MinChurnCount.
	for i := 0; i < 4; i++ {
		writeAndCommitStructural(t, dir, map[string]string{
			"service.go": src + "// v" + string(rune('0'+i)),
		}, "update service")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewAnalyzer(dir, logger)

	result, err := a.AnalyzeStructural(context.Background(), StructuralPerfOptions{
		WindowDays:    365,
		MinChurnCount: 3,
		Limit:         50,
	})
	if err != nil {
		t.Fatalf("AnalyzeStructural() error = %v", err)
	}

	if result.NoCGO {
		t.Skip("CGO not available at runtime — stub returned NoCGO=true")
	}

	if result.Summary.FilesScanned == 0 {
		t.Fatal("expected at least one file scanned")
	}

	// Find the call site in service.go.
	var found bool
	for _, cs := range result.LoopCallSites {
		if cs.File == "service.go" {
			found = true
			if cs.Line == 0 {
				t.Error("call site line should be non-zero")
			}
			if cs.LoopType == "" {
				t.Error("loop type should be set")
			}
			if cs.FunctionName == "" {
				t.Error("function name should be set")
			}
			if cs.CallText == "" {
				t.Error("call text should be set")
			}
			if cs.ChurnCount < 3 {
				t.Errorf("churn count = %d, want ≥3", cs.ChurnCount)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected a call site in service.go, got: %+v", result.LoopCallSites)
	}
}

func TestAnalyzeStructural_RespectsMinChurn(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := initGitRepoStructural(t)

	// Only 2 commits — below MinChurnCount=3.
	src := `package main
func work(items []int) {
	for _, v := range items {
		_ = v
	}
}
`
	for i := 0; i < 2; i++ {
		writeAndCommitStructural(t, dir, map[string]string{
			"cold.go": src + "// v" + string(rune('0'+i)),
		}, "cold file")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewAnalyzer(dir, logger)

	result, err := a.AnalyzeStructural(context.Background(), StructuralPerfOptions{
		WindowDays:    365,
		MinChurnCount: 3,
	})
	if err != nil {
		t.Fatalf("AnalyzeStructural() error = %v", err)
	}
	if result.NoCGO {
		t.Skip("CGO not available")
	}

	for _, cs := range result.LoopCallSites {
		if cs.File == "cold.go" {
			t.Errorf("cold.go should be filtered by MinChurnCount=3, but appeared: %+v", cs)
		}
	}
}

func TestAnalyzeStructural_RespectsLimit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := initGitRepoStructural(t)

	// A file with many loop call sites.
	src := `package main

import "fmt"

func manyLoops(items []string) {
	for _, a := range items {
		fmt.Println(a)
		fmt.Printf("%s\n", a)
		fmt.Sprint(a)
		fmt.Sprintf("%s", a)
	}
}
`
	for i := 0; i < 4; i++ {
		writeAndCommitStructural(t, dir, map[string]string{
			"multi.go": src + "// v" + string(rune('0'+i)),
		}, "multi")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewAnalyzer(dir, logger)

	result, err := a.AnalyzeStructural(context.Background(), StructuralPerfOptions{
		WindowDays:    365,
		MinChurnCount: 3,
		Limit:         2,
	})
	if err != nil {
		t.Fatalf("AnalyzeStructural() error = %v", err)
	}
	if result.NoCGO {
		t.Skip("CGO not available")
	}

	if len(result.LoopCallSites) > 2 {
		t.Errorf("expected ≤2 results, got %d", len(result.LoopCallSites))
	}
}

func TestAnalyzeStructural_SummaryConsistent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := initGitRepoStructural(t)
	src := `package main
func run(items []int) {
	for _, v := range items {
		_ = v
	}
}
`
	for i := 0; i < 4; i++ {
		writeAndCommitStructural(t, dir, map[string]string{
			"svc.go": src + "// v" + string(rune('0'+i)),
		}, "update")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewAnalyzer(dir, logger)

	result, err := a.AnalyzeStructural(context.Background(), StructuralPerfOptions{
		WindowDays: 365, MinChurnCount: 3,
	})
	if err != nil {
		t.Fatalf("AnalyzeStructural() error = %v", err)
	}
	if result.NoCGO {
		t.Skip("CGO not available")
	}

	if result.Summary.CallSitesFound != len(result.LoopCallSites) {
		t.Errorf("Summary.CallSitesFound=%d != len(LoopCallSites)=%d",
			result.Summary.CallSitesFound, len(result.LoopCallSites))
	}
	if result.Summary.HotFilesFound < result.Summary.FilesScanned {
		t.Errorf("HotFilesFound (%d) < FilesScanned (%d)",
			result.Summary.HotFilesFound, result.Summary.FilesScanned)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func initGitRepoStructural(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitStructural(t, dir, "init")
	runGitStructural(t, dir, "config", "user.email", "test@example.com")
	runGitStructural(t, dir, "config", "user.name", "Test")
	return dir
}

func writeAndCommitStructural(t *testing.T, dir string, files map[string]string, msg string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		runGitStructural(t, dir, "add", name)
	}
	runGitStructural(t, dir, "commit", "-m", msg)
}

func runGitStructural(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
