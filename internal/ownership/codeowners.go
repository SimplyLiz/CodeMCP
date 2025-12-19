package ownership

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CodeownersRule represents a single rule from a CODEOWNERS file
type CodeownersRule struct {
	// Pattern is the file pattern (glob-like)
	Pattern string `json:"pattern"`

	// Owners is the list of owners for this pattern
	Owners []string `json:"owners"`

	// LineNumber is the line number in the CODEOWNERS file
	LineNumber int `json:"lineNumber"`

	// IsNegation indicates if this is a negation pattern (starts with !)
	IsNegation bool `json:"isNegation,omitempty"`
}

// CodeownersFile represents a parsed CODEOWNERS file
type CodeownersFile struct {
	// Path is the path to the CODEOWNERS file
	Path string `json:"path"`

	// Rules is the list of rules in order (later rules override earlier)
	Rules []CodeownersRule `json:"rules"`
}

// ParseCodeownersFile parses a CODEOWNERS file from the given path
func ParseCodeownersFile(filePath string) (*CodeownersFile, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var rules []CodeownersRule
	scanner := bufio.NewScanner(file)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		rule := parseCodeownersLine(line, lineNumber)
		if rule != nil {
			rules = append(rules, *rule)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return &CodeownersFile{
		Path:  filePath,
		Rules: rules,
	}, nil
}

// parseCodeownersLine parses a single line from a CODEOWNERS file
func parseCodeownersLine(line string, lineNumber int) *CodeownersRule {
	// Split line into pattern and owners
	// Pattern is the first token, owners are the rest
	fields := strings.Fields(line)
	if len(fields) < 1 {
		return nil
	}

	pattern := fields[0]
	owners := fields[1:]

	// Check for negation
	isNegation := false
	if strings.HasPrefix(pattern, "!") {
		isNegation = true
		pattern = strings.TrimPrefix(pattern, "!")
	}

	// Negation patterns don't need owners
	if isNegation {
		return &CodeownersRule{
			Pattern:    pattern,
			Owners:     []string{},
			LineNumber: lineNumber,
			IsNegation: true,
		}
	}

	// Non-negation patterns need at least one owner
	if len(owners) == 0 {
		return nil
	}

	// Validate owners (should start with @ or be an email)
	validOwners := make([]string, 0, len(owners))
	for _, owner := range owners {
		if isValidOwner(owner) {
			validOwners = append(validOwners, owner)
		}
	}

	if len(validOwners) == 0 {
		return nil
	}

	return &CodeownersRule{
		Pattern:    pattern,
		Owners:     validOwners,
		LineNumber: lineNumber,
		IsNegation: isNegation,
	}
}

// isValidOwner checks if an owner string is valid
func isValidOwner(owner string) bool {
	// GitHub usernames start with @
	if strings.HasPrefix(owner, "@") {
		return len(owner) > 1
	}
	// Email addresses
	if strings.Contains(owner, "@") {
		return true
	}
	return false
}

// FindCodeownersFile looks for a CODEOWNERS file in standard locations
func FindCodeownersFile(repoRoot string) string {
	// Standard locations for CODEOWNERS
	locations := []string{
		".github/CODEOWNERS",
		"CODEOWNERS",
		"docs/CODEOWNERS",
	}

	for _, loc := range locations {
		path := filepath.Join(repoRoot, loc)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// GetOwnersForPath returns the owners for a given file path
// Rules are evaluated in order, with later rules overriding earlier ones
func (c *CodeownersFile) GetOwnersForPath(filePath string) []string {
	// Normalize path to use forward slashes
	normalizedPath := filepath.ToSlash(filePath)

	var matchedOwners []string

	for _, rule := range c.Rules {
		if matchPattern(rule.Pattern, normalizedPath) {
			if rule.IsNegation {
				// Negation clears owners
				matchedOwners = nil
			} else {
				// Later rules override earlier ones
				matchedOwners = rule.Owners
			}
		}
	}

	return matchedOwners
}

// matchPattern checks if a file path matches a CODEOWNERS pattern
func matchPattern(pattern, filePath string) bool {
	// Normalize pattern
	pattern = filepath.ToSlash(pattern)

	// Handle different pattern types:

	// 1. Root-relative directory match (starts with / and ends with /)
	if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/") {
		dirPattern := strings.TrimPrefix(pattern, "/")
		dirPattern = strings.TrimSuffix(dirPattern, "/")
		// Must start with this directory
		return strings.HasPrefix(filePath, dirPattern+"/")
	}

	// 2. Root-relative file/pattern (starts with /)
	if strings.HasPrefix(pattern, "/") {
		pattern = strings.TrimPrefix(pattern, "/")
		// Directory pattern without trailing /
		if !strings.ContainsAny(pattern, "*?") && !strings.Contains(pattern, ".") {
			// Could be a directory - match if path starts with it
			if strings.HasPrefix(filePath, pattern+"/") {
				return true
			}
		}
		return matchGlob(pattern, filePath)
	}

	// 3. Non-root directory match (ends with /)
	if strings.HasSuffix(pattern, "/") {
		dirPattern := strings.TrimSuffix(pattern, "/")
		// Match at any level
		return strings.HasPrefix(filePath, dirPattern+"/") || strings.Contains(filePath, "/"+dirPattern+"/")
	}

	// 4. Extension patterns (like *.go)
	if strings.HasPrefix(pattern, "*") && !strings.HasPrefix(pattern, "**") {
		// Match at any level
		return matchGlob("**/"+pattern, filePath)
	}

	// 5. Double asterisk patterns
	if strings.Contains(pattern, "**") {
		return matchGlob(pattern, filePath)
	}

	// 6. Exact file match (no wildcards)
	if !strings.ContainsAny(pattern, "*?") {
		// Match anywhere in path
		if filePath == pattern {
			return true
		}
		if strings.HasSuffix(filePath, "/"+pattern) {
			return true
		}
		// Also match as a directory prefix
		if strings.HasPrefix(filePath, pattern+"/") {
			return true
		}
	}

	// 7. Pattern with wildcards - try matching at any level
	return matchGlob(pattern, filePath) || matchGlob("**/"+pattern, filePath)
}

// matchGlob performs glob-style matching with support for ** (any path)
func matchGlob(pattern, path string) bool {
	// Convert CODEOWNERS glob to regex
	regexPattern := globToRegex(pattern)

	re, err := regexp.Compile("^" + regexPattern + "$")
	if err != nil {
		return false
	}

	return re.MatchString(path)
}

// globToRegex converts a glob pattern to a regex pattern
func globToRegex(glob string) string {
	var result strings.Builder

	i := 0
	for i < len(glob) {
		c := glob[i]

		switch c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				// ** matches any path
				if i+2 < len(glob) && glob[i+2] == '/' {
					result.WriteString("(?:.*)?")
					i += 3
					continue
				}
				result.WriteString(".*")
				i += 2
				continue
			}
			// * matches anything except /
			result.WriteString("[^/]*")
		case '?':
			result.WriteString("[^/]")
		case '.', '+', '^', '$', '(', ')', '[', ']', '{', '}', '|', '\\':
			result.WriteByte('\\')
			result.WriteByte(c)
		default:
			result.WriteByte(c)
		}
		i++
	}

	return result.String()
}

