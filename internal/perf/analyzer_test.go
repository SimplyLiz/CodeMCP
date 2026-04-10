package perf

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// ─── Pure function tests ──────────────────────────────────────────────────────

func TestCorrelationLevel(t *testing.T) {
	tests := []struct {
		corr  float64
		want  string
	}{
		{1.0, "high"},
		{0.8, "high"},
		{0.79, "medium"},
		{0.5, "medium"},
		{0.49, "low"},
		{0.3, "low"},
		{0.0, "low"},
	}
	for _, tt := range tests {
		got := correlationLevel(tt.corr)
		if got != tt.want {
			t.Errorf("correlationLevel(%v) = %q, want %q", tt.corr, got, tt.want)
		}
	}
}

func TestShouldIgnore(t *testing.T) {
	yes := []string{
		"testdata/fixtures/go/foo.go",
		"testdata/file.go",
		"vendor/github.com/foo/bar.go",
		"node_modules/lodash/index.js",
		".ckb/config.json",
	}
	no := []string{
		"internal/perf/analyzer.go",
		"cmd/ckb/main.go",
		"docs/README.md",
		"testable_code.go", // starts with "test" but not "testdata/"
	}
	for _, p := range yes {
		if !shouldIgnore(p) {
			t.Errorf("shouldIgnore(%q) = false, want true", p)
		}
	}
	for _, p := range no {
		if shouldIgnore(p) {
			t.Errorf("shouldIgnore(%q) = true, want false", p)
		}
	}
}

func TestImportCouldReferTo(t *testing.T) {
	tests := []struct {
		name       string
		imports    []string
		targetFile string
		want       bool
	}{
		{
			name:       "Go module import matches directory",
			imports:    []string{"github.com/org/repo/internal/jobs"},
			targetFile: "internal/jobs/job.go",
			want:       true,
		},
		{
			name:       "Go module import matches nested directory",
			imports:    []string{"github.com/org/repo/internal/api/handlers"},
			targetFile: "internal/api/handlers/auth.go",
			want:       true,
		},
		{
			// Without source-file context, relative imports only match when
			// the import string ends with the repo-relative path without ext.
			// This covers absolute-path alias setups (e.g. tsconfig paths).
			name:       "TypeScript alias import matches file without extension",
			imports:    []string{"utils/helper"},
			targetFile: "utils/helper.ts",
			want:       true,
		},
		{
			name:       "TypeScript relative import matches base directory",
			imports:    []string{"./utils"},
			targetFile: "src/utils/index.ts",
			want:       true,
		},
		{
			name:       "unrelated import — no match",
			imports:    []string{"github.com/org/repo/internal/auth"},
			targetFile: "internal/storage/db.go",
			want:       false,
		},
		{
			name:       "empty imports",
			imports:    []string{},
			targetFile: "internal/storage/db.go",
			want:       false,
		},
		{
			name:       "stdlib import — no match",
			imports:    []string{"fmt", "os", "context"},
			targetFile: "internal/os/wrapper.go",
			want:       false, // "os" != "/internal/os" suffix match
		},
		{
			name:       "partial path collision does not match",
			imports:    []string{"github.com/org/repo/internal/jobscheduler"},
			targetFile: "internal/jobs/job.go",
			want:       false, // "jobscheduler" ≠ "jobs"
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := importCouldReferTo(tt.imports, tt.targetFile)
			if got != tt.want {
				t.Errorf("importCouldReferTo(%v, %q) = %v, want %v", tt.imports, tt.targetFile, got, tt.want)
			}
		})
	}
}

// ─── recordCommit unit tests ─────────────────────────────────────────────────

func TestRecordCommit_Empty(t *testing.T) {
	a := &Analyzer{}
	pairs := make(map[filePair]int)
	totals := make(map[string]int)
	a.recordCommit(nil, pairs, totals, make(map[string]bool))
	if len(pairs) != 0 || len(totals) != 0 {
		t.Error("empty files should produce no pairs or totals")
	}
}

