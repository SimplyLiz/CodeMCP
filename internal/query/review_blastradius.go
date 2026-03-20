package query

import (
	"context"
	"fmt"
	"time"
)

// checkBlastRadius checks if changed symbols have high fan-out (many callers).
func (e *Engine) checkBlastRadius(ctx context.Context, changedFiles []string, opts ReviewPROptions) (ReviewCheck, []ReviewFinding) {
	start := time.Now()

	maxFanOut := opts.Policy.MaxFanOut
	informationalMode := maxFanOut <= 0

	// Collect symbols from changed files, cap at 30 total
	type symbolRef struct {
		stableId string
		name     string
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
			symbols = append(symbols, symbolRef{
				stableId: sym.StableId,
				name:     sym.Name,
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
			// No threshold — emit info-level findings for all symbols with callers
			if callerCount > 0 {
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
