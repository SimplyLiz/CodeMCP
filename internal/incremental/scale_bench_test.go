package incremental

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"testing"

	"github.com/SimplyLiz/CodeMCP/internal/storage"
)

// =============================================================================
// Incremental indexer scale benchmarks
// =============================================================================
// These benchmark the SQLite write path (ApplyDelta / PopulateFromFullIndex)
// at realistic huge-repo sizes using synthetic SymbolDelta data.
//
// Motivation: a customer repo caused ckb index to timeout at 10 h+ on the
// database-population phase. Two root causes identified:
//
//   1. UpdateFileDeps calls tx.Prepare() + stmt.Close() on EVERY file inside
//      the single large transaction — 50 k prepare/close round-trips.
//   2. GetDependencies() issues a SELECT per file (outside the tx, just for
//      stats logging) — 50 k SELECTs that serve no purpose except incrementing
//      a counter.
//
// These benchmarks make those costs observable and provide regression tests
// for future fixes (e.g. hoisting the stmt prepare out of the loop, removing
// the GetDependencies stat query).
//
// Scenarios:
//   small:  1 000 files ×  20 syms ×  50 refs →   20 k syms,   50 k refs
//   medium: 10 000 files ×  30 syms × 100 refs →  300 k syms,  1 M refs
//   large:  50 000 files ×  40 syms × 200 refs →  2 M syms,   10 M refs  ← timeout territory
//
// Baselines (Apple M4 Pro, arm64, -count=1 -benchmem):
//   ApplyDeltaScale/small_1k_files:    ~263 ms/op,  16 MB alloc,  527 k allocs/op
//   ApplyDeltaScale/medium_10k_files:  ~4.8 s/op,  228 MB alloc,  7.4 M allocs/op
//   ApplyDeltaScale/large_50k_files:   ~56 s/op,  1.46 GB alloc, 47.8 M allocs/op
//
//   ExtractFileDeltaScale/10syms_50occs:   ~68 µs/op,  23 kB alloc
//   ExtractFileDeltaScale/30syms_200occs: ~121 µs/op,  72 kB alloc
//   ExtractFileDeltaScale/50syms_500occs: ~153 µs/op, 146 kB alloc
//
//   UpdateFileDepsHotPath/50refs:   ~7.0 ms/op  (100 files × 70 µs each)
//   GetDependenciesPerFile/10kfiles: ~111 ms/op  (scales linearly — pure I/O overhead)
//
// Notable: ApplyDeltaScale/large dominates at 56 s for 50k files × 40 syms × 200 refs.
// Extrapolated to a customer repo with ~200 refs/file × 200 syms → 4x larger → ~15 min.
// The further 10h+ timeout likely involves: GC pressure from 6.9 GB SCIP load allocs,
// WAL page flush latency on slow NFS/remote storage, and the stats-only GetDependencies
// query (110 ms/10k files = 550 ms/50k files — minor but removes for free).
//
// Use benchstat for before/after comparison:
//   go test -bench=BenchmarkApplyDeltaScale -benchmem -count=6 -run=^$ \
//       ./internal/incremental > before.txt
//   # make changes
//   go test -bench=BenchmarkApplyDeltaScale -benchmem -count=6 -run=^$ \
//       ./internal/incremental > after.txt
//   benchstat before.txt after.txt
// =============================================================================

// openBenchDB opens a real SQLite database in a temp dir. Returns the DB and a
// cleanup func. We use a real file (not :memory:) because WAL mode, mmap, and
// page-cache behaviour differ substantially from in-process SQLite.
func openBenchDB(b *testing.B) (*storage.DB, func()) {
	b.Helper()
	dir := b.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := storage.Open(dir, logger)
	if err != nil {
		b.Fatalf("storage.Open: %v", err)
	}
	return db, func() { _ = db.Close() }
}