func TestRecordCommit_SingleFile(t *testing.T) {
	a := &Analyzer{}
	pairs := make(map[filePair]int)
	totals := make(map[string]int)
	a.recordCommit([]string{"a.go"}, pairs, totals, make(map[string]bool))
	if len(pairs) != 0 {
		t.Errorf("single file should produce no pairs, got %d", len(pairs))
	}
	if totals["a.go"] != 1 {
		t.Errorf("total for a.go = %d, want 1", totals["a.go"])
	}
}

func TestRecordCommit_TwoFiles(t *testing.T) {
	a := &Analyzer{}
	pairs := make(map[filePair]int)
	totals := make(map[string]int)
	a.recordCommit([]string{"a.go", "b.go"}, pairs, totals, make(map[string]bool))
	if len(pairs) != 1 {
		t.Fatalf("expected 1 pair, got %d", len(pairs))
	}
	// Pair key must be ordered (a <= b).
	key := filePair{"a.go", "b.go"}
	if pairs[key] != 1 {
		t.Errorf("pair count = %d, want 1", pairs[key])
	}
}

func TestRecordCommit_Deduplication(t *testing.T) {
	// Same file listed twice in a commit should only count once.
	a := &Analyzer{}
	pairs := make(map[filePair]int)
	totals := make(map[string]int)
	a.recordCommit([]string{"a.go", "a.go", "b.go"}, pairs, totals, make(map[string]bool))
	if totals["a.go"] != 1 {
		t.Errorf("a.go total = %d, want 1 (dedup within commit)", totals["a.go"])
	}
	if len(pairs) != 1 {
		t.Errorf("expected 1 pair after dedup, got %d", len(pairs))
	}
}

func TestRecordCommit_OrderedKey(t *testing.T) {
	// Regardless of input order, the pair key must have a <= b.
	a := &Analyzer{}
	pairs := make(map[filePair]int)
	totals := make(map[string]int)
	a.recordCommit([]string{"z.go", "a.go"}, pairs, totals, make(map[string]bool))
	key := filePair{"a.go", "z.go"}
	if pairs[key] != 1 {
		t.Errorf("pair not found with ordered key %v", key)
	}
}

func TestRecordCommit_IgnoredPaths(t *testing.T) {
	// testdata/ files should be silently dropped.
	a := &Analyzer{}
	pairs := make(map[filePair]int)
	totals := make(map[string]int)
	a.recordCommit([]string{"testdata/foo.go", "internal/bar.go"}, pairs, totals, make(map[string]bool))
	if _, ok := totals["testdata/foo.go"]; ok {
		t.Error("testdata file should be ignored")
	}
	if len(pairs) != 0 {
		t.Error("pair with ignored file should not appear")
	}
}

func TestRecordCommit_ThreeFiles(t *testing.T) {
	a := &Analyzer{}
	pairs := make(map[filePair]int)
	totals := make(map[string]int)
	a.recordCommit([]string{"a.go", "b.go", "c.go"}, pairs, totals, make(map[string]bool))
	// 3 files → 3 pairs: (a,b), (a,c), (b,c)
	if len(pairs) != 3 {
		t.Errorf("expected 3 pairs for 3 files, got %d", len(pairs))
	}
}

// ─── ScanOptions defaults test ────────────────────────────────────────────────

func TestScanOptionsDefaults(t *testing.T) {
	// Verify that zero-value options get sensible defaults applied inside Scan.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := initGitRepo(t)
	writeAndCommit(t, dir, map[string]string{"a.go": "package main"}, "init")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewAnalyzer(dir, logger)

	// A zero ScanOptions should not panic and should use defaults.
	result, err := a.Scan(context.Background(), ScanOptions{})
	if err != nil {
		t.Fatalf("Scan() with zero opts error = %v", err)
	}
	if result == nil {
		t.Fatal("Scan() returned nil")
	}
}

