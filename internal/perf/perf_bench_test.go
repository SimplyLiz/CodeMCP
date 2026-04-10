package perf

import (
	"fmt"
	"testing"
)

// =============================================================================
// perf package benchmarks
// =============================================================================
// These cover the hot paths of the two scan modes:
//
//   recordCommit         — O(files²) pair-building called once per commit
//   buildCoChangePairs   — full git-log parse loop (simulated, no git I/O)
//   importCouldReferTo   — per-candidate import string matching
//   shouldIgnore         — per-file path filter on every commit
//   correlationLevel     — per-pair level classification
//   ScanPipeline         — composite: pair-building + correlation + ignore filter
//
// Baselines (Apple M4 Pro, arm64, -count=1 -benchmem):
//   recordCommit/2files:            ~81 ns/op,      0 B/op,   0 allocs/op
//   recordCommit/5files:           ~512 ns/op,    744 B/op,   3 allocs/op
//   recordCommit/10files:         ~2.8 µs/op,   5816 B/op,  13 allocs/op
//   recordCommit/20files:          ~11 µs/op,  23544 B/op,  19 allocs/op
//   recordCommit/50files:          ~78 µs/op, 195624 B/op,  30 allocs/op
//   recordCommit_Reuse/10files:   ~1.1 µs/op,    456 B/op,   3 allocs/op  ← reused maps
//   recordCommit_WithIgnored:      ~625 ns/op,   1200 B/op,   6 allocs/op  ← 5 real + 5 ignored
//   importCouldReferTo/1import:    ~48 ns/op,      0 B/op,   0 allocs/op
//   importCouldReferTo/10imports: ~277 ns/op,      0 B/op,   0 allocs/op
//   importCouldReferTo/50imports: ~1.3 µs/op,      0 B/op,   0 allocs/op
//   importCouldReferTo_Hit:        ~34 ns/op,      0 B/op,   0 allocs/op  ← early exit
//   importCouldReferTo_Miss:      ~513 ns/op,      0 B/op,   0 allocs/op  ← full scan
//   shouldIgnore/ignored:         ~1.8 ns/op,      0 B/op,   0 allocs/op
//   shouldIgnore/not_ignored:     ~7.1 ns/op,      0 B/op,   0 allocs/op
//   correlationLevel:            ~0.26 ns/op,      0 B/op,   0 allocs/op
//   CoChangePipeline/100c_5f:     ~39 µs/op,   55888 B/op,  12 allocs/op
//   CoChangePipeline/500c_10f:   ~718 µs/op,  634348 B/op, 1526 allocs/op
//   CoChangePipeline/1kc_20f:    ~5.4 ms/op, 2522031 B/op, 3072 allocs/op  ← seen-map allocs per commit
//   correlationFilter/~20kpairs: ~372 µs/op,      0 B/op,   0 allocs/op
//
// Notable: recordCommit is O(files²) per commit — the dominant cost on repos
// with large commits (fmt sweeps, mass renames). The per-commit seen-map
// allocation drives alloc counts in CoChangePipeline (1k × make(map) ≈ 3k allocs).
// The ignore filter cuts pairing work by dropping testdata/vendor before O(n²).
//
// Use benchstat for before/after comparison:
//   go test -bench=. -benchmem -count=6 -run=^$ ./internal/perf > before.txt
//   # make changes
//   go test -bench=. -benchmem -count=6 -run=^$ ./internal/perf > after.txt
//   benchstat before.txt after.txt
// =============================================================================

// BenchmarkRecordCommit measures the O(files²) pair-building cost at increasing
// file counts. Large commits (e.g. formatting sweeps, mass renames) are the
// dominant cost in buildCoChangePairs.
func BenchmarkRecordCommit(b *testing.B) {
	a := &Analyzer{}
	sizes := []int{2, 5, 10, 20, 50}

	for _, n := range sizes {
		files := make([]string, n)
		for i := range files {
			files[i] = fmt.Sprintf("internal/pkg%d/file%d.go", i%10, i)
		}

		b.Run(fmt.Sprintf("%dfiles", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pairs := make(map[filePair]int)
				totals := make(map[string]int)
				a.recordCommit(files, pairs, totals)
			}
		})
	}
}

