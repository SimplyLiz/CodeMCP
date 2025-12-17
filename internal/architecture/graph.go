package architecture

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"ckb/internal/modules"
	"ckb/internal/paths"
)

// getGoModulePath reads the module path from go.mod in the repo root
func getGoModulePath(repoRoot string) string {
	goModPath := filepath.Join(repoRoot, "go.mod")
	file, err := os.Open(goModPath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

// BuildDependencyGraph creates edges between modules based on imports
func (g *ArchitectureGenerator) BuildDependencyGraph(
	mods []*modules.Module,
	importsByModule map[string][]*modules.ImportEdge,
	opts *GeneratorOptions,
) ([]DependencyEdge, error) {
	// Create module lookup maps
	moduleByPath := make(map[string]*modules.Module)
	moduleById := make(map[string]*modules.Module)

	for _, mod := range mods {
		moduleById[mod.ID] = mod
		// Normalize the root path for lookup
		normalizedPath := paths.NormalizePath(mod.RootPath)
		moduleByPath[normalizedPath] = mod
	}

	// Track edges with aggregated strength
	edgeMap := make(map[string]*DependencyEdge) // key: "from:to"

	// Process imports for each module
	for fromModuleId, imports := range importsByModule {
		fromModule, ok := moduleById[fromModuleId]
		if !ok {
			continue
		}

		for _, importEdge := range imports {
			// Classify the import to determine the target module
			toModuleId, kind := g.classifyImport(importEdge, fromModule, mods, moduleByPath)

			// Skip if we couldn't resolve the target
			if toModuleId == "" {
				continue
			}

			// Skip external dependencies unless explicitly requested
			if !opts.IncludeExternalDeps && kind == modules.ExternalDependency {
				continue
			}

			// Skip stdlib imports
			if kind == modules.Stdlib {
				continue
			}

			// Create or update edge
			edgeKey := fromModuleId + ":" + toModuleId
			if edge, exists := edgeMap[edgeKey]; exists {
				edge.Strength++
			} else {
				edgeMap[edgeKey] = &DependencyEdge{
					From:     fromModuleId,
					To:       toModuleId,
					Kind:     kind,
					Strength: 1,
				}
			}
		}
	}

	// Convert map to slice
	edges := make([]DependencyEdge, 0, len(edgeMap))
	for _, edge := range edgeMap {
		edges = append(edges, *edge)
	}

	return edges, nil
}

// classifyImport classifies an import and determines the target module
// Returns (targetModuleId, importKind)
func (g *ArchitectureGenerator) classifyImport(
	importEdge *modules.ImportEdge,
	fromModule *modules.Module,
	allModules []*modules.Module,
	moduleByPath map[string]*modules.Module,
) (string, modules.ImportEdgeKind) {
	importPath := importEdge.To

	// For Go: use go.mod module path to classify imports
	// - Contains dot (.) → external dependency
	// - Starts with module path (e.g., "ckb/") → local workspace
	// - Otherwise → stdlib
	if fromModule.Language == modules.LanguageGo {
		goModPath := getGoModulePath(g.repoRoot)

		// External: contains a domain (has dot)
		if strings.Contains(importPath, ".") {
			externalModuleId := "external:" + extractPackageName(importPath)
			return externalModuleId, modules.ExternalDependency
		}

		// Local workspace: starts with our module path
		if goModPath != "" && strings.HasPrefix(importPath, goModPath+"/") {
			// Try to match to a specific module
			for _, mod := range allModules {
				if mod.ID == fromModule.ID {
					continue
				}
				if strings.HasSuffix(importPath, "/"+mod.RootPath) ||
					strings.HasSuffix(importPath, mod.RootPath) {
					return mod.ID, modules.WorkspacePackage
				}
			}
			// Local but not matching a specific module
			return "", modules.Unknown
		}

		// No dot and not local → stdlib
		return "", modules.Stdlib
	}

	// Non-Go languages: use existing logic
	if isStdlibImport(importPath, fromModule.Language) {
		return "", modules.Stdlib
	}

	// Check for relative imports (./foo, ../bar)
	if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
		fromFilePath := importEdge.From
		fromDir := filepath.Dir(fromFilePath)
		resolvedPath := filepath.Join(fromDir, importPath)
		resolvedPath = paths.NormalizePath(resolvedPath)

		targetModule := findModuleForPath(resolvedPath, allModules)
		if targetModule != nil {
			if targetModule.ID == fromModule.ID {
				return targetModule.ID, modules.LocalFile
			}
			return targetModule.ID, modules.LocalModule
		}
		return "", modules.Unknown
	}

	// Check for workspace packages
	for _, mod := range allModules {
		if mod.ID == fromModule.ID {
			continue
		}
		if strings.HasPrefix(importPath, mod.Name) {
			return mod.ID, modules.WorkspacePackage
		}
	}

	// Default to external dependency
	externalModuleId := "external:" + extractPackageName(importPath)
	return externalModuleId, modules.ExternalDependency
}

// isStdlibImport checks if an import is from the standard library (non-Go languages)
// Note: Go stdlib detection is handled in classifyImport using go.mod module path
func isStdlibImport(importPath string, language string) bool {
	switch language {
	case modules.LanguageDart:
		return strings.HasPrefix(importPath, "dart:")
	case modules.LanguageTypeScript, modules.LanguageJavaScript:
		return strings.HasPrefix(importPath, "node:")
	case modules.LanguagePython:
		// Best-effort heuristic for common Python stdlib modules
		stdlibModules := map[string]bool{
			"os": true, "sys": true, "json": true, "re": true,
			"datetime": true, "collections": true, "itertools": true,
			"functools": true, "typing": true, "pathlib": true,
		}
		pkgName := strings.Split(importPath, ".")[0]
		return stdlibModules[pkgName]
	}
	return false
}

// findModuleForPath finds the module that contains a given path
func findModuleForPath(path string, allModules []*modules.Module) *modules.Module {
	normalizedPath := paths.NormalizePath(path)

	// Find the module with the longest matching root path
	var bestMatch *modules.Module
	bestMatchLen := 0

	for _, mod := range allModules {
		normalizedRoot := paths.NormalizePath(mod.RootPath)
		if strings.HasPrefix(normalizedPath, normalizedRoot) {
			if len(normalizedRoot) > bestMatchLen {
				bestMatch = mod
				bestMatchLen = len(normalizedRoot)
			}
		}
	}

	return bestMatch
}

// extractPackageName extracts the package name from an import path
func extractPackageName(importPath string) string {
	// For scoped packages like @scope/package
	if strings.HasPrefix(importPath, "@") {
		parts := strings.SplitN(importPath, "/", 3)
		if len(parts) >= 2 {
			return parts[0] + "/" + parts[1]
		}
	}

	// For regular packages
	parts := strings.Split(importPath, "/")
	if len(parts) > 0 {
		return parts[0]
	}

	return importPath
}

// FilterExternalDeps removes external dependencies from the edge list
func FilterExternalDeps(edges []DependencyEdge) []DependencyEdge {
	filtered := make([]DependencyEdge, 0, len(edges))

	for _, edge := range edges {
		if edge.Kind != modules.ExternalDependency {
			filtered = append(filtered, edge)
		}
	}

	return filtered
}

// ComputeStrength calculates edge strength from import count
func ComputeStrength(imports []*modules.ImportEdge) int {
	return len(imports)
}
