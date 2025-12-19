// Package symbols provides tree-sitter based symbol extraction for fallback mode.
// When SCIP index is unavailable, this provides basic symbol search capabilities.
package symbols

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"

	"ckb/internal/complexity"
	"ckb/internal/logging"
)

// SymbolKind represents the kind of a symbol.
type SymbolKind string

const (
	KindFunction  SymbolKind = "function"
	KindMethod    SymbolKind = "method"
	KindClass     SymbolKind = "class"
	KindInterface SymbolKind = "interface"
	KindType      SymbolKind = "type"
	KindStruct    SymbolKind = "struct"
	KindVariable  SymbolKind = "variable"
	KindConstant  SymbolKind = "constant"
)

// Symbol represents an extracted symbol.
type Symbol struct {
	Name          string     `json:"name"`
	Kind          SymbolKind `json:"kind"`
	Path          string     `json:"path"`
	StartLine     int        `json:"startLine"`
	StartColumn   int        `json:"startColumn"`
	EndLine       int        `json:"endLine"`
	EndColumn     int        `json:"endColumn"`
	ContainerName string     `json:"containerName,omitempty"`
	Signature     string     `json:"signature,omitempty"`
}

// Extractor extracts symbols from source files using tree-sitter.
type Extractor struct {
	parser *complexity.Parser
	logger *logging.Logger
	cache  *symbolCache
	mu     sync.RWMutex
}

// symbolCache caches extracted symbols by file path.
type symbolCache struct {
	symbols map[string][]Symbol
	mu      sync.RWMutex
}

// NewExtractor creates a new symbol extractor.
func NewExtractor(logger *logging.Logger) *Extractor {
	return &Extractor{
		parser: complexity.NewParser(),
		logger: logger,
		cache: &symbolCache{
			symbols: make(map[string][]Symbol),
		},
	}
}

// ExtractFile extracts symbols from a single file.
func (e *Extractor) ExtractFile(ctx context.Context, path string) ([]Symbol, error) {
	ext := strings.ToLower(filepath.Ext(path))
	lang, ok := complexity.LanguageFromExtension(ext)
	if !ok {
		return nil, nil // Unsupported language, skip silently
	}

	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return e.ExtractSource(ctx, path, source, lang)
}

// ExtractSource extracts symbols from source code.
func (e *Extractor) ExtractSource(ctx context.Context, path string, source []byte, lang complexity.Language) ([]Symbol, error) {
	root, err := e.parser.Parse(ctx, source, lang)
	if err != nil {
		return nil, err
	}

	var symbols []Symbol

	// Extract different symbol types based on language
	symbols = append(symbols, e.extractFunctions(root, source, lang, path)...)
	symbols = append(symbols, e.extractTypes(root, source, lang, path)...)

	return symbols, nil
}

