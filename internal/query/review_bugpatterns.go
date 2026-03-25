//go:build cgo

package query

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/SimplyLiz/CodeMCP/internal/backends/scip"
	"github.com/SimplyLiz/CodeMCP/internal/complexity"
)

// checkBugPatterns runs 8 high-confidence Go AST bug-pattern rules using tree-sitter.
func (e *Engine) checkBugPatterns(ctx context.Context, files []string, opts ReviewPROptions) (ReviewCheck, []ReviewFinding) {
	start := time.Now()

	if !complexity.IsAvailable() {
		return ReviewCheck{
			Name:     "bug-patterns",
			Status:   "skip",
			Severity: "warning",
			Summary:  "Tree-sitter not available (CGO required)",
			Duration: time.Since(start).Milliseconds(),
		}, nil
	}

	parser := complexity.NewParser()
	if parser == nil {
		return ReviewCheck{
			Name:     "bug-patterns",
			Status:   "skip",
			Severity: "warning",
			Summary:  "Could not create tree-sitter parser",
			Duration: time.Since(start).Milliseconds(),
		}, nil
	}

	// Filter to .go files, cap at 20
	var goFiles []string
	for _, f := range files {
		if strings.HasSuffix(f, ".go") && !strings.HasSuffix(f, "_test.go") {
			goFiles = append(goFiles, f)
		}
	}
	skippedFiles := 0
	if len(goFiles) > 20 {
		skippedFiles = len(goFiles) - 20
		goFiles = goFiles[:20]
	}

	var findings []ReviewFinding

	for _, file := range goFiles {
		absPath := filepath.Join(e.repoRoot, file)
		source, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}

		e.tsMu.Lock()
		root, err := parser.Parse(ctx, source, complexity.LangGo)
		e.tsMu.Unlock()
		if err != nil || root == nil {
			continue
		}

		findings = append(findings, checkDeferInLoop(root, source, file)...)
		findings = append(findings, checkUnreachableCode(root, source, file)...)
		findings = append(findings, checkEmptyErrorBranch(root, source, file)...)
		findings = append(findings, checkUncheckedTypeAssert(root, source, file)...)
		findings = append(findings, checkSelfAssignment(root, source, file)...)
		findings = append(findings, checkNilAfterDeref(root, source, file)...)
		findings = append(findings, checkIdenticalBranches(root, source, file)...)
		findings = append(findings, checkShadowedErr(root, source, file)...)
		findings = append(findings, checkDiscardedError(root, source, file)...)
		findings = append(findings, checkMissingDeferClose(root, source, file)...)
	}

	// Assign confidence per rule
	ruleConfidence := map[string]float64{
		"ckb/bug/defer-in-loop":         0.99,
		"ckb/bug/unreachable-code":      0.99,
		"ckb/bug/empty-error-branch":    0.95,
		"ckb/bug/unchecked-type-assert": 0.98,
		"ckb/bug/self-assignment":       0.99,
		"ckb/bug/nil-after-deref":       0.90,
		"ckb/bug/identical-branches":    0.99,
		"ckb/bug/shadowed-err":          0.85,
		"ckb/bug/discarded-error":       0.80,
		"ckb/bug/missing-defer-close":   0.85,
	}
	for i := range findings {
		if conf, ok := ruleConfidence[findings[i].RuleID]; ok {
			findings[i].Confidence = conf
		}
	}

	status := "pass"
	summary := "No bug patterns detected"
	if len(findings) > 0 {
		status = "warn"
		summary = fmt.Sprintf("%d bug pattern(s) detected", len(findings))
	}
	if skippedFiles > 0 {
		summary += fmt.Sprintf(" (%d file(s) over cap, not scanned)", skippedFiles)
	}

	// Build per-rule summary for Details
	ruleCounts := make(map[string]int)
	for _, f := range findings {
		ruleCounts[f.RuleID]++
	}
	type bugPatternSummary struct {
		RuleID string `json:"ruleId"`
		Count  int    `json:"count"`
	}
	var ruleSummaries []bugPatternSummary
	for rule, count := range ruleCounts {
		ruleSummaries = append(ruleSummaries, bugPatternSummary{RuleID: rule, Count: count})
	}

	details := map[string]interface{}{
		"filesScanned": len(goFiles),
		"filesSkipped": skippedFiles,
		"findings":     findings,
	}
	if len(ruleSummaries) > 0 {
		details["byRule"] = ruleSummaries
	}

	return ReviewCheck{
		Name:     "bug-patterns",
		Status:   status,
		Severity: "warning",
		Summary:  summary,
		Details:  details,
		Duration: time.Since(start).Milliseconds(),
	}, findings
}

