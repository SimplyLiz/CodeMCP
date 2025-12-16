package architecture

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"ckb/internal/modules"
)

// GetExternalDeps extracts external dependencies for a module
// Default: includeExternalDeps=false
// External deps are classified internally but filtered from output by default
func (g *ArchitectureGenerator) GetExternalDeps(mod *modules.Module) ([]ExternalDependency, error) {
	modulePath := filepath.Join(g.repoRoot, mod.RootPath)

	switch mod.ManifestType {
	case modules.ManifestPackageJSON:
		return g.getExternalDepsFromPackageJSON(modulePath)
	case modules.ManifestPubspecYaml:
		return g.getExternalDepsFromPubspec(modulePath)
	case modules.ManifestGoMod:
		return g.getExternalDepsFromGoMod(modulePath)
	case modules.ManifestCargoToml:
		return g.getExternalDepsFromCargoToml(modulePath)
	case modules.ManifestPyprojectToml:
		return g.getExternalDepsFromPyproject(modulePath)
	}

	return nil, nil
}

// getExternalDepsFromPackageJSON extracts dependencies from package.json
func (g *ArchitectureGenerator) getExternalDepsFromPackageJSON(modulePath string) ([]ExternalDependency, error) {
	packageJSONPath := filepath.Join(modulePath, "package.json")
	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return nil, err
	}

	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}

	var deps []ExternalDependency

	// Add production dependencies
	for name, version := range pkg.Dependencies {
		deps = append(deps, ExternalDependency{
			Name:    name,
			Version: version,
			Source:  "npm",
		})
	}

	return deps, nil
}

// getExternalDepsFromPubspec extracts dependencies from pubspec.yaml
func (g *ArchitectureGenerator) getExternalDepsFromPubspec(modulePath string) ([]ExternalDependency, error) {
	pubspecPath := filepath.Join(modulePath, "pubspec.yaml")
	data, err := os.ReadFile(pubspecPath)
	if err != nil {
		return nil, err
	}

	var deps []ExternalDependency
	lines := strings.Split(string(data), "\n")
	inDependencies := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "dependencies:" {
			inDependencies = true
			continue
		}

		// Stop at next section
		if inDependencies && strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, " ") {
			break
		}

		if inDependencies && strings.Contains(trimmed, ":") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				depName := strings.TrimSpace(parts[0])
				version := strings.TrimSpace(parts[1])

				// Skip SDK dependencies
				if depName == "sdk" {
					continue
				}

				deps = append(deps, ExternalDependency{
					Name:    depName,
					Version: version,
					Source:  "pub",
				})
			}
		}
	}

	return deps, nil
}

// getExternalDepsFromGoMod extracts dependencies from go.mod
func (g *ArchitectureGenerator) getExternalDepsFromGoMod(modulePath string) ([]ExternalDependency, error) {
	goModPath := filepath.Join(modulePath, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, err
	}

	var deps []ExternalDependency
	lines := strings.Split(string(data), "\n")
	inRequire := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "require (") {
			inRequire = true
			continue
		}

		if inRequire && trimmed == ")" {
			inRequire = false
			continue
		}

		if inRequire || strings.HasPrefix(trimmed, "require ") {
			// Remove "require " prefix if present
			depLine := strings.TrimPrefix(trimmed, "require ")
			depLine = strings.TrimSpace(depLine)

			// Skip comments and empty lines
			if depLine == "" || strings.HasPrefix(depLine, "//") {
				continue
			}

			// Parse "module version" format
			parts := strings.Fields(depLine)
			if len(parts) >= 2 {
				deps = append(deps, ExternalDependency{
					Name:    parts[0],
					Version: parts[1],
					Source:  "go",
				})
			}
		}
	}

	return deps, nil
}

// getExternalDepsFromCargoToml extracts dependencies from Cargo.toml
func (g *ArchitectureGenerator) getExternalDepsFromCargoToml(modulePath string) ([]ExternalDependency, error) {
	cargoTomlPath := filepath.Join(modulePath, "Cargo.toml")
	data, err := os.ReadFile(cargoTomlPath)
	if err != nil {
		return nil, err
	}

	var deps []ExternalDependency
	lines := strings.Split(string(data), "\n")
	inDependencies := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "[dependencies]" {
			inDependencies = true
			continue
		}

		// Stop at next section
		if inDependencies && strings.HasPrefix(trimmed, "[") {
			break
		}

		if inDependencies && strings.Contains(trimmed, "=") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				depName := strings.TrimSpace(parts[0])
				version := strings.TrimSpace(parts[1])
				// Remove quotes
				version = strings.Trim(version, `"'`)

				deps = append(deps, ExternalDependency{
					Name:    depName,
					Version: version,
					Source:  "cargo",
				})
			}
		}
	}

	return deps, nil
}

// getExternalDepsFromPyproject extracts dependencies from pyproject.toml
func (g *ArchitectureGenerator) getExternalDepsFromPyproject(modulePath string) ([]ExternalDependency, error) {
	pyprojectPath := filepath.Join(modulePath, "pyproject.toml")
	data, err := os.ReadFile(pyprojectPath)
	if err != nil {
		return nil, err
	}

	var deps []ExternalDependency
	lines := strings.Split(string(data), "\n")
	inDependencies := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.Contains(trimmed, "dependencies") && strings.Contains(trimmed, "[") {
			inDependencies = true
			continue
		}

		// Stop at end of array
		if inDependencies && trimmed == "]" {
			break
		}

		if inDependencies {
			// Parse dependencies like: "requests>=2.25.0"
			depLine := strings.Trim(trimmed, `"',`)
			if depLine == "" {
				continue
			}

			// Extract name and version
			var depName, version string
			if strings.Contains(depLine, ">=") {
				parts := strings.Split(depLine, ">=")
				depName = strings.TrimSpace(parts[0])
				if len(parts) > 1 {
					version = ">=" + strings.TrimSpace(parts[1])
				}
			} else if strings.Contains(depLine, "==") {
				parts := strings.Split(depLine, "==")
				depName = strings.TrimSpace(parts[0])
				if len(parts) > 1 {
					version = strings.TrimSpace(parts[1])
				}
			} else {
				depName = depLine
			}

			deps = append(deps, ExternalDependency{
				Name:    depName,
				Version: version,
				Source:  "pypi",
			})
		}
	}

	return deps, nil
}