// Owner represents an owner with metadata
type Owner struct {
	// ID is the owner identifier (@username, @org/team, or email)
	ID string `json:"id"`

	// Type is "user", "team", or "email"
	Type string `json:"type"`

	// Scope is the ownership scope: "maintainer", "reviewer", "contributor"
	Scope string `json:"scope"`

	// Source indicates where this ownership came from
	Source string `json:"source"` // "codeowners", "git-blame", "declared"

	// Confidence is how confident we are in this ownership (0-1)
	Confidence float64 `json:"confidence"`
}

// ParseOwnerID parses an owner ID and returns its type
func ParseOwnerID(ownerID string) (id string, ownerType string) {
	if strings.HasPrefix(ownerID, "@") {
		if strings.Contains(ownerID, "/") {
			return ownerID, "team"
		}
		return ownerID, "user"
	}
	if strings.Contains(ownerID, "@") {
		return ownerID, "email"
	}
	return ownerID, "unknown"
}

// CodeownersToOwners converts CODEOWNERS owners to Owner structs
func CodeownersToOwners(codeowners []string) []Owner {
	owners := make([]Owner, 0, len(codeowners))

	for _, ownerID := range codeowners {
		id, ownerType := ParseOwnerID(ownerID)
		owners = append(owners, Owner{
			ID:         id,
			Type:       ownerType,
			Scope:      "maintainer", // CODEOWNERS implies maintainer status
			Source:     "codeowners",
			Confidence: 1.0, // CODEOWNERS is authoritative
		})
	}

	return owners
}