// --- Individual bug-pattern rules ---

// checkDeferInLoop finds defer statements inside for loops.
func checkDeferInLoop(root *sitter.Node, source []byte, file string) []ReviewFinding {
	var findings []ReviewFinding
	forNodes := complexity.FindNodes(root, []string{"for_statement", "for_range_statement"})
	for _, forNode := range forNodes {
		// Skip func_literal children — a defer inside func(){...}() within a
		// loop is the correct pattern (defer fires at closure return, once per iteration).
		defers := complexity.FindNodesSkipping(forNode, []string{"defer_statement"}, []string{"func_literal"})
		for _, d := range defers {
			findings = append(findings, ReviewFinding{
				Check:      "bug-patterns",
				Severity:   "warning",
				File:       file,
				StartLine:  int(d.StartPoint().Row) + 1,
				Message:    "defer inside loop — deferred call won't execute until function returns, not loop iteration",
				Suggestion: "Move the deferred resource cleanup into a closure or helper function",
				Category:   "bug",
				RuleID:     "ckb/bug/defer-in-loop",
			})
		}
	}
	return findings
}

// checkUnreachableCode finds statements after return/panic in the same block.
func checkUnreachableCode(root *sitter.Node, source []byte, file string) []ReviewFinding {
	var findings []ReviewFinding
	blocks := complexity.FindNodes(root, []string{"block"})
	for _, block := range blocks {
		foundTerminator := false
		for i := uint32(0); i < block.ChildCount(); i++ {
			child := block.Child(int(i))
			if child == nil {
				continue
			}
			if child.Type() == "{" || child.Type() == "}" || child.Type() == "\n" || child.Type() == "comment" {
				continue
			}
			if foundTerminator {
				// This is a statement after a terminator
				findings = append(findings, ReviewFinding{
					Check:     "bug-patterns",
					Severity:  "warning",
					File:      file,
					StartLine: int(child.StartPoint().Row) + 1,
					Message:   "Unreachable code after return/panic",
					Category:  "bug",
					RuleID:    "ckb/bug/unreachable-code",
				})
				break // Only report first unreachable statement per block
			}
			if child.Type() == "return_statement" {
				foundTerminator = true
			} else if child.Type() == "expression_statement" {
				// Check for panic() calls
				callNodes := complexity.FindNodes(child, []string{"call_expression"})
				for _, call := range callNodes {
					fnNode := call.ChildByFieldName("function")
					if fnNode != nil && string(source[fnNode.StartByte():fnNode.EndByte()]) == "panic" {
						foundTerminator = true
					}
				}
			}
		}
	}
	return findings
}

// checkEmptyErrorBranch finds `if err != nil { }` with empty body.
func checkEmptyErrorBranch(root *sitter.Node, source []byte, file string) []ReviewFinding {
	var findings []ReviewFinding
	ifStmts := complexity.FindNodes(root, []string{"if_statement"})
	for _, ifNode := range ifStmts {
		cond := ifNode.ChildByFieldName("condition")
		if cond == nil {
			continue
		}
		condText := string(source[cond.StartByte():cond.EndByte()])
		if !strings.Contains(condText, "err") || !strings.Contains(condText, "nil") {
			continue
		}

		consequence := ifNode.ChildByFieldName("consequence")
		if consequence == nil {
			continue
		}
		// Check if block is empty (only { and })
		stmtCount := 0
		for i := uint32(0); i < consequence.ChildCount(); i++ {
			child := consequence.Child(int(i))
			if child != nil && child.Type() != "{" && child.Type() != "}" {
				stmtCount++
			}
		}
		if stmtCount == 0 {
			findings = append(findings, ReviewFinding{
				Check:      "bug-patterns",
				Severity:   "warning",
				File:       file,
				StartLine:  int(ifNode.StartPoint().Row) + 1,
				Message:    "Empty error handling branch — error is checked but silently ignored",
				Suggestion: "Handle the error or add a comment explaining why it's safe to ignore",
				Category:   "bug",
				RuleID:     "ckb/bug/empty-error-branch",
			})
		}
	}
	return findings
}

