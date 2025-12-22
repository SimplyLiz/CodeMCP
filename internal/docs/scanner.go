package docs

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Scanner scans markdown files for symbol mentions.
type Scanner struct {
	repoRoot string
}

// NewScanner creates a new Scanner.
func NewScanner(repoRoot string) *Scanner {
	return &Scanner{repoRoot: repoRoot}
}

// Regex patterns for detection
var (
	// Backtick: `Symbol.Name` or `pkg.Symbol`
	// Must have at least one delimiter (., ::, #, /, ->) to avoid matching single words
	// Supports: Foo.Bar, crate::mod::Type, Class#method, pkg/sub.Func, Class->method
	backtickPattern = regexp.MustCompile("`([A-Za-z_][A-Za-z0-9_]*(?:(?:::|->|[.#/])[A-Za-z_][A-Za-z0-9_]*)+)`")

	// File extension pattern - used to filter out file paths from symbol detection
	fileExtPattern = regexp.MustCompile(`\.(go|js|ts|tsx|jsx|py|rs|java|kt|rb|c|cpp|h|hpp|cs|swift|md|json|yaml|yml|toml|xml|html|css|scss|sql|sh|bash|zsh|scip|lock|sum|mod)$`)

	// Fence start/end - allow leading whitespace, support ``` and ~~~
	fenceStartPattern = regexp.MustCompile(`^\s*(` + "```" + `|~~~)(\w*)\s*$`)
	fenceEndPattern   = regexp.MustCompile(`^\s*(` + "```" + `|~~~)\s*$`)

	// Symbol directive: <!-- ckb:symbol fully.qualified.Name -->
	symbolDirectivePattern = regexp.MustCompile(`<!--\s*ckb:symbol\s+([^\s>]+)\s*-->`)

	// Module directive: <!-- ckb:module internal/auth -->
	moduleDirectivePattern = regexp.MustCompile(`<!--\s*ckb:module\s+([^\s>]+)\s*-->`)
)

// ScanFile scans a single markdown file for symbol mentions.
func (s *Scanner) ScanFile(path string) ScanResult {
	result := ScanResult{
		Doc: Document{
			Path: s.relativePath(path),
			Type: s.detectDocType(path),
		},
	}

	file, err := os.Open(path)
	if err != nil {
		result.Error = err
		return result
	}
	defer func() { _ = file.Close() }()

	// Compute hash
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		result.Error = err
		return result
	}
	result.Doc.Hash = fmt.Sprintf("%x", hash.Sum(nil))

	// Reset file for scanning
	if _, err := file.Seek(0, 0); err != nil {
		result.Error = err
		return result
	}

	scanner := bufio.NewScanner(file)
	// Support up to 1MB lines for large files
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineNum := 0
	inFence := false
	fenceDelimiter := "" // Track which delimiter started the fence (``` or ~~~)
	var lines []string

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		lines = append(lines, line)

		// Track fenced code blocks (must match same delimiter type)
		if !inFence {
			if match := fenceStartPattern.FindStringSubmatch(line); match != nil {
				inFence = true
				fenceDelimiter = match[1] // ``` or ~~~
				continue
			}
		} else {
			if match := fenceEndPattern.FindStringSubmatch(line); match != nil {
				// Only end fence if delimiter matches
				if match[1] == fenceDelimiter {
					inFence = false
					fenceDelimiter = ""
				}
				continue
			}
		}

		// Extract title from first heading
		if result.Doc.Title == "" && strings.HasPrefix(line, "# ") {
			result.Doc.Title = strings.TrimPrefix(line, "# ")
		}

		// Check for directives (work inside and outside fences)
		s.scanDirectives(line, lineNum, lines, &result)

		// Scan backticks (work inside and outside fences in v1)
		s.scanBackticks(line, lineNum, lines, &result)
	}

	if err := scanner.Err(); err != nil {
		result.Error = err
		return result
	}

	result.Doc.LastIndexed = time.Now()

	return result
}

