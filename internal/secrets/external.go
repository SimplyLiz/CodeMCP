package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ExternalToolConfig defines configuration for external secret scanning tools.
type ExternalToolConfig struct {
	Name        string
	Binary      string
	VersionArgs []string
	MinVersion  string
	InstallCmd  map[string]string // per-OS install commands
}

// GitleaksConfig is the configuration for gitleaks.
var GitleaksConfig = ExternalToolConfig{
	Name:        "gitleaks",
	Binary:      "gitleaks",
	VersionArgs: []string{"version"},
	MinVersion:  "8.0.0",
	InstallCmd: map[string]string{
		"darwin":  "brew install gitleaks",
		"linux":   "apt install gitleaks || snap install gitleaks",
		"default": "go install github.com/gitleaks/gitleaks/v8@latest",
	},
}

// TrufflehogConfig is the configuration for trufflehog.
var TrufflehogConfig = ExternalToolConfig{
	Name:        "trufflehog",
	Binary:      "trufflehog",
	VersionArgs: []string{"--version"},
	MinVersion:  "3.0.0",
	InstallCmd: map[string]string{
		"darwin":  "brew install trufflehog",
		"default": "pip install trufflehog",
	},
}

// ExternalScanner wraps external secret detection tools.
type ExternalScanner struct {
	repoRoot string
	timeout  time.Duration
}

// NewExternalScanner creates a new external scanner.
func NewExternalScanner(repoRoot string) *ExternalScanner {
	return &ExternalScanner{
		repoRoot: repoRoot,
		timeout:  5 * time.Minute,
	}
}

// IsAvailable checks if a tool is installed and meets version requirements.
func (e *ExternalScanner) IsAvailable(ctx context.Context, config ExternalToolConfig) (bool, string) {
	_, err := exec.LookPath(config.Binary)
	if err != nil {
		return false, ""
	}

	// Get version
	if len(config.VersionArgs) > 0 {
		versionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		cmd := exec.CommandContext(versionCtx, config.Binary, config.VersionArgs...)
		output, err := cmd.Output()
		if err != nil {
			return true, "" // Found but version unknown
		}

		version := parseVersion(string(output))
		if config.MinVersion != "" && !versionAtLeast(version, config.MinVersion) {
			return false, version
		}
		return true, version
	}

	return true, ""
}

// RunGitleaks runs gitleaks and returns findings.
func (e *ExternalScanner) RunGitleaks(ctx context.Context, opts ScanOptions) ([]SecretFinding, error) {
	args := []string{
		"detect",
		"--source", e.repoRoot,
		"--report-format", "json",
		"--exit-code", "0", // Don't fail on findings
		"--no-banner",
	}

	// Add scope-specific flags
	switch opts.Scope {
	case ScopeStaged:
		args = append(args, "--staged")
	case ScopeHistory:
		if opts.SinceCommit != "" {
			args = append(args, "--log-opts", fmt.Sprintf("--since='%s'", opts.SinceCommit))
		}
	}

	// Run with timeout
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gitleaks", args...)
	cmd.Dir = e.repoRoot

	// Use CombinedOutput to capture both stdout and stderr in a single call.
	// This avoids the bug where cmd.Output() can only be called once.
	output, err := cmd.CombinedOutput()
	if err != nil {
		// gitleaks exits with 1 when findings are found, which is not an error for us
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Exit code 1 = leaks found (not an error, output contains JSON)
			if exitErr.ExitCode() != 1 {
				return nil, fmt.Errorf("gitleaks failed with exit code %d: %s", exitErr.ExitCode(), string(output))
			}
			// Exit code 1 means findings exist - output already captured above
		} else {
			return nil, fmt.Errorf("gitleaks failed: %w", err)
		}
	}

	return e.parseGitleaksOutput(string(output))
}

