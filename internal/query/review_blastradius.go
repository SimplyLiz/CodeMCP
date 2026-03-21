package query

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// checkBlastRadius checks if changed symbols have high fan-out (many callers).
// Only reports functions and methods — variable/constant references are typically
// framework registrations (cobra commands, Qt signals, etc.), not real fan-out.
func (e *Engine) checkBlastRadius(ctx context.Context, changedFiles []string, opts ReviewPROptions) (ReviewCheck, []ReviewFinding) {
	start := time.Now()

	maxFanOut := opts.Policy.MaxFanOut
	informationalMode := maxFanOut <= 0

	// Collect symbols from changed files, cap at 30 total.
	// Only include functions and methods — variable references are typically
	// framework wiring (cobra commands, Spring beans, Qt signals) not real callers.
	type symbolRef struct {
		stableId string
		name     string
		kind     string
		file     string
	}
	var symbols []symbolRef

	for _, file := range changedFiles {
		if ctx.Err() != nil {
			break
		}
		if len(symbols) >= 30 {
			break
		}
		resp, err := e.SearchSymbols(ctx, SearchSymbolsOptions{
			Scope: file,
			Limit: 30 - len(symbols),
		})
		if err != nil || resp == nil {
			continue
		}
		for _, sym := range resp.Symbols {
			// Skip variables and constants — their "callers" are references
			// (reads, assignments, framework registrations), not real fan-out.
			if isFrameworkSymbol(sym.Kind, sym.Name, file) {
				continue
			}
			symbols = append(symbols, symbolRef{
				stableId: sym.StableId,
				name:     sym.Name,
				kind:     sym.Kind,
				file:     file,
			})
			if len(symbols) >= 30 {
				break
			}
		}
	}

	var findings []ReviewFinding
	for _, sym := range symbols {
		if ctx.Err() != nil {
			break
		}
		impactResp, err := e.AnalyzeImpact(ctx, AnalyzeImpactOptions{
			SymbolId: sym.stableId,
			Depth:    1,
		})
		if err != nil || impactResp == nil || impactResp.BlastRadius == nil {
			continue
		}

		callerCount := impactResp.BlastRadius.UniqueCallerCount

		if informationalMode {
			// In informational mode, only surface symbols with meaningful fan-out.
			// Symbols with 1-2 callers are normal coupling; 3+ suggests a change
			// that could ripple further than expected.
			if callerCount >= 3 {
				hint := ""
				if sym.name != "" {
					hint = fmt.Sprintf("→ ckb explain %s", sym.name)
				}
				findings = append(findings, ReviewFinding{
					Check:    "blast-radius",
					Severity: "info",
					File:     sym.file,
					Message:  fmt.Sprintf("Fan-out: %s has %d callers", sym.name, callerCount),
					Category: "risk",
					RuleID:   "ckb/blast-radius/high-fanout",
					Hint:     hint,
				})
			}
		} else if callerCount > maxFanOut {
			hint := ""
			if sym.name != "" {
				hint = fmt.Sprintf("→ ckb explain %s", sym.name)
			}
			findings = append(findings, ReviewFinding{
				Check:    "blast-radius",
				Severity: "warning",
				File:     sym.file,
				Message:  fmt.Sprintf("High fan-out: %s has %d callers (threshold: %d)", sym.name, callerCount, maxFanOut),
				Category: "risk",
				RuleID:   "ckb/blast-radius/high-fanout",
				Hint:     hint,
			})
		}
	}

	if informationalMode {
		status := "info"
		summary := "No symbols with callers in changes"
		if len(findings) > 0 {
			summary = fmt.Sprintf("%d symbol(s) have callers in changed files", len(findings))
		}
		return ReviewCheck{
			Name:     "blast-radius",
			Status:   status,
			Severity: "info",
			Summary:  summary,
			Duration: time.Since(start).Milliseconds(),
		}, findings
	}

	status := "pass"
	summary := "No high fan-out symbols in changes"
	if len(findings) > 0 {
		status = "warn"
		summary = fmt.Sprintf("%d symbol(s) exceed fan-out threshold of %d", len(findings), maxFanOut)
	}

	return ReviewCheck{
		Name:     "blast-radius",
		Status:   status,
		Severity: "warning",
		Summary:  summary,
		Duration: time.Since(start).Milliseconds(),
	}, findings
}

// isFrameworkSymbol returns true if this symbol is likely framework wiring
// rather than real application logic. These symbols have "callers" that are
// framework registrations, not actual fan-out.
//
// This works across languages because SCIP provides symbol kinds uniformly:
// - Go: cobra.Command vars, init() registrations
// - C++: Qt signal/slot vars, gtest TEST() macro expansions
// - Java: Spring @Bean fields, JUnit @Test annotations
// - Python: Flask route decorators, pytest fixtures
//
// The heuristic: variables and constants in CLI/test/config files are almost
// always framework wiring. Functions and methods are the real blast-radius targets.
func isFrameworkSymbol(kind, name, file string) bool {
	// Variables and constants are references, not call targets
	switch kind {
	case "variable", "constant", "property", "field":
		return true
	}

	// Known framework patterns by name (language-agnostic)
	lowerName := strings.ToLower(name)
	frameworkPatterns := []string{
		"init",      // Go init(), C++ static initializers
		"setup",     // Test setup functions
		"teardown",  // Test teardown functions
		"register",  // Framework registration
		"configure", // Framework configuration
	}
	for _, p := range frameworkPatterns {
		if lowerName == p {
			return true
		}
	}

	// CLI command patterns (Go cobra, Python click, etc.)
	if strings.HasPrefix(file, "cmd/") && strings.HasSuffix(lowerName, "cmd") {
		return true
	}

	return false
}