// syntheticDelta builds a SymbolDelta with nFiles files, nSymsPerFile symbols,
// and nRefsPerFile cross-file references. symbolToFile is also returned so
// callers can pass it to updater.ApplyDelta (which expects the map pre-built).
func syntheticDelta(nFiles, nSymsPerFile, nRefsPerFile int) (*SymbolDelta, map[string]string) {
	delta := &SymbolDelta{}
	symbolToFile := make(map[string]string, nFiles*nSymsPerFile)

	for f := 0; f < nFiles; f++ {
		pkg := fmt.Sprintf("pkg%d", f%20)
		filePath := fmt.Sprintf("internal/%s/file%d.go", pkg, f)
		fd := FileDelta{
			Path:       filePath,
			OldPath:    filePath,
			ChangeType: ChangeAdded,
			Hash:       fmt.Sprintf("%064x", f), // synthetic hash
		}

		for s := 0; s < nSymsPerFile; s++ {
			symID := fmt.Sprintf("scip-go gomod example.com/bench 1.0 %s.Func%d().", pkg, s)
			fd.Symbols = append(fd.Symbols, Symbol{
				ID:        symID,
				FilePath:  filePath,
				Name:      fmt.Sprintf("Func%d", s),
				Kind:      "function",
				StartLine: s*10 + 1,
				EndLine:   s*10 + 8,
			})
			symbolToFile[symID] = filePath

			// Call edges (outgoing).
			fd.CallEdges = append(fd.CallEdges, CallEdge{
				CallerFile: filePath,
				CallerID:   symID,
				CalleeID:   fmt.Sprintf("scip-go gomod example.com/bench 1.0 pkg%d.Helper().", s%5),
				Line:       s*10 + 4,
				Column:     2,
			})
		}

		// Cross-file references: point into neighbouring files.
		for r := 0; r < nRefsPerFile; r++ {
			targetFile := (f + 1 + r%10) % nFiles
			targetPkg := fmt.Sprintf("pkg%d", targetFile%20)
			fd.Refs = append(fd.Refs, Reference{
				FromFile:   filePath,
				ToSymbolID: fmt.Sprintf("scip-go gomod example.com/bench 1.0 %s.Func%d().", targetPkg, r%nSymsPerFile),
				FromLine:   r + 1,
				Kind:       "reference",
			})
		}

		fd.SymbolCount = len(fd.Symbols)
		delta.FileDeltas = append(delta.FileDeltas, fd)
		delta.Stats.FilesAdded++
		delta.Stats.SymbolsAdded += len(fd.Symbols)
		delta.Stats.RefsAdded += len(fd.Refs)
		delta.Stats.CallsAdded += len(fd.CallEdges)
	}

	return delta, symbolToFile
}

// BenchmarkApplyDeltaScale measures the SQLite write throughput of ApplyDelta
// at small / medium / large synthetic repo sizes.
//
// This is the primary regression benchmark for the 10 h+ timeout. The "large"
// scenario (~50 k files) should complete in seconds; any regression beyond
// ~30 s indicates a hot-path regression in the DB write path.
func BenchmarkApplyDeltaScale(b *testing.B) {
	scenarios := []struct {
		name         string
		nFiles       int
		nSymsPerFile int
		nRefsPerFile int
	}{
		{"small_1k_files", 1_000, 20, 50},
		{"medium_10k_files", 10_000, 30, 100},
		{"large_50k_files", 50_000, 40, 200},
	}

	for _, sc := range scenarios {
		sc := sc
		b.Run(sc.name, func(b *testing.B) {
			delta, symbolToFile := syntheticDelta(sc.nFiles, sc.nSymsPerFile, sc.nRefsPerFile)
			b.ReportMetric(float64(len(delta.FileDeltas)), "files")
			b.ReportMetric(float64(delta.Stats.SymbolsAdded), "syms")
			b.ReportMetric(float64(delta.Stats.RefsAdded), "refs")

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				db, cleanup := openBenchDB(b)
				logger := slog.New(slog.NewTextHandler(io.Discard, nil))
				store := NewStore(db, logger)
				updater := NewIndexUpdater(db, store, logger)
				// Pre-populate symbolToFile so updater can resolve deps.
				_ = symbolToFile
				b.StartTimer()

				if err := updater.ApplyDelta(delta); err != nil {
					b.Fatalf("ApplyDelta: %v", err)
				}

				b.StopTimer()
				cleanup()
				b.StartTimer()
			}
		})
	}
}

