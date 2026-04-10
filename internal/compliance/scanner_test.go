package compliance

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeIdentifier(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"firstName", "first_name"},
		{"first_name", "first_name"},
		{"FirstName", "first_name"},
		{"FIRST_NAME", "first_name"},
		{"email", "email"},
		{"emailAddress", "email_address"},
		{"SSN", "ssn"},
		{"userSSN", "user_ssn"},
		{"HTMLParser", "html_parser"},
		{"ipAddress", "ip_address"},
		{"IPAddress", "ip_address"},
		{"dateOfBirth", "date_of_birth"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeIdentifier(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeIdentifier(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestMatchPII(t *testing.T) {
	scanner := NewPIIScanner(nil)

	tests := []struct {
		identifier  string
		shouldMatch bool
		piiType     string
	}{
		{"email", true, "contact"},
		{"email_address", true, "contact"},
		{"user_email", true, "contact"},
		{"phone", true, "contact"},
		{"ssn", true, "government-id"},
		{"date_of_birth", true, "dob"},
		{"iban", true, "financial"},
		{"credit_card", true, "financial"},
		// Non-PII that used to false positive
		{"file_name", false, ""},
		{"symbol_name", false, ""},
		{"hostname", false, ""},
		{"module_name", false, ""},
		{"function_name", false, ""},
		// Generic "name" should NOT match (too broad)
		{"name", false, ""},
		{"config", false, ""},
		{"count", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.identifier, func(t *testing.T) {
			normalized := normalizeIdentifier(tt.identifier)
			p, matched := scanner.matchPII(normalized)
			if matched != tt.shouldMatch {
				t.Errorf("matchPII(%q) matched=%v, want %v", tt.identifier, matched, tt.shouldMatch)
			}
			if matched && tt.piiType != "" && p.PIIType != tt.piiType {
				t.Errorf("matchPII(%q) piiType=%q, want %q", tt.identifier, p.PIIType, tt.piiType)
			}
		})
	}
}

func TestIsNonPIIIdentifier(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"file_name", true},
		{"hostname", true},
		{"symbol_name", true},
		{"class_name", true},
		{"module_name", true},
		{"first_name", false},
		{"email", false},
		{"phone", false},
		{"user_email", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isNonPIIIdentifier(tt.input)
			if got != tt.expected {
				t.Errorf("isNonPIIIdentifier(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

// TestScanFiles_BufferReusedAcrossFiles verifies that ScanFiles produces correct
// results when scanning multiple files in sequence. The shared seen/identBuf
// buffers must not bleed state from one file into the next.
func TestScanFiles_BufferReusedAcrossFiles(t *testing.T) {
	dir := t.TempDir()

	// file1: a Go struct with a PII field.
	if err := os.WriteFile(filepath.Join(dir, "user.go"),
		[]byte("type User struct { userEmail string }"), 0644); err != nil {
		t.Fatal(err)
	}
	// file2: no PII — just a numeric counter.
	if err := os.WriteFile(filepath.Join(dir, "config.go"),
		[]byte("var count = 0"), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewPIIScanner(nil)
	scope := &ScanScope{
		RepoRoot: dir,
		Files:    []string{"user.go", "config.go"},
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	fields, err := s.ScanFiles(context.Background(), scope)
	if err != nil {
		t.Fatalf("ScanFiles: %v", err)
	}

	var foundInUser bool
	for _, f := range fields {
		if f.File == "user.go" {
			foundInUser = true
		}
		// Buffer state from user.go must not bleed into config.go results.
		if f.File == "config.go" {
			t.Errorf("unexpected PII finding in config.go (buffer contamination?): %+v", f)
		}
	}
	if !foundInUser {
		t.Error("expected at least one PII finding in user.go, got none")
	}
}

func TestExtractContainer(t *testing.T) {
	tests := []struct {
		line     string
		expected string
	}{
		{"type UserProfile struct {", "UserProfile"},
		{"class UserService {", "UserService"},
		{"interface DataStore {", "DataStore"},
		{"func doSomething() {", ""},
		{"// just a comment", ""},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := extractContainer(tt.line)
			if got != tt.expected {
				t.Errorf("extractContainer(%q) = %q, want %q", tt.line, got, tt.expected)
			}
		})
	}
}
