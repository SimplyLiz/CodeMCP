package architecture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"ckb/internal/modules"
	"ckb/internal/paths"
)

// DetectEntrypoints finds main/entry files in each module
// Detects:
// - main.go, main.ts, main.dart, main.py
// - index.ts, index.js
// - cli.ts, server.ts
// - *_test.go, *.test.ts (test entrypoints)
// - package.json "main" field
// - pubspec.yaml entry
func (g *ArchitectureGenerator) DetectEntrypoints(mods []*modules.Module) ([]Entrypoint, error) {
	var entrypoints []Entrypoint

	for _, mod := range mods {
		moduleEntrypoints := g.detectModuleEntrypoints(mod)
		entrypoints = append(entrypoints, moduleEntrypoints...)
	}

	return entrypoints, nil
}

// detectModuleEntrypoints detects entrypoints for a single module
func (g *ArchitectureGenerator) detectModuleEntrypoints(mod *modules.Module) []Entrypoint {
	var entrypoints []Entrypoint
	modulePath := filepath.Join(g.repoRoot, mod.RootPath)

	// Pattern-based detection
	entrypoints = append(entrypoints, g.detectByFilename(mod, modulePath)...)

	// Manifest-based detection
	entrypoints = append(entrypoints, g.detectFromManifest(mod, modulePath)...)

	return entrypoints
}

// detectByFilename detects entrypoints by filename patterns
func (g *ArchitectureGenerator) detectByFilename(mod *modules.Module, modulePath string) []Entrypoint {
	var entrypoints []Entrypoint

	// Entry file patterns by language
	entryPatterns := map[string][]string{
		modules.LanguageGo: {
			"main.go",
			"cmd/*/main.go",
		},
		modules.LanguageTypeScript: {
			"main.ts", "index.ts",
			"src/main.ts", "src/index.ts",
			"cli.ts", "server.ts",
		},
		modules.LanguageJavaScript: {
			"main.js", "index.js",
			"src/main.js", "src/index.js",
			"cli.js", "server.js",
		},
		modules.LanguageDart: {
			"main.dart",
			"lib/main.dart",
			"bin/*.dart",
		},
		modules.LanguagePython: {
			"__main__.py",
			"main.py",
			"cli.py",
		},
		modules.LanguageRust: {
			"src/main.rs",
			"src/bin/*.rs",
		},
		modules.LanguageJava: {
			"src/main/java/**/Main.java",
		},
	}

	// Test patterns
	testPatterns := map[string][]string{
		modules.LanguageGo:         {"*_test.go"},
		modules.LanguageTypeScript: {"*.test.ts", "*.spec.ts"},
		modules.LanguageJavaScript: {"*.test.js", "*.spec.js"},
		modules.LanguageDart:       {"*_test.dart"},
		modules.LanguagePython:     {"test_*.py", "*_test.py"},
		modules.LanguageRust:       {"tests/*.rs"},
	}

	// Check main entry patterns
	if patterns, ok := entryPatterns[mod.Language]; ok {
		for _, pattern := range patterns {
			matches := g.findFiles(modulePath, pattern)
			for _, match := range matches {
				relPath, _ := filepath.Rel(g.repoRoot, match)
				relPath = paths.NormalizePath(relPath)

				kind := g.inferEntrypointKind(match)
				entrypoints = append(entrypoints, Entrypoint{
					FileId:   relPath,
					Name:     filepath.Base(match),
					Kind:     kind,
					ModuleId: mod.ID,
				})
			}
		}
	}

	// Check test patterns
	if patterns, ok := testPatterns[mod.Language]; ok {
		for _, pattern := range patterns {
			matches := g.findFiles(modulePath, pattern)
			// Limit test entrypoints to avoid too many
			if len(matches) > 5 {
				matches = matches[:5]
			}
			for _, match := range matches {
				relPath, _ := filepath.Rel(g.repoRoot, match)
				relPath = paths.NormalizePath(relPath)

				entrypoints = append(entrypoints, Entrypoint{
					FileId:   relPath,
					Name:     filepath.Base(match),
					Kind:     EntrypointTest,
					ModuleId: mod.ID,
				})
			}
		}
	}

	return entrypoints
}

