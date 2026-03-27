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

// --- missing-health-endpoint: Art. 10 DORA — Health check endpoints ---

type missingHealthEndpointCheck struct{}

func (c *missingHealthEndpointCheck) ID() string       { return "missing-health-endpoint" }
func (c *missingHealthEndpointCheck) Name() string     { return "Missing Health Check Endpoint" }
func (c *missingHealthEndpointCheck) Article() string  { return "Art. 10 DORA" }
func (c *missingHealthEndpointCheck) Severity() string { return "warning" }

var healthEndpointPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)["/]health\b`),
	regexp.MustCompile(`(?i)["/]healthz\b`),
	regexp.MustCompile(`(?i)["/]ready\b`),
	regexp.MustCompile(`(?i)["/]readiness\b`),
	regexp.MustCompile(`(?i)["/]liveness\b`),
	regexp.MustCompile(`(?i)["/]status\b`),
}

var webServicePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bListenAndServe\b`),
	regexp.MustCompile(`(?i)\bapp\.listen\b`),
	regexp.MustCompile(`(?i)\bcreateServer\b`),
	regexp.MustCompile(`(?i)\bgin\.Default\b`),
	regexp.MustCompile(`(?i)\bexpress\(\)`),
	regexp.MustCompile(`(?i)\bFastAPI\b`),
	regexp.MustCompile(`(?i)\bFlask\b`),
	regexp.MustCompile(`(?i)\bSpringBoot\b`),
	regexp.MustCompile(`(?i)\b@RestController\b`),
	regexp.MustCompile(`(?i)\bhttp\.NewServeMux\b`),
}

func (c *missingHealthEndpointCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	hasWebService := false
	hasHealthEndpoint := false
	var serviceFile string

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

				for _, p := range healthEndpointPatterns {
					if p.MatchString(line) {
						hasHealthEndpoint = true
					}
				}

				if !hasWebService {
					for _, p := range webServicePatterns {
						if p.MatchString(line) {
							hasWebService = true
							serviceFile = file
						}
					}
				}
			}
		}()
	}

	if hasWebService && !hasHealthEndpoint {
		return []compliance.Finding{
			{
				Severity:   "warning",
				Article:    "Art. 10 DORA",
				File:       serviceFile,
				Message:    "Web service detected without health check endpoint (/health, /healthz, /ready, /liveness)",
				Suggestion: "Add health check endpoints to enable monitoring and anomaly detection as required by DORA",
				Confidence: 0.70,
			},
		}, nil
	}

	return nil, nil
}

// --- missing-correlation-id: Art. 10 DORA — Distributed tracing ---

type missingCorrelationIDCheck struct{}

func (c *missingCorrelationIDCheck) ID() string       { return "missing-correlation-id" }
func (c *missingCorrelationIDCheck) Name() string     { return "Missing Correlation/Trace ID Propagation" }
func (c *missingCorrelationIDCheck) Article() string  { return "Art. 10 DORA" }
func (c *missingCorrelationIDCheck) Severity() string { return "info" }

var correlationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)correlation[_\-]?id`),
	regexp.MustCompile(`(?i)trace[_\-]?id`),
	regexp.MustCompile(`(?i)request[_\-]?id`),
	regexp.MustCompile(`(?i)x-request-id`),
	regexp.MustCompile(`(?i)x-correlation-id`),
	regexp.MustCompile(`(?i)x-trace-id`),
	regexp.MustCompile(`(?i)\bopentelemetry\b`),
	regexp.MustCompile(`(?i)\botel\b`),
	regexp.MustCompile(`(?i)\bjaeger\b`),
	regexp.MustCompile(`(?i)\bzipkin\b`),
}

func (c *missingCorrelationIDCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	hasDistributedService := false
	hasCorrelation := false
	var serviceFile string

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

				for _, p := range correlationPatterns {
					if p.MatchString(line) {
						hasCorrelation = true
					}
				}

				// Detect distributed service patterns (multiple service calls)
				if !hasDistributedService {
					for _, p := range httpClientPatterns {
						if p.MatchString(line) {
							hasDistributedService = true
							serviceFile = file
						}
					}
				}
			}
		}()
	}

	if hasDistributedService && !hasCorrelation {
		return []compliance.Finding{
			{
				Severity:   "info",
				Article:    "Art. 10 DORA",
				File:       serviceFile,
				Message:    "Distributed service calls detected without correlation/trace ID propagation",
				Suggestion: "Implement correlation ID propagation (e.g., X-Request-ID, OpenTelemetry) across service boundaries for incident detection",
				Confidence: 0.55,
			},
		}, nil
	}

	return nil, nil
}