// checkUncheckedTypeAssert finds type assertions not in 2-value assignments (x.(T) without ok check).
func checkUncheckedTypeAssert(root *sitter.Node, source []byte, file string) []ReviewFinding {
	var findings []ReviewFinding
	typeAsserts := complexity.FindNodes(root, []string{"type_assertion_expression"})
	for _, ta := range typeAsserts {
		// Walk up to see if an ancestor is a multi-value assignment.
		// AST shape: short_var_declaration > expression_list > type_assertion_expression
		// So we check parent and grandparent.
		if isCheckedTypeAssert(ta) {
			continue
		}
		findings = append(findings, ReviewFinding{
			Check:      "bug-patterns",
			Severity:   "warning",
			File:       file,
			StartLine:  int(ta.StartPoint().Row) + 1,
			Message:    "Unchecked type assertion — will panic if type doesn't match",
			Suggestion: "Use two-value form: val, ok := x.(T)",
			Category:   "bug",
			RuleID:     "ckb/bug/unchecked-type-assert",
		})
	}
	return findings
}

// isCheckedTypeAssert returns true if the type assertion is in a two-value
// assignment (val, ok := x.(T)). The AST nests the assertion inside an
// expression_list, so we check parent and grandparent.
func isCheckedTypeAssert(ta *sitter.Node) bool {
	for n := ta.Parent(); n != nil; n = n.Parent() {
		switch n.Type() {
		case "short_var_declaration", "assignment_statement":
			left := n.ChildByFieldName("left")
			if left == nil {
				return false
			}
			idCount := 0
			for i := uint32(0); i < left.ChildCount(); i++ {
				child := left.Child(int(i))
				if child != nil && (child.Type() == "identifier" || child.Type() == "blank_identifier") {
					idCount++
				}
			}
			return idCount >= 2
		case "expression_list":
			// Keep walking up — the expression_list sits between the
			// type_assertion_expression and the declaration/assignment.
			continue
		default:
			return false
		}
	}
	return false
}

// checkSelfAssignment finds assignments where LHS == RHS.
func checkSelfAssignment(root *sitter.Node, source []byte, file string) []ReviewFinding {
	var findings []ReviewFinding
	assignments := complexity.FindNodes(root, []string{"assignment_statement"})
	for _, assign := range assignments {
		left := assign.ChildByFieldName("left")
		right := assign.ChildByFieldName("right")
		if left == nil || right == nil {
			continue
		}
		leftText := strings.TrimSpace(string(source[left.StartByte():left.EndByte()]))
		rightText := strings.TrimSpace(string(source[right.StartByte():right.EndByte()]))
		if leftText == rightText && leftText != "" {
			findings = append(findings, ReviewFinding{
				Check:     "bug-patterns",
				Severity:  "warning",
				File:      file,
				StartLine: int(assign.StartPoint().Row) + 1,
				Message:   fmt.Sprintf("Self-assignment: %s = %s", leftText, rightText),
				Category:  "bug",
				RuleID:    "ckb/bug/self-assignment",
			})
		}
	}
	return findings
}

