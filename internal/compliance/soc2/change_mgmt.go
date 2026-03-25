package soc2

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- todo-in-production: CC8.1 — TODO/FIXME/HACK in non-test source ---

type todoInProductionCheck struct{}

func (c *todoInProductionCheck) ID() string       { return "todo-in-production" }
func (c *todoInProductionCheck) Name() string     { return "TODO/FIXME in Production Code" }
func (c *todoInProductionCheck) Article() string   { return "CC8.1 SOC 2" }
func (c *todoInProductionCheck) Severity() string  { return "info" }

var todoPattern = regexp.MustCompile(`(?i)\b(TODO|FIXME|HACK|XXX|TEMP)\b`)

func (c *todoInProductionCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Skip test files
		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") ||
			strings.Contains(file, ".spec.") || strings.Contains(file, "testdata") {
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

				if todoPattern.MatchString(line) {
					match := todoPattern.FindString(line)
					findings = append(findings, compliance.Finding{
						Severity:   "info",
						Article:    "CC8.1 SOC 2",
						File:       file,
						StartLine:  lineNum,
						Message:    strings.ToUpper(match) + " comment in production code — indicates incomplete or temporary implementation",
						Suggestion: "Resolve TODO/FIXME items before release; track them in issue tracker for change management",
						Confidence: 0.95,
					})
				}
			}
		}()
	}

	return findings, nil
}

// --- debug-mode-enabled: CC8.1 — Debug flags left enabled ---

type debugModeEnabledCheck struct{}

func (c *debugModeEnabledCheck) ID() string       { return "debug-mode-enabled" }
func (c *debugModeEnabledCheck) Name() string     { return "Debug Mode Enabled" }
func (c *debugModeEnabledCheck) Article() string   { return "CC8.1 SOC 2" }
func (c *debugModeEnabledCheck) Severity() string  { return "warning" }

var debugPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)DEBUG\s*[:=]\s*(true|1|"true"|'true')`),
	regexp.MustCompile(`(?i)app\.debug\s*=\s*True`),
	regexp.MustCompile(`(?i)setDebug\(\s*true\s*\)`),
	regexp.MustCompile(`(?i)FLASK_DEBUG\s*[:=]\s*(1|true|"true"|'true')`),
	regexp.MustCompile(`(?i)DJANGO_DEBUG\s*[:=]\s*(True|true|1)`),
	regexp.MustCompile(`(?i)debug\s*:\s*true`),
	regexp.MustCompile(`(?i)log_?level\s*[:=]\s*["']?debug["']?`),
	regexp.MustCompile(`(?i)enable_?debug\s*[:=]\s*(true|1)`),
}

func (c *debugModeEnabledCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		// Skip test files, example files, and development configs
		lower := strings.ToLower(file)
		if strings.Contains(lower, "_test.") || strings.Contains(lower, ".test.") ||
			strings.Contains(lower, "example") || strings.Contains(lower, "sample") ||
			strings.Contains(lower, "dev.") || strings.Contains(lower, "development") {
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

				for _, pattern := range debugPatterns {
					if pattern.MatchString(line) {
						findings = append(findings, compliance.Finding{
							Severity:   "warning",
							Article:    "CC8.1 SOC 2",
							File:       file,
							StartLine:  lineNum,
							Message:    "Debug mode or verbose logging flag enabled in non-development code",
							Suggestion: "Ensure debug mode is disabled in production; use environment-based configuration for debug settings",
							Confidence: 0.75,
						})
						break
					}
				}
			}
		}()
	}

	return findings, nil
}
