package decisions

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ckb/internal/paths"
)

// Parser parses ADR markdown files
type Parser struct {
	repoRoot string
}

// NewParser creates a new ADR parser
func NewParser(repoRoot string) *Parser {
	return &Parser{repoRoot: repoRoot}
}

// FindADRDirectories returns directories that may contain ADRs (repo-local paths only)
// For the v6.0 global persistence path, use GetGlobalDecisionsDir()
func (p *Parser) FindADRDirectories() []string {
	candidates := []string{
		"docs/decisions",
		"docs/adr",
		"adr",
		"decisions",
		"doc/adr",
		"doc/decisions",
	}

	var found []string
	for _, dir := range candidates {
		fullPath := filepath.Join(p.repoRoot, dir)
		if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
			found = append(found, dir)
		}
	}

	return found
}

// GetGlobalDecisionsDir returns the v6.0 global persistence path for ADRs
// Path: ~/.ckb/repos/<hash>/decisions/
// Returns empty string if the directory doesn't exist
func (p *Parser) GetGlobalDecisionsDir() string {
	globalDir, err := paths.GetDecisionsDir(p.repoRoot)
	if err != nil {
		return ""
	}
	if info, err := os.Stat(globalDir); err == nil && info.IsDir() {
		return globalDir
	}
	return ""
}

// FindAllADRDirectories returns all directories that may contain ADRs,
// including both repo-local paths and the v6.0 global persistence path
func (p *Parser) FindAllADRDirectories() []ADRDirectory {
	var dirs []ADRDirectory

	// Add repo-local directories (relative paths)
	for _, dir := range p.FindADRDirectories() {
		dirs = append(dirs, ADRDirectory{
			Path:       dir,
			IsAbsolute: false,
		})
	}

	// Add v6.0 global persistence path (absolute path)
	if globalDir := p.GetGlobalDecisionsDir(); globalDir != "" {
		dirs = append(dirs, ADRDirectory{
			Path:       globalDir,
			IsAbsolute: true,
		})
	}

	return dirs
}

// ADRDirectory represents a directory containing ADRs
type ADRDirectory struct {
	Path       string // Relative path for repo-local, absolute path for global
	IsAbsolute bool   // True if Path is an absolute path
}

// ParseDirectory parses all ADRs in a directory (relative to repoRoot)
func (p *Parser) ParseDirectory(dirPath string) ([]*ArchitecturalDecision, error) {
	fullPath := filepath.Join(p.repoRoot, dirPath)
	return p.parseDirectoryInternal(fullPath, dirPath)
}

// ParseDirectoryAbsolute parses all ADRs in an absolute directory path
func (p *Parser) ParseDirectoryAbsolute(absPath string) ([]*ArchitecturalDecision, error) {
	return p.parseDirectoryInternal(absPath, absPath)
}

// parseDirectoryInternal is the internal implementation for parsing ADR directories
func (p *Parser) parseDirectoryInternal(fullPath, pathForFilePath string) ([]*ArchitecturalDecision, error) {
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var adrs []*ArchitecturalDecision
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}

		// Check if filename matches ADR pattern
		if !isADRFile(name) {
			continue
		}

		adrPath := filepath.Join(fullPath, name)
		adr, err := p.ParseFileAbsolute(adrPath)
		if err != nil {
			// Log but continue
			continue
		}

		// Set the file path for storage
		adr.FilePath = filepath.Join(pathForFilePath, name)
		adrs = append(adrs, adr)
	}

	return adrs, nil
}

// ParseFile parses a single ADR markdown file (relative to repoRoot)
func (p *Parser) ParseFile(relPath string) (*ArchitecturalDecision, error) {
	fullPath := filepath.Join(p.repoRoot, relPath)
	adr, err := p.parseFileInternal(fullPath)
	if err != nil {
		return nil, err
	}
	adr.FilePath = relPath
	return adr, nil
}

