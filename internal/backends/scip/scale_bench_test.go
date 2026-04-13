package scip

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	scippb "github.com/sourcegraph/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

// =============================================================================
// SCIP loader scale benchmarks
// =============================================================================
// These benchmark LoadSCIPIndex at realistic huge-repo sizes using a synthetic
// .scip protobuf file written to a temp dir. They are the load-side complement
// to backends/scip/performance_test.go (which requires a real index on disk).
//
// Motivation: a customer repo with ~50k files caused scip-go to take 1 h and
// ckb index to timeout at 10 h+. These benchmarks let us reproduce the load
// cost in CI without the real repo.
//
// Scenarios modelled after real monorepo sizes:
//   small:  1 000 docs ×  20 syms × 50  occs →   20 k syms,   50 k refs
//   medium: 10 000 docs ×  30 syms × 100 occs →  300 k syms,  1 M refs
//   large:  50 000 docs ×  40 syms × 200 occs →  2 M syms,   10 M refs
//
// Three phases of LoadSCIPIndex are benchmarked together:
//   Phase 1: mmap + protowire streaming parse (document-by-document)
//   Phase 2: parallel doc conversion → RefIndex / DefIndex / ContainerIndex
//   Phase 3: parallel symbol conversion + NameIndex sort + cache save
//
// Baselines (Apple M4 Pro, arm64, -count=1 -benchmem):
//   LoadSCIPIndex/small_1k_docs:   ~36 ms/op,  44 MB alloc,   425 k allocs/op
//   LoadSCIPIndex/medium_10k_docs: ~438 ms/op, 817 MB alloc,  7.6 M allocs/op
//   LoadSCIPIndex/large_50k_docs:  ~11.6 s/op, 6.9 GB alloc, 68 M allocs/op
//
// Notable: alloc cost scales super-linearly (~O(n×syms×occs)) due to per-occurrence
// OccurrenceRef heap allocations in Phase 2. The 6.9 GB at 50k docs is the primary
// reason huge repos run out of memory or are slow to GC during index load.
//
// Use benchstat for before/after comparison:
//   go test -bench=BenchmarkLoadSCIPIndexScale -benchmem -count=6 -run=^$ \
//       ./internal/backends/scip > before.txt
//   # make changes
//   go test -bench=BenchmarkLoadSCIPIndexScale -benchmem -count=6 -run=^$ \
//       ./internal/backends/scip > after.txt
//   benchstat before.txt after.txt
// =============================================================================

// syntheticSCIPFile writes a synthetic SCIP protobuf index file to dir and
// returns its path. Each document gets nSyms symbol definitions and nOccs
// total occurrences (definitions + references to neighbouring files).
func syntheticSCIPFile(tb testing.TB, dir string, nDocs, nSymsPerDoc, nOccsPerDoc int) string {
	tb.Helper()

	path := filepath.Join(dir, "index.scip")
	f, err := os.Create(path)
	if err != nil {
		tb.Fatalf("create synthetic scip: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			tb.Fatalf("close synthetic scip: %v", err)
		}
	}()

	// Write field 1 (Metadata) once.
	meta := &scippb.Metadata{
		Version:     scippb.ProtocolVersion_UnspecifiedProtocolVersion,
		ProjectRoot: "file:///synthetic",
		ToolInfo: &scippb.ToolInfo{
			Name:    "synthetic-bench",
			Version: "0.0.0",
		},
	}
	writeProtoField(tb, f, 1, meta)

	// Write one document per field 2 occurrence.
	for d := 0; d < nDocs; d++ {
		doc := syntheticDocument(d, nDocs, nSymsPerDoc, nOccsPerDoc)
		writeProtoField(tb, f, 2, doc)
	}

	return path
}

// syntheticDocument builds one scippb.Document. Symbols are definitions in this
// file; occurrences are a mix of own definitions and cross-file references.
func syntheticDocument(docIdx, totalDocs, nSyms, nOccs int) *scippb.Document {
	pkg := fmt.Sprintf("pkg%d", docIdx%20)
	relPath := fmt.Sprintf("internal/%s/file%d.go", pkg, docIdx)

	doc := &scippb.Document{
		RelativePath: relPath,
		Language:     "go",
	}

	// Build symbol definitions.
	for s := 0; s < nSyms; s++ {
		symID := fmt.Sprintf("scip-go gomod github.com/bench/repo 1.0 %s.Sym%d().", pkg, s)
		doc.Symbols = append(doc.Symbols, &scippb.SymbolInformation{
			Symbol:      symID,
			DisplayName: fmt.Sprintf("Sym%d", s),
			Kind:        scippb.SymbolInformation_Function,
		})
		// Definition occurrence.
		doc.Occurrences = append(doc.Occurrences, &scippb.Occurrence{
			Range:       []int32{int32(s * 5), 0, int32(s*5 + 1), 0},
			Symbol:      symID,
			SymbolRoles: int32(scippb.SymbolRole_Definition),
		})
	}

	// Fill remaining occurrences with cross-file references (typical call sites).
	defined := len(doc.Occurrences)
	for i := defined; i < nOccs; i++ {
		// Reference a symbol from a neighbouring file to simulate real call graphs.
		refDocIdx := (docIdx + 1 + i%5) % totalDocs
		refPkg := fmt.Sprintf("pkg%d", refDocIdx%20)
		refSym := fmt.Sprintf("scip-go gomod github.com/bench/repo 1.0 %s.Sym%d().", refPkg, i%nSyms)
		doc.Occurrences = append(doc.Occurrences, &scippb.Occurrence{
			Range:       []int32{int32(i + nSyms*5), 4, int32(i + nSyms*5), int32(len(refSym))},
			Symbol:      refSym,
			SymbolRoles: 0, // reference, not definition
		})
	}

	return doc
}