// parseGitleaksOutput converts gitleaks JSON output to SecretFindings.
func (e *ExternalScanner) parseGitleaksOutput(output string) ([]SecretFinding, error) {
	output = strings.TrimSpace(output)
	if output == "" || output == "null" || output == "[]" {
		return nil, nil
	}

	var findings []gitleaksFinding
	if err := json.Unmarshal([]byte(output), &findings); err != nil {
		// Try parsing as single object
		var single gitleaksFinding
		if err2 := json.Unmarshal([]byte(output), &single); err2 != nil {
			return nil, fmt.Errorf("failed to parse gitleaks output: %w", err)
		}
		findings = []gitleaksFinding{single}
	}

	var result []SecretFinding
	for _, f := range findings {
		result = append(result, SecretFinding{
			File:       f.File,
			Line:       f.StartLine,
			Type:       SecretTypeExternal,
			Severity:   SeverityHigh, // gitleaks doesn't provide severity
			Match:      redactSecret(f.Secret, 4),
			RawMatch:   f.Secret,
			Context:    f.Match,
			Rule:       f.RuleID,
			Confidence: 0.9,
			Source:     "gitleaks",
			Commit:     f.Commit,
			Author:     f.Author,
			CommitDate: f.Date,
		})
	}

	return result, nil
}

type gitleaksFinding struct {
	Description string `json:"Description"`
	File        string `json:"File"`
	StartLine   int    `json:"StartLine"`
	EndLine     int    `json:"EndLine"`
	Secret      string `json:"Secret"`
	Match       string `json:"Match"`
	Commit      string `json:"Commit"`
	Author      string `json:"Author"`
	Date        string `json:"Date"`
	RuleID      string `json:"RuleID"`
}

// RunTrufflehog runs trufflehog and returns findings.
func (e *ExternalScanner) RunTrufflehog(ctx context.Context, opts ScanOptions) ([]SecretFinding, error) {
	var args []string

	if opts.Scope == ScopeHistory {
		args = []string{"git", e.repoRoot, "--json", "--no-update"}
		if opts.SinceCommit != "" {
			args = append(args, "--since-commit", opts.SinceCommit)
		}
	} else {
		args = []string{"filesystem", e.repoRoot, "--json", "--no-update"}
	}

	// Run with timeout
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "trufflehog", args...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("trufflehog failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("trufflehog failed: %w", err)
	}

	return e.parseTrufflehogOutput(string(output))
}

// parseTrufflehogOutput converts trufflehog JSON output to SecretFindings.
func (e *ExternalScanner) parseTrufflehogOutput(output string) ([]SecretFinding, error) {
	// Trufflehog outputs NDJSON (newline-delimited JSON)
	var result []SecretFinding

	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var f trufflehogFinding
		if err := json.Unmarshal([]byte(line), &f); err != nil {
			continue // Skip malformed lines
		}

		// Map trufflehog severity
		severity := SeverityMedium
		if f.Verified {
			severity = SeverityCritical
		}

		result = append(result, SecretFinding{
			File:       f.SourceMetadata.Data.Filesystem.File,
			Line:       f.SourceMetadata.Data.Filesystem.Line,
			Type:       SecretTypeExternal,
			Severity:   severity,
			Match:      redactSecret(f.Raw, 4),
			RawMatch:   f.Raw,
			Rule:       f.DetectorName,
			Confidence: 0.85,
			Source:     "trufflehog",
		})
	}

	return result, nil
}

type trufflehogFinding struct {
	DetectorName   string `json:"DetectorName"`
	Verified       bool   `json:"Verified"`
	Raw            string `json:"Raw"`
	SourceMetadata struct {
		Data struct {
			Filesystem struct {
				File string `json:"file"`
				Line int    `json:"line"`
			} `json:"Filesystem"`
			Git struct {
				Commit string `json:"commit"`
				File   string `json:"file"`
				Line   int    `json:"line"`
			} `json:"Git"`
		} `json:"Data"`
	} `json:"SourceMetadata"`
}

// parseVersion extracts version number from output.
func parseVersion(output string) string {
	// Common patterns: "v1.2.3", "1.2.3", "version 1.2.3"
	re := regexp.MustCompile(`v?(\d+\.\d+\.\d+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) >= 2 {
		return matches[1]
	}
	return strings.TrimSpace(output)
}

// versionAtLeast checks if version >= minVersion.
func versionAtLeast(version, minVersion string) bool {
	v := parseVersionParts(version)
	m := parseVersionParts(minVersion)

	for i := 0; i < 3; i++ {
		if v[i] > m[i] {
			return true
		}
		if v[i] < m[i] {
			return false
		}
	}
	return true
}

func parseVersionParts(v string) [3]int {
	var parts [3]int
	split := strings.Split(strings.TrimPrefix(v, "v"), ".")
	for i := 0; i < 3 && i < len(split); i++ {
		parts[i], _ = strconv.Atoi(split[i])
	}
	return parts
}
