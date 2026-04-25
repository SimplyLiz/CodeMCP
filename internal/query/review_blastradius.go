package query

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/cartographer"
	"github.com/SimplyLiz/CodeMCP/internal/impact"
	"github.com/SimplyLiz/CodeMCP/internal/lip"
)

// checkBlastRadius checks if changed symbols have high fan-out (many callers).
// Only reports functions and methods — variable/constant references are typically
// framework registrations (cobra commands, Qt signals, etc.), not real fan-out.
func (e *Engine) checkBlastRadius(ctx context.Context, changedFiles []string, opts ReviewPROptions) (ReviewCheck, []ReviewFinding) {
	start := time.Now()

	maxFanOut := opts.Policy.MaxFanOut
	informationalMode := maxFanOut <= 0

	// Fetch git churn map from Cartographer to identify hotspot files.
	// Used to escalate blast-radius findings for high-churn files from info → warning.
	var churnMap map[string]int
	if cartographer.Available() {
		if churn, err := cartographer.GitChurn(e.repoRoot, 0); err == nil {
			churnMap = churn
		}
	}

	// Prefetch LIP blast radius for all changed files in a single round-trip.
	// Returns nil when LIP is unavailable or doesn't support the message — the
	// rest of the function degrades to SCIP-only blast radius unchanged.
	var lipBR map[string]*impact.ExternalBlastRadius
	if e.lipSupports("query_blast_radius_batch") {
		lipURIs := make([]string, len(changedFiles))
		for i, f := range changedFiles {
			lipURIs[i] = "lip://local/" + f
		}
		if raw, _ := lip.QueryBlastRadiusBatch(lipURIs, 0.6); raw != nil {
			lipBR = make(map[string]*impact.ExternalBlastRadius, len(raw.Entries))
			for k, v := range raw.Entries {
				vCopy := v
				lipBR[k] = lip.EntryToExternal(&vCopy)
			}
		}
	}

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

		// Merge LIP semantic enrichment into the SCIP-derived blast radius.
		// Keyed by symbol's stable ID which maps to LIP's symbol_uri via the
		// "lip://local/<file>#<symbol>" convention.
		semanticCount := 0
		if lipBR != nil {
			if enriched, ok := lip.LookupSymbol(lipBR, sym.file, sym.name); ok {
				// Convert BlastRadiusSummary → impact.BlastRadius for merge
				staticBR := &impact.BlastRadius{
					ModuleCount:       impactResp.BlastRadius.ModuleCount,
					FileCount:         impactResp.BlastRadius.FileCount,
					UniqueCallerCount: impactResp.BlastRadius.UniqueCallerCount,
					RiskLevel:         impactResp.BlastRadius.RiskLevel,
				}
				merged := impact.MergeBlastRadius(staticBR, enriched)
				if merged != nil {
					impactResp.BlastRadius = &BlastRadiusSummary{
						ModuleCount:         merged.ModuleCount,
						FileCount:           merged.FileCount,
						UniqueCallerCount:   merged.UniqueCallerCount,
						RiskLevel:           merged.RiskLevel,
						StaticCallerCount:   merged.StaticCallerCount,
						SemanticCallerCount: merged.SemanticCallerCount,
						ConfirmedCount:      merged.ConfirmedCount,
					}
					for _, sc := range merged.SemanticCallers {
						impactResp.BlastRadius.SemanticCallers = append(
							impactResp.BlastRadius.SemanticCallers,
							SemanticCallerInfo{
								SymbolURI:  sc.SymbolURI,
								FileURI:    sc.FileURI,
								Tier:       string(sc.Tier),
								Confidence: sc.Confidence,
								Similarity: sc.Similarity,
							},
						)
					}
					semanticCount = merged.SemanticCallerCount
				}
			}
		}

		callerCount := impactResp.BlastRadius.UniqueCallerCount

		if informationalMode {
			// In informational mode, only surface symbols with meaningful fan-out.
			// Symbols with 1-2 callers are normal coupling; 3+ suggests a change
			// that could ripple further than expected.
			if callerCount >= 3 || semanticCount > 0 {
				msg := fmt.Sprintf("Fan-out: %s has %d callers", sym.name, callerCount)
				if semanticCount > 0 {
					msg += fmt.Sprintf(" (+%d semantically coupled)", semanticCount)
				}
				hint := ""
				if sym.name != "" {
					hint = fmt.Sprintf("→ ckb explain %s", sym.name)
				}
				// Escalate from info → warning for hotspot files (high git churn).
				// A frequently-changing file with many callers is higher risk.
				severity := "info"
				if churnMap[sym.file] >= 15 {
					severity = "warning"
				}
				findings = append(findings, ReviewFinding{
					Check:    "blast-radius",
					Severity: severity,
					File:     sym.file,
					Message:  msg,
					Category: "risk",
					RuleID:   "ckb/blast-radius/high-fanout",
					Hint:     hint,
				})
			}
		} else if callerCount > maxFanOut {
			msg := fmt.Sprintf("High fan-out: %s has %d callers (threshold: %d)", sym.name, callerCount, maxFanOut)
			if semanticCount > 0 {
				msg += fmt.Sprintf(" (+%d semantically coupled)", semanticCount)
			}
			hint := ""
			if sym.name != "" {
				hint = fmt.Sprintf("→ ckb explain %s", sym.name)
			}
			findings = append(findings, ReviewFinding{
				Check:    "blast-radius",
				Severity: "warning",
				File:     sym.file,
				Message:  msg,
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
