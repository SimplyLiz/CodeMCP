package compliance

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// PIIField represents a detected PII field in source code.
type PIIField struct {
	Name       string  `json:"name"`
	Container  string  `json:"container,omitempty"` // Struct/class name
	File       string  `json:"file"`
	Line       int     `json:"line"`
	PIIType    string  `json:"piiType"`    // "name", "contact", "address", etc.
	Category   string  `json:"category"`   // "direct-identifier", "quasi-identifier", "sensitive"
	Confidence float64 `json:"confidence"` // 0.0-1.0
}

// PIIScanner detects PII fields in source code.
type PIIScanner struct {
	patterns   []PIIPattern
	normalized map[string]PIIPattern // Lookup by normalized pattern
}

// NewPIIScanner creates a scanner with default + custom patterns.
func NewPIIScanner(extraPatterns []string) *PIIScanner {
	patterns := DefaultPIIPatterns()

	// Add custom patterns as direct-identifiers
	for _, p := range extraPatterns {
		patterns = append(patterns, PIIPattern{
			Pattern:  normalizeIdentifier(p),
			Category: "direct-identifier",
			PIIType:  "custom",
		})
	}

	normalized := make(map[string]PIIPattern, len(patterns))
	for _, p := range patterns {
		normalized[p.Pattern] = p
	}

	return &PIIScanner{
		patterns:   patterns,
		normalized: normalized,
	}
}

// ScanFiles detects PII fields across all files in scope.
func (s *PIIScanner) ScanFiles(ctx context.Context, scope *ScanScope) ([]PIIField, error) {
	var allFields []PIIField

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return allFields, ctx.Err()
		}

		fields, err := s.scanFile(filepath.Join(scope.RepoRoot, file), file)
		if err != nil {
			scope.Logger.Debug("PII scan skipped file", "file", file, "error", err.Error())
			continue
		}
		allFields = append(allFields, fields...)
	}

	return allFields, nil
}

// scanFile scans a single file for PII field declarations.
func (s *PIIScanner) scanFile(fullPath, relPath string) ([]PIIField, error) {
	f, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var fields []PIIField
	scanner := bufio.NewScanner(f)
	lineNum := 0
	currentContainer := ""

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Track struct/class/type context
		if container := extractContainer(line); container != "" {
			currentContainer = container
		}
		// Reset container context on closing brace at column 0
		if strings.HasPrefix(strings.TrimSpace(line), "}") && !strings.Contains(line, "{") {
			if len(strings.TrimSpace(line)) <= 2 {
				currentContainer = ""
			}
		}

		// Extract identifiers from the line and check against PII patterns
		identifiers := extractIdentifiers(line)
		for _, ident := range identifiers {
			normalized := normalizeIdentifier(ident)
			if p, ok := s.matchPII(normalized); ok {
				confidence := 0.65
				if p.Category == "direct-identifier" {
					confidence = 0.70
				}
				if p.Category == "sensitive" {
					confidence = 0.75
				}
				// Higher confidence if in a struct/class declaration context
				if currentContainer != "" && isFieldDeclaration(line) {
					confidence += 0.10
				}

				fields = append(fields, PIIField{
					Name:       ident,
					Container:  currentContainer,
					File:       relPath,
					Line:       lineNum,
					PIIType:    p.PIIType,
					Category:   p.Category,
					Confidence: confidence,
				})
			}
		}
	}

	return fields, scanner.Err()
}

// CheckPIIInLogs finds PII field names used in log/print statements.
func (s *PIIScanner) CheckPIIInLogs(ctx context.Context, scope *ScanScope) ([]Finding, error) {
	var findings []Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Skip test files — test assertions naturally reference PII field names
		if strings.HasSuffix(file, "_test.go") || strings.HasSuffix(file, "_test.py") ||
			strings.HasSuffix(file, ".test.ts") || strings.HasSuffix(file, ".test.js") ||
			strings.Contains(file, "testdata/") || strings.Contains(file, "fixtures") {
			continue
		}

		func() {
			f, err := os.Open(filepath.Join(scope.RepoRoot, file))
			if err != nil {
				return
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			lineNum := 0

			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				lineLower := strings.ToLower(line)

				// Check if this is a log statement
				if !isLogStatement(lineLower) {
					continue
				}

				// Check for PII identifiers in the log line
				identifiers := extractIdentifiers(line)
				for _, ident := range identifiers {
					normalized := normalizeIdentifier(ident)
					if p, ok := s.matchPII(normalized); ok {
						findings = append(findings, Finding{
							Severity:   "error",
							File:       file,
							StartLine:  lineNum,
							Message:    "PII field '" + ident + "' (" + p.PIIType + ") found in log statement",
							Suggestion: "Remove PII from logs or use a redaction/masking function",
							Confidence: 0.85,
						})
					}
				}
			}
		}()
	}

	return findings, nil
}

