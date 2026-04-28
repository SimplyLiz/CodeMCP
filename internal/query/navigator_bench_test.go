//go:build navigator

package query

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SimplyLiz/CodeMCP/internal/navigator"
)

// repoRootForBench returns the CKB source root, used as a real-world target.
// Falls back to the package directory when the env var isn't set.
func repoRootForBench(b *testing.B) string {
	b.Helper()
	if root := os.Getenv("CKB_BENCH_REPO"); root != "" {
		return root
	}
	// Walk up from the package dir to find go.mod
	dir, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			b.Fatal("could not find repo root (no go.mod found)")
		}
		dir = parent
	}
}

// =============================================================================
// buildExploreOverview file-enumeration: Cartographer vs filepath.Walk
// =============================================================================

// BenchmarkExploreFileCount_Walk counts files under the repo root using
// filepath.Walk — the fallback path in buildExploreOverview.
func BenchmarkExploreFileCount_Walk(b *testing.B) {
	root := repoRootForBench(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fileCount := 0
		langs := make(map[string]int)
		//nolint:errcheck
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				if skipExploreDirectory(info.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			fileCount++
			if lang := detectLanguage(path); lang != "" {
				langs[lang]++
			}
			return nil
		})
		_ = fileCount
		_ = langs
	}
}

// BenchmarkExploreFileCount_Cartographer counts files under the repo root
// using Cartographer's pre-built graph — the fast path in buildExploreOverview.
func BenchmarkExploreFileCount_Cartographer(b *testing.B) {
	root := repoRootForBench(b)
	// Warm up: MapProject does disk I/O on the first call; subsequent calls
	// hit an internal cache inside the Rust library.
	if _, err := navigator.MapProject(root); err != nil {
		b.Skipf("navigator.MapProject unavailable: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		graph, err := navigator.MapProject(root)
		if err != nil {
			b.Fatal(err)
		}
		fileCount := 0
		langs := make(map[string]int)
		for _, node := range graph.Nodes {
			fileCount++
			if node.Language != "" {
				langs[node.Language]++
			}
		}
		_ = fileCount
		_ = langs
	}
}

// =============================================================================
// listKeyConcepts SCIP-fallback: Cartographer vs filepath.WalkDir
// =============================================================================

// BenchmarkKeyConceptExtraction_Walk extracts key concepts from file names
// using filepath.WalkDir — the last-resort path in listKeyConcepts.
func BenchmarkKeyConceptExtraction_Walk(b *testing.B) {
	root := repoRootForBench(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conceptCounts := make(map[string]int)
		//nolint:errcheck
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				name := d.Name()
				if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			ext := filepath.Ext(path)
			if ext != ".go" && ext != ".ts" && ext != ".js" && ext != ".py" {
				return nil
			}
			name := strings.TrimSuffix(filepath.Base(path), ext)
			name = strings.TrimSuffix(name, "_test")
			name = strings.TrimSuffix(name, ".test")
			if c := extractConcept(name); c != "" {
				conceptCounts[c]++
			}
			return nil
		})
		_ = conceptCounts
	}
}

// BenchmarkKeyConceptExtraction_Cartographer extracts key concepts from
// Cartographer nodes — the fast path added to listKeyConcepts.
func BenchmarkKeyConceptExtraction_Cartographer(b *testing.B) {
	root := repoRootForBench(b)
	if _, err := navigator.MapProject(root); err != nil {
		b.Skipf("navigator.MapProject unavailable: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		graph, err := navigator.MapProject(root)
		if err != nil {
			b.Fatal(err)
		}
		conceptCounts := make(map[string]int)
		for _, node := range graph.Nodes {
			ext := filepath.Ext(node.Path)
			name := strings.TrimSuffix(filepath.Base(node.Path), ext)
			name = strings.TrimSuffix(name, "_test")
			name = strings.TrimSuffix(name, ".test")
			if c := extractConcept(name); c != "" {
				conceptCounts[c]++
			}
			if node.ModuleID != "" {
				if mc := extractConcept(filepath.Base(node.ModuleID)); mc != "" {
					conceptCounts[mc]++
				}
			}
		}
		_ = conceptCounts
	}
}
