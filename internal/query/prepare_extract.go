package query

import (
	"os"
	"path/filepath"
	"strings"
)

// ExtractParameter describes an input variable to the extracted function.
type ExtractParameter struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"` // inferred type if available
	Line int    `json:"line"`          // where it's defined
}

// ExtractReturn describes a return value from the extracted function.
type ExtractReturn struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	Line int    `json:"line"` // where it's used after the block
}

// BoundaryAnalysis describes the start/end boundaries of the extraction region.
type BoundaryAnalysis struct {
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Lines     int    `json:"lines"`
	Language  string `json:"language,omitempty"`
}

// ExtractDetail provides extract-specific information for prepareChange.
type ExtractDetail struct {
	SuggestedSignature string             `json:"suggestedSignature"`
	Parameters         []ExtractParameter `json:"parameters,omitempty"`
	Returns            []ExtractReturn    `json:"returns,omitempty"`
	BoundaryAnalysis   *BoundaryAnalysis  `json:"boundaryAnalysis"`
}

// getPrepareExtractDetail builds extract-specific detail.
// Phase 1: minimal boundary analysis from file content.
// Phase 2 (future): full variable flow analysis via tree-sitter AST walking.
func (e *Engine) getPrepareExtractDetail(target *PrepareChangeTarget) *ExtractDetail {
	if target == nil || target.Path == "" {
		return nil
	}

	absPath := filepath.Join(e.repoRoot, target.Path)
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}

	lines := strings.Split(string(content), "\n")
	totalLines := len(lines)

	// Default boundary: whole file (agents should specify precise boundaries)
	startLine := 1
	endLine := totalLines

	lang := inferLanguage(target.Path)

	detail := &ExtractDetail{
		BoundaryAnalysis: &BoundaryAnalysis{
			StartLine: startLine,
			EndLine:   endLine,
			Lines:     endLine - startLine + 1,
			Language:  lang,
		},
	}

	// Generate a basic suggested signature
	if target.SymbolId != "" {
		detail.SuggestedSignature = "func extracted() // TODO: determine parameters and returns"
	}

	return detail
}

// inferLanguage returns the language name from a file path.
func inferLanguage(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".java":
		return "java"
	case ".kt":
		return "kotlin"
	default:
		return ""
	}
}
