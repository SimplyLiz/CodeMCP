package query

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/backends"
)

// RenameCallSite describes a location where the renamed symbol is called.
type RenameCallSite struct {
	File           string `json:"file"`
	Line           int    `json:"line"`
	Column         int    `json:"column"`
	ContextSnippet string `json:"contextSnippet"` // surrounding code, capped at 120 chars
	Kind           string `json:"kind"`           // "call", "type-ref", "import"
}

// RenameTypeRef describes a type reference to the renamed symbol.
type RenameTypeRef struct {
	File           string `json:"file"`
	Line           int    `json:"line"`
	ContextSnippet string `json:"contextSnippet"`
}

// RenameImport describes an import path that references the renamed symbol.
type RenameImport struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	ImportPath string `json:"importPath"`
}

// RenameDetail provides rename-specific information for prepareChange.
type RenameDetail struct {
	CallSites      []RenameCallSite `json:"callSites"`
	TypeReferences []RenameTypeRef  `json:"typeReferences,omitempty"`
	ImportPaths    []RenameImport   `json:"importPaths,omitempty"`
	TotalSites     int              `json:"totalSites"`
}

// getPrepareRenameDetail builds rename-specific detail by finding all references
// and classifying them as call sites, type references, or imports.
func (e *Engine) getPrepareRenameDetail(ctx context.Context, symbolId string) *RenameDetail {
	if symbolId == "" || e.scipAdapter == nil || !e.scipAdapter.IsAvailable() {
		return nil
	}

	refsResult, err := e.scipAdapter.FindReferences(ctx, symbolId, backends.RefOptions{
		MaxResults:   200,
		IncludeTests: true,
	})
	if err != nil || refsResult == nil || len(refsResult.References) == 0 {
		return nil
	}

	detail := &RenameDetail{
		TotalSites: refsResult.TotalReferences,
	}

	for _, ref := range refsResult.References {
		snippet := e.getContextSnippet(ref.Location.Path, ref.Location.Line, 120)
		kind := classifyReference(ref, snippet)

		switch kind {
		case "import":
			detail.ImportPaths = append(detail.ImportPaths, RenameImport{
				File:       ref.Location.Path,
				Line:       ref.Location.Line,
				ImportPath: extractImportPath(snippet),
			})
		case "type-ref":
			detail.TypeReferences = append(detail.TypeReferences, RenameTypeRef{
				File:           ref.Location.Path,
				Line:           ref.Location.Line,
				ContextSnippet: snippet,
			})
		default:
			detail.CallSites = append(detail.CallSites, RenameCallSite{
				File:           ref.Location.Path,
				Line:           ref.Location.Line,
				Column:         ref.Location.Column,
				ContextSnippet: snippet,
				Kind:           kind,
			})
		}
	}

	return detail
}

// getContextSnippet reads a single line from a file for context, capped at maxLen chars.
func (e *Engine) getContextSnippet(relPath string, line int, maxLen int) string {
	absPath := filepath.Join(e.repoRoot, relPath)
	content, err := os.ReadFile(absPath)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(content), "\n")
	if line < 1 || line > len(lines) {
		return ""
	}

	snippet := strings.TrimSpace(lines[line-1])
	if len(snippet) > maxLen {
		snippet = snippet[:maxLen]
	}
	return snippet
}

// classifyReference determines if a reference is a call, type-ref, or import.
func classifyReference(ref backends.Reference, snippet string) string {
	snippetLower := strings.ToLower(snippet)

	// Check for import patterns
	if strings.Contains(snippetLower, "import ") || strings.Contains(snippetLower, "require(") {
		return "import"
	}

	// Use SCIP reference kind if available
	if ref.Kind == "definition" || ref.Kind == "def" {
		return "definition"
	}

	// Check for type usage patterns
	if strings.Contains(snippet, ": ") && !strings.Contains(snippet, "(") {
		return "type-ref"
	}
	if strings.HasPrefix(strings.TrimSpace(snippet), "var ") || strings.HasPrefix(strings.TrimSpace(snippet), "type ") {
		return "type-ref"
	}

	return "call"
}

// extractImportPath extracts the import path from an import statement snippet.
func extractImportPath(snippet string) string {
	// Handle Go-style: "github.com/foo/bar"
	if idx := strings.Index(snippet, `"`); idx >= 0 {
		end := strings.Index(snippet[idx+1:], `"`)
		if end >= 0 {
			return snippet[idx+1 : idx+1+end]
		}
	}
	return snippet
}

// FormatRenamePreview generates a human-readable summary of rename impact.
func FormatRenamePreview(detail *RenameDetail) string {
	if detail == nil {
		return "No rename detail available"
	}
	return fmt.Sprintf("%d call sites, %d type references, %d import paths (%d total)",
		len(detail.CallSites), len(detail.TypeReferences), len(detail.ImportPaths), detail.TotalSites)
}