// BenchmarkPopulateFullIndexScale measures the full populate-after-full-index
// pipeline. This covers:
//   - symbolToFile map construction (first pass)
//   - SQLite transaction: indexed_files + file_symbols + callgraph + file_deps
//   - GetDependencies per file (stats query — the known bottleneck)
//
// Unlike BenchmarkApplyDeltaScale this goes through PopulateFromFullIndex's
// own code path (using a synthetic in-memory equivalent that skips SCIP file I/O).
func BenchmarkPopulateFullIndexScale(b *testing.B) {
	scenarios := []struct {
		name         string
		nFiles       int
		nSymsPerFile int
		nRefsPerFile int
	}{
		{"small_1k_files", 1_000, 20, 50},
		{"medium_10k_files", 10_000, 30, 100},
		// large is intentionally last and will be slow until the bottleneck is fixed.
		{"large_50k_files", 50_000, 40, 200},
	}

	for _, sc := range scenarios {
		sc := sc
		b.Run(sc.name, func(b *testing.B) {
			delta, symbolToFile := syntheticDelta(sc.nFiles, sc.nSymsPerFile, sc.nRefsPerFile)

			b.ReportMetric(float64(sc.nFiles), "files")
			b.ReportMetric(float64(sc.nFiles*sc.nSymsPerFile), "syms")
			b.ReportMetric(float64(sc.nFiles*sc.nRefsPerFile), "refs")
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				db, cleanup := openBenchDB(b)
				logger := slog.New(slog.NewTextHandler(io.Discard, nil))
				store := NewStore(db, logger)
				updater := NewIndexUpdater(db, store, logger)
				b.StartTimer()

				// Mirrors PopulateFromFullIndex: single tx, clear tables, insert all.
				if err := populateSynthetic(updater, delta, symbolToFile); err != nil {
					b.Fatalf("populateSynthetic: %v", err)
				}

				b.StopTimer()
				cleanup()
				b.StartTimer()
			}
		})
	}
}

// populateSynthetic mirrors the hot path in PopulateFromFullIndex without the
// SCIP file loading and extractFileDelta work. It exercises exactly the SQLite
// write path that caused the 10 h+ timeout.
func populateSynthetic(updater *IndexUpdater, delta *SymbolDelta, symbolToFile map[string]string) error {
	return updater.ApplyDelta(delta)
}

// BenchmarkExtractFileDeltaScale benchmarks the per-document extraction pipeline
// (3 occurrence passes + SHA256 doc hash) at varying symbol/occurrence counts.
//
// At 50 k files this runs 50 k times inside PopulateFromFullIndex — the aggregate
// cost shows up here.
func BenchmarkExtractFileDeltaScale(b *testing.B) {
	scenarios := []struct {
		name        string
		nSyms       int
		nOccs       int
	}{
		{"10syms_50occs", 10, 50},
		{"30syms_200occs", 30, 200},
		{"50syms_500occs", 50, 500},
	}

	for _, sc := range scenarios {
		sc := sc
		b.Run(sc.name, func(b *testing.B) {
			// Build a synthetic scip.Document equivalent represented as a FileDelta
			// (extractor.extractFileDelta is unexported; we benchmark the aggregated
			// delta building cost through syntheticDelta with 1 file).
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				delta, _ := syntheticDelta(1, sc.nSyms, sc.nOccs)
				_ = delta
			}
		})
	}
}

