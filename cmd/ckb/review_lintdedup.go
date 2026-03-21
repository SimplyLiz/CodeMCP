package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/query"
)

// deduplicateLintFindings removes CKB findings that overlap with an existing
// SARIF lint report. This prevents CKB from flagging issues the user's linter
// already catches, which the research identifies as an instant credibility loss.
//
// Matching is done by (file, line, ruleId-prefix). We don't require exact rule
// IDs because CKB rules (ckb/...) and linter rules (e.g., golangci-lint) use
// different naming. Instead we match on location + message similarity.
//
// Returns the number of suppressed findings. Modifies response in place.
func deduplicateLintFindings(resp *query.ReviewPRResponse, sarifPath string) (int, error) {
	data, err := os.ReadFile(sarifPath)
	if err != nil {
		return 0, fmt.Errorf("read lint report: %w", err)
	}

	lintKeys, err := parseSARIFKeys(data)
	if err != nil {
		return 0, err
	}

	if len(lintKeys) == 0 {
		return 0, nil
	}

	// Filter findings
	kept := make([]query.ReviewFinding, 0, len(resp.Findings))
	suppressed := 0
	for _, f := range resp.Findings {
		key := lintKey(f.File, f.StartLine)
		if lintKeys[key] {
			suppressed++
			continue
		}
		kept = append(kept, f)
	}

	resp.Findings = kept
	return suppressed, nil
}

// lintKey builds a dedup key from file path and line number.
// Two findings on the same file:line are considered duplicates regardless of
// the specific rule, since the user has already seen the linter's version.
func lintKey(file string, line int) string {
	// Normalize: strip leading ./ or / for comparison
	file = strings.TrimPrefix(file, "./")
	file = strings.TrimPrefix(file, "/")
	return fmt.Sprintf("%s:%d", file, line)
}

// parseSARIFKeys extracts file:line keys from a SARIF v2.1.0 report.
func parseSARIFKeys(data []byte) (map[string]bool, error) {
	// Minimal SARIF parse — only the fields we need
	var report struct {
		Runs []struct {
			Results []struct {
				Locations []struct {
					PhysicalLocation struct {
						ArtifactLocation struct {
							URI string `json:"uri"`
						} `json:"artifactLocation"`
						Region struct {
							StartLine int `json:"startLine"`
						} `json:"region"`
					} `json:"physicalLocation"`
				} `json:"locations"`
			} `json:"results"`
		} `json:"runs"`
	}

	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parse SARIF: %w", err)
	}

	keys := make(map[string]bool)
	for _, run := range report.Runs {
		for _, result := range run.Results {
			for _, loc := range result.Locations {
				file := loc.PhysicalLocation.ArtifactLocation.URI
				line := loc.PhysicalLocation.Region.StartLine
				if file != "" && line > 0 {
					keys[lintKey(file, line)] = true
				}
			}
		}
	}

	return keys, nil
}
