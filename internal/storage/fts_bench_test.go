package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupBenchFTSDB(tb testing.TB) (*sql.DB, func()) {
	tb.Helper()
	tmpDir := tb.TempDir()
	dbPath := filepath.Join(tmpDir, "bench.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		tb.Fatalf("failed to open bench database: %v", err)
	}
	_, _ = db.Exec("PRAGMA journal_mode=WAL")
	return db, func() {
		_ = db.Close()
		os.RemoveAll(tmpDir)
	}
}

// BenchmarkBulkInsertVsFunc compares two caller patterns:
//
//   - BulkInsert: caller materialises the full []SymbolFTSRecord slice up front,
//     then hands it to BulkInsert — this is the old PopulateFTSFromSCIP code path.
//   - BulkInsertFunc: caller generates records in 10k chunks inside the callback,
//     never materialising the full slice — this is the new code path.
//
// The key metric is B/op: BulkInsertFunc should be ~(N/10k)× smaller in peak
// allocation for the caller-side slice.  At 500k symbols that is ~200 MB saved.
func BenchmarkBulkInsertVsFunc(b *testing.B) {
	sizes := []struct {
		name string
		n    int
	}{
		{"10k_syms", 10_000},
		{"100k_syms", 100_000},
		{"500k_syms", 500_000},
	}

	for _, sc := range sizes {
		sc := sc

		b.Run(sc.name+"/BulkInsert", func(b *testing.B) {
			// Simulate old caller: build full slice, then insert.
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				db, cleanup := setupBenchFTSDB(b)
				m := NewFTSManager(db, DefaultFTSConfig())
				if err := m.InitSchema(); err != nil {
					b.Fatal(err)
				}

				// This is what the old PopulateFTSFromSCIP did.
				records := make([]SymbolFTSRecord, sc.n)
				for j := range records {
					records[j] = SymbolFTSRecord{
						ID:       fmt.Sprintf("sym%d", j),
						Name:     fmt.Sprintf("Sym%d", j),
						Kind:     "function",
						FilePath: fmt.Sprintf("pkg%d/file.go", j%200),
						Language: "go",
					}
				}

				if err := m.BulkInsert(context.Background(), records); err != nil {
					b.Fatal(err)
				}
				cleanup()
			}
		})

		b.Run(sc.name+"/BulkInsertFunc", func(b *testing.B) {
			// Simulate new caller: generate records lazily in 10k chunks.
			const chunkSize = 10_000
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				db, cleanup := setupBenchFTSDB(b)
				m := NewFTSManager(db, DefaultFTSConfig())
				if err := m.InitSchema(); err != nil {
					b.Fatal(err)
				}

				err := m.BulkInsertFunc(context.Background(), func(flush func([]SymbolFTSRecord) error) error {
					chunk := make([]SymbolFTSRecord, 0, chunkSize)
					for j := 0; j < sc.n; j++ {
						chunk = append(chunk, SymbolFTSRecord{
							ID:       fmt.Sprintf("sym%d", j),
							Name:     fmt.Sprintf("Sym%d", j),
							Kind:     "function",
							FilePath: fmt.Sprintf("pkg%d/file.go", j%200),
							Language: "go",
						})
						if len(chunk) >= chunkSize {
							if err := flush(chunk); err != nil {
								return err
							}
							chunk = chunk[:0]
						}
					}
					if len(chunk) > 0 {
						return flush(chunk)
					}
					return nil
				})
				if err != nil {
					b.Fatal(err)
				}
				cleanup()
			}
		})
	}
}

// BenchmarkSymbolsForFileVsBatch compares N individual WHERE file_path = ?
// queries against one WHERE file_path IN (…) batch query. Models the
// SemanticSearchWithLIP path where LIP returns up to 20 file URIs.
func BenchmarkSymbolsForFileVsBatch(b *testing.B) {
	nFiles := []int{5, 10, 20}

	for _, nf := range nFiles {
		nf := nf

		db, cleanup := setupBenchFTSDB(b)
		b.Cleanup(cleanup)
		m := NewFTSManager(db, DefaultFTSConfig())
		if err := m.InitSchema(); err != nil {
			b.Fatal(err)
		}

		// 20 symbols per file.
		syms := make([]SymbolFTSRecord, 0, nf*20)
		filePaths := make([]string, nf)
		for f := 0; f < nf; f++ {
			fp := fmt.Sprintf("internal/pkg%d/file.go", f)
			filePaths[f] = fp
			for s := 0; s < 20; s++ {
				syms = append(syms, SymbolFTSRecord{
					ID:       fmt.Sprintf("sym_%d_%d", f, s),
					Name:     fmt.Sprintf("Sym%d", s),
					Kind:     "function",
					FilePath: fp,
					Language: "go",
				})
			}
		}
		if err := m.BulkInsert(context.Background(), syms); err != nil {
			b.Fatal(err)
		}

		b.Run(fmt.Sprintf("%d_files/N_queries", nf), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				for _, fp := range filePaths {
					rows, err := db.QueryContext(context.Background(),
						`SELECT id, name, kind, COALESCE(documentation,''), COALESCE(signature,''), file_path, COALESCE(language,'')
						 FROM symbols_fts_content WHERE file_path = ? LIMIT 60`,
						fp)
					if err != nil {
						b.Fatal(err)
					}
					for rows.Next() {
						var r FTSSearchResult
						_ = rows.Scan(&r.ID, &r.Name, &r.Kind, &r.Documentation, &r.Signature, &r.FilePath, &r.Language)
					}
					rows.Close() //nolint:errcheck
				}
			}
		})

		b.Run(fmt.Sprintf("%d_files/batch_IN", nf), func(b *testing.B) {
			placeholders := strings.Repeat("?,", nf)
			placeholders = placeholders[:len(placeholders)-1]
			args := make([]interface{}, nf)
			for i, fp := range filePaths {
				args[i] = fp
			}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				rows, err := db.QueryContext(context.Background(),
					`SELECT id, name, kind, COALESCE(documentation,''), COALESCE(signature,''), file_path, COALESCE(language,'')
					 FROM symbols_fts_content WHERE file_path IN (`+placeholders+`)`,
					args...)
				if err != nil {
					b.Fatal(err)
				}
				for rows.Next() {
					var r FTSSearchResult
					_ = rows.Scan(&r.ID, &r.Name, &r.Kind, &r.Documentation, &r.Signature, &r.FilePath, &r.Language)
				}
				rows.Close() //nolint:errcheck
			}
		})
	}
}
