package export

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ckb/internal/logging"
	"ckb/internal/modules"
)

// Exporter provides LLM-friendly codebase export functionality
type Exporter struct {
	repoRoot string
	logger   *logging.Logger
}

// NewExporter creates a new exporter
func NewExporter(repoRoot string, logger *logging.Logger) *Exporter {
	return &Exporter{
		repoRoot: repoRoot,
		logger:   logger,
	}
}

// Export generates an LLM-friendly export of the codebase
func (e *Exporter) Export(ctx context.Context, opts ExportOptions) (*LLMExport, error) {
	// Set defaults
	if opts.RepoRoot == "" {
		opts.RepoRoot = e.repoRoot
	}
	if opts.Format == "" {
		opts.Format = "text"
	}

	e.logger.Debug("Starting LLM export", map[string]interface{}{
		"repoRoot": opts.RepoRoot,
		"format":   opts.Format,
	})

	// Detect modules using the modules package
	detectionResult, err := modules.DetectModules(opts.RepoRoot, nil, nil, "", e.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to detect modules: %w", err)
	}
	detectedModules := detectionResult.Modules

	// Build export structure
	export := &LLMExport{
		Metadata: ExportMetadata{
			Repo:        filepath.Base(opts.RepoRoot),
			Generated:   time.Now().Format(time.RFC3339),
			ModuleCount: len(detectedModules),
		},
		Modules: make([]ExportModule, 0, len(detectedModules)),
	}

	totalSymbols := 0
	totalFiles := 0

	// Process each module
	for _, mod := range detectedModules {
		exportMod := ExportModule{
			Path:  mod.RootPath,
			Files: make([]ExportFile, 0),
		}

		// Find files in module
		files, err := e.findFilesInModule(opts.RepoRoot, mod.RootPath)
		if err != nil {
			e.logger.Warn("Failed to find files in module", map[string]interface{}{
				"module": mod.RootPath,
				"error":  err.Error(),
			})
			continue
		}

		for _, file := range files {
			// Get symbols in file
			symbols, err := e.extractSymbols(filepath.Join(opts.RepoRoot, file))
			if err != nil {
				continue
			}

			// Filter symbols based on options
			filteredSymbols := e.filterSymbols(symbols, opts)

			if len(filteredSymbols) > 0 {
				exportFile := ExportFile{
					Name:    filepath.Base(file),
					Symbols: filteredSymbols,
				}
				exportMod.Files = append(exportMod.Files, exportFile)
				totalFiles++
				totalSymbols += len(filteredSymbols)
			}

			// Check max symbols limit
			if opts.MaxSymbols > 0 && totalSymbols >= opts.MaxSymbols {
				break
			}
		}

		if len(exportMod.Files) > 0 {
			export.Modules = append(export.Modules, exportMod)
		}

		// Check max symbols limit
		if opts.MaxSymbols > 0 && totalSymbols >= opts.MaxSymbols {
			break
		}
	}

	export.Metadata.SymbolCount = totalSymbols
	export.Metadata.FileCount = totalFiles

	return export, nil
}

// FormatText formats the export as text for LLM consumption
func (e *Exporter) FormatText(export *LLMExport, opts ExportOptions) string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# Codebase: %s\n", export.Metadata.Repo))
	sb.WriteString(fmt.Sprintf("# Generated: %s\n", export.Metadata.Generated))
	sb.WriteString(fmt.Sprintf("# Symbols: %d | Files: %d | Modules: %d\n\n",
		export.Metadata.SymbolCount, export.Metadata.FileCount, export.Metadata.ModuleCount))

	// Modules
	for _, mod := range export.Modules {
		// Module header
		header := fmt.Sprintf("## %s/", mod.Path)
		if mod.Owner != "" {
			header += fmt.Sprintf(" (owner: %s)", mod.Owner)
		}
		sb.WriteString(header + "\n\n")

		for _, file := range mod.Files {
			sb.WriteString(fmt.Sprintf("  ! %s\n", file.Name))

			for _, sym := range file.Symbols {
				line := e.formatSymbolLine(sym, opts)
				sb.WriteString(line + "\n")
			}
			sb.WriteString("\n")
		}
	}

	// Legend
	sb.WriteString("---\n")
	sb.WriteString("Legend:\n")
	sb.WriteString("  !  = file\n")
	sb.WriteString("  $  = class/struct\n")
	sb.WriteString("  #  = function/method\n")
	if opts.IncludeComplexity {
		sb.WriteString("  c  = complexity (cyclomatic)\n")
	}
	if opts.IncludeUsage {
		sb.WriteString("  ★  = importance (usage × complexity)\n")
	}
	if opts.IncludeContracts {
		sb.WriteString("  contract: = exposes or consumes a contract\n")
	}

	return sb.String()
}

