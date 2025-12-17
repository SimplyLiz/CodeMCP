package modules

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ckb/internal/config"
	"ckb/internal/logging"
	"ckb/internal/paths"
)

// LanguagePattern defines import patterns for a specific language
type LanguagePattern struct {
	// Extensions lists file extensions for this language
	Extensions []string

	// Patterns contains regex patterns to extract imports
	Patterns []*regexp.Regexp

	// Language name
	Language string
}

// Built-in import patterns per Section 15.2 of the design document
var builtinPatterns = map[string]*LanguagePattern{
	LanguageTypeScript: {
		Extensions: []string{".ts", ".tsx", ".js", ".jsx"},
		Language:   LanguageTypeScript,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`import\s+.*?from\s+['"]([^'"]+)['"]`),
			regexp.MustCompile(`export\s+.*?from\s+['"]([^'"]+)['"]`),
			regexp.MustCompile(`require\s*\(\s*['"]([^'"]+)['"]\s*\)`),
			regexp.MustCompile(`import\s*\(\s*['"]([^'"]+)['"]\s*\)`), // Dynamic import
		},
	},
	LanguageJavaScript: {
		Extensions: []string{".js", ".jsx", ".mjs", ".cjs"},
		Language:   LanguageJavaScript,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`import\s+.*?from\s+['"]([^'"]+)['"]`),
			regexp.MustCompile(`export\s+.*?from\s+['"]([^'"]+)['"]`),
			regexp.MustCompile(`require\s*\(\s*['"]([^'"]+)['"]\s*\)`),
			regexp.MustCompile(`import\s*\(\s*['"]([^'"]+)['"]\s*\)`),
		},
	},
	LanguageDart: {
		Extensions: []string{".dart"},
		Language:   LanguageDart,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`import\s+['"]([^'"]+)['"]`),
			regexp.MustCompile(`export\s+['"]([^'"]+)['"]`),
		},
	},
	LanguageGo: {
		Extensions: []string{".go"},
		Language:   LanguageGo,
		Patterns: []*regexp.Regexp{
			// Single line: import "path"
			regexp.MustCompile(`^\s*import\s+"([^"]+)"`),
			// Multi-line block: lines that are just whitespace + optional alias + "path"
			// e.g., "fmt" or alias "pkg/path" or . "pkg"
			regexp.MustCompile(`^\s*(?:\w+\s+)?"([^"]+)"\s*$`),
		},
	},
	LanguagePython: {
		Extensions: []string{".py", ".pyx"},
		Language:   LanguagePython,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`from\s+([^\s]+)\s+import`),
			regexp.MustCompile(`import\s+([^\s,;]+)`),
		},
	},
	LanguageRust: {
		Extensions: []string{".rs"},
		Language:   LanguageRust,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`use\s+([^;{]+)`),
			regexp.MustCompile(`extern\s+crate\s+([^;]+)`),
		},
	},
	LanguageJava: {
		Extensions: []string{".java"},
		Language:   LanguageJava,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`import\s+([^;]+);`),
			regexp.MustCompile(`import\s+static\s+([^;]+);`),
		},
	},
	LanguageKotlin: {
		Extensions: []string{".kt", ".kts"},
		Language:   LanguageKotlin,
		Patterns: []*regexp.Regexp{
			regexp.MustCompile(`import\s+([^\s;]+)`),
		},
	},
}

// ImportScanner scans files for import statements
type ImportScanner struct {
	config   *config.ImportScanConfig
	patterns map[string]*LanguagePattern
	logger   *logging.Logger
}

// NewImportScanner creates a new import scanner
func NewImportScanner(cfg *config.ImportScanConfig, logger *logging.Logger) *ImportScanner {
	return &ImportScanner{
		config:   cfg,
		patterns: builtinPatterns,
		logger:   logger,
	}
}