// checkNilAfterDeref finds patterns where a variable is dereferenced (used in selector_expression)
// before being checked for nil.
func checkNilAfterDeref(root *sitter.Node, source []byte, file string) []ReviewFinding {
	var findings []ReviewFinding
	// Look at function bodies
	funcBodies := complexity.FindNodes(root, []string{"function_declaration", "method_declaration", "func_literal"})
	for _, fn := range funcBodies {
		body := fn.ChildByFieldName("body")
		if body == nil {
			continue
		}
		// Track first dereference and first nil check per variable in this function
		derefLines := make(map[string]int)    // var -> first deref line
		nilCheckLines := make(map[string]int) // var -> first nil check line

		var walk func(node *sitter.Node)
		walk = func(node *sitter.Node) {
			if node == nil {
				return
			}
			line := int(node.StartPoint().Row) + 1

			if node.Type() == "selector_expression" {
				operand := node.ChildByFieldName("operand")
				if operand != nil {
					name := string(source[operand.StartByte():operand.EndByte()])
					if _, ok := derefLines[name]; !ok {
						derefLines[name] = line
					}
				}
			}

			if node.Type() == "if_statement" {
				cond := node.ChildByFieldName("condition")
				if cond != nil {
					condText := string(source[cond.StartByte():cond.EndByte()])
					if strings.Contains(condText, "!= nil") || strings.Contains(condText, "== nil") {
						// Extract the variable being checked
						parts := strings.Fields(condText)
						if len(parts) >= 1 {
							varName := parts[0]
							if _, ok := nilCheckLines[varName]; !ok {
								nilCheckLines[varName] = line
							}
						}
					}
				}
			}

			for i := uint32(0); i < node.ChildCount(); i++ {
				walk(node.Child(int(i)))
			}
		}
		walk(body)

		// Report cases where deref comes before nil check
		for varName, derefLine := range derefLines {
			if nilLine, ok := nilCheckLines[varName]; ok && derefLine < nilLine {
				findings = append(findings, ReviewFinding{
					Check:      "bug-patterns",
					Severity:   "warning",
					File:       file,
					StartLine:  derefLine,
					Message:    fmt.Sprintf("Variable '%s' dereferenced before nil check (nil check on line %d)", varName, nilLine),
					Suggestion: "Move the nil check before the first use",
					Category:   "bug",
					RuleID:     "ckb/bug/nil-after-deref",
				})
			}
		}
	}
	return findings
}

// checkIdenticalBranches finds if/else where both branches have identical source text.
func checkIdenticalBranches(root *sitter.Node, source []byte, file string) []ReviewFinding {
	var findings []ReviewFinding
	ifStmts := complexity.FindNodes(root, []string{"if_statement"})
	for _, ifNode := range ifStmts {
		consequence := ifNode.ChildByFieldName("consequence")
		alternative := ifNode.ChildByFieldName("alternative")
		if consequence == nil || alternative == nil {
			continue
		}
		// The alternative might be an else block or else-if
		if alternative.Type() != "block" {
			continue
		}
		consText := strings.TrimSpace(string(source[consequence.StartByte():consequence.EndByte()]))
		altText := strings.TrimSpace(string(source[alternative.StartByte():alternative.EndByte()]))
		if consText == altText && consText != "{}" && consText != "{ }" {
			findings = append(findings, ReviewFinding{
				Check:     "bug-patterns",
				Severity:  "warning",
				File:      file,
				StartLine: int(ifNode.StartPoint().Row) + 1,
				Message:   "Identical if/else branches — both branches do the same thing",
				Category:  "bug",
				RuleID:    "ckb/bug/identical-branches",
			})
		}
	}
	return findings
}