// formatSymbolLine formats a single symbol line
func (e *Exporter) formatSymbolLine(sym ExportSymbol, opts ExportOptions) string {
	// Determine prefix based on type
	var prefix string
	var indent string
	switch sym.Type {
	case SymbolTypeClass, "struct":
		prefix = "$"
		indent = "    "
	default:
		prefix = "#"
		indent = "      "
	}

	line := fmt.Sprintf("%s%s %s", indent, prefix, sym.Name)
	if sym.Type == SymbolTypeFunction || sym.Type == SymbolTypeMethod {
		line += "()"
	}

	// Pad for alignment
	for len(line) < 30 {
		line += " "
	}

	// Add complexity
	if opts.IncludeComplexity && sym.Complexity > 0 {
		line += fmt.Sprintf("  c=%d", sym.Complexity)
	}

	// Add usage
	if opts.IncludeUsage && sym.CallsPerDay > 0 {
		line += fmt.Sprintf("  calls=%s", formatCalls(sym.CallsPerDay))
	}

	// Add importance stars
	if sym.Importance > 0 {
		stars := strings.Repeat("★", sym.Importance)
		line += " " + stars
	}

	// Add contracts
	if opts.IncludeContracts && len(sym.Contracts) > 0 {
		line += fmt.Sprintf("  contract:%s", sym.Contracts[0])
	}

	// Add warnings
	if len(sym.Warnings) > 0 {
		line += fmt.Sprintf("  ⚠️ %s", sym.Warnings[0])
	}

	// Add interface marker
	if sym.IsInterface {
		line += "  interface"
	}

	return line
}

// findFilesInModule finds all relevant source files in a module
func (e *Exporter) findFilesInModule(repoRoot, modPath string) ([]string, error) {
	var files []string

	fullPath := filepath.Join(repoRoot, modPath)
	err := filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			// Skip common non-source directories
			name := info.Name()
			if name == "node_modules" || name == "vendor" || name == ".git" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if it's a source file
		ext := filepath.Ext(path)
		if isSourceFile(ext) {
			relPath, err := filepath.Rel(repoRoot, path)
			if err == nil {
				files = append(files, relPath)
			}
		}

		return nil
	})

	// Sort files for consistent output
	sort.Strings(files)

	return files, err
}

// extractSymbols extracts symbols from a source file
// This is a simplified implementation - could be enhanced with SCIP data
func (e *Exporter) extractSymbols(filePath string) ([]ExportSymbol, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	ext := filepath.Ext(filePath)
	return e.parseSymbols(string(content), ext)
}

// parseSymbols parses symbols from source code
// This is a simplified regex-based parser - could be enhanced with tree-sitter
func (e *Exporter) parseSymbols(content, ext string) ([]ExportSymbol, error) {
	var symbols []ExportSymbol

	lines := strings.Split(content, "\n")

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#") {
			continue
		}

		// Go patterns
		if ext == ".go" {
			// Function
			if strings.HasPrefix(line, "func ") {
				name := extractGoFuncName(line)
				if name != "" && isExportedGo(name) {
					symbols = append(symbols, ExportSymbol{
						Type:       SymbolTypeFunction,
						Name:       name,
						IsExported: true,
						Line:       lineNum + 1,
					})
				}
			}
			// Type (struct/interface)
			if strings.HasPrefix(line, "type ") {
				name, isInterface := extractGoTypeName(line)
				if name != "" && isExportedGo(name) {
					symType := SymbolTypeClass
					if isInterface {
						symType = SymbolTypeInterface
					}
					symbols = append(symbols, ExportSymbol{
						Type:        symType,
						Name:        name,
						IsInterface: isInterface,
						IsExported:  true,
						Line:        lineNum + 1,
					})
				}
			}
		}

		// TypeScript/JavaScript patterns
		if ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx" {
			// Export function
			if strings.Contains(line, "export") && strings.Contains(line, "function") {
				name := extractJSFuncName(line)
				if name != "" {
					symbols = append(symbols, ExportSymbol{
						Type:       SymbolTypeFunction,
						Name:       name,
						IsExported: true,
						Line:       lineNum + 1,
					})
				}
			}
			// Export class
			if strings.Contains(line, "export") && strings.Contains(line, "class") {
				name := extractJSClassName(line)
				if name != "" {
					symbols = append(symbols, ExportSymbol{
						Type:       SymbolTypeClass,
						Name:       name,
						IsExported: true,
						Line:       lineNum + 1,
					})
				}
			}
		}

		// Python patterns
		if ext == ".py" {
			// Function
			if strings.HasPrefix(line, "def ") {
				name := extractPyFuncName(line)
				if name != "" && !strings.HasPrefix(name, "_") {
					symbols = append(symbols, ExportSymbol{
						Type:       SymbolTypeFunction,
						Name:       name,
						IsExported: true,
						Line:       lineNum + 1,
					})
				}
			}
			// Class
			if strings.HasPrefix(line, "class ") {
				name := extractPyClassName(line)
				if name != "" && !strings.HasPrefix(name, "_") {
					symbols = append(symbols, ExportSymbol{
						Type:       SymbolTypeClass,
						Name:       name,
						IsExported: true,
						Line:       lineNum + 1,
					})
				}
			}
		}
	}

	return symbols, nil
}

