package query

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// formatFuncRe matches functions named format*Human or format*Markdown.
var formatFuncRe = regexp.MustCompile(`^func\s+(\w+)?\s*(format\w+)(Human|Markdown)\s*\(`)

// numericLiteralRe matches numeric literals in Go code (integers and floats).
var numericLiteralRe = regexp.MustCompile(`\b(\d+(?:\.\d+)?)\b`)

// formatFuncInfo holds metadata about a formatter function.
type formatFuncInfo struct {
	name      string // full function name
	baseName  string // e.g., "formatReview"
	variant   string // "Human" or "Markdown"
	file      string
	startLine int
	literals  map[string]bool // set of numeric literals in the function body
}

// checkFormatConsistency detects divergent numeric literals between paired
// Human/Markdown formatter functions in changed files.
func (e *Engine) checkFormatConsistency(ctx context.Context, changedFiles []string) (ReviewCheck, []ReviewFinding) {
	start := time.Now()

	var findings []ReviewFinding

	// Collect formatter functions from changed files
	var funcs []formatFuncInfo
	for _, file := range changedFiles {
		if ctx.Err() != nil {
			break
		}
		if !strings.HasSuffix(file, ".go") {
			continue
		}
		ff := extractFormatFunctions(filepath.Join(e.repoRoot, file), file)
		funcs = append(funcs, ff...)
	}

	// Group by baseName
	groups := make(map[string][]formatFuncInfo)
	for _, f := range funcs {
		groups[f.baseName] = append(groups[f.baseName], f)
	}

	// For each group, check if the pair has divergent literals
	for baseName, group := range groups {
		if ctx.Err() != nil {
			break
		}
		var human, markdown *formatFuncInfo
		for i := range group {
			switch group[i].variant {
			case "Human":
				human = &group[i]
			case "Markdown":
				markdown = &group[i]
			}
		}
		if human == nil || markdown == nil {
			continue
		}

		// Find numeric literals present in one but not the other
		humanOnly := setDiff(human.literals, markdown.literals)
		markdownOnly := setDiff(markdown.literals, human.literals)

		if len(humanOnly) > 0 || len(markdownOnly) > 0 {
			var parts []string
			if len(humanOnly) > 0 {
				parts = append(parts, fmt.Sprintf("Human-only: %s", joinSorted(humanOnly)))
			}
			if len(markdownOnly) > 0 {
				parts = append(parts, fmt.Sprintf("Markdown-only: %s", joinSorted(markdownOnly)))
			}

			findings = append(findings, ReviewFinding{
				Check:      "format-consistency",
				Severity:   "info",
				File:       human.file,
				StartLine:  human.startLine,
				Message:    fmt.Sprintf("Divergent numeric literals in %sHuman vs %sMarkdown: %s", baseName, baseName, strings.Join(parts, "; ")),
				Suggestion: fmt.Sprintf("Verify that %sHuman and %sMarkdown use the same constants", baseName, baseName),
				Category:   "consistency",
				RuleID:     "ckb/format-consistency/divergent-literal",
			})
		}

		_ = baseName // already used above
	}

	status := "pass"
	summary := "No format consistency issues"
	if len(findings) > 0 {
		status = "info"
		summary = fmt.Sprintf("%d format consistency issue(s)", len(findings))
	}

	return ReviewCheck{
		Name:     "format-consistency",
		Status:   status,
		Severity: "info",
		Summary:  summary,
		Duration: time.Since(start).Milliseconds(),
	}, findings
}

// extractFormatFunctions scans a Go file for format*Human/format*Markdown functions
// and collects the numeric literals from their bodies.
func extractFormatFunctions(absPath, relPath string) []formatFuncInfo {
	f, err := os.Open(absPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var funcs []formatFuncInfo
	scanner := bufio.NewScanner(f)
	lineNum := 0
	var current *formatFuncInfo
	braceDepth := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if current == nil {
			// Look for format function declarations
			m := formatFuncRe.FindStringSubmatch(line)
			if m != nil {
				baseName := m[2]
				variant := m[3]
				fullName := baseName + variant
				if m[1] != "" {
					// Method receiver
					fullName = m[1] + "." + fullName
				}
				current = &formatFuncInfo{
					name:      fullName,
					baseName:  baseName,
					variant:   variant,
					file:      relPath,
					startLine: lineNum,
					literals:  make(map[string]bool),
				}
				braceDepth = strings.Count(line, "{") - strings.Count(line, "}")
				continue
			}
		} else {
			// Track brace depth
			braceDepth += strings.Count(line, "{") - strings.Count(line, "}")

			// Collect numeric literals from function body
			// Skip comment lines and string format specifiers
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "//") {
				matches := numericLiteralRe.FindAllString(line, -1)
				for _, m := range matches {
					// Skip trivially common numbers
					if m == "0" || m == "1" || m == "2" {
						continue
					}
					current.literals[m] = true
				}
			}

			if braceDepth <= 0 {
				funcs = append(funcs, *current)
				current = nil
			}
		}
	}

	return funcs
}

// setDiff returns elements in a but not in b.
func setDiff(a, b map[string]bool) map[string]bool {
	diff := make(map[string]bool)
	for k := range a {
		if !b[k] {
			diff[k] = true
		}
	}
	return diff
}

// joinSorted returns a sorted comma-separated list of map keys.
func joinSorted(m map[string]bool) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}
