package query

import (
	"context"
	"fmt"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/navigator"
)

// checkArchitecturalHealth uses Cartographer to assess overall project health
// and report on cycles, god modules, and architectural violations.
// Returns a skip check when Cartographer is not compiled in.
func (e *Engine) checkArchitecturalHealth(_ context.Context, _ []string) (ReviewCheck, []ReviewFinding) {
	start := time.Now()

	if !navigator.Available() {
		return ReviewCheck{
			Name:     "arch-health",
			Status:   "skip",
			Severity: "info",
			Summary:  "Cartographer not compiled in this build",
			Duration: time.Since(start).Milliseconds(),
		}, nil
	}

	report, err := navigator.Health(e.repoRoot)
	if err != nil {
		return ReviewCheck{
			Name:     "arch-health",
			Status:   "skip",
			Severity: "info",
			Summary:  fmt.Sprintf("arch health skipped: %v", err),
			Duration: time.Since(start).Milliseconds(),
		}, nil
	}

	var findings []ReviewFinding

	if report.CycleCount > 0 {
		severity := "warning"
		if report.CycleCount >= 3 {
			severity = "error"
		}
		findings = append(findings, ReviewFinding{
			Check:    "arch-health",
			Severity: severity,
			Message:  fmt.Sprintf("%d circular dependency cycle(s) in project", report.CycleCount),
			Category: "architecture",
			RuleID:   "ckb/arch-health/cycles",
		})
	}

	if report.GodModuleCount > 0 {
		findings = append(findings, ReviewFinding{
			Check:    "arch-health",
			Severity: "warning",
			Message:  fmt.Sprintf("%d god module(s) detected (excessively connected)", report.GodModuleCount),
			Category: "architecture",
			RuleID:   "ckb/arch-health/god-modules",
		})
	}

	if report.LayerViolationCount > 0 {
		findings = append(findings, ReviewFinding{
			Check:    "arch-health",
			Severity: "warning",
			Message:  fmt.Sprintf("%d architectural layer violation(s) in project", report.LayerViolationCount),
			Category: "architecture",
			RuleID:   "ckb/arch-health/layer-violations",
		})
	}

	status := "pass"
	summary := fmt.Sprintf("Architectural health: %.0f/100", report.HealthScore)
	if report.HealthScore < 60 {
		status = "warn"
		summary = fmt.Sprintf("Architectural health degraded: %.0f/100 (%d cycles, %d god modules)", report.HealthScore, report.CycleCount, report.GodModuleCount)
	} else if len(findings) > 0 {
		status = "warn"
	}

	return ReviewCheck{
		Name:     "arch-health",
		Status:   status,
		Severity: "warning",
		Summary:  summary,
		Duration: time.Since(start).Milliseconds(),
	}, findings
}