// ExtractDirectory extracts symbols from all supported files in a directory.
func (e *Extractor) ExtractDirectory(ctx context.Context, root string, ignoreDirs []string) ([]Symbol, error) {
	var allSymbols []Symbol
	var mu sync.Mutex

	// Default ignore patterns
	ignoreSet := make(map[string]bool)
	for _, d := range ignoreDirs {
		ignoreSet[d] = true
	}
	// Always ignore these
	ignoreSet[".git"] = true
	ignoreSet["node_modules"] = true
	ignoreSet["vendor"] = true
	ignoreSet[".ckb"] = true

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip ignored directories
		if d.IsDir() {
			name := d.Name()
			if ignoreSet[name] || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Extract from supported files
		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := complexity.LanguageFromExtension(ext); ok {
			symbols, err := e.ExtractFile(ctx, path)
			if err != nil {
				e.logger.Debug("failed to extract symbols", map[string]interface{}{
					"path":  path,
					"error": err.Error(),
				})
				return nil // Continue on error
			}

			mu.Lock()
			allSymbols = append(allSymbols, symbols...)
			mu.Unlock()
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return allSymbols, nil
}

// Search searches for symbols matching the query.
func (e *Extractor) Search(ctx context.Context, root string, query string, limit int) ([]Symbol, error) {
	// Extract all symbols (with caching in the future)
	allSymbols, err := e.ExtractDirectory(ctx, root, nil)
	if err != nil {
		return nil, err
	}

	// Filter by query
	queryLower := strings.ToLower(query)
	var matches []Symbol

	for _, sym := range allSymbols {
		if strings.Contains(strings.ToLower(sym.Name), queryLower) {
			matches = append(matches, sym)
			if limit > 0 && len(matches) >= limit {
				break
			}
		}
	}

	return matches, nil
}

// extractFunctions extracts function and method declarations.
func (e *Extractor) extractFunctions(root *sitter.Node, source []byte, lang complexity.Language, path string) []Symbol {
	var symbols []Symbol

	nodeTypes := getFunctionNodeTypes(lang)
	nodes := findNodes(root, nodeTypes)

	for _, node := range nodes {
		name := getFunctionName(node, source, lang)
		if name == "" || name == "<unknown>" || name == "<anonymous>" {
			continue
		}

		kind := KindFunction
		container := ""

		// Detect methods
		if isMethod(node, lang) {
			kind = KindMethod
			container = getMethodReceiver(node, source, lang)
		}

		symbols = append(symbols, Symbol{
			Name:          name,
			Kind:          kind,
			Path:          path,
			StartLine:     int(node.StartPoint().Row) + 1,
			StartColumn:   int(node.StartPoint().Column),
			EndLine:       int(node.EndPoint().Row) + 1,
			EndColumn:     int(node.EndPoint().Column),
			ContainerName: container,
			Signature:     extractSignature(node, source, lang),
		})
	}

	return symbols
}

// extractTypes extracts type, class, struct, and interface declarations.
func (e *Extractor) extractTypes(root *sitter.Node, source []byte, lang complexity.Language, path string) []Symbol {
	var symbols []Symbol

	nodeTypes := getTypeNodeTypes(lang)
	nodes := findNodes(root, nodeTypes)

	for _, node := range nodes {
		name := getTypeName(node, source, lang)
		if name == "" {
			continue
		}

		kind := classifyTypeKind(node, lang)

		symbols = append(symbols, Symbol{
			Name:        name,
			Kind:        kind,
			Path:        path,
			StartLine:   int(node.StartPoint().Row) + 1,
			StartColumn: int(node.StartPoint().Column),
			EndLine:     int(node.EndPoint().Row) + 1,
			EndColumn:   int(node.EndPoint().Column),
		})
	}

	return symbols
}

// getFunctionNodeTypes returns node types for functions/methods.
func getFunctionNodeTypes(lang complexity.Language) []string {
	switch lang {
	case complexity.LangGo:
		return []string{"function_declaration", "method_declaration"}
	case complexity.LangJavaScript, complexity.LangTypeScript, complexity.LangTSX:
		return []string{"function_declaration", "method_definition", "arrow_function"}
	case complexity.LangPython:
		return []string{"function_definition"}
	case complexity.LangRust:
		return []string{"function_item"}
	case complexity.LangJava:
		return []string{"method_declaration", "constructor_declaration"}
	case complexity.LangKotlin:
		return []string{"function_declaration"}
	default:
		return nil
	}
}

// getTypeNodeTypes returns node types for type declarations.
func getTypeNodeTypes(lang complexity.Language) []string {
	switch lang {
	case complexity.LangGo:
		return []string{"type_declaration", "type_spec"}
	case complexity.LangJavaScript, complexity.LangTypeScript, complexity.LangTSX:
		return []string{"class_declaration", "interface_declaration", "type_alias_declaration"}
	case complexity.LangPython:
		return []string{"class_definition"}
	case complexity.LangRust:
		return []string{"struct_item", "enum_item", "trait_item", "impl_item"}
	case complexity.LangJava:
		return []string{"class_declaration", "interface_declaration", "enum_declaration"}
	case complexity.LangKotlin:
		return []string{"class_declaration", "interface_declaration", "object_declaration"}
	default:
		return nil
	}
}

// getFunctionName extracts the name of a function node.
func getFunctionName(node *sitter.Node, source []byte, lang complexity.Language) string {
	var nameNode *sitter.Node

	switch lang {
	case complexity.LangGo:
		nameNode = node.ChildByFieldName("name")
		if nameNode == nil {
			for i := uint32(0); i < node.ChildCount(); i++ {
				child := node.Child(int(i))
				if child != nil && child.Type() == "identifier" {
					nameNode = child
					break
				}
			}
		}

	case complexity.LangJavaScript, complexity.LangTypeScript, complexity.LangTSX:
		nameNode = node.ChildByFieldName("name")

	case complexity.LangPython:
		nameNode = node.ChildByFieldName("name")

	case complexity.LangRust:
		nameNode = node.ChildByFieldName("name")

	case complexity.LangJava:
		nameNode = node.ChildByFieldName("name")

	case complexity.LangKotlin:
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

	return ""
}

// getTypeName extracts the name of a type declaration.
func getTypeName(node *sitter.Node, source []byte, lang complexity.Language) string {
	var nameNode *sitter.Node

	switch lang {
	case complexity.LangGo:
		// type_spec has name as first child identifier
		if node.Type() == "type_spec" {
			nameNode = node.ChildByFieldName("name")
			if nameNode == nil {
				for i := uint32(0); i < node.ChildCount(); i++ {
					child := node.Child(int(i))
					if child != nil && child.Type() == "type_identifier" {
						nameNode = child
						break
					}
				}
			}
		}

	case complexity.LangJavaScript, complexity.LangTypeScript, complexity.LangTSX:
		nameNode = node.ChildByFieldName("name")

	case complexity.LangPython:
		nameNode = node.ChildByFieldName("name")

	case complexity.LangRust:
		nameNode = node.ChildByFieldName("name")

	case complexity.LangJava, complexity.LangKotlin:
		nameNode = node.ChildByFieldName("name")
	}

	if nameNode != nil {
		return string(source[nameNode.StartByte():nameNode.EndByte()])
	}

	return ""
}

// classifyTypeKind determines the specific kind of a type node.
func classifyTypeKind(node *sitter.Node, lang complexity.Language) SymbolKind {
	nodeType := node.Type()

	switch lang {
	case complexity.LangGo:
		// In Go, need to look at the type_spec's underlying type
		return KindType

	case complexity.LangJavaScript, complexity.LangTypeScript, complexity.LangTSX:
		switch nodeType {
		case "class_declaration":
			return KindClass
		case "interface_declaration":
			return KindInterface
		default:
			return KindType
		}

	case complexity.LangPython:
		return KindClass

	case complexity.LangRust:
		switch nodeType {
		case "struct_item":
			return KindStruct
		case "trait_item":
			return KindInterface
		default:
			return KindType
		}

	case complexity.LangJava, complexity.LangKotlin:
		switch nodeType {
		case "class_declaration":
			return KindClass
		case "interface_declaration":
			return KindInterface
		default:
			return KindType
		}
	}

	return KindType
}

// isMethod checks if a function node is a method.
func isMethod(node *sitter.Node, lang complexity.Language) bool {
	switch lang {
	case complexity.LangGo:
		return node.Type() == "method_declaration"
	case complexity.LangJavaScript, complexity.LangTypeScript, complexity.LangTSX:
		return node.Type() == "method_definition"
	case complexity.LangPython:
		// Check if inside a class
		parent := node.Parent()
		for parent != nil {
			if parent.Type() == "class_definition" {
				return true
			}
			parent = parent.Parent()
		}
		return false
	case complexity.LangJava:
		return node.Type() == "method_declaration"
	default:
		return false
	}
}

// getMethodReceiver extracts the receiver/class name for a method.
func getMethodReceiver(node *sitter.Node, source []byte, lang complexity.Language) string {
	switch lang {
	case complexity.LangGo:
		// Go methods have receiver in parameters
		params := node.ChildByFieldName("receiver")
		if params != nil {
			// Find the type identifier
			for i := uint32(0); i < params.ChildCount(); i++ {
				child := params.Child(int(i))
				if child != nil && child.Type() == "parameter_declaration" {
					// Look for type_identifier
					for j := uint32(0); j < child.ChildCount(); j++ {
						typeChild := child.Child(int(j))
						if typeChild != nil {
							if typeChild.Type() == "type_identifier" {
								return string(source[typeChild.StartByte():typeChild.EndByte()])
							}
							// Handle pointer receivers
							if typeChild.Type() == "pointer_type" {
								for k := uint32(0); k < typeChild.ChildCount(); k++ {
									ptrChild := typeChild.Child(int(k))
									if ptrChild != nil && ptrChild.Type() == "type_identifier" {
										return string(source[ptrChild.StartByte():ptrChild.EndByte()])
									}
								}
							}
						}
					}
				}
			}
		}

	case complexity.LangPython:
		// Find containing class
		parent := node.Parent()
		for parent != nil {
			if parent.Type() == "class_definition" {
				nameNode := parent.ChildByFieldName("name")
				if nameNode != nil {
					return string(source[nameNode.StartByte():nameNode.EndByte()])
				}
			}
			parent = parent.Parent()
		}

	case complexity.LangJavaScript, complexity.LangTypeScript, complexity.LangTSX:
		// Find containing class
		parent := node.Parent()
		for parent != nil {
			if parent.Type() == "class_declaration" || parent.Type() == "class_body" {
				if parent.Type() == "class_body" {
					parent = parent.Parent()
				}
				if parent != nil {
					nameNode := parent.ChildByFieldName("name")
					if nameNode != nil {
						return string(source[nameNode.StartByte():nameNode.EndByte()])
					}
				}
			}
			parent = parent.Parent()
		}

	case complexity.LangJava:
		// Find containing class
		parent := node.Parent()
		for parent != nil {
			if parent.Type() == "class_declaration" {
				nameNode := parent.ChildByFieldName("name")
				if nameNode != nil {
					return string(source[nameNode.StartByte():nameNode.EndByte()])
				}
			}
			parent = parent.Parent()
		}
	}

	return ""
}

// extractSignature extracts a simplified signature for the function.
func extractSignature(node *sitter.Node, source []byte, lang complexity.Language) string {
	// For now, just extract the first line as a rough signature
	start := node.StartByte()
	end := node.EndByte()

	// Limit to first 200 chars or first newline
	content := source[start:end]
	if len(content) > 200 {
		content = content[:200]
	}

	// Find first newline
	for i, b := range content {
		if b == '\n' {
			content = content[:i]
			break
		}
	}

	return strings.TrimSpace(string(content))
}

// findNodes finds all nodes of the given types in the AST.
func findNodes(root *sitter.Node, types []string) []*sitter.Node {
	if len(types) == 0 {
		return nil
	}

	typeSet := make(map[string]bool)
	for _, t := range types {
		typeSet[t] = true
	}

	var result []*sitter.Node

	var walk func(*sitter.Node)
	walk = func(node *sitter.Node) {
		if node == nil {
			return
		}

		if typeSet[node.Type()] {
			result = append(result, node)
		}

		for i := uint32(0); i < node.ChildCount(); i++ {
			walk(node.Child(int(i)))
		}
	}

	walk(root)
	return result
}