// ScanFile scans a single file for imports
func (s *ImportScanner) ScanFile(filePath string, repoRoot string) ([]*ImportEdge, error) {
	// Get canonical path
	canonicalPath, err := paths.CanonicalizePath(filePath, repoRoot)
	if err != nil {
		return nil, err
	}

	// Check file size
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	if info.Size() > int64(s.config.MaxFileSizeBytes) {
		s.logger.Debug("Skipping file: too large", map[string]interface{}{
			"file": canonicalPath,
			"size": info.Size(),
		})
		return nil, nil
	}

	// Detect language from extension
	language := s.detectLanguage(filePath)
	if language == "" {
		return nil, nil // Unsupported file type
	}

	pattern, ok := s.patterns[language]
	if !ok {
		return nil, nil
	}

	// Open and scan file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var edges []*ImportEdge
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Try each pattern for this language
		for _, re := range pattern.Patterns {
			matches := re.FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				if len(match) > 1 {
					importStr := strings.TrimSpace(match[1])
					if importStr != "" {
						edge := &ImportEdge{
							From:       canonicalPath,
							To:         importStr,
							RawImport:  importStr,
							Line:       lineNum,
							Confidence: 1.0, // Will be adjusted during classification
						}
						edges = append(edges, edge)
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return edges, nil
}

// ScanDirectory scans all files in a directory for imports
func (s *ImportScanner) ScanDirectory(dirPath string, repoRoot string, ignoreDirs []string) ([]*ImportEdge, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.config.ScanTimeoutMs)*time.Millisecond)
	defer cancel()

	return s.scanDirectoryWithContext(ctx, dirPath, repoRoot, ignoreDirs)
}

// scanDirectoryWithContext scans directory with context for timeout
func (s *ImportScanner) scanDirectoryWithContext(ctx context.Context, dirPath string, repoRoot string, ignoreDirs []string) ([]*ImportEdge, error) {
	var allEdges []*ImportEdge
	ignoreMap := make(map[string]bool)
	for _, dir := range ignoreDirs {
		ignoreMap[dir] = true
	}

	filesScanned := 0
	maxFiles := 10000 // Per Section 15.1: maxFilesPerModule

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		// Check context for timeout
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			relPath, _ := filepath.Rel(repoRoot, path)
			if relPath != "." && shouldIgnore(relPath, ignoreMap) {
				return filepath.SkipDir
			}
			return nil
		}

		// Check max files limit
		if filesScanned >= maxFiles {
			s.logger.Warn("Reached max files limit during import scan", map[string]interface{}{
				"maxFiles": maxFiles,
			})
			return filepath.SkipAll
		}

		// Skip binary files
		if s.config.SkipBinary && isBinaryFile(path) {
			return nil
		}

		// Scan file
		edges, err := s.ScanFile(path, repoRoot)
		if err != nil {
			// Log error but continue scanning
			s.logger.Warn("Error scanning file", map[string]interface{}{
				"file":  path,
				"error": err.Error(),
			})
			return nil //nolint:nilerr // intentionally continue on scan errors
		}

		allEdges = append(allEdges, edges...)
		filesScanned++

		return nil
	})

	if err != nil {
		return nil, err
	}

	s.logger.Info("Import scan completed", map[string]interface{}{
		"filesScanned": filesScanned,
		"importsFound": len(allEdges),
	})

	return allEdges, nil
}

// detectLanguage detects language from file extension
func (s *ImportScanner) detectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))

	for language, pattern := range s.patterns {
		for _, patternExt := range pattern.Extensions {
			if ext == patternExt {
				return language
			}
		}
	}

	return ""
}

// isBinaryFile attempts to detect if a file is binary
func isBinaryFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))

	// Common binary extensions
	binaryExts := map[string]bool{
		".exe": true, ".dll": true, ".so": true, ".dylib": true,
		".a": true, ".o": true, ".obj": true,
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true,
		".pdf": true, ".zip": true, ".tar": true, ".gz": true, ".bz2": true,
		".mp3": true, ".mp4": true, ".avi": true, ".mov": true,
		".wasm": true, ".class": true, ".pyc": true,
	}

	return binaryExts[ext]
}

// GetPatternForLanguage returns the import pattern for a language
func (s *ImportScanner) GetPatternForLanguage(language string) (*LanguagePattern, bool) {
	pattern, ok := s.patterns[language]
	return pattern, ok
}

// AddCustomPattern adds a custom import pattern for a language
func (s *ImportScanner) AddCustomPattern(language string, pattern *LanguagePattern) {
	s.patterns[language] = pattern
}
