//go:build cgo

package extract

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/SimplyLiz/CodeMCP/internal/complexity"
)

// Analyzer performs tree-sitter-based variable flow analysis for extract refactoring.
type Analyzer struct {
	parser *complexity.Parser
}

// NewAnalyzer creates a new extract flow analyzer.
func NewAnalyzer() *Analyzer {
	return &Analyzer{
		parser: complexity.NewParser(),
	}
}

// Analyze performs variable flow analysis on the given source code region.
func (a *Analyzer) Analyze(ctx context.Context, opts AnalyzeOptions) (*FlowAnalysis, error) {
	if a == nil || a.parser == nil {
		return nil, nil
	}

	lang, ok := complexity.LanguageFromExtension(langToExtension(opts.Language))
	if !ok {
		return nil, nil
	}

	root, err := a.parser.Parse(ctx, opts.Source, lang)
	if err != nil {
		return nil, nil
	}

	lines := strings.Split(string(opts.Source), "\n")
	_ = lines

	// Find the containing function for StartLine
	containingFunc := findContainingFunction(root, opts.StartLine, opts.Language)
	if containingFunc == nil {
		// No containing function found, fall back to root scope
		containingFunc = root
	}

	// Collect all variable declarations in the containing function scope
	declarations := collectDeclarations(containingFunc, opts.Source, opts.Language)

	// Collect all identifier references with line numbers and write context
	references := collectReferences(containingFunc, opts.Source, opts.Language)

	// Classify variables into parameters, returns, and locals
	return classifyVariables(declarations, references, opts.StartLine, opts.EndLine), nil
}

// varDecl represents a variable declaration found in the AST.
type varDecl struct {
	name string
	typ  string
	line int
}

// varRef represents a variable reference found in the AST.
type varRef struct {
	name       string
	line       int
	isModified bool // true if LHS of assignment
}

// findContainingFunction finds the function declaration that contains the given line.
func findContainingFunction(root *sitter.Node, line int, language string) *sitter.Node {
	funcTypes := getFunctionNodeTypes(language)
	funcs := findNodes(root, funcTypes)

	for _, fn := range funcs {
		startLine := int(fn.StartPoint().Row) + 1
		endLine := int(fn.EndPoint().Row) + 1
		if startLine <= line && line <= endLine {
			return fn
		}
	}
	return nil
}

// collectDeclarations collects all variable declarations in the given AST subtree.
func collectDeclarations(node *sitter.Node, source []byte, language string) []varDecl {
	declTypes := getDeclarationNodeTypes(language)
	declNodes := findNodes(node, declTypes)

	var decls []varDecl
	for _, dn := range declNodes {
		name, typ := extractDeclInfo(dn, source, language)
		if name == "" {
			continue
		}
		decls = append(decls, varDecl{
			name: name,
			typ:  typ,
			line: int(dn.StartPoint().Row) + 1,
		})
	}

	// Also collect function parameters
	paramTypes := getParameterNodeTypes(language)
	paramNodes := findNodes(node, paramTypes)
	for _, pn := range paramNodes {
		name, typ := extractParamInfo(pn, source, language)
		if name == "" {
			continue
		}
		decls = append(decls, varDecl{
			name: name,
			typ:  typ,
			line: int(pn.StartPoint().Row) + 1,
		})
	}

	return decls
}

// collectReferences collects all identifier references in the given AST subtree.
func collectReferences(node *sitter.Node, source []byte, language string) []varRef {
	identNodes := findNodes(node, []string{"identifier", "shorthand_property_identifier"})

	var refs []varRef
	for _, id := range identNodes {
		name := id.Content(source)
		if name == "" || isKeyword(name, language) {
			continue
		}

		isModified := false
		parent := id.Parent()
		if parent != nil {
			parentType := parent.Type()
			// Check if this is the LHS of an assignment
			switch parentType {
			case "assignment_expression", "assignment_statement", "augmented_assignment":
				if parent.ChildCount() > 0 && parent.Child(0) == id {
					isModified = true
				}
			case "short_var_declaration":
				isModified = true
			case "update_expression", "increment_statement":
				isModified = true
			}
		}

		refs = append(refs, varRef{
			name:       name,
			line:       int(id.StartPoint().Row) + 1,
			isModified: isModified,
		})
	}

	return refs
}

// classifyVariables classifies variables into parameters, returns, and locals
// based on where they're defined and used relative to the extraction region.
func classifyVariables(decls []varDecl, refs []varRef, startLine, endLine int) *FlowAnalysis {
	// Build a map of declaration info per variable name
	type varInfo struct {
		decl     varDecl
		declared bool
	}
	declMap := make(map[string]varInfo)
	for _, d := range decls {
		if _, exists := declMap[d.name]; !exists {
			declMap[d.name] = varInfo{decl: d, declared: true}
		}
	}

	// Track usage per variable
	type usageInfo struct {
		usedInRegion    bool
		usedAfterRegion bool
		firstUsedAt     int
		usageCount      int
		isModified      bool
	}
	usageMap := make(map[string]*usageInfo)

	for _, ref := range refs {
		u, exists := usageMap[ref.name]
		if !exists {
			u = &usageInfo{firstUsedAt: ref.line}
			usageMap[ref.name] = u
		}
		u.usageCount++
		if ref.isModified {
			u.isModified = true
		}
		if ref.line >= startLine && ref.line <= endLine {
			u.usedInRegion = true
		}
		if ref.line > endLine {
			u.usedAfterRegion = true
		}
		if ref.line < u.firstUsedAt {
			u.firstUsedAt = ref.line
		}
	}

	result := &FlowAnalysis{}

	for name, info := range declMap {
		usage := usageMap[name]
		if usage == nil || !usage.usedInRegion {
			continue
		}

		fv := FlowVariable{
			Name:        name,
			Type:        info.decl.typ,
			DefinedAt:   info.decl.line,
			FirstUsedAt: usage.firstUsedAt,
			UsageCount:  usage.usageCount,
			IsModified:  usage.isModified,
		}

		if info.decl.line < startLine && usage.usedInRegion {
			// Defined before region, used in region → Parameter
			result.Parameters = append(result.Parameters, fv)
		} else if info.decl.line >= startLine && info.decl.line <= endLine {
			if usage.usedAfterRegion {
				// Defined in region, used after → Return
				result.Returns = append(result.Returns, fv)
			} else {
				// Defined in region, not used after → Local
				result.Locals = append(result.Locals, fv)
			}
		}
	}

	return result
}

