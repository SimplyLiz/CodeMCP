package dora

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- missing-circuit-breaker: Art. 9 DORA — Circuit breaker patterns ---

type missingCircuitBreakerCheck struct{}

func (c *missingCircuitBreakerCheck) ID() string       { return "missing-circuit-breaker" }
func (c *missingCircuitBreakerCheck) Name() string     { return "Missing Circuit Breaker Pattern" }
func (c *missingCircuitBreakerCheck) Article() string  { return "Art. 9 DORA" }
func (c *missingCircuitBreakerCheck) Severity() string { return "warning" }

var circuitBreakerPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)circuit[_\-]?breaker`),
	regexp.MustCompile(`(?i)\bhystrix\b`),
	regexp.MustCompile(`(?i)\bresilience4j\b`),
	regexp.MustCompile(`(?i)\bgobreaker\b`),
	regexp.MustCompile(`(?i)\bpolly\b`),
	regexp.MustCompile(`(?i)\bcircuitbreaker\b`),
}

var httpClientPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bhttp\.Client\b`),
	regexp.MustCompile(`\bhttp\.Get\b`),
	regexp.MustCompile(`\bhttp\.Post\b`),
	regexp.MustCompile(`(?i)\brequests\.(get|post|put|delete|patch)\b`),
	regexp.MustCompile(`(?i)\bfetch\(`),
	regexp.MustCompile(`(?i)\baxios\b`),
	regexp.MustCompile(`(?i)\bhttpClient\b`),
	regexp.MustCompile(`(?i)\bRestTemplate\b`),
	regexp.MustCompile(`(?i)\bWebClient\b`),
}

func (c *missingCircuitBreakerCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	hasExternalCalls := false
	hasCircuitBreaker := false
	var callFiles []string

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
			continue
		}

		func() {
			f, err := os.Open(filepath.Join(scope.RepoRoot, file))
			if err != nil {
				return
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()

				for _, p := range circuitBreakerPatterns {
					if p.MatchString(line) {
						hasCircuitBreaker = true
					}
				}

				for _, p := range httpClientPatterns {
					if p.MatchString(line) {
						hasExternalCalls = true
						callFiles = append(callFiles, file)
					}
				}
			}
		}()
	}

	if hasExternalCalls && !hasCircuitBreaker {
		// Deduplicate files, report on first occurrence
		seen := make(map[string]bool)
		var findings []compliance.Finding
		for _, file := range callFiles {
			if seen[file] {
				continue
			}
			seen[file] = true
			findings = append(findings, compliance.Finding{
				Severity:   "warning",
				Article:    "Art. 9 DORA",
				File:       file,
				Message:    "External HTTP client usage without circuit breaker pattern detected in codebase",
				Suggestion: "Implement circuit breaker patterns (e.g., gobreaker, hystrix, resilience4j, Polly) for external service calls",
				Confidence: 0.65,
			})
			if len(findings) >= 5 {
				break // Cap at 5 findings to avoid noise
			}
		}
		return findings, nil
	}

	return nil, nil
}

// --- missing-timeout: Art. 9 DORA — HTTP clients without timeout ---

type missingTimeoutCheck struct{}

func (c *missingTimeoutCheck) ID() string       { return "missing-timeout" }
func (c *missingTimeoutCheck) Name() string     { return "Missing Timeout on HTTP Client" }
func (c *missingTimeoutCheck) Article() string  { return "Art. 9 DORA" }
func (c *missingTimeoutCheck) Severity() string { return "warning" }

var noTimeoutPatterns = []struct {
	pattern *regexp.Regexp
	name    string
}{
	{regexp.MustCompile(`http\.Client\{\s*\}`), "http.Client{} without Timeout"},
	{regexp.MustCompile(`&http\.Client\{\s*\}`), "&http.Client{} without Timeout"},
	{regexp.MustCompile(`(?i)requests\.(get|post|put|delete|patch)\([^)]*\)\s*$`), "requests call without timeout parameter"},
	{regexp.MustCompile(`(?i)\bfetch\([^,)]+\)\s*$`), "fetch() without AbortController/signal"},
}

var timeoutExclusions = []*regexp.Regexp{
	regexp.MustCompile(`(?i)timeout`),
	regexp.MustCompile(`(?i)Timeout:`),
	regexp.MustCompile(`(?i)AbortController`),
	regexp.MustCompile(`(?i)signal:`),
	regexp.MustCompile(`(?i)timeout=`),
}

func (c *missingTimeoutCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
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

				for _, nt := range noTimeoutPatterns {
					if nt.pattern.MatchString(line) {
						// Check if timeout is configured nearby (same line)
						hasTimeout := false
						for _, excl := range timeoutExclusions {
							if excl.MatchString(line) {
								hasTimeout = true
								break
							}
						}
						if hasTimeout {
							continue
						}

						findings = append(findings, compliance.Finding{
							Severity:   "warning",
							Article:    "Art. 9 DORA",
							File:       file,
							StartLine:  lineNum,
							Message:    "HTTP client without timeout configuration: " + nt.name,
							Suggestion: "Configure explicit timeouts on all external HTTP calls to prevent cascading failures",
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

// --- missing-retry-logic: Art. 9 DORA — External calls without retry/backoff ---

type missingRetryLogicCheck struct{}

func (c *missingRetryLogicCheck) ID() string       { return "missing-retry-logic" }
func (c *missingRetryLogicCheck) Name() string     { return "Missing Retry/Backoff Logic" }
func (c *missingRetryLogicCheck) Article() string  { return "Art. 9 DORA" }
func (c *missingRetryLogicCheck) Severity() string { return "info" }

var retryPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bretry\b`),
	regexp.MustCompile(`(?i)\bbackoff\b`),
	regexp.MustCompile(`(?i)\bexponential\b`),
	regexp.MustCompile(`(?i)\bretrier\b`),
	regexp.MustCompile(`(?i)\bRetryPolicy\b`),
	regexp.MustCompile(`(?i)\bRetryTemplate\b`),
	regexp.MustCompile(`(?i)\bwith_retries\b`),
	regexp.MustCompile(`(?i)\bretry_count\b`),
	regexp.MustCompile(`(?i)\bmaxRetries\b`),
}

func (c *missingRetryLogicCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	hasExternalCalls := false
	hasRetryLogic := false
	var firstCallFile string
	var firstCallLine int

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
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

				for _, p := range retryPatterns {
					if p.MatchString(line) {
						hasRetryLogic = true
					}
				}

				if !hasExternalCalls {
					for _, p := range httpClientPatterns {
						if p.MatchString(line) {
							hasExternalCalls = true
							firstCallFile = file
							firstCallLine = lineNum
						}
					}
				}
			}
		}()
	}

	if hasExternalCalls && !hasRetryLogic {
		return []compliance.Finding{
			{
				Severity:   "info",
				Article:    "Art. 9 DORA",
				File:       firstCallFile,
				StartLine:  firstCallLine,
				Message:    "External service calls detected without retry/backoff logic in the codebase",
				Suggestion: "Implement retry with exponential backoff for external service calls to improve operational resilience",
				Confidence: 0.55,
			},
		}, nil
	}

	return nil, nil
}
