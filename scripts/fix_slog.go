//go:build ignore

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run fix_slog.go <directory>")
		os.Exit(1)
	}

	root := os.Args[1]
	var fixed int

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "vendor" || info.Name() == "scripts" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		changed, err := fixFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", path, err)
			return nil
		}
		if changed {
			fixed++
			fmt.Printf("Fixed: %s\n", path)
		}
		return nil
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nFixed %d files\n", fixed)
}

func fixFile(path string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	// Quick check if file contains the pattern
	if !bytes.Contains(content, []byte("map[string]interface{}")) {
		return false, nil
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return false, err
	}

	changed := false
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check if it's a logger method call
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		method := sel.Sel.Name
		if method != "Debug" && method != "Info" && method != "Warn" && method != "Error" {
			return true
		}

		// Check for map[string]interface{} argument (typically the last argument)
		for i := 1; i < len(call.Args); i++ {
			comp, ok := call.Args[i].(*ast.CompositeLit)
			if !ok {
				continue
			}

			// Check if it's map[string]interface{}
			mapType, ok := comp.Type.(*ast.MapType)
			if !ok {
				continue
			}

			keyIdent, ok := mapType.Key.(*ast.Ident)
			if !ok || keyIdent.Name != "string" {
				continue
			}

			valueIface, ok := mapType.Value.(*ast.InterfaceType)
			if !ok || valueIface.Methods == nil || len(valueIface.Methods.List) != 0 {
				continue
			}

			// Convert map literal to variadic args
			newArgs := call.Args[:i]
			for _, elt := range comp.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}

				// Key should be a string literal
				keyLit, ok := kv.Key.(*ast.BasicLit)
				if !ok || keyLit.Kind != token.STRING {
					continue
				}

				newArgs = append(newArgs, keyLit, kv.Value)
			}

			// Add any remaining args after the map
			newArgs = append(newArgs, call.Args[i+1:]...)
			call.Args = newArgs
			changed = true
		}

		return true
	})

	if !changed {
		return false, nil
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return false, err
	}

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return false, err
	}

	return true, nil
}
