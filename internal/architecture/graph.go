package architecture

import (
	"path/filepath"
	"strings"

	"ckb/internal/modules"
	"ckb/internal/paths"
)

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

	// Check for stdlib imports
	if isStdlibImport(importPath, fromModule.Language) {
		return "", modules.Stdlib
	}

	// Check for relative imports (./foo, ../bar)
	if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
		// Resolve relative path
		fromFilePath := importEdge.From
		fromDir := filepath.Dir(fromFilePath)
		resolvedPath := filepath.Join(fromDir, importPath)
		resolvedPath = paths.NormalizePath(resolvedPath)

		// Find which module this resolves to
		targetModule := findModuleForPath(resolvedPath, allModules)
		if targetModule != nil {
			if targetModule.ID == fromModule.ID {
				return targetModule.ID, modules.LocalFile
			}
			return targetModule.ID, modules.LocalModule
		}
		return "", modules.Unknown
	}

	// Check for workspace packages (imports within the monorepo)
	for _, mod := range allModules {
		if mod.ID == fromModule.ID {
			continue
		}

		// Check if import matches module name or is a subpath
		if strings.HasPrefix(importPath, mod.Name) {
			return mod.ID, modules.WorkspacePackage
		}

		// For Go imports: check if import path ends with or contains the module's root path
		// e.g., import "ckb/internal/config" should match module with RootPath "internal/config"
		if fromModule.Language == modules.LanguageGo {
			if strings.HasSuffix(importPath, "/"+mod.RootPath) ||
				strings.HasSuffix(importPath, mod.RootPath) {
				return mod.ID, modules.WorkspacePackage
			}
		}
	}

	// Default to external dependency
	// Create a synthetic module ID for external dependencies
	externalModuleId := "external:" + extractPackageName(importPath)
	return externalModuleId, modules.ExternalDependency
}

// isStdlibImport checks if an import is from the standard library
func isStdlibImport(importPath string, language string) bool {
	switch language {
	case modules.LanguageDart:
		return strings.HasPrefix(importPath, "dart:")
	case modules.LanguageGo:
		// Go stdlib packages don't have dots (external) AND don't have slashes (internal)
		// Examples:
		//   "fmt", "context", "os" - stdlib (no dots, no slashes)
		//   "path/filepath", "encoding/json" - stdlib (no dots, has slashes but is standard)
		//   "github.com/foo/bar" - external (has dots)
		//   "ckb/internal/config" - local (no dots, but has slashes indicating project path)
		// The key insight: stdlib packages either have no slashes OR have specific patterns
		// like "encoding/json", "net/http", etc.
		if strings.Contains(importPath, ".") {
			return false // External package (has domain)
		}
		// Check if it looks like a project-local import (contains certain patterns)
		if strings.Contains(importPath, "/internal/") ||
			strings.Contains(importPath, "/cmd/") ||
			strings.Contains(importPath, "/pkg/") {
			return false // Local project package
		}
		// If it has a slash, check if the first part looks like a stdlib category
		parts := strings.Split(importPath, "/")
		if len(parts) > 1 {
			// Common Go stdlib top-level packages that have subpackages
			stdlibPrefixes := map[string]bool{
				"archive": true, "bufio": true, "bytes": true, "compress": true,
				"container": true, "context": true, "crypto": true, "database": true,
				"debug": true, "embed": true, "encoding": true, "errors": true,
				"expvar": true, "flag": true, "fmt": true, "go": true, "hash": true,
				"html": true, "image": true, "index": true, "io": true, "log": true,
				"math": true, "mime": true, "net": true, "os": true, "path": true,
				"plugin": true, "reflect": true, "regexp": true, "runtime": true,
				"sort": true, "strconv": true, "strings": true, "sync": true,
				"syscall": true, "testing": true, "text": true, "time": true,
				"unicode": true, "unsafe": true,
			}
			return stdlibPrefixes[parts[0]]
		}
		// Single-segment package without dots is stdlib
		return true
	case modules.LanguageTypeScript, modules.LanguageJavaScript:
		// Node.js built-in modules
		return strings.HasPrefix(importPath, "node:")
	case modules.LanguagePython:
		// Common Python stdlib modules
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
