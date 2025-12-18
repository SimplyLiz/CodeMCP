package ownership

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCodeownersFile(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-codeowners-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a CODEOWNERS file
	codeownersContent := `# This is a comment
* @default-owner

# Documentation
/docs/ @docs-team
*.md @docs-team

# Go files
*.go @backend-team
/internal/api/ @api-team @backend-team

# Frontend
/src/ @frontend-team
/src/components/ @ui-team

# Specific file
/config/settings.json @ops-team

# Email owner
/scripts/ ops@example.com
`

	codeownersPath := filepath.Join(tempDir, "CODEOWNERS")
	if err := os.WriteFile(codeownersPath, []byte(codeownersContent), 0644); err != nil {
		t.Fatalf("Failed to write CODEOWNERS: %v", err)
	}

	// Parse the file
	cf, err := ParseCodeownersFile(codeownersPath)
	if err != nil {
		t.Fatalf("Failed to parse CODEOWNERS: %v", err)
	}

	// Verify rules count (excluding comments and empty lines)
	if len(cf.Rules) != 9 {
		t.Errorf("Expected 9 rules, got %d", len(cf.Rules))
		for i, rule := range cf.Rules {
			t.Logf("Rule %d: pattern=%s owners=%v", i, rule.Pattern, rule.Owners)
		}
	}

	// Verify first rule
	if cf.Rules[0].Pattern != "*" {
		t.Errorf("Expected pattern '*', got '%s'", cf.Rules[0].Pattern)
	}
	if len(cf.Rules[0].Owners) != 1 || cf.Rules[0].Owners[0] != "@default-owner" {
		t.Errorf("Expected owner '@default-owner', got %v", cf.Rules[0].Owners)
	}

	// Find the rule with multiple owners
	var apiRule *CodeownersRule
	for i := range cf.Rules {
		if cf.Rules[i].Pattern == "/internal/api/" {
			apiRule = &cf.Rules[i]
			break
		}
	}
	if apiRule == nil {
		t.Error("Expected to find /internal/api/ rule")
	} else if len(apiRule.Owners) != 2 {
		t.Errorf("Expected 2 owners for /internal/api/, got %d: %v", len(apiRule.Owners), apiRule.Owners)
	}

	// Find the email owner rule
	var scriptsRule *CodeownersRule
	for i := range cf.Rules {
		if cf.Rules[i].Pattern == "/scripts/" {
			scriptsRule = &cf.Rules[i]
			break
		}
	}
	if scriptsRule == nil {
		t.Error("Expected to find /scripts/ rule")
	} else if scriptsRule.Owners[0] != "ops@example.com" {
		t.Errorf("Expected email owner, got %s", scriptsRule.Owners[0])
	}
}

func TestGetOwnersForPath(t *testing.T) {
	// Create a CODEOWNERS structure
	cf := &CodeownersFile{
		Rules: []CodeownersRule{
			{Pattern: "*", Owners: []string{"@default"}, LineNumber: 1},
			{Pattern: "/docs/", Owners: []string{"@docs-team"}, LineNumber: 2},
			{Pattern: "*.go", Owners: []string{"@backend"}, LineNumber: 3},
			{Pattern: "/internal/api/", Owners: []string{"@api-team"}, LineNumber: 4},
			{Pattern: "/src/components/", Owners: []string{"@ui-team"}, LineNumber: 5},
		},
	}

	tests := []struct {
		path     string
		expected []string
	}{
		// Default owner
		{"random.txt", []string{"@default"}},

		// Docs directory
		{"docs/README.md", []string{"@docs-team"}},
		{"docs/guide/intro.md", []string{"@docs-team"}},

		// Go files
		{"main.go", []string{"@backend"}},
		{"internal/query/engine.go", []string{"@backend"}},

		// API directory (more specific than *.go)
		{"internal/api/handler.go", []string{"@api-team"}},

		// UI components
		{"src/components/Button.tsx", []string{"@ui-team"}},
	}

	for _, tt := range tests {
		owners := cf.GetOwnersForPath(tt.path)
		if len(owners) != len(tt.expected) {
			t.Errorf("GetOwnersForPath(%s): expected %v, got %v", tt.path, tt.expected, owners)
			continue
		}
		for i, owner := range owners {
			if owner != tt.expected[i] {
				t.Errorf("GetOwnersForPath(%s): expected %v, got %v", tt.path, tt.expected, owners)
				break
			}
		}
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		path     string
		expected bool
	}{
		// Wildcard all
		{"*", "anything.txt", true},
		{"*", "path/to/file.txt", true},

		// Directory patterns
		{"/docs/", "docs/README.md", true},
		{"/docs/", "docs/guide/intro.md", true},
		{"/docs/", "other/docs/file.md", false},

		// Extension patterns
		{"*.go", "main.go", true},
		{"*.go", "internal/query/engine.go", true},
		{"*.go", "main.txt", false},

		// Specific directory
		{"/internal/api/", "internal/api/handler.go", true},
		{"/internal/api/", "internal/api/middleware/auth.go", true},
		{"/internal/api/", "internal/query/engine.go", false},

		// Double asterisk
		{"**/test/**", "src/test/file.go", true},
		{"**/test/**", "deep/path/test/more/file.go", true},

		// Specific file
		{"/config.json", "config.json", true},
		{"/config.json", "dir/config.json", false},
	}

	for _, tt := range tests {
		result := matchPattern(tt.pattern, tt.path)
		if result != tt.expected {
			t.Errorf("matchPattern(%s, %s): expected %v, got %v", tt.pattern, tt.path, tt.expected, result)
		}
	}
}

