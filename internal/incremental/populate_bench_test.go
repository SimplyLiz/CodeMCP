package incremental

// BenchmarkPopulateFromFullIndex compares the current path (load full SCIPIndex
// into memory, then process) against the streaming path (two-pass over the
// on-disk file, never materialising the full index).
//
// The key metric is B/op (bytes allocated per operation). For a 50k-doc repo
// the current path allocates ~6.5 GB; the streaming path stays near ~160 MB
// (the symbol→file map).
//
// Run isolated per size to avoid GC interference between scenarios:
//
//	go test -bench=BenchmarkPopulateFromFullIndex/small  -benchmem -count=6 -run=^$ ./internal/incremental/ > /tmp/pop_small.txt
//	go test -bench=BenchmarkPopulateFromFullIndex/medium -benchmem -count=6 -run=^$ ./internal/incremental/ > /tmp/pop_medium.txt
//	go test -bench=BenchmarkPopulateFromFullIndex/large  -benchmem -count=3 -run=^$ ./internal/incremental/ > /tmp/pop_large.txt
//	benchstat bench/baselines/populate_before.txt /tmp/pop_large.txt
//
// To capture the "before" baseline (current implementation):
//
//	cp /tmp/pop_small.txt  bench/baselines/populate_before.txt
//	cat /tmp/pop_medium.txt >> bench/baselines/populate_before.txt
//	cat /tmp/pop_large.txt  >> bench/baselines/populate_before.txt

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	scippb "github.com/sourcegraph/scip/bindings/go/scip"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"

	"github.com/SimplyLiz/CodeMCP/internal/storage"
)

// =============================================================================
// Benchmark
// =============================================================================

func BenchmarkPopulateFromFullIndex(b *testing.B) {
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
			// Write the synthetic SCIP file once; reuse across all iterations.
			dir := b.TempDir()
			indexPath := benchSCIPFile(b, dir, sc.nDocs, sc.nSymsPerDoc, sc.nOccsPerDoc)
			fi, _ := os.Stat(indexPath)
			b.ReportMetric(float64(fi.Size())/(1024*1024), "MB/index")
			b.ReportMetric(float64(sc.nDocs), "docs")

			b.Run("current", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					dbDir := b.TempDir()
					db, extractor, updater := benchSetup(b, dbDir, indexPath)
					b.StartTimer()
					if err := updater.PopulateFromFullIndex(extractor); err != nil {
						b.Fatalf("PopulateFromFullIndex: %v", err)
					}
					b.StopTimer()
					db.Close() //nolint:errcheck
				}
			})

			b.Run("streaming", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					dbDir := b.TempDir()
					db, extractor, updater := benchSetup(b, dbDir, indexPath)
					b.StartTimer()
					if err := updater.PopulateFromFullIndexStreaming(extractor); err != nil {
						b.Fatalf("PopulateFromFullIndexStreaming: %v", err)
					}
					b.StopTimer()
					db.Close() //nolint:errcheck
				}
			})
		})
	}
}

// =============================================================================
// Helpers
// =============================================================================

func benchSetup(b *testing.B, dbDir, indexPath string) (*storage.DB, *SCIPExtractor, *IndexUpdater) {
	b.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	db, err := storage.Open(dbDir, logger)
	if err != nil {
		b.Fatalf("storage.Open: %v", err)
	}
	extractor := NewSCIPExtractor(dbDir, indexPath, logger)
	store := NewStore(db, logger)
	updater := NewIndexUpdater(db, store, logger)
	return db, extractor, updater
}

// benchSCIPFile writes a synthetic SCIP wire-format file to dir and returns
// its path. Mirrors syntheticSCIPFile in the scip package.
func benchSCIPFile(b *testing.B, dir string, nDocs, nSymsPerDoc, nOccsPerDoc int) string {
	b.Helper()
	path := filepath.Join(dir, "index.scip")
	f, err := os.Create(path)
	if err != nil {
		b.Fatalf("create scip file: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			b.Fatalf("close scip file: %v", err)
		}
	}()

	meta := &scippb.Metadata{
		Version:     scippb.ProtocolVersion_UnspecifiedProtocolVersion,
		ProjectRoot: "file:///bench",
		ToolInfo:    &scippb.ToolInfo{Name: "bench", Version: "0.0.0"},
	}
	benchWriteProtoField(b, f, 1, meta)

	for d := 0; d < nDocs; d++ {
		doc := benchSyntheticDoc(d, nDocs, nSymsPerDoc, nOccsPerDoc)
		benchWriteProtoField(b, f, 2, doc)
	}
	return path
}

func benchSyntheticDoc(docIdx, totalDocs, nSyms, nOccs int) *scippb.Document {
	pkg := fmt.Sprintf("pkg%d", docIdx%20)
	doc := &scippb.Document{
		RelativePath: fmt.Sprintf("internal/%s/file%d.go", pkg, docIdx),
		Language:     "go",
	}
	for s := 0; s < nSyms; s++ {
		symID := fmt.Sprintf("scip-go gomod github.com/bench/repo 1.0 %s.Sym%d().", pkg, s)
		doc.Symbols = append(doc.Symbols, &scippb.SymbolInformation{
			Symbol:      symID,
			DisplayName: fmt.Sprintf("Sym%d", s),
			Kind:        scippb.SymbolInformation_Function,
		})
		doc.Occurrences = append(doc.Occurrences, &scippb.Occurrence{
			Range:       []int32{int32(s * 5), 0, int32(s*5 + 1), 0},
			Symbol:      symID,
			SymbolRoles: int32(scippb.SymbolRole_Definition),
		})
	}
	defined := len(doc.Occurrences)
	for i := defined; i < nOccs; i++ {
		refDocIdx := (docIdx + 1 + i%5) % totalDocs
		refPkg := fmt.Sprintf("pkg%d", refDocIdx%20)
		refSym := fmt.Sprintf("scip-go gomod github.com/bench/repo 1.0 %s.Sym%d().", refPkg, i%nSyms)
		doc.Occurrences = append(doc.Occurrences, &scippb.Occurrence{
			Range:       []int32{int32(i + nSyms*5), 4, int32(i + nSyms*5), int32(len(refSym))},
			Symbol:      refSym,
			SymbolRoles: 0,
		})
	}
	return doc
}

func benchWriteProtoField(b *testing.B, f *os.File, fieldNum uint32, msg proto.Message) {
	b.Helper()
	byt, err := proto.Marshal(msg)
	if err != nil {
		b.Fatalf("proto.Marshal: %v", err)
	}
	tag := (fieldNum << 3) | 2
	var buf [10]byte
	n := benchEncodeVarint(buf[:], uint64(tag))
	if _, err := f.Write(buf[:n]); err != nil {
		b.Fatalf("write tag: %v", err)
	}
	n = benchEncodeVarint(buf[:], uint64(len(byt)))
	if _, err := f.Write(buf[:n]); err != nil {
		b.Fatalf("write len: %v", err)
	}
	if _, err := f.Write(byt); err != nil {
		b.Fatalf("write body: %v", err)
	}
}

func benchEncodeVarint(buf []byte, v uint64) int {
	n := 0
	for v >= 0x80 {
		buf[n] = byte(v) | 0x80
		v >>= 7
		n++
	}
	buf[n] = byte(v)
	return n + 1
}

// Ensure protowire is imported (used by benchEncodeVarint's dependency context).
var _ = protowire.ConsumeTag
