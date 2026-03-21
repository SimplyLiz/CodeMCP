package query

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// TraceabilityResult holds the outcome of traceability analysis.
type TraceabilityResult struct {
	TicketRefs     []TicketReference `json:"ticketRefs"`
	Linked         bool              `json:"linked"`         // At least one ticket reference found
	OrphanFiles    []string          `json:"orphanFiles"`    // Files with no ticket linkage
	CriticalOrphan bool              `json:"criticalOrphan"` // Critical-path files without ticket
}

// TicketReference is a detected ticket/requirement reference.
type TicketReference struct {
	ID     string `json:"id"`     // e.g., "JIRA-1234"
	Source string `json:"source"` // "commit-message", "branch-name"
	Commit string `json:"commit"` // Commit hash where found
}

// checkTraceability verifies that changes are linked to tickets/requirements.
func (e *Engine) checkTraceability(ctx context.Context, files []string, opts ReviewPROptions) (ReviewCheck, []ReviewFinding) {
	start := time.Now()

	policy := opts.Policy
	patterns := policy.TraceabilityPatterns
	if len(patterns) == 0 {
		return ReviewCheck{
			Name:     "traceability",
			Status:   "skip",
			Severity: "info",
			Summary:  "No traceability patterns configured",
			Duration: time.Since(start).Milliseconds(),
		}, nil
	}

	sources := policy.TraceabilitySources
	if len(sources) == 0 {
		sources = []string{"commit-message", "branch-name"}
	}

	// Compile regex patterns
	regexps := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		regexps = append(regexps, re)
	}

	if len(regexps) == 0 {
		return ReviewCheck{
			Name:     "traceability",
			Status:   "skip",
			Severity: "info",
			Summary:  "No valid traceability patterns",
			Duration: time.Since(start).Milliseconds(),
		}, nil
	}

	var refs []TicketReference
	refSet := make(map[string]bool)

	// Search commit messages
	if containsSource(sources, "commit-message") && e.gitAdapter != nil {
		commits, err := e.gitAdapter.GetCommitRange(opts.BaseBranch, opts.HeadBranch)
		if err == nil {
			for _, c := range commits {
				for _, re := range regexps {
					matches := re.FindAllString(c.Message, -1)
					for _, m := range matches {
						if !refSet[m] {
							refSet[m] = true
							refs = append(refs, TicketReference{
								ID:     m,
								Source: "commit-message",
								Commit: c.Hash,
							})
						}
					}
				}
			}
		}
	}

	// Search branch name
	if containsSource(sources, "branch-name") {
		branchName := opts.HeadBranch
		if branchName == "" || branchName == "HEAD" {
			if e.gitAdapter != nil {
				branchName, _ = e.gitAdapter.GetCurrentBranch()
			}
		}
		if branchName != "" {
			for _, re := range regexps {
				matches := re.FindAllString(branchName, -1)
				for _, m := range matches {
					if !refSet[m] {
						refSet[m] = true
						refs = append(refs, TicketReference{
							ID:     m,
							Source: "branch-name",
						})
					}
				}
			}
		}
	}

	linked := len(refs) > 0

	// Determine critical-path orphans
	var findings []ReviewFinding
	hasCriticalOrphan := false

	if !linked && policy.RequireTraceForCriticalPaths && len(policy.CriticalPaths) > 0 {
		for _, f := range files {
			for _, pattern := range policy.CriticalPaths {
				matched, _ := matchGlob(pattern, f)
				if matched {
					hasCriticalOrphan = true
					findings = append(findings, ReviewFinding{
						Check:      "traceability",
						Severity:   "error",
						File:       f,
						Message:    fmt.Sprintf("Safety-critical file changed without ticket reference (pattern: %s)", pattern),
						Suggestion: fmt.Sprintf("Add a ticket reference matching one of: %s", strings.Join(patterns, ", ")),
						Category:   "compliance",
						RuleID:     "ckb/traceability/critical-orphan",
					})
					break
				}
			}
		}
	}

	if !linked && policy.RequireTraceability {
		findings = append(findings, ReviewFinding{
			Check:      "traceability",
			Severity:   "warning",
			Message:    fmt.Sprintf("No ticket reference found in commits or branch name (expected: %s)", strings.Join(patterns, ", ")),
			Suggestion: "Reference a ticket in your commit message or branch name",
			Category:   "compliance",
			RuleID:     "ckb/traceability/no-ticket",
		})
	}

	// Identify orphan files (files with no ticket linkage)
	var orphanFiles []string
	if !linked {
		orphanFiles = files
	}

	status := "pass"
	summary := fmt.Sprintf("%d ticket reference(s) found", len(refs))
	if !linked {
		if hasCriticalOrphan {
			status = "fail"
			summary = "Critical-path changes without ticket reference"
		} else if policy.RequireTraceability {
			status = "warn"
			summary = "No ticket references found"
		}
	}

	return ReviewCheck{
		Name:     "traceability",
		Status:   status,
		Severity: "warning",
		Summary:  summary,
		Details: TraceabilityResult{
			TicketRefs:     refs,
			Linked:         linked,
			OrphanFiles:    orphanFiles,
			CriticalOrphan: hasCriticalOrphan,
		},
		Duration: time.Since(start).Milliseconds(),
	}, findings
}

func containsSource(sources []string, target string) bool {
	for _, s := range sources {
		if s == target {
			return true
		}
	}
	return false
}