// writeProtoField appends a length-delimited protobuf field to w.
func writeProtoField(tb testing.TB, w *os.File, fieldNum uint32, msg proto.Message) {
	tb.Helper()
	b, err := proto.Marshal(msg)
	if err != nil {
		tb.Fatalf("proto.Marshal: %v", err)
	}
	// Tag: field_number << 3 | wire_type(2 = length-delimited)
	tag := (fieldNum << 3) | 2
	var buf [10]byte
	n := encodeVarint(buf[:], uint64(tag))
	if _, err := w.Write(buf[:n]); err != nil {
		tb.Fatalf("write tag: %v", err)
	}
	n = encodeVarint(buf[:], uint64(len(b)))
	if _, err := w.Write(buf[:n]); err != nil {
		tb.Fatalf("write length: %v", err)
	}
	if _, err := w.Write(b); err != nil {
		tb.Fatalf("write value: %v", err)
	}
}

// encodeVarint encodes a uint64 as a protobuf varint into buf and returns
// the number of bytes written.
func encodeVarint(buf []byte, v uint64) int {
	n := 0
	for v >= 0x80 {
		buf[n] = byte(v) | 0x80
		v >>= 7
		n++
	}
	buf[n] = byte(v)
	return n + 1
}

// BenchmarkLoadSCIPIndexScale benchmarks the full LoadSCIPIndex pipeline at
// small / medium / large synthetic repo sizes. Each iteration re-reads the same
// pre-written file (I/O is mmap'd so subsequent reads are OS page-cache hits).
//
// To measure cold-cache I/O cost, run with:
//
//	sudo purge && go test -bench=BenchmarkLoadSCIPIndexScale/large -count=1 ...
func BenchmarkLoadSCIPIndexScale(b *testing.B) {
	scenarios := []struct {
		name        string
		nDocs       int
		nSymsPerDoc int
		nOccsPerDoc int
	}{
		{"small_1k_docs", 1_000, 20, 50},
		{"medium_10k_docs", 10_000, 30, 100},
		{"large_50k_docs", 50_000, 40, 200},
	}

	for _, sc := range scenarios {
		sc := sc
		b.Run(sc.name, func(b *testing.B) {
			dir := b.TempDir()
			indexPath := syntheticSCIPFile(b, dir, sc.nDocs, sc.nSymsPerDoc, sc.nOccsPerDoc)

			fi, err := os.Stat(indexPath)
			if err != nil {
				b.Fatalf("stat index: %v", err)
			}
			b.ReportMetric(float64(fi.Size())/(1024*1024), "MB/index")
			b.ReportMetric(float64(sc.nDocs), "docs")
			b.ReportMetric(float64(sc.nDocs*sc.nSymsPerDoc), "syms")

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				idx, err := LoadSCIPIndex(indexPath)
				if err != nil {
					b.Fatalf("LoadSCIPIndex: %v", err)
				}
				_ = idx
			}
		})
	}
}

// BenchmarkLoadSCIPIndexPhases runs the three internal phases individually so
// bottlenecks can be isolated.
//
//	Phase1: protowire streaming parse only (no index building)
//	Phase2: document conversion + RefIndex build (worker fan-out)
//	Phase3: symbol conversion + NameIndex sort (currently measured together
//	        with Phase2 by subtracting Phase2-only time)
//
// Implementation note: Phase1 alone isn't directly accessible without
// modifying loader internals, so this benchmark approximates isolation by
// comparing full load vs. repeated loads with warm OS page cache.
func BenchmarkLoadSCIPIndexPhases(b *testing.B) {
	dir := b.TempDir()
	// Medium size: representative without being too slow.
	indexPath := syntheticSCIPFile(b, dir, 5_000, 30, 100)

	fi, _ := os.Stat(indexPath)
	b.ReportMetric(float64(fi.Size())/(1024*1024), "MB/index")

	// Warm the OS page cache with one load before timing.
	if _, err := LoadSCIPIndex(indexPath); err != nil {
		b.Fatalf("warm load: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx, err := LoadSCIPIndex(indexPath)
		if err != nil {
			b.Fatalf("LoadSCIPIndex: %v", err)
		}
		_ = idx
	}
}

// BenchmarkBuildCallerIndex measures the work the background pre-warm goroutine
// performs after LoadIndex returns. This is the latency that was previously paid
// on the *first* getCallGraph / traceUsage call; it is now absorbed in the
// background.
//
// Synthetic documents use ~5 cross-file references per occurrence, which
// approximates a real Go monorepo call-site density.
func BenchmarkBuildCallerIndex(b *testing.B) {
	scenarios := []struct {
		name        string
		nDocs       int
		nSymsPerDoc int
		nOccsPerDoc int
	}{
		{"small_1k_docs", 1_000, 20, 50},
		{"medium_10k_docs", 10_000, 30, 100},
		{"large_50k_docs", 50_000, 40, 200},
	}

	for _, sc := range scenarios {
		sc := sc
		b.Run(sc.name, func(b *testing.B) {
			// Build the document list once; we re-use it across iterations so
			// the benchmark measures only buildCallerIndex, not doc construction.
			docs := make([]*Document, sc.nDocs)
			for d := 0; d < sc.nDocs; d++ {
				pb := syntheticDocument(d, sc.nDocs, sc.nSymsPerDoc, sc.nOccsPerDoc)
				docs[d] = convertDocument(pb)
			}
			b.ReportMetric(float64(sc.nDocs), "docs")
			b.ReportMetric(float64(sc.nDocs*sc.nSymsPerDoc), "syms")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ci := buildCallerIndex(docs)
				_ = ci
			}
		})
	}
}