// BenchmarkRecordCommit_Reuse measures recordCommit when the maps are reused
// across calls (as in buildCoChangePairs). Avoids measuring map allocation.
func BenchmarkRecordCommit_Reuse(b *testing.B) {
	a := &Analyzer{}
	files := make([]string, 10)
	for i := range files {
		files[i] = fmt.Sprintf("internal/pkg%d/file.go", i)
	}
	pairs := make(map[filePair]int, 64)
	totals := make(map[string]int, 64)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.recordCommit(files, pairs, totals)
	}
}

// BenchmarkRecordCommit_WithIgnored measures the cost when a fraction of files
// are in testdata/ or vendor/ — the ignore filter should cut pairing work.
func BenchmarkRecordCommit_WithIgnored(b *testing.B) {
	a := &Analyzer{}
	// 5 real files + 5 ignored files per commit.
	files := []string{
		"internal/api/handler.go",
		"internal/query/engine.go",
		"internal/mcp/tools.go",
		"internal/storage/db.go",
		"internal/audit/analyzer.go",
		"testdata/fixtures/go/expected/symbol.json",
		"testdata/fixtures/go/expected/refs.json",
		"vendor/github.com/spf13/cobra/command.go",
		"vendor/github.com/spf13/cobra/args.go",
		"node_modules/lodash/lodash.js",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pairs := make(map[filePair]int)
		totals := make(map[string]int)
		a.recordCommit(files, pairs, totals)
	}
}

// BenchmarkCoChangePipelineSimulated benchmarks the full pair-building inner
// loop without git I/O by calling recordCommit in a loop over synthetic commit
// batches. This is the dominant CPU cost during a Scan call.
//
// Sizes represent realistic repo histories:
//   - 100 commits × 5 files  ≈ focused feature branch
//   - 500 commits × 10 files ≈ mid-size service, 6 months history
//   - 1k  commits × 20 files ≈ busy monorepo module, 1 year history
func BenchmarkCoChangePipelineSimulated(b *testing.B) {
	a := &Analyzer{}

	scenarios := []struct {
		name    string
		commits int
		files   int
	}{
		{"100commits_5files", 100, 5},
		{"500commits_10files", 500, 10},
		{"1kcommits_20files", 1_000, 20},
	}

	for _, sc := range scenarios {
		// Pre-build commit batches so the benchmark doesn't measure string alloc.
		batches := make([][]string, sc.commits)
		for c := range batches {
			batch := make([]string, sc.files)
			for f := range batch {
				batch[f] = fmt.Sprintf("internal/pkg%d/file%d.go", f%8, (c+f)%15)
			}
			batches[c] = batch
		}

		b.Run(sc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pairs := make(map[filePair]int, sc.commits*sc.files)
				totals := make(map[string]int, sc.files*2)
				for _, batch := range batches {
					a.recordCommit(batch, pairs, totals)
				}
			}
		})
	}
}

// BenchmarkImportCouldReferTo measures per-candidate import matching.
// Called once per candidate pair (after correlation filtering) — typically
// a small fraction of total pairs, but the miss path scans all imports.
func BenchmarkImportCouldReferTo(b *testing.B) {
	targetFile := "internal/jobs/scheduler.go"

	sizes := []struct {
		name    string
		imports []string
	}{
		{
			"1import",
			[]string{"github.com/SimplyLiz/CodeMCP/internal/query"},
		},
		{
			"10imports",
			[]string{
				"context", "fmt", "os", "time", "sync",
				"github.com/SimplyLiz/CodeMCP/internal/config",
				"github.com/SimplyLiz/CodeMCP/internal/storage",
				"github.com/SimplyLiz/CodeMCP/internal/errors",
				"github.com/spf13/cobra",
				"go.opentelemetry.io/otel",
			},
		},
		{
			"50imports",
			func() []string {
				imps := make([]string, 50)
				for i := range imps {
					imps[i] = fmt.Sprintf("github.com/SimplyLiz/CodeMCP/internal/pkg%d", i)
				}
				return imps
			}(),
		},
	}

	for _, sz := range sizes {
		b.Run(sz.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				importCouldReferTo(sz.imports, targetFile)
			}
		})
	}
}

