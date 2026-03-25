package misra

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- implicit-conversion: Rule 10.1 — no implicit type conversions ---

type implicitConversionCheck struct{}

func (c *implicitConversionCheck) ID() string       { return "implicit-conversion" }
func (c *implicitConversionCheck) Name() string     { return "Implicit Type Conversion" }
func (c *implicitConversionCheck) Article() string   { return "Rule 10.1 MISRA C" }
func (c *implicitConversionCheck) Severity() string  { return "warning" }

// Patterns detecting signed/unsigned mixing and narrowing conversions
var implicitConversionPatterns = []*regexp.Regexp{
	// Signed to unsigned assignment: unsigned x = signed_var
	regexp.MustCompile(`\bunsigned\s+\w+\s*=\s*[^;]*\b(int|short|long|char)\b`),
	// Unsigned to signed assignment: int x = unsigned_var
	regexp.MustCompile(`\b(int|short|long|char)\s+\w+\s*=\s*[^;]*\bunsigned\b`),
	// Narrowing: int = long, short = int, char = int
	regexp.MustCompile(`\b(char|short)\s+\w+\s*=\s*[^;]*\b(int|long|size_t)\b`),
	regexp.MustCompile(`\bint\s+\w+\s*=\s*[^;]*\b(long|long\s+long|size_t)\b`),
}

func (c *implicitConversionCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}
		if !isMISRAFile(file) {
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

				if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
					continue
				}

				for _, pattern := range implicitConversionPatterns {
					if pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							CheckID:    "implicit-conversion",
							Framework:  compliance.FrameworkMISRA,
							Severity:   "warning",
							Article:    "Rule 10.1 MISRA C",
							File:       file,
							StartLine:  lineNum,
							Message:    "Potential implicit type conversion between signed/unsigned or narrowing types",
							Suggestion: "Use explicit casts to make type conversions visible and intentional",
							Confidence: 0.65,
						})
						break // One finding per line
					}
				}
			}
		}()
	}

	return findings, nil
}
