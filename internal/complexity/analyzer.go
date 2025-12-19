//go:build cgo

package complexity

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// Analyzer computes complexity metrics for source files.
type Analyzer struct {
	parser *Parser
}

// NewAnalyzer creates a new complexity analyzer.
func NewAnalyzer() *Analyzer {
	return &Analyzer{
		parser: NewParser(),
	}
}

// AnalyzeFile analyzes a source file and returns complexity metrics.
func (a *Analyzer) AnalyzeFile(ctx context.Context, path string) (*FileComplexity, error) {
	ext := strings.ToLower(filepath.Ext(path))
	lang, ok := LanguageFromExtension(ext)
	if !ok {
		return &FileComplexity{
			Path:  path,
			Error: "unsupported file extension: " + ext,
		}, nil
	}

	source, err := os.ReadFile(path)
	if err != nil {
		return &FileComplexity{
			Path:  path,
			Error: "failed to read file: " + err.Error(),
		}, nil
	}

	return a.AnalyzeSource(ctx, path, source, lang)
}

// AnalyzeSource analyzes source code and returns complexity metrics.
func (a *Analyzer) AnalyzeSource(ctx context.Context, path string, source []byte, lang Language) (*FileComplexity, error) {
	root, err := a.parser.Parse(ctx, source, lang)
	if err != nil {
		return &FileComplexity{
			Path:     path,
			Language: lang,
			Error:    err.Error(),
		}, nil
	}

	fc := &FileComplexity{
		Path:      path,
		Language:  lang,
		Functions: make([]ComplexityResult, 0),
	}

	// Find all function nodes
	functionTypes := GetFunctionNodeTypes(lang)
	functions := findNodes(root, functionTypes)

	for _, fn := range functions {
		result := a.analyzeFunction(fn, source, lang)
		fc.Functions = append(fc.Functions, result)
	}

	fc.Aggregate()
	return fc, nil
}

// analyzeFunction computes complexity for a single function.
func (a *Analyzer) analyzeFunction(node *sitter.Node, source []byte, lang Language) ComplexityResult {
	name := getFunctionName(node, source, lang)
	startLine := int(node.StartPoint().Row) + 1
	endLine := int(node.EndPoint().Row) + 1

	cyclomatic := computeCyclomaticComplexity(node, source, lang)
	cognitive := computeCognitiveComplexity(node, source, lang)

	return ComplexityResult{
		Name:       name,
		StartLine:  startLine,
		EndLine:    endLine,
		Lines:      endLine - startLine + 1,
		Cyclomatic: cyclomatic,
		Cognitive:  cognitive,
	}
}

// getFunctionName extracts the function name from a node.
func getFunctionName(node *sitter.Node, source []byte, lang Language) string {
	// Different languages have different structures for function names
	var nameNode *sitter.Node

	switch lang {
	case LangGo:
		// For Go: function_declaration has name child, method_declaration has name child
		nameNode = node.ChildByFieldName("name")
		if nameNode == nil {
			// Try to find identifier child directly
			for i := uint32(0); i < node.ChildCount(); i++ {
				child := node.Child(int(i))
				if child != nil && child.Type() == "identifier" {
					nameNode = child
					break
				}
			}
		}

	case LangJavaScript, LangTypeScript, LangTSX:
		nameNode = node.ChildByFieldName("name")
		if nameNode == nil && node.Type() == "method_definition" {
			// Method definitions have the name as first child
			nameNode = node.ChildByFieldName("name")
		}

	case LangPython:
		nameNode = node.ChildByFieldName("name")

	case LangRust:
		nameNode = node.ChildByFieldName("name")

	case LangJava:
		nameNode = node.ChildByFieldName("name")

	case LangKotlin:
		// Kotlin function_declaration has simple_identifier as name
		for i := uint32(0); i < node.ChildCount(); i++ {
			child := node.Child(int(i))
			if child != nil && child.Type() == "simple_identifier" {
				nameNode = child
				break
			}
		}
	}

	if nameNode != nil {
		return string(source[nameNode.StartByte():nameNode.EndByte()])
	}

	// Anonymous function
	switch node.Type() {
	case "arrow_function", "func_literal", "lambda", "lambda_expression",
		"closure_expression", "lambda_literal", "anonymous_function":
		return "<anonymous>"
	}

	return "<unknown>"
}

// computeCyclomaticComplexity calculates cyclomatic complexity.
// Cyclomatic = E - N + 2P, but simpler: count decision points + 1
func computeCyclomaticComplexity(node *sitter.Node, source []byte, lang Language) int {
	complexity := 1 // Base complexity

	decisionTypes := GetDecisionNodeTypes(lang)
	decisionNodes := findNodes(node, decisionTypes)

	for _, dn := range decisionNodes {
		// For binary expressions, only count if it's && or ||
		if dn.Type() == "binary_expression" || dn.Type() == "boolean_operator" {
			if IsBooleanOperator(dn, source, lang) {
				complexity++
			}
		} else {
			complexity++
		}
	}

	return complexity
}

// computeCognitiveComplexity calculates cognitive complexity.
// Cognitive complexity adds weight for nesting depth.
func computeCognitiveComplexity(node *sitter.Node, source []byte, lang Language) int {
	return computeCognitiveRecursive(node, source, lang, 0)
}

func computeCognitiveRecursive(node *sitter.Node, source []byte, lang Language, nestingLevel int) int {
	complexity := 0

	decisionTypes := GetDecisionNodeTypes(lang)
	nestingTypes := GetNestingNodeTypes(lang)

	nodeType := node.Type()

	// Check if this node adds to complexity
	isDecision := contains(decisionTypes, nodeType)
	isNesting := contains(nestingTypes, nodeType)

	if isDecision {
		// For binary expressions, only count if it's && or ||
		if nodeType == "binary_expression" || nodeType == "boolean_operator" {
			if IsBooleanOperator(node, source, lang) {
				complexity += 1 + nestingLevel
			}
		} else {
			// Add 1 for the construct plus nesting penalty
			complexity += 1 + nestingLevel
		}
	}

	// Determine nesting level for children
	childNesting := nestingLevel
	if isNesting {
		childNesting++
	}

	// Recurse into children
	for i := uint32(0); i < node.ChildCount(); i++ {
		child := node.Child(int(i))
		if child != nil {
			complexity += computeCognitiveRecursive(child, source, lang, childNesting)
		}
	}

	return complexity
}

// findNodes finds all nodes of the given types in the AST.
func findNodes(root *sitter.Node, types []string) []*sitter.Node {
	var result []*sitter.Node

	var walk func(*sitter.Node)
	walk = func(node *sitter.Node) {
		if node == nil {
			return
		}

		if contains(types, node.Type()) {
			result = append(result, node)
		}

		for i := uint32(0); i < node.ChildCount(); i++ {
			walk(node.Child(int(i)))
		}
	}

	walk(root)
	return result
}

// contains checks if a slice contains a string.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// IsAvailable returns whether complexity analysis is available.
// Returns true when CGO is enabled.
func IsAvailable() bool {
	return true
}