// checkShadowedErr finds `:=` redeclarations of `err` in inner blocks
// when `err` is already declared in an outer scope within the same function.
func checkShadowedErr(root *sitter.Node, source []byte, file string) []ReviewFinding {
	var findings []ReviewFinding
	funcBodies := complexity.FindNodes(root, []string{"function_declaration", "method_declaration", "func_literal"})
	for _, fn := range funcBodies {
		body := fn.ChildByFieldName("body")
		if body == nil {
			continue
		}

		// Find all short var declarations of err and their nesting depth
		type errDecl struct {
			line  int
			depth int
		}
		var errDecls []errDecl

		var walk func(node *sitter.Node, depth int)
		walk = func(node *sitter.Node, depth int) {
			if node == nil {
				return
			}
			if node.Type() == "block" && node != body {
				depth++
			}
			if node.Type() == "short_var_declaration" {
				left := node.ChildByFieldName("left")
				if left != nil {
					leftText := string(source[left.StartByte():left.EndByte()])
					// Check if any of the declared vars is "err"
					for _, part := range strings.Split(leftText, ",") {
						if strings.TrimSpace(part) == "err" {
							errDecls = append(errDecls, errDecl{
								line:  int(node.StartPoint().Row) + 1,
								depth: depth,
							})
							break
						}
					}
				}
			}
			for i := uint32(0); i < node.ChildCount(); i++ {
				walk(node.Child(int(i)), depth)
			}
		}
		walk(body, 0)

		// Report inner declarations that shadow outer ones
		for i, inner := range errDecls {
			for j, outer := range errDecls {
				if i != j && inner.depth > outer.depth && inner.line > outer.line {
					findings = append(findings, ReviewFinding{
						Check:      "bug-patterns",
						Severity:   "info",
						File:       file,
						StartLine:  inner.line,
						Message:    fmt.Sprintf("'err' shadowed — redeclared with := at depth %d (outer declaration at line %d)", inner.depth, outer.line),
						Suggestion: "Use = instead of := to avoid shadowing the outer err variable",
						Category:   "bug",
						RuleID:     "ckb/bug/shadowed-err",
					})
					break // Only report once per inner declaration
				}
			}
		}
	}
	return findings
}

// checkDiscardedError finds function calls whose return values are completely discarded,
// where the function likely returns an error. It tracks variable declarations within
// each function body to suppress false positives for types like strings.Builder and
// bytes.Buffer whose Write methods never return non-nil errors.
func checkDiscardedError(root *sitter.Node, source []byte, file string) []ReviewFinding {
	var findings []ReviewFinding

	// Process each function body separately so we can track variable types.
	funcBodies := complexity.FindNodes(root, []string{"function_declaration", "method_declaration", "func_literal"})
	for _, fn := range funcBodies {
		body := fn.ChildByFieldName("body")
		if body == nil {
			continue
		}

		// Build a map of variable names to their declared types within this function.
		// For closures (func_literal), also include vars from enclosing scopes.
		varTypes := buildVarTypeMap(body, source)
		if fn.Type() == "func_literal" {
			mergeEnclosingVarTypes(fn, source, varTypes)
		}

		// Find discarded calls in this function body.
		// Skip func_literal children — closures are processed as separate funcBodies above,
		// so we must not recurse into them here (their internal calls are properly handled).
		exprStmts := complexity.FindNodesSkipping(body, []string{"expression_statement"}, []string{"func_literal"})
		for _, stmt := range exprStmts {
			// Also skip func_literals when finding calls — an IIFE like func(){...}()
			// is a call_expression containing a func_literal; we must not recurse into
			// the closure body and flag its internal (properly handled) calls.
			calls := complexity.FindNodesSkipping(stmt, []string{"call_expression"}, []string{"func_literal"})
			for _, call := range calls {
				// Skip nested calls whose return value IS consumed (e.g., Register(NewFramework()))
				// A call is "discarded" only if its parent is the expression_statement itself,
				// not if it's inside an argument_list of another call.
				if call.Parent() != nil && call.Parent().Type() == "argument_list" {
					continue
				}

				fnNode := call.ChildByFieldName("function")
				if fnNode == nil {
					continue
				}
				fullName := string(source[fnNode.StartByte():fnNode.EndByte()])

				// Check if this is a selector expression (e.g., "b.WriteString")
				// and suppress if the receiver is a known infallible-write type.
				if fnNode.Type() == "selector_expression" {
					receiver, method := splitSelector(fullName)
					if isInfallibleCall(receiver, method, varTypes) {
						continue
					}
					// Suppress standalone .Close() calls — discarding Close() errors on
					// read-only file handles is standard Go convention (e.g., f.Close()
					// after os.Open for reading). Write-path Close errors are caught by
					// the missing-defer-close rule instead.
					if method == "Close" {
						continue
					}
				}

				// Extract the simple name (last segment of selector)
				simpleName := fullName
				if idx := strings.LastIndex(fullName, "."); idx >= 0 {
					simpleName = fullName[idx+1:]
				}
				if scip.LikelyReturnsError(simpleName) {
					findings = append(findings, ReviewFinding{
						Check:      "bug-patterns",
						Severity:   "warning",
						File:       file,
						StartLine:  int(stmt.StartPoint().Row) + 1,
						Message:    fmt.Sprintf("Discarded return value from '%s' which likely returns an error", simpleName),
						Suggestion: "Capture and handle the error: err := " + string(source[call.StartByte():call.EndByte()]),
						Category:   "bug",
						RuleID:     "ckb/bug/discarded-error",
					})
				}
			}
		}
	}
	return findings
}

