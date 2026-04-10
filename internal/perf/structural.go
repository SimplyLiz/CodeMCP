//go:build cgo

package perf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/SimplyLiz/CodeMCP/internal/complexity"
)

// AnalyzeStructural detects structural performance anti-patterns in high-churn files.
// It uses tree-sitter to find call expressions inside loop bodies — the primary
// structural signal for O(n) or O(n²) hidden costs that do not surface in profiling
// until production load.
//
// The scan runs in three stages:
//  1. Git log to identify hot files (frequently changed in the window).
//  2. Tree-sitter parse of each hot file to locate loops and call sites within them.
//  3. Annotation: entrypoint proximity and severity ranking.
func (a *Analyzer) AnalyzeStructural(ctx context.Context, opts StructuralPerfOptions) (*StructuralPerfResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.WindowDays <= 0 {
		opts.WindowDays = 90
	}
	if opts.MinChurnCount <= 0 {
		opts.MinChurnCount = 3
	}

	since := time.Now().AddDate(0, 0, -opts.WindowDays)

	a.logger.Debug("Starting structural perf scan",
		"scope", opts.Scope,
		"windowDays", opts.WindowDays,
		"minChurnCount", opts.MinChurnCount,
		"entrypoints", len(opts.EntrypointFiles),
	)

	// Build entrypoint set for O(1) lookup.
	epSet := make(map[string]bool, len(opts.EntrypointFiles))
	for _, f := range opts.EntrypointFiles {
		epSet[filepath.ToSlash(f)] = true
	}

	// Step 1: get per-file commit totals from git log.
	// buildCoChangePairs also returns fileTotals as a side-effect.
	_, fileTotals, err := a.buildCoChangePairs(ctx, since, opts.Scope)
	if err != nil {
		return nil, fmt.Errorf("getting file churn data: %w", err)
	}

	// Step 2: collect hot files above the churn threshold, sorted by churn descending.
	type hotFile struct {
		path  string
		churn int
	}
	var hotFiles []hotFile
	for f, count := range fileTotals {
		if count < opts.MinChurnCount {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f))
		if _, ok := complexity.LanguageFromExtension(ext); !ok {
			continue // skip files tree-sitter cannot parse
		}
		hotFiles = append(hotFiles, hotFile{f, count})
	}
	sort.Slice(hotFiles, func(i, j int) bool {
		return hotFiles[i].churn > hotFiles[j].churn
	})

	// Step 3: parse each hot file with tree-sitter and find loop call sites.
	parser := complexity.NewParser()
	complexityAnalyzer := complexity.NewAnalyzer()

	var callSites []LoopCallSite
	filesScanned := 0

	for _, hf := range hotFiles {
		if len(callSites) >= opts.Limit*3 { // collect extra before sort/cap
			break
		}

		ext := strings.ToLower(filepath.Ext(hf.path))
		lang, _ := complexity.LanguageFromExtension(ext)

		absPath := filepath.Join(a.repoRoot, hf.path)
		source, err := os.ReadFile(absPath)
		if err != nil {
			a.logger.Debug("skipping unreadable file", "path", hf.path, "err", err)
			continue
		}

		root, err := parser.Parse(ctx, source, lang)
		if err != nil {
			a.logger.Debug("parse error", "path", hf.path, "err", err)
			continue
		}

		// Get function line ranges for enclosing-function lookup.
		var functions []complexity.ComplexityResult
		if complexityAnalyzer != nil {
			if fc, fcErr := complexityAnalyzer.AnalyzeSource(ctx, absPath, source, lang); fcErr == nil && fc != nil {
				functions = fc.Functions
			}
		}

		filesScanned++
		nearEP := epSet[filepath.ToSlash(hf.path)]

		sites := findLoopCallSites(root, source, lang, hf.path, hf.churn, nearEP, functions)
		callSites = append(callSites, sites...)
	}

	// Sort by severity descending, then churn descending.
	sort.SliceStable(callSites, func(i, j int) bool {
		ri, rj := severityRank(callSites[i].Severity), severityRank(callSites[j].Severity)
		if ri != rj {
			return ri > rj
		}
		return callSites[i].ChurnCount > callSites[j].ChurnCount
	})
	if len(callSites) > opts.Limit {
		callSites = callSites[:opts.Limit]
	}

	return &StructuralPerfResult{
		LoopCallSites: callSites,
		Summary: StructuralPerfSummary{
			FilesScanned:   filesScanned,
			HotFilesFound:  len(hotFiles),
			CallSitesFound: len(callSites),
		},
	}, nil
}