// scanDirectives scans for ckb: directives in a line.
func (s *Scanner) scanDirectives(line string, lineNum int, lines []string, result *ScanResult) {
	// Symbol directives
	if matches := symbolDirectivePattern.FindAllStringSubmatchIndex(line, -1); matches != nil {
		for _, match := range matches {
			rawText := line[match[2]:match[3]]
			result.Mentions = append(result.Mentions, Mention{
				RawText: rawText,
				Line:    lineNum,
				Column:  match[2] + 1,
				Context: s.extractContext(lines, lineNum-1),
				Method:  DetectDirective,
			})
		}
	}

	// Module directives
	if matches := moduleDirectivePattern.FindAllStringSubmatchIndex(line, -1); matches != nil {
		for _, match := range matches {
			moduleID := line[match[2]:match[3]]
			result.Modules = append(result.Modules, ModuleLink{
				ModuleID: moduleID,
				Line:     lineNum,
			})
		}
	}
}

// scanBackticks scans for backtick-quoted symbol references.
func (s *Scanner) scanBackticks(line string, lineNum int, lines []string, result *ScanResult) {
	matches := backtickPattern.FindAllStringSubmatchIndex(line, -1)
	for _, match := range matches {
		rawText := line[match[0]:match[1]] // Include backticks
		innerText := line[match[2]:match[3]]

		// Skip file paths (things with file extensions)
		if fileExtPattern.MatchString(innerText) {
			continue
		}

		result.Mentions = append(result.Mentions, Mention{
			RawText: rawText,
			Line:    lineNum,
			Column:  match[0] + 1,
			Context: s.extractContext(lines, lineNum-1),
			Method:  DetectBacktick,
		})
	}
}

// extractContext extracts surrounding text for context.
func (s *Scanner) extractContext(lines []string, lineIndex int) string {
	if lineIndex < 0 || lineIndex >= len(lines) {
		return ""
	}

	line := lines[lineIndex]
	if len(line) <= 100 {
		return line
	}
	return line[:100] + "..."
}

// detectDocType determines the document type from the path.
func (s *Scanner) detectDocType(path string) DocType {
	lower := strings.ToLower(filepath.Base(path))

	// ADR detection
	if strings.HasPrefix(lower, "adr-") ||
		strings.HasPrefix(lower, "adr_") ||
		strings.Contains(filepath.Dir(path), "adr") ||
		strings.Contains(filepath.Dir(path), "decisions") {
		return DocTypeADR
	}

	return DocTypeMarkdown
}

// relativePath converts an absolute path to a repo-relative path.
func (s *Scanner) relativePath(path string) string {
	if rel, err := filepath.Rel(s.repoRoot, path); err == nil {
		return rel
	}
	return path
}

// ScanDirectory scans a directory for markdown files.
func (s *Scanner) ScanDirectory(dir string, exclude []string) ([]ScanResult, error) {
	var results []ScanResult

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip excluded directories
		if info.IsDir() {
			name := info.Name()
			for _, ex := range exclude {
				if name == ex || strings.HasPrefix(name, ".") {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Only process markdown files
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".markdown" {
			return nil
		}

		result := s.ScanFile(path)
		results = append(results, result)

		return nil
	})

	return results, err
}

// Normalize cleans up a raw mention for matching.
func Normalize(raw string) string {
	s := raw

	// Remove backticks
	s = strings.Trim(s, "`")

	// Normalize language-specific delimiters to dots
	s = strings.ReplaceAll(s, "::", ".") // Rust: crate::module::Type
	s = strings.ReplaceAll(s, "#", ".")  // JS/TS: Class#method
	s = strings.ReplaceAll(s, "->", ".") // PHP: Class->method
	s = strings.ReplaceAll(s, "/", ".")  // Go: package/subpkg.Func

	// Remove leading/trailing dots
	s = strings.Trim(s, ".")

	return s
}

// CountSegments returns the number of dot-separated segments.
func CountSegments(normalized string) int {
	if normalized == "" {
		return 0
	}
	return len(strings.Split(normalized, "."))
}
