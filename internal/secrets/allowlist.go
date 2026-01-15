package secrets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
)

// AllowlistEntry defines a suppression rule.
type AllowlistEntry struct {
	ID          string       `json:"id"`
	Type        string       `json:"type"`                  // "path", "pattern", "hash", "rule"
	Value       string       `json:"value"`                 // Path glob, regex, finding hash, or rule name
	Reason      string       `json:"reason"`                // Why it's suppressed
	SecretTypes []SecretType `json:"secretTypes,omitempty"` // Limit to specific types
	CreatedAt   string       `json:"createdAt,omitempty"`
	CreatedBy   string       `json:"createdBy,omitempty"`
}

// AllowlistFile is the structure of the allowlist JSON file.
type AllowlistFile struct {
	Version string           `json:"version"`
	Entries []AllowlistEntry `json:"entries"`
}

// Allowlist manages secret suppression rules.
type Allowlist struct {
	entries []AllowlistEntry

	// Compiled patterns for efficient matching
	pathPatterns  []*pathMatcher
	valuePatterns []*regexp.Regexp
	hashes        map[string]string // hash -> entry ID
	rules         map[string]string // rule name -> entry ID
}

type pathMatcher struct {
	pattern string
	entryID string
}

// LoadAllowlist loads the allowlist from .ckb/secrets-allowlist.json
func LoadAllowlist(repoRoot string) (*Allowlist, error) {
	path := filepath.Join(repoRoot, ".ckb", "secrets-allowlist.json")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Return empty allowlist if file doesn't exist
		return &Allowlist{
			hashes: make(map[string]string),
			rules:  make(map[string]string),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	var file AllowlistFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}

	al := &Allowlist{
		entries: file.Entries,
		hashes:  make(map[string]string),
		rules:   make(map[string]string),
	}

	al.compile()
	return al, nil
}

// compile pre-compiles patterns for efficient matching.
func (al *Allowlist) compile() {
	for _, e := range al.entries {
		switch e.Type {
		case "path":
			al.pathPatterns = append(al.pathPatterns, &pathMatcher{
				pattern: e.Value,
				entryID: e.ID,
			})
		case "pattern":
			if re, err := regexp.Compile(e.Value); err == nil {
				al.valuePatterns = append(al.valuePatterns, re)
			}
		case "hash":
			al.hashes[e.Value] = e.ID
		case "rule":
			al.rules[e.Value] = e.ID
		}
	}
}

// IsSuppressed checks if a finding should be suppressed.
func (al *Allowlist) IsSuppressed(f *SecretFinding) (bool, string) {
	if al == nil {
		return false, ""
	}

	// Check path patterns
	for _, pm := range al.pathPatterns {
		if matched, _ := filepath.Match(pm.pattern, f.File); matched {
			return true, pm.entryID
		}
		// Also try with ** for recursive matching
		if matchGlob(pm.pattern, f.File) {
			return true, pm.entryID
		}
	}

	// Check rule suppression
	if id, ok := al.rules[f.Rule]; ok {
		return true, id
	}

	// Check value patterns
	for i, re := range al.valuePatterns {
		if re.MatchString(f.RawMatch) {
			// Find the corresponding entry ID
			idx := 0
			for _, e := range al.entries {
				if e.Type == "pattern" {
					if idx == i {
						return true, e.ID
					}
					idx++
				}
			}
			return true, ""
		}
	}

	// Check hash
	hash := hashFinding(f)
	if id, ok := al.hashes[hash]; ok {
		return true, id
	}

	return false, ""
}

// hashFinding creates a stable hash for a finding.
func hashFinding(f *SecretFinding) string {
	// Hash based on file, line, and raw match
	data := f.File + ":" + f.RawMatch
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8]) // Use first 8 bytes for brevity
}

// matchGlob matches a pattern with ** support.
func matchGlob(pattern, path string) bool {
	// Simple ** handling
	if pattern == "**" {
		return true
	}

	// Convert ** to regex-like matching
	parts := splitPattern(pattern)
	pathParts := filepath.SplitList(path)
	if len(pathParts) == 0 {
		pathParts = []string{path}
	}

	return matchParts(parts, pathParts)
}

func splitPattern(pattern string) []string {
	// Split by path separator but keep **
	var parts []string
	current := ""

	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '/' || pattern[i] == filepath.Separator {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(pattern[i])
		}
	}
	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

func matchParts(pattern, path []string) bool {
	pi, pathi := 0, 0

	for pi < len(pattern) && pathi < len(path) {
		if pattern[pi] == "**" {
			// ** matches zero or more path segments
			if pi == len(pattern)-1 {
				return true
			}
			// Try matching rest of pattern against remaining path
			for i := pathi; i <= len(path); i++ {
				if matchParts(pattern[pi+1:], path[i:]) {
					return true
				}
			}
			return false
		}

		// Regular glob match
		matched, _ := filepath.Match(pattern[pi], path[pathi])
		if !matched {
			return false
		}

		pi++
		pathi++
	}

	// Check if we consumed both
	return pi == len(pattern) && pathi == len(path)
}

// GenerateHash generates a suppression hash for a finding.
// This can be used to create allowlist entries.
func GenerateHash(f *SecretFinding) string {
	return hashFinding(f)
}