// findLoopCallSites finds all call expressions inside loop bodies in one parsed file.
// It skips nested function definitions so it only reports calls made directly inside
// the loop, not calls inside closures/lambdas defined within the loop.
func findLoopCallSites(
	root *sitter.Node,
	source []byte,
	lang complexity.Language,
	file string,
	churnCount int,
	nearEP bool,
	functions []complexity.ComplexityResult,
) []LoopCallSite {
	loopTypes := getLoopNodeTypes(lang)
	callTypes := getCallNodeTypes(lang)
	fnTypes := complexity.GetFunctionNodeTypes(lang)

	loops := complexity.FindNodes(root, loopTypes)

	var results []LoopCallSite
	for _, loop := range loops {
		loopTypeName := humanLoopType(loop.Type(), lang)

		// Find calls inside this loop, not descending into nested function bodies.
		calls := complexity.FindNodesSkipping(loop, callTypes, fnTypes)
		for _, call := range calls {
			line := int(call.StartPoint().Row) + 1

			callText := string(source[call.StartByte():call.EndByte()])
			if len(callText) > 120 {
				callText = callText[:117] + "…"
			}

			fnName := findEnclosingFunction(line, functions)
			severity := computeSeverity(churnCount, nearEP)
			explanation := buildExplanation(file, fnName, callText, loopTypeName, churnCount, nearEP)

			results = append(results, LoopCallSite{
				File:           file,
				Line:           line,
				FunctionName:   fnName,
				CallText:       callText,
				LoopType:       loopTypeName,
				ChurnCount:     churnCount,
				NearEntrypoint: nearEP,
				Severity:       severity,
				Explanation:    explanation,
			})
		}
	}
	return results
}

// findEnclosingFunction returns the name of the smallest function whose line range
// contains line. Returns "<global>" if no function matches.
func findEnclosingFunction(line int, functions []complexity.ComplexityResult) string {
	best := "<global>"
	bestSize := 1<<31 - 1 // MaxInt32 sentinel
	for _, fn := range functions {
		if fn.StartLine <= line && fn.EndLine >= line {
			size := fn.EndLine - fn.StartLine
			if size < bestSize {
				bestSize = size
				best = fn.Name
			}
		}
	}
	return best
}

// getLoopNodeTypes returns the tree-sitter node types that represent loop constructs.
func getLoopNodeTypes(lang complexity.Language) []string {
	switch lang {
	case complexity.LangGo:
		return []string{"for_statement"}
	case complexity.LangJavaScript, complexity.LangTypeScript, complexity.LangTSX:
		return []string{"for_statement", "for_in_statement", "for_of_statement", "while_statement", "do_statement"}
	case complexity.LangPython:
		return []string{"for_statement", "while_statement"}
	case complexity.LangRust:
		return []string{"for_expression", "while_expression", "loop_expression"}
	case complexity.LangJava:
		return []string{"for_statement", "enhanced_for_statement", "while_statement", "do_statement"}
	case complexity.LangKotlin:
		return []string{"for_statement", "while_statement", "do_while_statement"}
	default:
		return nil
	}
}

// getCallNodeTypes returns the tree-sitter node types that represent call expressions.
func getCallNodeTypes(lang complexity.Language) []string {
	switch lang {
	case complexity.LangGo:
		return []string{"call_expression"}
	case complexity.LangJavaScript, complexity.LangTypeScript, complexity.LangTSX:
		return []string{"call_expression", "new_expression"}
	case complexity.LangPython:
		return []string{"call"}
	case complexity.LangRust:
		return []string{"call_expression", "method_call_expression"}
	case complexity.LangJava:
		return []string{"method_invocation", "object_creation_expression"}
	case complexity.LangKotlin:
		return []string{"call_expression"}
	default:
		return nil
	}
}

// humanLoopType converts a tree-sitter node type to a human-readable loop name.
func humanLoopType(nodeType string, lang complexity.Language) string {
	switch nodeType {
	case "for_statement":
		if lang == complexity.LangGo {
			return "for/range"
		}
		return "for"
	case "enhanced_for_statement":
		return "for-each"
	case "for_in_statement":
		return "for-in"
	case "for_of_statement":
		return "for-of"
	case "for_expression":
		return "for"
	case "while_statement", "while_expression":
		return "while"
	case "do_statement", "do_while_statement":
		return "do-while"
	case "loop_expression":
		return "loop"
	default:
		return nodeType
	}
}

// computeSeverity returns the severity based on churn count and entrypoint proximity.
func computeSeverity(churnCount int, nearEP bool) string {
	switch {
	case nearEP && churnCount >= 10:
		return "high"
	case nearEP || churnCount >= 10:
		return "medium"
	default:
		return "low"
	}
}

// severityRank converts severity to an integer for sorting (higher = more severe).
func severityRank(s string) int {
	switch s {
	case "high":
		return 2
	case "medium":
		return 1
	default:
		return 0
	}
}

// buildExplanation constructs a human-readable explanation for a loop call site.
// Uses strings.Builder to avoid fmt.Sprintf's intermediate allocations.
func buildExplanation(file, fnName, callText, loopType string, churnCount int, nearEP bool) string {
	hotness := "frequently changed"
	if churnCount >= 20 {
		hotness = "very frequently changed"
	} else if churnCount < 5 {
		hotness = "recently changed"
	}

	// Pre-size to avoid buffer growth: typical output is ~320 chars.
	var b strings.Builder
	b.Grow(320)
	b.WriteString(fnName)
	b.WriteString(" in ")
	b.WriteString(file)
	b.WriteString(" contains a call to ")
	b.WriteString(strconv.Quote(callText))
	b.WriteString(" inside a ")
	b.WriteString(loopType)
	b.WriteString(" loop. This file is ")
	b.WriteString(hotness)
	b.WriteString(" (")
	b.WriteString(strconv.Itoa(churnCount))
	b.WriteString(" commits). Each loop iteration may trigger additional I/O, database queries, or expensive computation.")
	if nearEP {
		b.WriteString(" It is a system entrypoint, meaning this loop runs on every request.")
	}
	return b.String()
}