// ParseFileAbsolute parses a single ADR markdown file from an absolute path
func (p *Parser) ParseFileAbsolute(absPath string) (*ArchitecturalDecision, error) {
	adr, err := p.parseFileInternal(absPath)
	if err != nil {
		return nil, err
	}
	adr.FilePath = absPath
	return adr, nil
}

// ParseFileAbsolute is a package-level helper to parse an ADR from an absolute path
// without needing to create a Parser instance
func ParseFileAbsolute(absPath string) (*ArchitecturalDecision, error) {
	// Create a temporary parser - repoRoot not needed for absolute paths
	p := &Parser{}
	return p.ParseFileAbsolute(absPath)
}

// parseFileInternal is the internal implementation for parsing ADR files
func (p *Parser) parseFileInternal(fullPath string) (*ArchitecturalDecision, error) {
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	adr := &ArchitecturalDecision{}

	scanner := bufio.NewScanner(file)
	var currentSection string
	var sectionContent []string

	// Patterns for extracting metadata
	titlePattern := regexp.MustCompile(`^#\s*(ADR[-\s]?\d+)[:\s]*(.*)$`)
	statusPattern := regexp.MustCompile(`(?i)\*?\*?Status:?\*?\*?\s*(.+)`)
	datePattern := regexp.MustCompile(`(?i)\*?\*?Date:?\*?\*?\s*(.+)`)
	authorPattern := regexp.MustCompile(`(?i)\*?\*?Author:?\*?\*?\s*(.+)`)
	supersededPattern := regexp.MustCompile(`(?i)\*?\*?Superseded\s*by:?\*?\*?\s*(.+)`)
	sectionPattern := regexp.MustCompile(`^##\s*(.+)$`)

	for scanner.Scan() {
		line := scanner.Text()

		// Parse title line
		if matches := titlePattern.FindStringSubmatch(line); len(matches) > 2 {
			adr.ID = normalizeADRID(matches[1])
			adr.Title = strings.TrimSpace(matches[2])
			continue
		}

		// Parse status
		if matches := statusPattern.FindStringSubmatch(line); len(matches) > 1 {
			adr.Status = strings.ToLower(strings.TrimSpace(matches[1]))
			continue
		}

		// Parse date
		if matches := datePattern.FindStringSubmatch(line); len(matches) > 1 {
			dateStr := strings.TrimSpace(matches[1])
			if t, err := parseDate(dateStr); err == nil {
				adr.Date = t
			}
			continue
		}

		// Parse author
		if matches := authorPattern.FindStringSubmatch(line); len(matches) > 1 {
			adr.Author = strings.TrimSpace(matches[1])
			continue
		}

		// Parse superseded by
		if matches := supersededPattern.FindStringSubmatch(line); len(matches) > 1 {
			adr.SupersededBy = strings.TrimSpace(matches[1])
			continue
		}

		// Section header
		if matches := sectionPattern.FindStringSubmatch(line); len(matches) > 1 {
			// Save previous section
			if currentSection != "" {
				saveSection(adr, currentSection, sectionContent)
			}
			currentSection = strings.ToLower(strings.TrimSpace(matches[1]))
			sectionContent = nil
			continue
		}

		// Content line
		if currentSection != "" {
			sectionContent = append(sectionContent, line)
		}
	}

	// Save last section
	if currentSection != "" {
		saveSection(adr, currentSection, sectionContent)
	}

	// Extract ID from filename if not found in content
	if adr.ID == "" {
		adr.ID = extractIDFromFilename(fullPath)
	}

	// Default status
	if adr.Status == "" {
		adr.Status = string(StatusProposed)
	}

	// Default date to file modification time
	if adr.Date.IsZero() {
		if info, err := os.Stat(fullPath); err == nil {
			adr.Date = info.ModTime()
		} else {
			adr.Date = time.Now()
		}
	}

	return adr, scanner.Err()
}