// infallibleWriteTypes are types whose Write methods never return non-nil errors.
// hash.Hash.Write is documented as "It never returns an error" in the Go stdlib.
var infallibleWriteTypes = map[string]bool{
	"strings.Builder": true,
	"bytes.Buffer":    true,
	"hash.Hash":       true,
}

// infallibleMethods are methods that never error on infallible-write types.
var infallibleMethods = map[string]bool{
	"WriteString": true,
	"WriteByte":   true,
	"WriteRune":   true,
	"Write":       true,
	"Grow":        true,
	"Reset":       true,
}

// buildVarTypeMap scans a function body for variable declarations and maps
// variable names to their type strings (e.g., "b" -> "strings.Builder").
func buildVarTypeMap(body *sitter.Node, source []byte) map[string]string {
	result := make(map[string]string)

	// Find var declarations: var b strings.Builder
	varDecls := complexity.FindNodes(body, []string{"var_declaration"})
	for _, decl := range varDecls {
		specs := complexity.FindNodes(decl, []string{"var_spec"})
		for _, spec := range specs {
			nameNode := spec.ChildByFieldName("name")
			typeNode := spec.ChildByFieldName("type")
			if nameNode != nil && typeNode != nil {
				name := string(source[nameNode.StartByte():nameNode.EndByte()])
				typeName := string(source[typeNode.StartByte():typeNode.EndByte()])
				result[name] = typeName
			}
		}
	}

	// Find short var declarations: b := strings.Builder{}, b := &bytes.Buffer{}, etc.
	shortDecls := complexity.FindNodes(body, []string{"short_var_declaration"})
	for _, decl := range shortDecls {
		left := decl.ChildByFieldName("left")
		right := decl.ChildByFieldName("right")
		if left == nil || right == nil {
			continue
		}

		varName := strings.TrimSpace(string(source[left.StartByte():left.EndByte()]))
		// Handle multi-value: take first var before comma
		if idx := strings.Index(varName, ","); idx >= 0 {
			varName = strings.TrimSpace(varName[:idx])
		}

		rightText := strings.TrimSpace(string(source[right.StartByte():right.EndByte()]))

		if strings.Contains(rightText, "strings.Builder") {
			result[varName] = "strings.Builder"
		} else if strings.Contains(rightText, "bytes.Buffer") {
			result[varName] = "bytes.Buffer"
		} else if strings.Contains(rightText, "bytes.NewBuffer") || strings.Contains(rightText, "bytes.NewBufferString") {
			result[varName] = "bytes.Buffer"
		} else if strings.Contains(rightText, "new(bytes.Buffer)") {
			result[varName] = "bytes.Buffer"
		} else if strings.Contains(rightText, "new(strings.Builder)") {
			result[varName] = "strings.Builder"
		} else if strings.Contains(rightText, "md5.New()") ||
			strings.Contains(rightText, "sha1.New()") ||
			strings.Contains(rightText, "sha256.New()") ||
			strings.Contains(rightText, "sha512.New()") ||
			strings.Contains(rightText, "fnv.New") ||
			strings.Contains(rightText, "crc32.New") ||
			strings.Contains(rightText, "hmac.New(") {
			result[varName] = "hash.Hash"
		}
	}

	return result
}

