package decisions

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

// Writer generates ADR markdown files
type Writer struct {
	repoRoot   string
	outputDir  string // relative to repoRoot
}

// NewWriter creates a new ADR writer
func NewWriter(repoRoot, outputDir string) *Writer {
	return &Writer{
		repoRoot:  repoRoot,
		outputDir: outputDir,
	}
}

// CreateADR creates a new ADR file and returns the relative path
func (w *Writer) CreateADR(adr *ArchitecturalDecision) (string, error) {
	// Ensure output directory exists
	fullDir := filepath.Join(w.repoRoot, w.outputDir)
	if err := os.MkdirAll(fullDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Generate filename
	filename := generateFilename(adr.ID, adr.Title)
	relPath := filepath.Join(w.outputDir, filename)
	fullPath := filepath.Join(w.repoRoot, relPath)

	// Check if file already exists
	if _, err := os.Stat(fullPath); err == nil {
		return "", fmt.Errorf("ADR file already exists: %s", relPath)
	}

	// Generate content
	content, err := w.generateContent(adr)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	// Write file
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	adr.FilePath = relPath
	return relPath, nil
}

// UpdateADR updates an existing ADR file
func (w *Writer) UpdateADR(adr *ArchitecturalDecision) error {
	if adr.FilePath == "" {
		return fmt.Errorf("ADR has no file path")
	}

	fullPath := filepath.Join(w.repoRoot, adr.FilePath)

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return fmt.Errorf("ADR file not found: %s", adr.FilePath)
	}

	// Generate content
	content, err := w.generateContent(adr)
	if err != nil {
		return fmt.Errorf("failed to generate content: %w", err)
	}

	// Write file
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// generateContent generates the markdown content for an ADR
func (w *Writer) generateContent(adr *ArchitecturalDecision) (string, error) {
	tmpl, err := template.New("adr").Parse(adrTemplate)
	if err != nil {
		return "", err
	}

	// Prepare template data
	data := struct {
		ID              string
		Title           string
		Status          string
		Date            string
		Author          string
		Context         string
		Decision        string
		Consequences    []string
		AffectedModules []string
		Alternatives    []string
		SupersededBy    string
	}{
		ID:              adr.ID,
		Title:           adr.Title,
		Status:          adr.Status,
		Date:            adr.Date.Format("2006-01-02"),
		Author:          adr.Author,
		Context:         adr.Context,
		Decision:        adr.Decision,
		Consequences:    adr.Consequences,
		AffectedModules: adr.AffectedModules,
		Alternatives:    adr.Alternatives,
		SupersededBy:    adr.SupersededBy,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// generateFilename creates a filename for an ADR
func generateFilename(id, title string) string {
	// Clean title for filename
	cleanTitle := strings.ToLower(title)
	cleanTitle = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return '-'
		}
		return -1
	}, cleanTitle)

	// Remove multiple dashes
	for strings.Contains(cleanTitle, "--") {
		cleanTitle = strings.ReplaceAll(cleanTitle, "--", "-")
	}
	cleanTitle = strings.Trim(cleanTitle, "-")

	// Truncate if too long
	if len(cleanTitle) > 50 {
		cleanTitle = cleanTitle[:50]
		cleanTitle = strings.TrimSuffix(cleanTitle, "-")
	}

	return fmt.Sprintf("%s-%s.md", strings.ToLower(id), cleanTitle)
}

// adrTemplate is the template for generating ADR markdown
const adrTemplate = `# {{.ID}}: {{.Title}}

**Status:** {{.Status}}

**Date:** {{.Date}}
{{if .Author}}
**Author:** {{.Author}}
{{end}}{{if .SupersededBy}}
**Superseded by:** {{.SupersededBy}}
{{end}}
## Context

{{.Context}}

## Decision

{{.Decision}}

## Consequences

{{range .Consequences}}- {{.}}
{{end}}
{{if .AffectedModules}}
## Affected Modules

{{range .AffectedModules}}- {{.}}
{{end}}{{end}}{{if .Alternatives}}
## Alternatives Considered

{{range .Alternatives}}- {{.}}
{{end}}{{end}}`

// EnsureOutputDir ensures the output directory exists and returns its path
func (w *Writer) EnsureOutputDir() (string, error) {
	fullDir := filepath.Join(w.repoRoot, w.outputDir)
	if err := os.MkdirAll(fullDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	return w.outputDir, nil
}

// GetDefaultOutputDir returns the default ADR output directory
func GetDefaultOutputDir(repoRoot string) string {
	// Check for existing ADR directories
	candidates := []string{
		"docs/decisions",
		"docs/adr",
		"adr",
		"decisions",
	}

	for _, dir := range candidates {
		fullPath := filepath.Join(repoRoot, dir)
		if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
			return dir
		}
	}

	// Default to docs/decisions
	return "docs/decisions"
}

// NewADR creates a new ADR with default values
func NewADR(id int, title string) *ArchitecturalDecision {
	return &ArchitecturalDecision{
		ID:           fmt.Sprintf("ADR-%03d", id),
		Title:        title,
		Status:       string(StatusProposed),
		Date:         time.Now(),
		Consequences: []string{},
	}
}
