package query

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MoveDetail provides move-specific information for prepareChange.
type MoveDetail struct {
	SourcePath      string         `json:"sourcePath"`
	TargetPath      string         `json:"targetPath"`
	AffectedImports []MoveImport   `json:"affectedImports"`
	TargetConflicts []MoveConflict `json:"targetConflicts"`
	PackageChanges  int            `json:"packageChanges"`
}

// MoveImport describes an import statement that needs updating.
type MoveImport struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	OldImport string `json:"oldImport"`
	NewImport string `json:"newImport"`
}

// MoveConflict describes a naming conflict at the target location.
type MoveConflict struct {
	Name         string `json:"name"`
	ExistingAt   string `json:"existingAt"`
	ConflictKind string `json:"conflictKind"` // "symbol", "file"
}

// getPrepareMove builds move-specific detail for prepareChange.
func (e *Engine) getPrepareMove(ctx context.Context, target *PrepareChangeTarget, targetPath string) *MoveDetail {
	if target == nil || target.Path == "" || targetPath == "" {
		return nil
	}

	sourcePath := target.Path
	detail := &MoveDetail{
		SourcePath: sourcePath,
		TargetPath: targetPath,
	}

	// Determine source package path for import scanning
	sourceDir := filepath.Dir(sourcePath)
	targetDir := filepath.Dir(targetPath)
	if targetDir == "." {
		targetDir = targetPath // targetPath might be a directory
	}

	// Find affected imports
	if e.scipAdapter != nil && e.scipAdapter.IsAvailable() {
		// With SCIP: use precise symbol references
		detail.AffectedImports = e.findAffectedImportsSCIP(ctx, target, sourceDir, targetDir)
	} else {
		// Without SCIP: scan files for import statements matching source package
		detail.AffectedImports = e.findAffectedImportsHeuristic(sourceDir, targetDir)
	}

	detail.PackageChanges = len(detail.AffectedImports)

	// Check target for conflicts
	detail.TargetConflicts = e.checkTargetConflicts(sourcePath, targetPath)

	return detail
}

// findAffectedImportsSCIP uses SCIP to find all import sites for the source package.
func (e *Engine) findAffectedImportsSCIP(ctx context.Context, target *PrepareChangeTarget, sourceDir, targetDir string) []MoveImport {
	if target.SymbolId == "" {
		return nil
	}

	refs, err := e.FindReferences(ctx, FindReferencesOptions{
		SymbolId: target.SymbolId,
		Limit:    500,
	})
	if err != nil || refs == nil {
		return nil
	}

	var imports []MoveImport
	seen := make(map[string]bool)

	for _, ref := range refs.References {
		if ref.Location == nil {
			continue
		}
		file := ref.Location.FileId
		if file == target.Path {
			continue
		}
		if seen[file] {
			continue
		}
		seen[file] = true

		imports = append(imports, MoveImport{
			File:      file,
			Line:      ref.Location.StartLine,
			OldImport: sourceDir,
			NewImport: targetDir,
		})
	}

	return imports
}

// findAffectedImportsHeuristic scans files for import statements matching the source package.
func (e *Engine) findAffectedImportsHeuristic(sourceDir, targetDir string) []MoveImport {
	var imports []MoveImport

	// Build import pattern based on source directory
	sourcePackage := filepath.ToSlash(sourceDir)
	if sourcePackage == "" || sourcePackage == "." {
		return nil
	}

	importPattern := regexp.MustCompile(`import\s.*"[^"]*` + regexp.QuoteMeta(sourcePackage) + `[^"]*"`)

	// Walk the repo scanning for matching imports
	_ = filepath.Walk(e.repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".ts" && ext != ".js" && ext != ".py" {
			return nil
		}

		relPath, err := filepath.Rel(e.repoRoot, path)
		if err != nil {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if importPattern.MatchString(line) {
				imports = append(imports, MoveImport{
					File:      relPath,
					Line:      lineNum,
					OldImport: sourcePackage,
					NewImport: filepath.ToSlash(targetDir),
				})
			}
			// Stop scanning after the import block (optimization for Go files)
			if ext == ".go" && lineNum > 50 && !strings.Contains(line, "import") {
				break
			}
		}

		return nil
	})

	return imports
}

// checkTargetConflicts checks the target location for naming conflicts.
func (e *Engine) checkTargetConflicts(sourcePath, targetPath string) []MoveConflict {
	var conflicts []MoveConflict

	targetDir := targetPath
	if filepath.Ext(targetPath) != "" {
		// targetPath is a file, check if it exists
		absTarget := filepath.Join(e.repoRoot, targetPath)
		if _, err := os.Stat(absTarget); err == nil {
			conflicts = append(conflicts, MoveConflict{
				Name:         filepath.Base(targetPath),
				ExistingAt:   targetPath,
				ConflictKind: "file",
			})
		}
		targetDir = filepath.Dir(targetPath)
	}

	// Check if a file with the same base name exists in the target directory
	sourceBase := filepath.Base(sourcePath)
	candidatePath := filepath.Join(e.repoRoot, targetDir, sourceBase)
	if _, err := os.Stat(candidatePath); err == nil {
		relPath, _ := filepath.Rel(e.repoRoot, candidatePath)
		if relPath != sourcePath {
			conflicts = append(conflicts, MoveConflict{
				Name:         sourceBase,
				ExistingAt:   relPath,
				ConflictKind: "file",
			})
		}
	}

	return conflicts
}
