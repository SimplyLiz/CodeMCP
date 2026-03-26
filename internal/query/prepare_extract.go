package query

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/extract"
)

// ExtractParameter describes an input variable to the extracted function.
type ExtractParameter struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"` // inferred type if available
	Line int    `json:"line"`           // where it's defined
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
// Uses tree-sitter-based variable flow analysis when CGO is available,
// falls back to minimal boundary analysis otherwise.
func (e *Engine) getPrepareExtractDetail(target *PrepareChangeTarget, reqStartLine, reqEndLine int) *ExtractDetail {
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

	// Use requested lines if provided, otherwise default to whole file
	startLine := 1
	endLine := totalLines
	if reqStartLine > 0 {
		startLine = reqStartLine
	}
	if reqEndLine > 0 {
		endLine = reqEndLine
	}

	lang := inferLanguage(target.Path)

	detail := &ExtractDetail{
		BoundaryAnalysis: &BoundaryAnalysis{
			StartLine: startLine,
			EndLine:   endLine,
			Lines:     endLine - startLine + 1,
			Language:  lang,
		},
	}

	// Try tree-sitter-based flow analysis (Phase 2)
	analyzer := extract.NewAnalyzer()
	if analyzer != nil && lang != "" {
		ctx := context.Background()
		flow, err := analyzer.Analyze(ctx, extract.AnalyzeOptions{
			Source:    content,
			Language:  lang,
			StartLine: startLine,
			EndLine:   endLine,
		})
		if err == nil && flow != nil {
			// Populate parameters from flow analysis
			for _, p := range flow.Parameters {
				detail.Parameters = append(detail.Parameters, ExtractParameter{
					Name: p.Name,
					Type: p.Type,
					Line: p.DefinedAt,
				})
			}
			// Populate returns from flow analysis
			for _, r := range flow.Returns {
				detail.Returns = append(detail.Returns, ExtractReturn{
					Name: r.Name,
					Type: r.Type,
					Line: r.FirstUsedAt,
				})
			}
			// Generate language-appropriate suggested signature
			detail.SuggestedSignature = generateSignature(lang, detail.Parameters, detail.Returns)
			return detail
		}
	}

	// Fallback: basic suggested signature (Phase 1 behavior)
	if target.SymbolId != "" {
		detail.SuggestedSignature = "func extracted() // parameters and returns must be determined from usage"
	}

	return detail
}

// generateSignature produces a language-appropriate function signature from parameters and returns.
func generateSignature(lang string, params []ExtractParameter, returns []ExtractReturn) string {
	switch lang {
	case "go":
		return generateGoSignature(params, returns)
	case "javascript", "typescript":
		return generateJSSignature(params, returns)
	case "python":
		return generatePySignature(params, returns)
	default:
		return generateGoSignature(params, returns)
	}
}

func generateGoSignature(params []ExtractParameter, returns []ExtractReturn) string {
	var paramParts []string
	for _, p := range params {
		if p.Type != "" {
			paramParts = append(paramParts, fmt.Sprintf("%s %s", p.Name, p.Type))
		} else {
			paramParts = append(paramParts, p.Name)
		}
	}

	var returnParts []string
	for _, r := range returns {
		if r.Type != "" {
			returnParts = append(returnParts, r.Type)
		} else {
			returnParts = append(returnParts, r.Name)
		}
	}

	sig := fmt.Sprintf("func extracted(%s)", strings.Join(paramParts, ", "))
	if len(returnParts) == 1 {
		sig += " " + returnParts[0]
	} else if len(returnParts) > 1 {
		sig += " (" + strings.Join(returnParts, ", ") + ")"
	}
	return sig
}

func generateJSSignature(params []ExtractParameter, returns []ExtractReturn) string {
	var paramNames []string
	for _, p := range params {
		paramNames = append(paramNames, p.Name)
	}
	return fmt.Sprintf("function extracted(%s)", strings.Join(paramNames, ", "))
}

func generatePySignature(params []ExtractParameter, returns []ExtractReturn) string {
	var paramNames []string
	for _, p := range params {
		if p.Type != "" {
			paramNames = append(paramNames, fmt.Sprintf("%s: %s", p.Name, p.Type))
		} else {
			paramNames = append(paramNames, p.Name)
		}
	}

	sig := fmt.Sprintf("def extracted(%s)", strings.Join(paramNames, ", "))
	if len(returns) > 0 {
		var retNames []string
		for _, r := range returns {
			retNames = append(retNames, r.Name)
		}
		sig += " -> (" + strings.Join(retNames, ", ") + ")"
	}
	return sig
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