// filterSymbols filters symbols based on options
func (e *Exporter) filterSymbols(symbols []ExportSymbol, opts ExportOptions) []ExportSymbol {
	filtered := make([]ExportSymbol, 0, len(symbols))

	for _, sym := range symbols {
		// Filter by complexity
		if opts.MinComplexity > 0 && sym.Complexity < opts.MinComplexity {
			continue
		}

		// Filter by calls
		if opts.MinCalls > 0 && sym.CallsPerDay < opts.MinCalls {
			continue
		}

		// Only include exported symbols
		if !sym.IsExported {
			continue
		}

		filtered = append(filtered, sym)
	}

	return filtered
}

// Helper functions

func isSourceFile(ext string) bool {
	switch ext {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".java", ".kt", ".rs", ".rb", ".c", ".cpp", ".h", ".hpp":
		return true
	}
	return false
}

func isExportedGo(name string) bool {
	if len(name) == 0 {
		return false
	}
	return name[0] >= 'A' && name[0] <= 'Z'
}

func extractGoFuncName(line string) string {
	// func Name(...) or func (r *Receiver) Name(...)
	line = strings.TrimPrefix(line, "func ")

	// Handle receiver
	if strings.HasPrefix(line, "(") {
		idx := strings.Index(line, ")")
		if idx < 0 {
			return ""
		}
		line = strings.TrimSpace(line[idx+1:])
	}

	// Extract name
	idx := strings.Index(line, "(")
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(line[:idx])
}

func extractGoTypeName(line string) (name string, isInterface bool) {
	// type Name struct { or type Name interface {
	line = strings.TrimPrefix(line, "type ")
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", false
	}
	return parts[0], parts[1] == "interface"
}

func extractJSFuncName(line string) string {
	// Various patterns: export function name, export const name =, etc.
	if idx := strings.Index(line, "function "); idx >= 0 {
		rest := line[idx+9:]
		if idx2 := strings.Index(rest, "("); idx2 > 0 {
			return strings.TrimSpace(rest[:idx2])
		}
	}
	return ""
}

func extractJSClassName(line string) string {
	if idx := strings.Index(line, "class "); idx >= 0 {
		rest := line[idx+6:]
		// Find end of class name (space, { or extends)
		for i, c := range rest {
			if c == ' ' || c == '{' {
				return strings.TrimSpace(rest[:i])
			}
		}
	}
	return ""
}

func extractPyFuncName(line string) string {
	line = strings.TrimPrefix(line, "def ")
	if idx := strings.Index(line, "("); idx > 0 {
		return strings.TrimSpace(line[:idx])
	}
	return ""
}

func extractPyClassName(line string) string {
	line = strings.TrimPrefix(line, "class ")
	// Find end of class name
	for i, c := range line {
		if c == '(' || c == ':' {
			return strings.TrimSpace(line[:i])
		}
	}
	return ""
}

func formatCalls(calls int) string {
	if calls >= 1000000 {
		return fmt.Sprintf("%dM/day", calls/1000000)
	}
	if calls >= 1000 {
		return fmt.Sprintf("%dk/day", calls/1000)
	}
	return fmt.Sprintf("%d/day", calls)
}
