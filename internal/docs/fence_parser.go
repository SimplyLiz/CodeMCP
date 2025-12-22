//go:build cgo

// Package docs provides documentation ↔ symbol linking.
package docs

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"ckb/internal/complexity"
)

// FenceParser extracts identifiers from fenced code blocks.
type FenceParser struct {
	parser *complexity.Parser
}

// NewFenceParser creates a new fence parser.
func NewFenceParser() *FenceParser {
	return &FenceParser{
		parser: complexity.NewParser(),
	}
}

// Fence represents a fenced code block in markdown.
type Fence struct {
	Language  string // Language hint from fence (e.g., "go", "python")
	StartLine int    // 1-indexed line number where fence starts
	EndLine   int    // 1-indexed line number where fence ends
	Content   string // Code content inside the fence
}

// FenceIdentifier represents an identifier found in a fence.
type FenceIdentifier struct {
	Name       string // The identifier name (may be qualified: "pkg.Func")
	Line       int    // 1-indexed line number within the fence content
	Confidence float64
}

// ExtractIdentifiers extracts potential symbol identifiers from fence content.
// Only returns identifiers that look like symbol references (qualified names like Foo.Bar).
func (fp *FenceParser) ExtractIdentifiers(fence Fence) []FenceIdentifier {
	lang := fenceLangToComplexity(fence.Language)
	if lang == "" {
		return nil
	}

	ctx := context.Background()
	root, err := fp.parser.Parse(ctx, []byte(fence.Content), lang)
	if err != nil || root == nil {
		return nil
	}

	var identifiers []FenceIdentifier
	seen := make(map[string]bool)

	// Walk the AST looking for qualified identifiers
	fp.walkNode(root, []byte(fence.Content), lang, &identifiers, seen)

	return identifiers
}

// walkNode recursively walks the AST to find identifiers.
func (fp *FenceParser) walkNode(node *sitter.Node, source []byte, lang complexity.Language, identifiers *[]FenceIdentifier, seen map[string]bool) {
	if node == nil {
		return
	}

	// Check if this node represents a qualified identifier (e.g., pkg.Func, Type.Method)
	if name := fp.extractQualifiedName(node, source, lang); name != "" {
		if !seen[name] && isQualifiedName(name) {
			seen[name] = true
			*identifiers = append(*identifiers, FenceIdentifier{
				Name:       name,
				Line:       int(node.StartPoint().Row) + 1, // 0-indexed to 1-indexed
				Confidence: 0.7,
			})
		}
	}

	// Recurse into children
	for i := uint32(0); i < node.ChildCount(); i++ {
		fp.walkNode(node.Child(int(i)), source, lang, identifiers, seen)
	}
}

// extractQualifiedName extracts a qualified name from a node if it represents one.
func (fp *FenceParser) extractQualifiedName(node *sitter.Node, source []byte, lang complexity.Language) string {
	nodeType := node.Type()

	switch lang {
	case complexity.LangGo:
		// Go: selector_expression (pkg.Func), qualified_type (pkg.Type)
		if nodeType == "selector_expression" || nodeType == "qualified_type" {
			return string(source[node.StartByte():node.EndByte()])
		}
	case complexity.LangPython:
		// Python: attribute (obj.attr)
		if nodeType == "attribute" {
			return string(source[node.StartByte():node.EndByte()])
		}
	case complexity.LangJavaScript, complexity.LangTypeScript, complexity.LangTSX:
		// JS/TS: member_expression (obj.prop)
		if nodeType == "member_expression" {
			return string(source[node.StartByte():node.EndByte()])
		}
	case complexity.LangRust:
		// Rust: scoped_identifier (crate::mod::Type)
		if nodeType == "scoped_identifier" {
			return string(source[node.StartByte():node.EndByte()])
		}
	case complexity.LangJava, complexity.LangKotlin:
		// Java/Kotlin: field_access, method_invocation
		if nodeType == "field_access" || nodeType == "method_invocation" {
			return string(source[node.StartByte():node.EndByte()])
		}
	}

	return ""
}

// isQualifiedName checks if a name contains delimiters (dots, colons, etc.).
func isQualifiedName(name string) bool {
	return strings.ContainsAny(name, ".::#")
}

// fenceLangToComplexity maps fence language hints to complexity.Language.
func fenceLangToComplexity(lang string) complexity.Language {
	switch strings.ToLower(lang) {
	case "go", "golang":
		return complexity.LangGo
	case "python", "py":
		return complexity.LangPython
	case "javascript", "js":
		return complexity.LangJavaScript
	case "typescript", "ts":
		return complexity.LangTypeScript
	case "tsx":
		return complexity.LangTSX
	case "rust", "rs":
		return complexity.LangRust
	case "java":
		return complexity.LangJava
	case "kotlin", "kt":
		return complexity.LangKotlin
	default:
		return ""
	}
}