func TestFindCodeownersFile(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-codeowners-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test: no CODEOWNERS file
	result := FindCodeownersFile(tempDir)
	if result != "" {
		t.Errorf("Expected empty result when no CODEOWNERS exists, got %s", result)
	}

	// Test: CODEOWNERS in .github/
	githubDir := filepath.Join(tempDir, ".github")
	if err := os.MkdirAll(githubDir, 0755); err != nil {
		t.Fatalf("Failed to create .github dir: %v", err)
	}
	codeownersPath := filepath.Join(githubDir, "CODEOWNERS")
	if err := os.WriteFile(codeownersPath, []byte("* @owner"), 0644); err != nil {
		t.Fatalf("Failed to write CODEOWNERS: %v", err)
	}

	result = FindCodeownersFile(tempDir)
	if result != codeownersPath {
		t.Errorf("Expected %s, got %s", codeownersPath, result)
	}

	// Test: CODEOWNERS in root (should still find .github one first)
	rootCodeowners := filepath.Join(tempDir, "CODEOWNERS")
	if err := os.WriteFile(rootCodeowners, []byte("* @root-owner"), 0644); err != nil {
		t.Fatalf("Failed to write root CODEOWNERS: %v", err)
	}

	result = FindCodeownersFile(tempDir)
	if result != codeownersPath { // .github/CODEOWNERS has priority
		t.Errorf("Expected .github/CODEOWNERS to have priority, got %s", result)
	}
}

func TestParseOwnerID(t *testing.T) {
	tests := []struct {
		input        string
		expectedID   string
		expectedType string
	}{
		{"@username", "@username", "user"},
		{"@org/team-name", "@org/team-name", "team"},
		{"user@example.com", "user@example.com", "email"},
		{"invalid", "invalid", "unknown"},
	}

	for _, tt := range tests {
		id, ownerType := ParseOwnerID(tt.input)
		if id != tt.expectedID {
			t.Errorf("ParseOwnerID(%s): expected id %s, got %s", tt.input, tt.expectedID, id)
		}
		if ownerType != tt.expectedType {
			t.Errorf("ParseOwnerID(%s): expected type %s, got %s", tt.input, tt.expectedType, ownerType)
		}
	}
}

func TestCodeownersToOwners(t *testing.T) {
	codeowners := []string{"@user1", "@org/team", "email@example.com"}

	owners := CodeownersToOwners(codeowners)

	if len(owners) != 3 {
		t.Errorf("Expected 3 owners, got %d", len(owners))
	}

	// Check first owner
	if owners[0].ID != "@user1" {
		t.Errorf("Expected ID '@user1', got '%s'", owners[0].ID)
	}
	if owners[0].Type != "user" {
		t.Errorf("Expected type 'user', got '%s'", owners[0].Type)
	}
	if owners[0].Source != "codeowners" {
		t.Errorf("Expected source 'codeowners', got '%s'", owners[0].Source)
	}
	if owners[0].Confidence != 1.0 {
		t.Errorf("Expected confidence 1.0, got %f", owners[0].Confidence)
	}

	// Check team owner
	if owners[1].Type != "team" {
		t.Errorf("Expected type 'team', got '%s'", owners[1].Type)
	}

	// Check email owner
	if owners[2].Type != "email" {
		t.Errorf("Expected type 'email', got '%s'", owners[2].Type)
	}
}

func TestNegationPattern(t *testing.T) {
	// Create a temp directory
	tempDir, err := os.MkdirTemp("", "ckb-codeowners-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create CODEOWNERS with negation
	codeownersContent := `* @default-owner
/docs/ @docs-team
!/docs/internal/
`

	codeownersPath := filepath.Join(tempDir, "CODEOWNERS")
	if err := os.WriteFile(codeownersPath, []byte(codeownersContent), 0644); err != nil {
		t.Fatalf("Failed to write CODEOWNERS: %v", err)
	}

	cf, err := ParseCodeownersFile(codeownersPath)
	if err != nil {
		t.Fatalf("Failed to parse CODEOWNERS: %v", err)
	}

	// Check negation rule was parsed
	if len(cf.Rules) != 3 {
		t.Errorf("Expected 3 rules, got %d", len(cf.Rules))
	}

	negationRule := cf.Rules[2]
	if !negationRule.IsNegation {
		t.Error("Expected negation rule to be marked as negation")
	}
	if negationRule.Pattern != "/docs/internal/" {
		t.Errorf("Expected pattern '/docs/internal/', got '%s'", negationRule.Pattern)
	}
}