// CheckPIIInErrors finds PII field names used in error messages/returns.
func (s *PIIScanner) CheckPIIInErrors(ctx context.Context, scope *ScanScope) ([]Finding, error) {
	var findings []Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		func() {
			f, err := os.Open(filepath.Join(scope.RepoRoot, file))
			if err != nil {
				return
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			lineNum := 0

			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				lineLower := strings.ToLower(line)

				if !isErrorStatement(lineLower) {
					continue
				}

				identifiers := extractIdentifiers(line)
				for _, ident := range identifiers {
					normalized := normalizeIdentifier(ident)
					if p, ok := s.matchPII(normalized); ok {
						findings = append(findings, Finding{
							Severity:   "error",
							File:       file,
							StartLine:  lineNum,
							Message:    "PII field '" + ident + "' (" + p.PIIType + ") exposed in error message",
							Suggestion: "Do not include PII in error messages returned to clients",
							Confidence: 0.80,
						})
					}
				}
			}
		}()
	}

	return findings, nil
}

// matchPII checks if a normalized identifier matches any PII pattern.
func (s *PIIScanner) matchPII(normalized string) (PIIPattern, bool) {
	// Skip known non-PII identifiers that contain PII-like substrings
	if isNonPIIIdentifier(normalized) {
		return PIIPattern{}, false
	}

	// Exact match
	if p, ok := s.normalized[normalized]; ok {
		return p, true
	}

	// Suffix match: "user_email" matches "email", "customer_phone" matches "phone"
	// Only suffix match for patterns > 4 chars to avoid false positives with short words like "name"
	for _, p := range s.patterns {
		if len(p.Pattern) > 4 && strings.HasSuffix(normalized, "_"+p.Pattern) {
			return p, true
		}
	}

	return PIIPattern{}, false
}

// isNonPIIIdentifier filters out identifiers that look like PII but aren't.
func isNonPIIIdentifier(normalized string) bool {
	// "fingerprint" in code almost always means hash/checksum, not biometric.
	// Only flag it when paired with actual biometric context.
	if strings.Contains(normalized, "fingerprint") {
		// Allow: biometric_fingerprint, user_fingerprint
		// Reject: build_fingerprint, symbol_fingerprint, sarif_fingerprint, etc.
		if !strings.Contains(normalized, "biometric") && !strings.Contains(normalized, "user_fingerprint") {
			return true
		}
	}

	// "display_name" / "language_display_name" refer to UI labels, not people
	if strings.Contains(normalized, "display_name") {
		return true
	}

	// Identifiers where "name" refers to code entities, not people
	nonPIISuffixes := []string{
		"file_name", "filename", "func_name", "function_name",
		"method_name", "class_name", "package_name", "module_name",
		"table_name", "column_name", "field_name", "type_name",
		"var_name", "variable_name", "param_name", "parameter_name",
		"tag_name", "symbol_name", "check_name", "rule_name",
		"host_name", "hostname", "repo_name", "branch_name",
		"command_name", "tool_name", "test_name", "config_name",
		"event_name", "metric_name", "key_name", "flag_name",
		"header_name", "cookie_name", "schema_name", "index_name",
		"service_name", "container_name", "image_name", "node_name",
		"cluster_name", "namespace_name", "resource_name",
		"framework_name", "backend_name", "frontend_name",
	}

	for _, suffix := range nonPIISuffixes {
		if normalized == suffix || strings.HasSuffix(normalized, "_"+suffix) {
			return true
		}
	}

	return false
}