// mergeEnclosingVarTypes walks up from a func_literal to find the enclosing
// function's variable declarations. This catches cases like:
//
//	var rawContent strings.Builder
//	provider.GenerateStream(ctx, prompt, func(chunk string) {
//	    rawContent.WriteString(chunk)  // closure captures outer var
//	})
func mergeEnclosingVarTypes(closureNode *sitter.Node, source []byte, varTypes map[string]string) {
	for n := closureNode.Parent(); n != nil; n = n.Parent() {
		if n.Type() == "function_declaration" || n.Type() == "method_declaration" {
			body := n.ChildByFieldName("body")
			if body != nil {
				enclosing := buildVarTypeMap(body, source)
				for k, v := range enclosing {
					if _, exists := varTypes[k]; !exists {
						varTypes[k] = v
					}
				}
			}
			return
		}
	}
}

// splitSelector splits "b.WriteString" into ("b", "WriteString").
func splitSelector(fullName string) (receiver, method string) {
	idx := strings.LastIndex(fullName, ".")
	if idx < 0 {
		return "", fullName
	}
	return fullName[:idx], fullName[idx+1:]
}

// isInfallibleCall returns true if this is a call on a type whose method never errors.
func isInfallibleCall(receiver, method string, varTypes map[string]string) bool {
	if !infallibleMethods[method] {
		return false
	}
	typeName, ok := varTypes[receiver]
	if !ok {
		return false
	}
	return infallibleWriteTypes[typeName]
}

// checkMissingDeferClose finds calls to Open/Create/Dial/NewReader where the returned
// resource is not closed with a deferred Close() call in the same function.
func checkMissingDeferClose(root *sitter.Node, source []byte, file string) []ReviewFinding {
	var findings []ReviewFinding
	// Resource-opening function names
	openFuncs := map[string]bool{
		"Open": true, "OpenFile": true, "Create": true,
		"Dial": true, "DialContext": true, "NewReader": true,
		"NewWriter": true, "NewFile": true,
		// Note: NewScanner (bufio.Scanner) is NOT included — Scanner doesn't implement io.Closer
	}

	funcBodies := complexity.FindNodes(root, []string{"function_declaration", "method_declaration", "func_literal"})
	for _, fn := range funcBodies {
		body := fn.ChildByFieldName("body")
		if body == nil {
			continue
		}

		// Find short_var_declarations with resource-opening calls.
		// Skip func_literal children — closures are processed separately as funcBodies.
		shortDecls := complexity.FindNodesSkipping(body, []string{"short_var_declaration"}, []string{"func_literal"})
		for _, decl := range shortDecls {
			right := decl.ChildByFieldName("right")
			if right == nil {
				continue
			}
			calls := complexity.FindNodes(right, []string{"call_expression"})
			for _, call := range calls {
				fnNode := call.ChildByFieldName("function")
				if fnNode == nil {
					continue
				}
				fnName := string(source[fnNode.StartByte():fnNode.EndByte()])
				if idx := strings.LastIndex(fnName, "."); idx >= 0 {
					fnName = fnName[idx+1:]
				}
				if !openFuncs[fnName] {
					continue
				}

				// Get the variable name from LHS
				left := decl.ChildByFieldName("left")
				if left == nil {
					continue
				}
				leftText := string(source[left.StartByte():left.EndByte()])
				// Get first identifier (before comma)
				varName := strings.Split(leftText, ",")[0]
				varName = strings.TrimSpace(varName)
				if varName == "_" || varName == "" {
					continue
				}

				// Check if there's a defer <varName>.Close() in the same function body
				bodyText := string(source[body.StartByte():body.EndByte()])
				hasClose := strings.Contains(bodyText, "defer "+varName+".Close()") ||
					strings.Contains(bodyText, "defer func() {") || // common pattern with anon func
					strings.Contains(bodyText, varName+".Close()") // inline close at end of loop/block
				if !hasClose {
					findings = append(findings, ReviewFinding{
						Check:      "bug-patterns",
						Severity:   "warning",
						File:       file,
						StartLine:  int(decl.StartPoint().Row) + 1,
						Message:    fmt.Sprintf("Resource from '%s' assigned to '%s' without defer Close()", fnName, varName),
						Suggestion: fmt.Sprintf("Add: defer %s.Close()", varName),
						Category:   "bug",
						RuleID:     "ckb/bug/missing-defer-close",
					})
				}
			}
		}
	}
	return findings
}

