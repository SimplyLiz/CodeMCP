package do178c

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- missing-requirement-tag: §6.3.1 — requirement traceability ---

type missingRequirementTagCheck struct{}

func (c *missingRequirementTagCheck) ID() string       { return "missing-requirement-tag" }
func (c *missingRequirementTagCheck) Name() string     { return "Missing Requirement Traceability Tag" }
func (c *missingRequirementTagCheck) Article() string   { return "§6.3.1 DO-178C" }
func (c *missingRequirementTagCheck) Severity() string  { return "warning" }

var requirementTagPattern = regexp.MustCompile(`(?i)(@req|@requirement|REQ-|SRS-|HLR-|LLR-)`)

func (c *missingRequirementTagCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	// Source file extensions that should have traceability
	sourceExts := map[string]bool{
		".c": true, ".cpp": true, ".h": true, ".hpp": true,
		".go": true, ".py": true, ".rs": true, ".java": true,
		".ts": true, ".js": true,
	}

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		ext := strings.ToLower(filepath.Ext(file))
		if !sourceExts[ext] {
			continue
		}

		// Skip test files
		if strings.Contains(file, "_test.") || strings.Contains(file, "test_") ||
			strings.Contains(file, ".spec.") || strings.Contains(file, ".test.") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		if !requirementTagPattern.Match(content) {
			findings = append(findings, compliance.Finding{
				CheckID:    "missing-requirement-tag",
				Framework:  compliance.FrameworkDO178C,
				Severity:   "warning",
				Article:    "§6.3.1 DO-178C",
				File:       file,
				StartLine:  1,
				Message:    "Source file has no requirement traceability tags",
				Suggestion: "Add @req, @requirement, REQ-, SRS-, HLR-, or LLR- tags in comments to link code to requirements",
				Confidence: 0.55,
			})
		}
	}

	return findings, nil
}