// normalizeIdentifier converts any casing convention to snake_case for matching.
func normalizeIdentifier(s string) string {
	if s == "" {
		return ""
	}

	var result []rune
	runes := []rune(s)

	for i, r := range runes {
		if unicode.IsUpper(r) {
			// Insert underscore before uppercase letter (camelCase/PascalCase boundary)
			// but not if previous char is already an underscore
			if i > 0 && runes[i-1] != '_' && !unicode.IsUpper(runes[i-1]) {
				result = append(result, '_')
			}
			// Handle consecutive uppercase: "HTMLParser" -> "html_parser"
			if i > 0 && unicode.IsUpper(runes[i-1]) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				result = append(result, '_')
			}
			result = append(result, unicode.ToLower(r))
		} else {
			result = append(result, unicode.ToLower(r))
		}
	}

	// Collapse double underscores that may result from SCREAMING_SNAKE_CASE
	normalized := string(result)
	for strings.Contains(normalized, "__") {
		normalized = strings.ReplaceAll(normalized, "__", "_")
	}

	return normalized
}

// extractContainer detects struct/class/type declarations.
var containerPatterns = []*regexp.Regexp{
	regexp.MustCompile(`type\s+(\w+)\s+struct\b`),              // Go
	regexp.MustCompile(`class\s+(\w+)`),                        // Java/Python/TS
	regexp.MustCompile(`interface\s+(\w+)`),                    // TS/Java/Go
	regexp.MustCompile(`(?:export\s+)?type\s+(\w+)\s*=?\s*\{`), // TypeScript type
	regexp.MustCompile(`data\s+class\s+(\w+)`),                 // Kotlin
	regexp.MustCompile(`struct\s+(\w+)`),                       // Rust/C
	regexp.MustCompile(`(?:pub\s+)?struct\s+(\w+)`),            // Rust
}

func extractContainer(line string) string {
	trimmed := strings.TrimSpace(line)
	for _, re := range containerPatterns {
		if m := re.FindStringSubmatch(trimmed); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

// identifierRe matches identifiers in source code.
var identifierRe = regexp.MustCompile(`[a-zA-Z_][a-zA-Z0-9_]*`)

func extractIdentifiers(line string) []string {
	// Skip comments
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "*") {
		return nil
	}

	matches := identifierRe.FindAllString(line, -1)
	// Deduplicate and filter short identifiers
	seen := make(map[string]bool, len(matches))
	var result []string
	for _, m := range matches {
		if len(m) < 3 || seen[m] {
			continue
		}
		// Skip common keywords
		if isCommonKeyword(m) {
			continue
		}
		seen[m] = true
		result = append(result, m)
	}
	return result
}

func isFieldDeclaration(line string) bool {
	trimmed := strings.TrimSpace(line)
	// Go struct field: "Name string `json:"name"`"
	// TypeScript: "name: string;"
	// Java/Kotlin: "private String name;"
	return !strings.HasPrefix(trimmed, "func ") &&
		!strings.HasPrefix(trimmed, "function ") &&
		!strings.HasPrefix(trimmed, "def ") &&
		!strings.HasPrefix(trimmed, "if ") &&
		!strings.HasPrefix(trimmed, "for ") &&
		!strings.HasPrefix(trimmed, "return ") &&
		(strings.Contains(line, "string") || strings.Contains(line, "String") ||
			strings.Contains(line, "int") || strings.Contains(line, "Int") ||
			strings.Contains(line, "`json:") || strings.Contains(line, ":") ||
			strings.Contains(line, "="))
}

func isLogStatement(lineLower string) bool {
	for _, pattern := range LogFunctionPatterns {
		if strings.Contains(lineLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

func isErrorStatement(lineLower string) bool {
	patterns := []string{
		"fmt.errorf", "errors.new", "error(", "raise ",
		"throw new", "throw ", "errorf(",
		"return err", "return error",
		"new error(", "new exception(",
		"httperror", "apierror", "responseerror",
	}
	for _, p := range patterns {
		if strings.Contains(lineLower, p) {
			return true
		}
	}
	return false
}

func isCommonKeyword(s string) bool {
	switch strings.ToLower(s) {
	case "func", "function", "def", "class", "struct", "interface", "type",
		"var", "let", "const", "val", "pub", "private", "public", "protected",
		"return", "import", "package", "from", "export", "default",
		"string", "int", "bool", "float", "byte", "void", "nil", "null",
		"true", "false", "err", "error", "context", "ctx",
		"for", "if", "else", "switch", "case", "break", "continue",
		"new", "make", "append", "len", "map", "range", "select",
		"this", "self", "super", "try", "catch", "finally", "async", "await",
		"json", "xml", "http", "https", "api", "url", "uri",
		"get", "set", "put", "post", "delete", "patch",
		"test", "main", "init", "fmt", "log", "slog":
		return true
	}
	return false
}