// detectFromManifest detects entrypoints from manifest files
func (g *ArchitectureGenerator) detectFromManifest(mod *modules.Module, modulePath string) []Entrypoint {
	var entrypoints []Entrypoint

	switch mod.ManifestType {
	case modules.ManifestPackageJSON:
		if entry := g.detectFromPackageJSON(mod, modulePath); entry != nil {
			entrypoints = append(entrypoints, *entry)
		}
	case modules.ManifestPubspecYaml:
		if entry := g.detectFromPubspec(mod, modulePath); entry != nil {
			entrypoints = append(entrypoints, *entry)
		}
	case modules.ManifestCargoToml:
		if entry := g.detectFromCargoToml(mod, modulePath); entry != nil {
			entrypoints = append(entrypoints, *entry)
		}
	}

	return entrypoints
}

// detectFromPackageJSON extracts main entry from package.json
func (g *ArchitectureGenerator) detectFromPackageJSON(mod *modules.Module, modulePath string) *Entrypoint {
	packageJSONPath := filepath.Join(modulePath, "package.json")
	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return nil
	}

	var pkg struct {
		Main string   `json:"main"`
		Bin  interface{} `json:"bin"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}

	// Check main field
	if pkg.Main != "" {
		mainPath := filepath.Join(modulePath, pkg.Main)
		relPath, _ := filepath.Rel(g.repoRoot, mainPath)
		relPath = paths.NormalizePath(relPath)

		return &Entrypoint{
			FileId:   relPath,
			Name:     filepath.Base(mainPath),
			Kind:     EntrypointMain,
			ModuleId: mod.ID,
		}
	}

	// Check bin field (can be string or object)
	if pkg.Bin != nil {
		switch bin := pkg.Bin.(type) {
		case string:
			binPath := filepath.Join(modulePath, bin)
			relPath, _ := filepath.Rel(g.repoRoot, binPath)
			relPath = paths.NormalizePath(relPath)

			return &Entrypoint{
				FileId:   relPath,
				Name:     filepath.Base(binPath),
				Kind:     EntrypointCLI,
				ModuleId: mod.ID,
			}
		}
	}

	return nil
}

// detectFromPubspec extracts entry from pubspec.yaml
func (g *ArchitectureGenerator) detectFromPubspec(mod *modules.Module, modulePath string) *Entrypoint {
	// Dart convention: main.dart in lib/ or bin/
	mainPaths := []string{
		filepath.Join(modulePath, "lib", "main.dart"),
		filepath.Join(modulePath, "bin", mod.Name+".dart"),
	}

	for _, mainPath := range mainPaths {
		if _, err := os.Stat(mainPath); err == nil {
			relPath, _ := filepath.Rel(g.repoRoot, mainPath)
			relPath = paths.NormalizePath(relPath)

			return &Entrypoint{
				FileId:   relPath,
				Name:     filepath.Base(mainPath),
				Kind:     EntrypointMain,
				ModuleId: mod.ID,
			}
		}
	}

	return nil
}

// detectFromCargoToml extracts entry from Cargo.toml
func (g *ArchitectureGenerator) detectFromCargoToml(mod *modules.Module, modulePath string) *Entrypoint {
	// Rust convention: src/main.rs for binaries
	mainPath := filepath.Join(modulePath, "src", "main.rs")
	if _, err := os.Stat(mainPath); err == nil {
		relPath, _ := filepath.Rel(g.repoRoot, mainPath)
		relPath = paths.NormalizePath(relPath)

		return &Entrypoint{
			FileId:   relPath,
			Name:     "main.rs",
			Kind:     EntrypointMain,
			ModuleId: mod.ID,
		}
	}

	return nil
}

// findFiles finds files matching a simple glob pattern
func (g *ArchitectureGenerator) findFiles(basePath string, pattern string) []string {
	var matches []string

	// Handle simple wildcard patterns
	if strings.Contains(pattern, "*") {
		// Use filepath.Glob for wildcards
		fullPattern := filepath.Join(basePath, pattern)
		globMatches, err := filepath.Glob(fullPattern)
		if err == nil {
			matches = append(matches, globMatches...)
		}
	} else {
		// Direct file check
		fullPath := filepath.Join(basePath, pattern)
		if _, err := os.Stat(fullPath); err == nil {
			matches = append(matches, fullPath)
		}
	}

	return matches
}

// inferEntrypointKind infers the kind of entrypoint from filename
func (g *ArchitectureGenerator) inferEntrypointKind(filePath string) string {
	baseName := strings.ToLower(filepath.Base(filePath))

	if strings.Contains(baseName, "test") {
		return EntrypointTest
	}
	if strings.Contains(baseName, "cli") {
		return EntrypointCLI
	}
	if strings.Contains(baseName, "server") {
		return EntrypointServer
	}

	return EntrypointMain
}
