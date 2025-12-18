package responsibilities

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Responsibility represents extracted responsibility information
type Responsibility struct {
	TargetID     string    `json:"targetId"`
	TargetType   string    `json:"targetType"` // "module" | "file" | "symbol"
	Summary      string    `json:"summary"`
	Capabilities []string  `json:"capabilities"`
	Source       string    `json:"source"` // "declared" | "inferred"
	Confidence   float64   `json:"confidence"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Extractor extracts responsibilities from source code and documentation
type Extractor struct {
	repoRoot string
}

// NewExtractor creates a new responsibility extractor
func NewExtractor(repoRoot string) *Extractor {
	return &Extractor{repoRoot: repoRoot}
}

// ExtractFromModule extracts responsibility from a module directory
func (e *Extractor) ExtractFromModule(modulePath string) (*Responsibility, error) {
	fullPath := filepath.Join(e.repoRoot, modulePath)

	// Try README first (highest confidence)
	readmeSummary, readmeCaps := e.extractFromReadme(fullPath)
	if readmeSummary != "" {
		return &Responsibility{
			TargetID:     modulePath,
			TargetType:   "module",
			Summary:      readmeSummary,
			Capabilities: readmeCaps,
			Source:       "declared",
			Confidence:   0.89,
			UpdatedAt:    time.Now(),
		}, nil
	}

	// Try package doc comment (Go-specific)
	docSummary, docCaps := e.extractFromGoPackageDoc(fullPath)
	if docSummary != "" {
		return &Responsibility{
			TargetID:     modulePath,
			TargetType:   "module",
			Summary:      docSummary,
			Capabilities: docCaps,
			Source:       "declared",
			Confidence:   0.89,
			UpdatedAt:    time.Now(),
		}, nil
	}

	// Fall back to inferring from exports
	inferredSummary, inferredCaps := e.inferFromExports(fullPath)
	if inferredSummary != "" {
		return &Responsibility{
			TargetID:     modulePath,
			TargetType:   "module",
			Summary:      inferredSummary,
			Capabilities: inferredCaps,
			Source:       "inferred",
			Confidence:   0.59,
			UpdatedAt:    time.Now(),
		}, nil
	}

	// Heuristic-only fallback
	return &Responsibility{
		TargetID:     modulePath,
		TargetType:   "module",
		Summary:      "Module at " + modulePath,
		Capabilities: []string{},
		Source:       "inferred",
		Confidence:   0.39,
		UpdatedAt:    time.Now(),
	}, nil
}

// ExtractFromFile extracts responsibility from a file
func (e *Extractor) ExtractFromFile(filePath string) (*Responsibility, error) {
	fullPath := filepath.Join(e.repoRoot, filePath)

	// Try file-level doc comment
	docSummary := e.extractFileDocComment(fullPath)
	if docSummary != "" {
		return &Responsibility{
			TargetID:     filePath,
			TargetType:   "file",
			Summary:      docSummary,
			Capabilities: []string{},
			Source:       "declared",
			Confidence:   0.89,
			UpdatedAt:    time.Now(),
		}, nil
	}

	// Infer from file name and contents
	inferredSummary := e.inferFromFileName(filePath)
	return &Responsibility{
		TargetID:     filePath,
		TargetType:   "file",
		Summary:      inferredSummary,
		Capabilities: []string{},
		Source:       "inferred",
		Confidence:   0.49,
		UpdatedAt:    time.Now(),
	}, nil
}

// extractFromReadme extracts summary from README.md
func (e *Extractor) extractFromReadme(dirPath string) (string, []string) {
	readmeNames := []string{"README.md", "README", "readme.md"}

	for _, name := range readmeNames {
		readmePath := filepath.Join(dirPath, name)
		if _, err := os.Stat(readmePath); err == nil {
			return e.parseReadme(readmePath)
		}
	}

	return "", nil
}

// parseReadme extracts the first paragraph and any bullet points as capabilities
func (e *Extractor) parseReadme(path string) (string, []string) {
	file, err := os.Open(path)
	if err != nil {
		return "", nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var summary string
	var capabilities []string
	inFirstParagraph := false
	paragraphLines := []string{}

	for scanner.Scan() {
		line := scanner.Text()

		// Skip title lines (# heading)
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Skip badges and empty lines at start
		if strings.TrimSpace(line) == "" {
			if inFirstParagraph {
				// End of first paragraph
				break
			}
			continue
		}

		// Skip badge lines (usually start with ![)
		if strings.HasPrefix(strings.TrimSpace(line), "![") {
			continue
		}

		// Collect bullet points as capabilities
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			cap := strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), "* ")
			if len(cap) > 0 && len(cap) < 100 {
				capabilities = append(capabilities, cap)
			}
			continue
		}

		// First real content line starts the paragraph
		inFirstParagraph = true
		paragraphLines = append(paragraphLines, line)
	}

	if len(paragraphLines) > 0 {
		summary = strings.Join(paragraphLines, " ")
		// Trim to reasonable length
		if len(summary) > 200 {
			summary = summary[:197] + "..."
		}
	}

	return summary, capabilities
}

// extractFromGoPackageDoc extracts the package doc comment from Go files
func (e *Extractor) extractFromGoPackageDoc(dirPath string) (string, []string) {
	// Look for doc.go first, then any .go file
	docGoPath := filepath.Join(dirPath, "doc.go")
	if _, err := os.Stat(docGoPath); err == nil {
		return e.parseGoDocComment(docGoPath)
	}

	// Try any .go file
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return "", nil
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") && !strings.HasSuffix(entry.Name(), "_test.go") {
			goPath := filepath.Join(dirPath, entry.Name())
			summary, caps := e.parseGoDocComment(goPath)
			if summary != "" {
				return summary, caps
			}
		}
	}

	return "", nil
}

// parseGoDocComment extracts the package-level doc comment
func (e *Extractor) parseGoDocComment(path string) (string, []string) {
	file, err := os.Open(path)
	if err != nil {
		return "", nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var docLines []string
	inDocComment := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Start of package declaration ends doc comment
		if strings.HasPrefix(trimmed, "package ") {
			break
		}

		// Doc comment line
		if strings.HasPrefix(trimmed, "//") {
			inDocComment = true
			// Remove // prefix and leading space
			docLine := strings.TrimPrefix(trimmed, "//")
			docLine = strings.TrimPrefix(docLine, " ")
			docLines = append(docLines, docLine)
		} else if strings.TrimSpace(line) == "" && inDocComment {
			// Empty line in comment section
			docLines = append(docLines, "")
		} else if inDocComment {
			// Non-comment, non-empty line before package - reset
			docLines = nil
			inDocComment = false
		}
	}

	if len(docLines) == 0 {
		return "", nil
	}

	// First line is usually "Package X does Y"
	summary := strings.Join(docLines, " ")
	summary = strings.TrimSpace(summary)

	// Extract "Package X" pattern
	packagePattern := regexp.MustCompile(`^Package\s+\w+\s+(.+)`)
	if matches := packagePattern.FindStringSubmatch(summary); len(matches) > 1 {
		summary = strings.TrimSuffix(matches[1], ".")
	}

	if len(summary) > 200 {
		summary = summary[:197] + "..."
	}

	return summary, nil
}

// extractFileDocComment extracts the first doc comment from a file
func (e *Extractor) extractFileDocComment(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var docLines []string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip empty lines at start
		if trimmed == "" {
			continue
		}

		// Comment line
		if strings.HasPrefix(trimmed, "//") {
			docLine := strings.TrimPrefix(trimmed, "//")
			docLine = strings.TrimPrefix(docLine, " ")
			docLines = append(docLines, docLine)
		} else {
			// Non-comment line - stop
			break
		}
	}

	if len(docLines) == 0 {
		return ""
	}

	summary := strings.Join(docLines, " ")
	if len(summary) > 200 {
		summary = summary[:197] + "..."
	}

	return summary
}

// inferFromExports creates a summary from exported symbols
func (e *Extractor) inferFromExports(dirPath string) (string, []string) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return "", nil
	}

	var exports []string
	exportPattern := regexp.MustCompile(`^(func|type|var|const)\s+([A-Z]\w*)`)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		filePath := filepath.Join(dirPath, entry.Name())
		file, err := os.Open(filePath)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			if matches := exportPattern.FindStringSubmatch(line); len(matches) > 2 {
				exports = append(exports, matches[2])
				if len(exports) >= 10 {
					break
				}
			}
		}
		file.Close()

		if len(exports) >= 10 {
			break
		}
	}

	if len(exports) == 0 {
		return "", nil
	}

	// Create summary from exports
	summary := "Provides " + strings.Join(exports[:min(5, len(exports))], ", ")
	if len(exports) > 5 {
		summary += fmt.Sprintf(" and %d more", len(exports)-5)
	}

	return summary, exports
}

// inferFromFileName creates a summary from the file name
func (e *Extractor) inferFromFileName(filePath string) string {
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// Convert snake_case or camelCase to words
	name = regexp.MustCompile(`[_-]`).ReplaceAllString(name, " ")
	name = regexp.MustCompile(`([a-z])([A-Z])`).ReplaceAllString(name, "$1 $2")
	name = strings.ToLower(name)

	// Determine role from name patterns
	switch {
	case strings.Contains(name, "test"):
		return "Contains tests"
	case strings.Contains(name, "config"):
		return "Configuration for " + name
	case strings.Contains(name, "util") || strings.Contains(name, "helper"):
		return "Utility functions"
	case strings.Contains(name, "handler"):
		return "Request handler"
	case strings.Contains(name, "service"):
		return "Service implementation"
	case strings.Contains(name, "model") || strings.Contains(name, "types"):
		return "Data types and models"
	default:
		return "Implements " + name + " functionality"
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