// findNodes finds all nodes of the given types in the AST.
func findNodes(root *sitter.Node, types []string) []*sitter.Node {
	var result []*sitter.Node
	var walk func(*sitter.Node)
	walk = func(node *sitter.Node) {
		if node == nil {
			return
		}
		for _, t := range types {
			if node.Type() == t {
				result = append(result, node)
				break
			}
		}
		for i := uint32(0); i < node.ChildCount(); i++ {
			walk(node.Child(int(i)))
		}
	}
	walk(root)
	return result
}

// getFunctionNodeTypes returns AST node types for function declarations per language.
func getFunctionNodeTypes(language string) []string {
	switch language {
	case "go":
		return []string{"function_declaration", "method_declaration", "func_literal"}
	case "javascript", "typescript":
		return []string{"function_declaration", "method_definition", "arrow_function", "function"}
	case "python":
		return []string{"function_definition"}
	default:
		return []string{"function_declaration", "method_declaration", "function_definition"}
	}
}

// getDeclarationNodeTypes returns AST node types for variable declarations.
func getDeclarationNodeTypes(language string) []string {
	switch language {
	case "go":
		return []string{"short_var_declaration", "var_declaration", "var_spec"}
	case "javascript", "typescript":
		return []string{"variable_declarator", "lexical_declaration"}
	case "python":
		return []string{"assignment", "augmented_assignment"}
	default:
		return []string{"short_var_declaration", "var_declaration", "variable_declarator", "assignment"}
	}
}

// getParameterNodeTypes returns AST node types for function parameters.
func getParameterNodeTypes(language string) []string {
	switch language {
	case "go":
		return []string{"parameter_declaration"}
	case "javascript", "typescript":
		return []string{"formal_parameters", "required_parameter", "optional_parameter"}
	case "python":
		return []string{"parameters", "default_parameter", "typed_parameter"}
	default:
		return []string{"parameter_declaration", "formal_parameters"}
	}
}

// extractDeclInfo extracts the variable name and type from a declaration node.
func extractDeclInfo(node *sitter.Node, source []byte, language string) (string, string) {
	switch language {
	case "go":
		// short_var_declaration: name := value
		// var_spec: name type = value
		for i := uint32(0); i < node.ChildCount(); i++ {
			child := node.Child(int(i))
			if child.Type() == "identifier" || child.Type() == "expression_list" {
				name := child.Content(source)
				// Try to find type
				for j := i + 1; j < node.ChildCount(); j++ {
					tc := node.Child(int(j))
					if tc.Type() != ":=" && tc.Type() != "=" && tc.Type() != "," {
						return name, tc.Content(source)
					}
				}
				return name, ""
			}
		}
	case "javascript", "typescript":
		for i := uint32(0); i < node.ChildCount(); i++ {
			child := node.Child(int(i))
			if child.Type() == "identifier" {
				return child.Content(source), ""
			}
		}
	case "python":
		if node.ChildCount() > 0 {
			lhs := node.Child(0)
			return lhs.Content(source), ""
		}
	}
	return "", ""
}

// extractParamInfo extracts parameter name and type.
func extractParamInfo(node *sitter.Node, source []byte, language string) (string, string) {
	switch language {
	case "go":
		// parameter_declaration: name type
		var name, typ string
		for i := uint32(0); i < node.ChildCount(); i++ {
			child := node.Child(int(i))
			if child.Type() == "identifier" {
				if name == "" {
					name = child.Content(source)
				}
			} else if name != "" {
				typ = child.Content(source)
			}
		}
		return name, typ
	default:
		for i := uint32(0); i < node.ChildCount(); i++ {
			child := node.Child(int(i))
			if child.Type() == "identifier" {
				return child.Content(source), ""
			}
		}
	}
	return "", ""
}

// isKeyword returns true if the name is a language keyword (not a variable).
func isKeyword(name, language string) bool {
	switch language {
	case "go":
		switch name {
		case "true", "false", "nil", "iota", "append", "cap", "close", "complex",
			"copy", "delete", "imag", "len", "make", "new", "panic", "print",
			"println", "real", "recover":
			return true
		}
	case "javascript", "typescript":
		switch name {
		case "true", "false", "null", "undefined", "NaN", "Infinity",
			"console", "window", "document", "this", "super":
			return true
		}
	case "python":
		switch name {
		case "True", "False", "None", "self", "cls", "print", "len",
			"range", "type", "int", "str", "float", "list", "dict":
			return true
		}
	}
	return false
}

// langToExtension converts a language name to a file extension for LanguageFromExtension.
func langToExtension(lang string) string {
	switch lang {
	case "go":
		return ".go"
	case "typescript":
		return ".ts"
	case "javascript":
		return ".js"
	case "python":
		return ".py"
	case "rust":
		return ".rs"
	case "java":
		return ".java"
	case "kotlin":
		return ".kt"
	default:
		return "." + lang
	}
}