// BenchmarkImportCouldReferTo_Hit measures the early-exit path where the first
// or second import matches, avoiding a full scan.
func BenchmarkImportCouldReferTo_Hit(b *testing.B) {
	// Match is at position 0 — best case.
	imports := []string{
		"github.com/SimplyLiz/CodeMCP/internal/jobs",
		"github.com/SimplyLiz/CodeMCP/internal/query",
		"github.com/SimplyLiz/CodeMCP/internal/storage",
	}
	target := "internal/jobs/scheduler.go"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		importCouldReferTo(imports, target)
	}
}

// BenchmarkImportCouldReferTo_Miss measures the full-scan path where no import
// matches. This is the common case — most co-changing pairs are unrelated.
func BenchmarkImportCouldReferTo_Miss(b *testing.B) {
	imports := make([]string, 20)
	for i := range imports {
		imports[i] = fmt.Sprintf("github.com/SimplyLiz/CodeMCP/internal/unrelated%d", i)
	}
	target := "internal/jobs/scheduler.go"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		importCouldReferTo(imports, target)
	}
}

// BenchmarkShouldIgnore measures the path-prefix filter called on every file
// in every commit. Should be branch-predictor-friendly (most files don't match).
func BenchmarkShouldIgnore(b *testing.B) {
	cases := []struct {
		name string
		path string
	}{
		{"ignored_testdata", "testdata/fixtures/go/expected/symbol.json"},
		{"ignored_vendor", "vendor/github.com/spf13/cobra/command.go"},
		{"not_ignored_internal", "internal/query/engine.go"},
		{"not_ignored_cmd", "cmd/ckb/main.go"},
		{"not_ignored_testfile", "internal/perf/analyzer_test.go"},
	}

	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				shouldIgnore(c.path)
			}
		})
	}
}

// BenchmarkCorrelationLevel measures the hot classification call made once per
// surviving candidate pair.
func BenchmarkCorrelationLevel(b *testing.B) {
	values := []float64{1.0, 0.9, 0.8, 0.7, 0.5, 0.4, 0.3, 0.1}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		correlationLevel(values[i%len(values)])
	}
}

// BenchmarkCorrelationFilter measures the candidate filtering pass: iterate pairs,
// apply minCorrelation + minCoChanges, compute correlation. This is the in-memory
// work between buildCoChangePairs and the import-edge check.
func BenchmarkCorrelationFilter(b *testing.B) {
	// Build a realistic pair map: 200 files × 200 files / 2 = ~20k pairs.
	const nFiles = 200
	pairCounts := make(map[filePair]int, nFiles*nFiles/2)
	fileTotals := make(map[string]int, nFiles)

	for i := 0; i < nFiles; i++ {
		f := fmt.Sprintf("internal/pkg%d/file%d.go", i%10, i)
		fileTotals[f] = 5 + (i % 20)
		for j := i + 1; j < nFiles; j++ {
			g := fmt.Sprintf("internal/pkg%d/file%d.go", j%10, j)
			if i < j {
				pairCounts[filePair{f, g}] = 1 + (i+j)%8
			}
		}
	}

	const minCorrelation = 0.3
	const minCoChanges = 3

	b.ReportAllocs()
	b.ResetTimer()
	for iter := 0; iter < b.N; iter++ {
		var kept int
		for pair, count := range pairCounts {
			if count < minCoChanges {
				continue
			}
			totalA := fileTotals[pair.a]
			totalB := fileTotals[pair.b]
			minTotal := totalA
			if totalB < minTotal {
				minTotal = totalB
			}
			if minTotal == 0 {
				continue
			}
			corr := float64(count) / float64(minTotal)
			if corr >= minCorrelation {
				kept++
			}
		}
		_ = kept
	}
}
