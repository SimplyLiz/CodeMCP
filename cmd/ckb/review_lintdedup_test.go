package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SimplyLiz/CodeMCP/internal/query"
)

func TestDeduplicateLintFindings(t *testing.T) {
	t.Parallel()

	sarifReport := `{
  "version": "2.1.0",
  "runs": [{
    "tool": {"driver": {"name": "golangci-lint"}},
    "results": [
      {
        "ruleId": "errcheck",
        "level": "warning",
        "message": {"text": "error return value not checked"},
        "locations": [{
          "physicalLocation": {
            "artifactLocation": {"uri": "internal/query/engine.go"},
            "region": {"startLine": 42}
          }
        }]
      },
      {
        "ruleId": "unused",
        "level": "warning",
        "message": {"text": "unused variable"},
        "locations": [{
          "physicalLocation": {
            "artifactLocation": {"uri": "pkg/config.go"},
            "region": {"startLine": 10}
          }
        }]
      }
    ]
  }]
}`

	dir := t.TempDir()
	sarifPath := filepath.Join(dir, "lint.sarif")
	if err := os.WriteFile(sarifPath, []byte(sarifReport), 0644); err != nil {
		t.Fatal(err)
	}

	resp := &query.ReviewPRResponse{
		Findings: []query.ReviewFinding{
			{Check: "complexity", Severity: "warning", File: "internal/query/engine.go", StartLine: 42, Message: "Complexity increase"},
			{Check: "breaking", Severity: "error", File: "internal/query/engine.go", StartLine: 100, Message: "Breaking change"},
			{Check: "coupling", Severity: "warning", File: "pkg/config.go", StartLine: 10, Message: "Missing co-change"},
			{Check: "secrets", Severity: "error", File: "cmd/main.go", StartLine: 5, Message: "Potential secret"},
		},
	}

	suppressed, err := deduplicateLintFindings(resp, sarifPath)
	if err != nil {
		t.Fatalf("deduplicateLintFindings: %v", err)
	}

	if suppressed != 2 {
		t.Errorf("expected 2 suppressed, got %d", suppressed)
	}
	if len(resp.Findings) != 2 {
		t.Errorf("expected 2 remaining findings, got %d", len(resp.Findings))
	}

	// Verify the right findings survived
	for _, f := range resp.Findings {
		if f.File == "internal/query/engine.go" && f.StartLine == 42 {
			t.Error("finding at engine.go:42 should have been suppressed")
		}
		if f.File == "pkg/config.go" && f.StartLine == 10 {
			t.Error("finding at config.go:10 should have been suppressed")
		}
	}
}

func TestDeduplicateLintFindings_EmptyReport(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sarifPath := filepath.Join(dir, "empty.sarif")
	if err := os.WriteFile(sarifPath, []byte(`{"version":"2.1.0","runs":[{"results":[]}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	resp := &query.ReviewPRResponse{
		Findings: []query.ReviewFinding{
			{Check: "breaking", Severity: "error", File: "a.go", StartLine: 1, Message: "test"},
		},
	}

	suppressed, err := deduplicateLintFindings(resp, sarifPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if suppressed != 0 {
		t.Errorf("expected 0 suppressed, got %d", suppressed)
	}
	if len(resp.Findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(resp.Findings))
	}
}

func TestDeduplicateLintFindings_MissingFile(t *testing.T) {
	t.Parallel()

	resp := &query.ReviewPRResponse{}
	_, err := deduplicateLintFindings(resp, "/nonexistent/path.sarif")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestDeduplicateLintFindings_InvalidJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	sarifPath := filepath.Join(dir, "bad.sarif")
	if err := os.WriteFile(sarifPath, []byte(`not json`), 0644); err != nil {
		t.Fatal(err)
	}

	resp := &query.ReviewPRResponse{}
	_, err := deduplicateLintFindings(resp, sarifPath)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLintKey_NormalizesPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		file string
		line int
		want string
	}{
		{"internal/query/engine.go", 42, "internal/query/engine.go:42"},
		{"./internal/query/engine.go", 42, "internal/query/engine.go:42"},
		{"/internal/query/engine.go", 42, "internal/query/engine.go:42"},
	}

	for _, tt := range tests {
		got := lintKey(tt.file, tt.line)
		if got != tt.want {
			t.Errorf("lintKey(%q, %d) = %q, want %q", tt.file, tt.line, got, tt.want)
		}
	}
}