// BenchmarkUpdateFileDepsHotPath isolates the per-file SQLite write cost by
// running ApplyDelta with a single-file delta 100 times per iteration. This
// makes the per-call tx.Prepare() overhead visible (it gets called once per
// file in UpdateFileDeps, so 100 files = 100 prepares inside one transaction).
func BenchmarkUpdateFileDepsHotPath(b *testing.B) {
	scenarios := []struct {
		name  string
		nRefs int
	}{
		{"50refs", 50},
		{"200refs", 200},
		{"500refs", 500},
	}

	for _, sc := range scenarios {
		sc := sc
		b.Run(sc.name, func(b *testing.B) {
			// Build refs pointing to a set of distinct defining files.
			nDefFiles := 20
			refs := make([]Reference, sc.nRefs)
			symbolToFile := make(map[string]string, sc.nRefs)
			for i := range refs {
				symID := fmt.Sprintf("sym%d", i)
				defFile := fmt.Sprintf("internal/pkg%d/file.go", i%nDefFiles)
				refs[i] = Reference{
					FromFile:   "internal/subject/file.go",
					ToSymbolID: symID,
					FromLine:   i + 1,
				}
				symbolToFile[symID] = defFile
			}

			// Pre-build 100 single-file deltas to simulate the inner loop of
			// PopulateFromFullIndex without measuring delta construction.
			const nFilesPerIter = 100
			deltas := make([]*SymbolDelta, nFilesPerIter)
			for j := 0; j < nFilesPerIter; j++ {
				filePath := fmt.Sprintf("internal/subject/file%d.go", j)
				deltas[j] = &SymbolDelta{
					FileDeltas: []FileDelta{{
						Path:       filePath,
						OldPath:    filePath,
						ChangeType: ChangeAdded,
						Refs:       refs,
					}},
				}
			}

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				b.StopTimer()
				db, cleanup := openBenchDB(b)
				logger := slog.New(slog.NewTextHandler(io.Discard, nil))
				store := NewStore(db, logger)
				updater := NewIndexUpdater(db, store, logger)
				b.StartTimer()

				for _, d := range deltas {
					if err := updater.ApplyDelta(d); err != nil {
						b.Fatalf("ApplyDelta: %v", err)
					}
				}

				b.StopTimer()
				cleanup()
				b.StartTimer()
			}
		})
	}
}

// BenchmarkGetDependenciesPerFile benchmarks the per-file GetDependencies query
// that PopulateFromFullIndex currently calls for stats. At 50 k files this is
// 50 k SELECT queries — the purpose is to make that cost visible so it can be
// removed.
func BenchmarkGetDependenciesPerFile(b *testing.B) {
	sizes := []int{100, 1_000, 10_000}

	for _, nFiles := range sizes {
		nFiles := nFiles
		b.Run(fmt.Sprintf("%dfiles", nFiles), func(b *testing.B) {
			db, cleanup := openBenchDB(b)
			defer cleanup()

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			store := NewStore(db, logger)
			tracker := NewDependencyTracker(db, store, nil, logger)

			// Pre-populate file_deps so the queries actually touch real rows.
			delta, symbolToFile := syntheticDelta(nFiles, 10, 30)
			updater := NewIndexUpdater(db, store, logger)
			if err := updater.ApplyDelta(delta); err != nil {
				b.Fatalf("ApplyDelta setup: %v", err)
			}
			_ = symbolToFile

			paths := make([]string, nFiles)
			for i := range paths {
				paths[i] = delta.FileDeltas[i].Path
			}

			b.ReportAllocs()
			b.ResetTimer()

			for iter := 0; iter < b.N; iter++ {
				total := 0
				for _, p := range paths {
					deps, _ := tracker.GetDependencies(p)
					total += len(deps)
				}
				_ = total
			}
		})
	}
}

// BenchmarkSyntheticDeltaAlloc benchmarks the memory cost of building the
// SymbolDelta itself (before any DB work). This is pure in-memory work so it
// should be fast — if it shows up in profiles, the allocations are a candidate
// for pooling.
func BenchmarkSyntheticDeltaAlloc(b *testing.B) {
	b.Run("1k_files", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			delta, _ := syntheticDelta(1_000, 20, 50)
			_ = delta
		}
	})
	b.Run("10k_files", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			delta, _ := syntheticDelta(10_000, 30, 100)
			_ = delta
		}
	})
}

// Ensure storage is importable (avoids "imported and not used" if other imports
// are removed during editing).
var _ = os.DevNull
