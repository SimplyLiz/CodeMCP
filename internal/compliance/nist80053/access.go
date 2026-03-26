package nist80053

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- missing-access-enforcement: AC-3 — Data-modifying endpoints without auth ---

type missingAccessEnforcementCheck struct{}

func (c *missingAccessEnforcementCheck) ID() string       { return "missing-access-enforcement" }
func (c *missingAccessEnforcementCheck) Name() string     { return "Missing Access Enforcement" }
func (c *missingAccessEnforcementCheck) Article() string   { return "AC-3 NIST 800-53" }
func (c *missingAccessEnforcementCheck) Severity() string  { return "error" }

var modifyingHandlerPatterns = []*regexp.Regexp{
	// Go
	regexp.MustCompile(`(?i)router\.(POST|PUT|DELETE|PATCH)\(`),
	regexp.MustCompile(`(?i)\.Methods\(\s*["'](POST|PUT|DELETE|PATCH)["']\)`),
	// Node/Express
	regexp.MustCompile(`(?i)app\.(post|put|delete|patch)\(\s*["']`),
	regexp.MustCompile(`(?i)router\.(post|put|delete|patch)\(\s*["']`),
	// Python/Flask
	regexp.MustCompile(`(?i)methods\s*=\s*\[.*["'](POST|PUT|DELETE|PATCH)["']`),
	// Java/Spring
	regexp.MustCompile(`(?i)@(Post|Put|Delete|Patch)Mapping`),
}

var accessEnforcementIndicators = []string{
	"auth", "authorize", "permission", "rbac", "acl",
	"middleware", "guard", "interceptor", "policy",
	"login_required", "requires_auth", "authenticated",
	"@secured", "@preauthorize", "@rolesallowed",
}

func (c *missingAccessEnforcementCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		text := string(content)

		// Check if file has data-modifying handlers
		hasModifyingHandlers := false
		for _, pattern := range modifyingHandlerPatterns {
			if pattern.MatchString(text) {
				hasModifyingHandlers = true
				break
			}
		}

		if !hasModifyingHandlers {
			continue
		}

		// Check for authorization patterns
		textLower := strings.ToLower(text)
		hasAccessControl := false
		for _, indicator := range accessEnforcementIndicators {
			if strings.Contains(textLower, indicator) {
				hasAccessControl = true
				break
			}
		}

		if !hasAccessControl {
			// Report on the first modifying handler line
			lines := strings.Split(text, "\n")
			for i, line := range lines {
				for _, pattern := range modifyingHandlerPatterns {
					if pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "AC-3 NIST 800-53",
							File:       file,
							StartLine:  i + 1,
							Message:    "Data-modifying HTTP endpoint without visible access control enforcement",
							Suggestion: "Implement authorization checks for all POST/PUT/DELETE/PATCH endpoints; apply role-based access control",
							Confidence: 0.60,
						})
						goto nextFile
					}
				}
			}
		}
	nextFile:
	}

	return findings, nil
}

// isStructFieldOrPathAssignment returns true for lines that are Go struct field
// assignments, file path values, or variable-only assignments without credential-like values.
func isStructFieldOrPathAssignment(trimmed string) bool {
	// File path extensions — not credentials
	for _, ext := range []string{".db", ".json", ".yaml", ".yml", ".toml", ".conf", ".cfg", ".sqlite"} {
		if strings.Contains(trimmed, ext) {
			return true
		}
	}

	// Go struct field assignment: `FieldName: variableOrExpr,`
	// Match `word:` followed by a non-quoted value (variable reference, not a hardcoded credential)
	if strings.Contains(trimmed, ":") && !strings.Contains(trimmed, "://") {
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) == 2 {
			val := strings.TrimSpace(parts[1])
			val = strings.TrimSuffix(val, ",")
			val = strings.TrimSpace(val)
			// Value is a bare identifier (variable), not a quoted string — not a hardcoded cred
			if len(val) > 0 && !strings.HasPrefix(val, `"`) && !strings.HasPrefix(val, `'`) {
				return true
			}
		}
	}

	return false
}

// --- default-credentials: IA-5(1) — Default/hardcoded passwords ---

type defaultCredentialsCheck struct{}

func (c *defaultCredentialsCheck) ID() string       { return "default-credentials" }
func (c *defaultCredentialsCheck) Name() string     { return "Default Credentials" }
func (c *defaultCredentialsCheck) Article() string   { return "IA-5(1) NIST 800-53" }
func (c *defaultCredentialsCheck) Severity() string  { return "error" }

var defaultCredentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*["'](admin|password|root|default|123456|changeme|letmein|welcome|qwerty)["']`),
	regexp.MustCompile(`(?i)(username|user)\s*[:=]\s*["'](admin|root|administrator|sa|test)["'].*\n?.*(password|passwd|pwd)\s*[:=]\s*["']`),
	regexp.MustCompile(`(?i)default.*(password|credential|secret)\s*[:=]\s*["'][^"']+["']`),
	regexp.MustCompile(`(?i)(admin|root)\s*[:/@]\s*(admin|root|password|passwd)`),
}

func (c *defaultCredentialsCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		lower := strings.ToLower(file)
		if strings.Contains(lower, "_test.") || strings.Contains(lower, ".test.") ||
			strings.Contains(lower, "example") || strings.Contains(lower, "sample") ||
			strings.Contains(lower, "fixture") || strings.Contains(lower, "mock") {
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
				trimmed := strings.TrimSpace(line)

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
					continue
				}

				// Skip struct field assignments (e.g., `Root: rootId`) and file path values
				if isStructFieldOrPathAssignment(trimmed) {
					continue
				}

				for _, pattern := range defaultCredentialPatterns {
					if pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "error",
							Article:    "IA-5(1) NIST 800-53",
							File:       file,
							StartLine:  lineNum,
							Message:    "Default or well-known credential detected in source code",
							Suggestion: "Remove default credentials; require strong, unique credentials configured via environment variables or secret management",
							Confidence: 0.85,
							CWE:        "CWE-798",
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}
