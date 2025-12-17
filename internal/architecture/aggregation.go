package architecture

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"

	"ckb/internal/modules"
	"ckb/internal/paths"
)

// AggregateModules collects statistics for each module
func (g *ArchitectureGenerator) AggregateModules(mods []*modules.Module) ([]ModuleSummary, error) {
	summaries := make([]ModuleSummary, 0, len(mods))

	for _, mod := range mods {
		summary := ModuleSummary{
			ModuleId:    mod.ID,
			Name:        mod.Name,
			RootPath:    mod.RootPath,
			Language:    mod.Language,
			SymbolCount: 0, // Populated by query layer from SCIP backend
		}

		// Count files
		fileCount, err := g.CountFiles(mod)
		if err != nil {
			g.logger.Warn("Failed to count files for module", map[string]interface{}{
				"moduleId": mod.ID,
				"error":    err.Error(),
			})
			fileCount = 0
		}
		summary.FileCount = fileCount

		// Count lines of code
		loc, err := g.CountLOC(mod)
		if err != nil {
			g.logger.Warn("Failed to count LOC for module", map[string]interface{}{
				"moduleId": mod.ID,
				"error":    err.Error(),
			})
			loc = 0
		}
		summary.LOC = loc

		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// CountFiles returns the number of source files in a module
func (g *ArchitectureGenerator) CountFiles(mod *modules.Module) (int, error) {
	modulePath := filepath.Join(g.repoRoot, mod.RootPath)
	count := 0

	ignoreMap := make(map[string]bool)
	for _, dir := range g.config.Modules.Ignore {
		ignoreMap[dir] = true
	}

	err := filepath.Walk(modulePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			relPath, _ := filepath.Rel(g.repoRoot, path)
			if relPath != "." && shouldIgnoreDir(relPath, ignoreMap) {
				return filepath.SkipDir
			}
			return nil
		}

		// Only count source files
		if isSourceFile(path, mod.Language) {
			count++
		}

		return nil
	})

	if err != nil {
		return 0, err
	}

	return count, nil
}

// CountLOC returns the total lines of code in a module
func (g *ArchitectureGenerator) CountLOC(mod *modules.Module) (int, error) {
	modulePath := filepath.Join(g.repoRoot, mod.RootPath)
	totalLOC := 0

	ignoreMap := make(map[string]bool)
	for _, dir := range g.config.Modules.Ignore {
		ignoreMap[dir] = true
	}

	err := filepath.Walk(modulePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			relPath, _ := filepath.Rel(g.repoRoot, path)
			if relPath != "." && shouldIgnoreDir(relPath, ignoreMap) {
				return filepath.SkipDir
			}
			return nil
		}

		// Only count lines in source files
		if isSourceFile(path, mod.Language) {
			loc, err := countFileLines(path)
			if err != nil {
				g.logger.Debug("Failed to count lines in file", map[string]interface{}{
					"file":  path,
					"error": err.Error(),
				})
				return nil //nolint:nilerr // intentionally continue on file read errors
			}
			totalLOC += loc
		}

		return nil
	})

	if err != nil {
		return 0, err
	}

	return totalLOC, nil
}

// isSourceFile checks if a file is a source code file
func isSourceFile(filePath string, language string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))

	// Language-specific extensions
	sourceExts := map[string][]string{
		modules.LanguageTypeScript: {".ts", ".tsx", ".js", ".jsx"},
		modules.LanguageJavaScript: {".js", ".jsx", ".mjs", ".cjs"},
		modules.LanguageDart:       {".dart"},
		modules.LanguageGo:         {".go"},
		modules.LanguageRust:       {".rs"},
		modules.LanguagePython:     {".py", ".pyx"},
		modules.LanguageJava:       {".java"},
		modules.LanguageKotlin:     {".kt", ".kts"},
	}

	// If language is known, use its extensions
	if exts, ok := sourceExts[language]; ok {
		for _, validExt := range exts {
			if ext == validExt {
				return true
			}
		}
		return false
	}

	// If language is unknown, check against all known source extensions
	for _, exts := range sourceExts {
		for _, validExt := range exts {
			if ext == validExt {
				return true
			}
		}
	}

	return false
}

// countFileLines counts the number of lines in a file
func countFileLines(filePath string) (int, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	count := 0

	for scanner.Scan() {
		count++
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return count, nil
}

// shouldIgnoreDir checks if a directory should be ignored
func shouldIgnoreDir(relPath string, ignoreMap map[string]bool) bool {
	// Normalize path
	normalizedPath := paths.NormalizePath(relPath)

	// Check exact match
	if ignoreMap[normalizedPath] {
		return true
	}

	// Check if any parent directory is ignored
	parts := strings.Split(normalizedPath, "/")
	for _, part := range parts {
		if ignoreMap[part] {
			return true
		}
	}

	// Always ignore hidden directories
	for _, part := range parts {
		if strings.HasPrefix(part, ".") && part != "." {
			return true
		}
	}

	return false
}
