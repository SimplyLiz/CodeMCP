//go:build cgo

package perf

import (
	"fmt"
	"testing"

	"github.com/SimplyLiz/CodeMCP/internal/complexity"
)

// =============================================================================
// structural perf benchmarks (CGO build only)
// =============================================================================
// These cover the per-file hot path inside AnalyzeStructural:
//
//   computeSeverity       — called once per loop call site
//   buildExplanation      — called once per loop call site (string formatting)
//   findEnclosingFunction — O(n functions) scan per call site
//   humanLoopType         — switch lookup per call site
//
// Baselines (Apple M4 Pro, arm64, -count=1 -benchmem):
//   computeSeverity:               ~0.26 ns/op,     0 B/op,  0 allocs/op
//   buildExplanation/non_ep:       ~208 ns/op,    432 B/op,  3 allocs/op  ← strings.Builder (was 352ns/6allocs)
//   buildExplanation/entrypoint:   ~188 ns/op,    416 B/op,  3 allocs/op  ← strings.Builder (was 350ns/7allocs)
//   findEnclosingFunction/1fn:    ~0.51 ns/op,     0 B/op,  0 allocs/op
//   findEnclosingFunction/10fns:    ~3.0 ns/op,    0 B/op,  0 allocs/op
//   findEnclosingFunction/50fns:    ~14 ns/op,     0 B/op,  0 allocs/op
//   humanLoopType:                  ~1.1 ns/op,    0 B/op,  0 allocs/op
//   CallSitePipeline/10sites:      ~1.5 µs/op,   3392 B/op,  14 allocs/op  ← strings.Builder (was 3.2µs/62allocs)
//   CallSitePipeline/100sites:      ~15 µs/op,  33920 B/op, 140 allocs/op  ← strings.Builder (was 33µs/620allocs)
//   CallSitePipeline/500sites:      ~75 µs/op, 169600 B/op, 700 allocs/op  ← strings.Builder (was 160µs/3100allocs)
//
// Notable: buildExplanation uses strings.Builder + strconv, halving allocs (6→3)
// and cutting latency ~40% vs fmt.Sprintf. Everything else is zero-alloc.
//
// Use benchstat for before/after comparison:
//   go test -tags cgo -bench=. -benchmem -count=6 -run=^$ ./internal/perf > before.txt
// =============================================================================

func BenchmarkComputeSeverity(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		computeSeverity(10+i%15, i%2 == 0)
	}
}

// BenchmarkBuildExplanation measures the string formatting cost per call site.
// This is the only allocating function in the per-site pipeline.
func BenchmarkBuildExplanation(b *testing.B) {
	b.Run("non_entrypoint", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buildExplanation(
				"internal/query/engine.go",
				"processResults",
				"db.QueryContext(ctx, query, args...)",
				"for/range",
				12,
				false,
			)
		}
	})

	b.Run("entrypoint", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buildExplanation(
				"cmd/ckb/serve.go",
				"handleRequest",
				"engine.SearchSymbols(ctx, opts)",
				"for",
				25,
				true,
			)
		}
	})
}

// BenchmarkFindEnclosingFunction measures the linear scan at increasing
// function counts. Typical Go files have 10–50 functions.
func BenchmarkFindEnclosingFunction(b *testing.B) {
	sizes := []int{1, 10, 50}

	for _, n := range sizes {
		fns := make([]complexity.ComplexityResult, n)
		for i := range fns {
			start := i*20 + 1
			fns[i] = complexity.ComplexityResult{
				Name:      fmt.Sprintf("func%d", i),
				StartLine: start,
				EndLine:   start + 18,
			}
		}
		// Target a line in the middle function.
		targetLine := (n/2)*20 + 10

		b.Run(fmt.Sprintf("%dfns", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				findEnclosingFunction(targetLine, fns)
			}
		})
	}
}

// BenchmarkHumanLoopType measures the switch lookup per call site.
func BenchmarkHumanLoopType(b *testing.B) {
	types := []string{
		"for_statement",
		"enhanced_for_statement",
		"while_statement",
		"for_in_statement",
		"loop_expression",
	}
	langs := []complexity.Language{
		complexity.LangGo,
		complexity.LangJava,
		complexity.LangPython,
		complexity.LangJavaScript,
		complexity.LangRust,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		humanLoopType(types[i%len(types)], langs[i%len(langs)])
	}
}

// BenchmarkCallSitePipeline simulates the per-call-site annotation work:
// computeSeverity + buildExplanation + findEnclosingFunction.
// This is what AnalyzeStructural does for every loop call site found.
func BenchmarkCallSitePipeline(b *testing.B) {
	fns := make([]complexity.ComplexityResult, 20)
	for i := range fns {
		start := i*25 + 1
		fns[i] = complexity.ComplexityResult{
			Name:      fmt.Sprintf("processItems%d", i),
			StartLine: start,
			EndLine:   start + 23,
		}
	}

	callSites := []struct {
		line     int
		callText string
		loopType string
		churn    int
		nearEP   bool
	}{
		{12, "db.QueryContext(ctx, query)", "for/range", 15, true},
		{34, "http.Get(url)", "for", 8, false},
		{67, "json.Unmarshal(data, &v)", "for/range", 22, false},
		{102, "os.ReadFile(path)", "for-each", 5, false},
		{145, "time.Sleep(backoff)", "while", 3, false},
	}

	sizes := []int{10, 100, 500}
	for _, n := range sizes {
		b.Run(fmt.Sprintf("%dsites", n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for iter := 0; iter < b.N; iter++ {
				for i := 0; i < n; i++ {
					cs := callSites[i%len(callSites)]
					fnName := findEnclosingFunction(cs.line, fns)
					sev := computeSeverity(cs.churn, cs.nearEP)
					exp := buildExplanation("internal/service.go", fnName, cs.callText, cs.loopType, cs.churn, cs.nearEP)
					_ = sev
					_ = exp
				}
			}
		})
	}
}