// saveSection saves parsed section content to the ADR
func saveSection(adr *ArchitecturalDecision, section string, content []string) {
	text := strings.TrimSpace(strings.Join(content, "\n"))

	switch {
	case strings.Contains(section, "context"):
		adr.Context = text
	case strings.Contains(section, "decision"):
		adr.Decision = text
	case strings.Contains(section, "consequence"):
		adr.Consequences = extractBulletPoints(content)
	case strings.Contains(section, "affected") || strings.Contains(section, "module"):
		adr.AffectedModules = extractBulletPoints(content)
	case strings.Contains(section, "alternative"):
		adr.Alternatives = extractBulletPoints(content)
	}
}

// extractBulletPoints extracts bullet point items from lines
func extractBulletPoints(lines []string) []string {
	var items []string
	bulletPattern := regexp.MustCompile(`^\s*[-*]\s*(.+)$`)

	for _, line := range lines {
		if matches := bulletPattern.FindStringSubmatch(line); len(matches) > 1 {
			item := strings.TrimSpace(matches[1])
			if item != "" {
				items = append(items, item)
			}
		}
	}

	return items
}

// isADRFile checks if a filename looks like an ADR
func isADRFile(name string) bool {
	lower := strings.ToLower(name)
	// Match patterns like: ADR-001.md, 001-some-title.md, adr-001-title.md
	adrPatterns := []string{
		`^adr[-_]?\d+`,
		`^\d{3,4}[-_]`,
	}

	for _, pattern := range adrPatterns {
		if matched, _ := regexp.MatchString(pattern, lower); matched {
			return true
		}
	}

	return false
}

// normalizeADRID normalizes an ADR ID to "ADR-NNN" format
func normalizeADRID(id string) string {
	// Extract number
	numPattern := regexp.MustCompile(`\d+`)
	numStr := numPattern.FindString(id)
	if numStr == "" {
		return id
	}

	// Pad to 3 digits
	var num int
	fmt.Sscanf(numStr, "%d", &num)
	return fmt.Sprintf("ADR-%03d", num)
}

// extractIDFromFilename extracts an ADR ID from a filename
func extractIDFromFilename(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, ".md")

	// Try to find ADR number
	numPattern := regexp.MustCompile(`\d{3,4}`)
	numStr := numPattern.FindString(base)
	if numStr != "" {
		var num int
		fmt.Sscanf(numStr, "%d", &num)
		return fmt.Sprintf("ADR-%03d", num)
	}

	return base
}

// parseDate attempts to parse various date formats
func parseDate(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02",
		"January 2, 2006",
		"Jan 2, 2006",
		"2006/01/02",
		"02-01-2006",
		"02/01/2006",
	}

	s = strings.TrimSpace(s)
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", s)
}

// GetNextADRNumber finds the next available ADR number
// Scans both repo-local directories and v6.0 global persistence path
func (p *Parser) GetNextADRNumber() (int, error) {
	maxNum := 0
	numPattern := regexp.MustCompile(`ADR-(\d+)`)

	// Helper to extract max ADR number from a list of ADRs
	updateMaxNum := func(adrs []*ArchitecturalDecision) {
		for _, adr := range adrs {
			if matches := numPattern.FindStringSubmatch(adr.ID); len(matches) > 1 {
				var num int
				fmt.Sscanf(matches[1], "%d", &num)
				if num > maxNum {
					maxNum = num
				}
			}
		}
	}

	// Scan repo-local directories
	dirs := p.FindADRDirectories()
	for _, dir := range dirs {
		adrs, err := p.ParseDirectory(dir)
		if err != nil {
			continue
		}
		updateMaxNum(adrs)
	}

	// Also scan v6.0 global persistence path (~/.ckb/repos/<hash>/decisions/)
	if globalDir, err := paths.GetDecisionsDir(p.repoRoot); err == nil {
		if adrs, err := p.ParseDirectoryAbsolute(globalDir); err == nil {
			updateMaxNum(adrs)
		}
	}

	return maxNum + 1, nil
}