// checkBugPatternsWithDiff wraps checkBugPatterns and filters out findings
// that already existed in the base branch, reporting only genuinely new issues.
func (e *Engine) checkBugPatternsWithDiff(ctx context.Context, files []string, opts ReviewPROptions) (ReviewCheck, []ReviewFinding) {
	check, headFindings := e.checkBugPatterns(ctx, files, opts)
	if len(headFindings) == 0 || opts.BaseBranch == "" {
		return check, headFindings
	}

	parser := complexity.NewParser()
	if parser == nil {
		return check, headFindings
	}

	// Build base findings count keyed by ruleID + file + message.
	// Using counts (not a set) so that if the head introduces a second
	// instance of the same rule+message in the same file, we still report it.
	baseCounts := make(map[string]int)
	for _, file := range files {
		if !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
			continue
		}

		// Get base version via git show (runs without tsMu)
		cmd := exec.CommandContext(ctx, "git", "-C", e.repoRoot, "show", opts.BaseBranch+":"+file)
		baseSource, err := cmd.Output()
		if err != nil {
			continue // New file — all findings are new
		}

		e.tsMu.Lock()
		baseRoot, err := parser.Parse(ctx, baseSource, complexity.LangGo)
		e.tsMu.Unlock()
		if err != nil || baseRoot == nil {
			continue
		}

		// Run all rules on base
		var baseFindings []ReviewFinding
		baseFindings = append(baseFindings, checkDeferInLoop(baseRoot, baseSource, file)...)
		baseFindings = append(baseFindings, checkUnreachableCode(baseRoot, baseSource, file)...)
		baseFindings = append(baseFindings, checkEmptyErrorBranch(baseRoot, baseSource, file)...)
		baseFindings = append(baseFindings, checkUncheckedTypeAssert(baseRoot, baseSource, file)...)
		baseFindings = append(baseFindings, checkSelfAssignment(baseRoot, baseSource, file)...)
		baseFindings = append(baseFindings, checkNilAfterDeref(baseRoot, baseSource, file)...)
		baseFindings = append(baseFindings, checkIdenticalBranches(baseRoot, baseSource, file)...)
		baseFindings = append(baseFindings, checkShadowedErr(baseRoot, baseSource, file)...)
		baseFindings = append(baseFindings, checkDiscardedError(baseRoot, baseSource, file)...)
		baseFindings = append(baseFindings, checkMissingDeferClose(baseRoot, baseSource, file)...)

		for _, bf := range baseFindings {
			key := bugPatternKey(bf)
			baseCounts[key]++
		}
	}

	// Filter: for each key, only report head findings beyond the base count
	headSeen := make(map[string]int)
	var newFindings []ReviewFinding
	for _, f := range headFindings {
		key := bugPatternKey(f)
		headSeen[key]++
		if headSeen[key] > baseCounts[key] {
			newFindings = append(newFindings, f)
		}
	}

	// Update check summary
	if len(newFindings) == 0 && len(headFindings) > 0 {
		check.Status = "pass"
		check.Summary = fmt.Sprintf("No new bug patterns (%d pre-existing)", len(headFindings))
	} else if len(newFindings) < len(headFindings) {
		check.Summary = fmt.Sprintf("%d new bug pattern(s) (%d pre-existing filtered)", len(newFindings), len(headFindings)-len(newFindings))
	}

	return check, newFindings
}

// bugPatternKey creates a stable key for deduplication that survives line shifts.
// Uses ruleID + file + message content (which includes function/variable names).
func bugPatternKey(f ReviewFinding) string {
	return f.RuleID + ":" + f.File + ":" + f.Message
}