// ─── Integration tests with real git repo ────────────────────────────────────

func TestScan_EmptyRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := initGitRepo(t)
	writeAndCommit(t, dir, map[string]string{"a.go": "package main"}, "init")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewAnalyzer(dir, logger)

	result, err := a.Scan(context.Background(), ScanOptions{WindowDays: 365, MinCoChanges: 2})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(result.HiddenCoupling) != 0 {
		t.Errorf("expected no hidden coupling, got %d pairs", len(result.HiddenCoupling))
	}
}

func TestScan_DetectsHiddenCoupling(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := initGitRepo(t)

	// api.go and storage.go co-change 3 times, no import edge between them.
	for i := 0; i < 3; i++ {
		writeAndCommit(t, dir, map[string]string{
			"api.go":     "package main\n// version " + string(rune('0'+i)),
			"storage.go": "package main\n// version " + string(rune('0'+i)),
		}, "update api and storage")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewAnalyzer(dir, logger)

	result, err := a.Scan(context.Background(), ScanOptions{
		WindowDays:     365,
		MinCorrelation: 0.3,
		MinCoChanges:   3,
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	found := findPair(result.HiddenCoupling, "api.go", "storage.go")
	if !found {
		t.Errorf("expected hidden coupling between api.go and storage.go, got: %v", result.HiddenCoupling)
	}
}

func TestScan_SkipsPairWithImportEdge(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := initGitRepo(t)

	// main.go imports the "util" package (Go module path fragment = "util").
	// util/helper.go is in the "util" package.
	// They co-change 3 times — should NOT be flagged as hidden coupling.
	for i := 0; i < 3; i++ {
		writeAndCommit(t, dir, map[string]string{
			"main.go":         `package main` + "\n" + `import "testmodule/util"` + "\n// v" + string(rune('0'+i)),
			"util/helper.go":  "package util\n// v" + string(rune('0'+i)),
		}, "update main and util")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewAnalyzer(dir, logger)

	result, err := a.Scan(context.Background(), ScanOptions{
		WindowDays:     365,
		MinCorrelation: 0.3,
		MinCoChanges:   3,
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if findPair(result.HiddenCoupling, "main.go", "util/helper.go") {
		t.Error("main.go → util/helper.go has a static import; should NOT be hidden coupling")
	}
}

func TestScan_MinCorrelationFilters(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := initGitRepo(t)

	// a.go and b.go co-change 3 out of 6 commits (50% correlation for a.go).
	for i := 0; i < 3; i++ {
		writeAndCommit(t, dir, map[string]string{
			"a.go": "package main\n// v" + string(rune('0'+i)),
			"b.go": "package main\n// v" + string(rune('0'+i)),
		}, "both")
	}
	for i := 0; i < 3; i++ {
		writeAndCommit(t, dir, map[string]string{
			"a.go": "package main\n// solo" + string(rune('0'+i)),
		}, "solo a")
	}
	// a.go: 6 commits, b.go: 3 commits, shared: 3 → correlation = 3/3 = 1.0

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewAnalyzer(dir, logger)

	// High threshold: should still find the pair (correlation is 1.0).
	result, err := a.Scan(context.Background(), ScanOptions{
		WindowDays:     365,
		MinCorrelation: 0.9,
		MinCoChanges:   3,
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if !findPair(result.HiddenCoupling, "a.go", "b.go") {
		t.Errorf("pair should appear at 0.9 threshold (actual correlation 1.0), got: %v", result.HiddenCoupling)
	}
}

func TestScan_MinCoChangesFilters(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := initGitRepo(t)

	// Only 2 shared commits — below the default minimum of 3.
	for i := 0; i < 2; i++ {
		writeAndCommit(t, dir, map[string]string{
			"x.go": "package main\n// v" + string(rune('0'+i)),
			"y.go": "package main\n// v" + string(rune('0'+i)),
		}, "update x and y")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewAnalyzer(dir, logger)

	result, err := a.Scan(context.Background(), ScanOptions{
		WindowDays:     365,
		MinCorrelation: 0.3,
		MinCoChanges:   3, // requires at least 3
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if findPair(result.HiddenCoupling, "x.go", "y.go") {
		t.Error("pair with only 2 co-changes should be filtered out by MinCoChanges=3")
	}

	// Lower the threshold: pair should now appear.
	result2, err := a.Scan(context.Background(), ScanOptions{
		WindowDays:     365,
		MinCorrelation: 0.3,
		MinCoChanges:   2,
	})
	if err != nil {
		t.Fatalf("Scan() with MinCoChanges=2 error = %v", err)
	}
	if !findPair(result2.HiddenCoupling, "x.go", "y.go") {
		t.Error("pair with 2 co-changes should appear at MinCoChanges=2")
	}
}

func TestScan_LimitRespected(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := initGitRepo(t)

	// Create 5 files that all co-change, unique content each commit.
	names := []string{"a.go", "b.go", "c.go", "d.go", "e.go"}
	for i := 0; i < 3; i++ {
		files := map[string]string{}
		for _, name := range names {
			files[name] = "package main\n// v" + string(rune('0'+i))
		}
		writeAndCommit(t, dir, files, "update all v"+string(rune('0'+i)))
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewAnalyzer(dir, logger)

	result, err := a.Scan(context.Background(), ScanOptions{
		WindowDays:     365,
		MinCorrelation: 0.3,
		MinCoChanges:   3,
		Limit:          3, // cap at 3
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(result.HiddenCoupling) > 3 {
		t.Errorf("expected ≤3 results, got %d", len(result.HiddenCoupling))
	}
}

func TestScan_FilterTestdataPaths(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := initGitRepo(t)

	// testdata files co-change with a real file; they should be invisible.
	for i := 0; i < 3; i++ {
		writeAndCommit(t, dir, map[string]string{
			"internal/service.go":    "package internal\n// v" + string(rune('0'+i)),
			"testdata/fixture.json":  `{"v":` + string(rune('0'+i)) + `}`,
		}, "update service and fixture")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewAnalyzer(dir, logger)

	result, err := a.Scan(context.Background(), ScanOptions{
		WindowDays:     365,
		MinCorrelation: 0.3,
		MinCoChanges:   3,
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	for _, p := range result.HiddenCoupling {
		if p.FileA == "testdata/fixture.json" || p.FileB == "testdata/fixture.json" {
			t.Errorf("testdata path should be filtered, but appeared in pair: %+v", p)
		}
	}
}

func TestScan_SummaryFields(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := initGitRepo(t)
	for i := 0; i < 3; i++ {
		writeAndCommit(t, dir, map[string]string{
			"a.go": "package main\n// " + string(rune('0'+i)),
			"b.go": "package main\n// " + string(rune('0'+i)),
		}, "commit")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewAnalyzer(dir, logger)

	result, err := a.Scan(context.Background(), ScanOptions{
		WindowDays:     365,
		MinCorrelation: 0.3,
		MinCoChanges:   3,
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	s := result.Summary
	if s.FilesObserved == 0 {
		t.Error("Summary.FilesObserved should be > 0")
	}
	if s.AnalysisFrom.IsZero() || s.AnalysisTo.IsZero() {
		t.Error("Summary analysis times should be set")
	}
	if s.AnalysisFrom.After(s.AnalysisTo) {
		t.Error("AnalysisFrom should be before AnalysisTo")
	}
	if s.HiddenPairsFound != len(result.HiddenCoupling) {
		t.Errorf("Summary.HiddenPairsFound = %d, but len(HiddenCoupling) = %d",
			s.HiddenPairsFound, len(result.HiddenCoupling))
	}
}

// ─── MaxCommitFiles tests ─────────────────────────────────────────────────────

func TestScan_SkipsCommitsAboveMaxFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := initGitRepo(t)

	// Large commit: 10 files — will be skipped when MaxCommitFiles=5.
	largeFiles := map[string]string{}
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go", "h.go", "i.go", "j.go"} {
		largeFiles[name] = "package main"
	}
	writeAndCommit(t, dir, largeFiles, "large commit")

	// Two small commits with a.go + b.go — they need 3 co-changes total to trigger coupling.
	for i := 0; i < 2; i++ {
		writeAndCommit(t, dir, map[string]string{
			"a.go": "package main // v" + string(rune('0'+i)),
			"b.go": "package main // v" + string(rune('0'+i)),
		}, "small commit")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewAnalyzer(dir, logger)

	// MaxCommitFiles=5: large commit (10 files) is skipped.
	// a.go+b.go only share 2 commits → below MinCoChanges=3 → no coupling.
	result, err := a.Scan(context.Background(), ScanOptions{
		WindowDays:     365,
		MinCorrelation: 0.3,
		MinCoChanges:   3,
		MaxCommitFiles: 5,
	})
	if err != nil {
		t.Fatalf("Scan() with MaxCommitFiles=5 error = %v", err)
	}
	if findPair(result.HiddenCoupling, "a.go", "b.go") {
		t.Error("large commit should be skipped; a.go+b.go should not reach MinCoChanges=3")
	}

	// MaxCommitFiles=0 (unlimited): large commit counts → 3 co-changes → coupling detected.
	result2, err := a.Scan(context.Background(), ScanOptions{
		WindowDays:     365,
		MinCorrelation: 0.3,
		MinCoChanges:   3,
		MaxCommitFiles: 0,
	})
	if err != nil {
		t.Fatalf("Scan() unlimited error = %v", err)
	}
	if !findPair(result2.HiddenCoupling, "a.go", "b.go") {
		t.Error("with MaxCommitFiles=0 (unlimited), a.go+b.go should reach MinCoChanges=3 via large commit")
	}
}

// ─── Early-prune regression tests ─────────────────────────────────────────────

// TestScan_EarlyPrunePreservesEligiblePairs ensures that pairs which exactly
// meet MinCoChanges are NOT incorrectly dropped by the early prune step.
func TestScan_EarlyPrunePreservesEligiblePairs(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := initGitRepo(t)

	// a.go + b.go co-change exactly 3 times (== MinCoChanges).
	for i := 0; i < 3; i++ {
		writeAndCommit(t, dir, map[string]string{
			"a.go": "package main // " + string(rune('0'+i)),
			"b.go": "package main // " + string(rune('0'+i)),
		}, "co-change")
	}
	// c.go appears alone — its pair with a.go/b.go has count=0, should be pruned.
	writeAndCommit(t, dir, map[string]string{"c.go": "package main"}, "solo c")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	a := NewAnalyzer(dir, logger)

	result, err := a.Scan(context.Background(), ScanOptions{
		WindowDays:     365,
		MinCorrelation: 0.3,
		MinCoChanges:   3,
	})
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if !findPair(result.HiddenCoupling, "a.go", "b.go") {
		t.Error("early prune must not drop pairs that exactly meet MinCoChanges")
	}
	// Pairs involving c.go should not appear (count < MinCoChanges).
	for _, p := range result.HiddenCoupling {
		if p.FileA == "c.go" || p.FileB == "c.go" {
			t.Errorf("pair involving c.go should be pruned (count < MinCoChanges): %+v", p)
		}
	}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	return dir
}

func writeAndCommit(t *testing.T, dir string, files map[string]string, msg string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		runGit(t, dir, "add", name)
	}
	runGit(t, dir, "commit", "-m", msg)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func findPair(pairs []HiddenCouplingPair, a, b string) bool {
	for _, p := range pairs {
		if (p.FileA == a && p.FileB == b) || (p.FileA == b && p.FileB == a) {
			return true
		}
	}
	return false
}
