//go:build navigator

// ckb-bench compares navigator-backed file discovery against filepath.Walk.
// Run as a standalone binary to avoid the CGo fork-safety issue that affects
// go test binaries when the Rust library spawns git subprocesses.
//
//	go run -tags navigator ./cmd/ckb-bench [repo-root]
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/navigator"
)

func detectLanguage(path string) string {
	switch filepath.Ext(path) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	default:
		return ""
	}
}

func skipDir(name string) bool {
	return name == "vendor" || name == "node_modules" || name == ".git" ||
		name == ".ckb" || name == "target" || name == "__pycache__"
}

// bench runs fn N times and returns the mean duration.
func bench(name string, n int, fn func()) time.Duration {
	// warmup
	fn()

	start := time.Now()
	for i := 0; i < n; i++ {
		fn()
	}
	mean := time.Since(start) / time.Duration(n)
	fmt.Printf("%-42s %6d iters   mean %v\n", name, n, mean)
	return mean
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	root = abs

	fmt.Printf("Benchmark target: %s\n", root)
	fmt.Printf("GOMAXPROCS: %d\n\n", runtime.GOMAXPROCS(0))

	const n = 10

	// -------------------------------------------------------------------------
	// 1. buildExploreOverview: directory file-count + language breakdown
	// -------------------------------------------------------------------------
	fmt.Println("=== buildExploreOverview: file-count + language breakdown ===")

	walkFileCount := func() {
		fileCount := 0
		langs := make(map[string]int)
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				if skipDir(info.Name()) {
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

	navigatorFileCount := func() {
		graph, err := navigator.MapProject(root)
		if err != nil {
			fmt.Fprintln(os.Stderr, "MapProject error:", err)
			return
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

	walkDur := bench("filepath.Walk + language detect", n, walkFileCount)
	navDur := bench("navigator.MapProject + node iterate", n, navigatorFileCount)
	speedup := float64(walkDur) / float64(navDur)
	fmt.Printf("  → speedup: %.2fx\n\n", speedup)

	// -------------------------------------------------------------------------
	// 2. listKeyConcepts (SCIP fallback): concept extraction from file names
	// -------------------------------------------------------------------------
	fmt.Println("=== listKeyConcepts fallback: concept extraction from file names ===")

	extractConcept := func(name string) string {
		if len(name) < 3 {
			return ""
		}
		// Camel-case split: take the first meaningful word >= 4 chars
		words := splitCamelCase(name)
		for _, w := range words {
			if len(w) >= 4 {
				return strings.ToLower(w)
			}
		}
		return ""
	}

	walkConcepts := func() {
		concepts := make(map[string]int)
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				n := d.Name()
				if strings.HasPrefix(n, ".") || n == "vendor" || n == "node_modules" {
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
				concepts[c]++
			}
			return nil
		})
		_ = concepts
	}

	navigatorConcepts := func() {
		graph, err := navigator.MapProject(root)
		if err != nil {
			return
		}
		concepts := make(map[string]int)
		for _, node := range graph.Nodes {
			ext := filepath.Ext(node.Path)
			name := strings.TrimSuffix(filepath.Base(node.Path), ext)
			name = strings.TrimSuffix(name, "_test")
			name = strings.TrimSuffix(name, ".test")
			if c := extractConcept(name); c != "" {
				concepts[c]++
			}
			if node.ModuleID != "" {
				if mc := extractConcept(filepath.Base(node.ModuleID)); mc != "" {
					concepts[mc]++
				}
			}
		}
		_ = concepts
	}

	walkConceptDur := bench("filepath.WalkDir + concept extract", n, walkConcepts)
	navConceptDur := bench("navigator.MapProject + concept extract", n, navigatorConcepts)
	conceptSpeedup := float64(walkConceptDur) / float64(navConceptDur)
	fmt.Printf("  → speedup: %.2fx\n", conceptSpeedup)
}

func splitCamelCase(s string) []string {
	var words []string
	start := 0
	for i := 1; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			words = append(words, s[start:i])
			start = i
		}
	}
	words = append(words, s[start:])
	return words
}
