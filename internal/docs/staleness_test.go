package docs

import (
	"testing"
	"time"

	"ckb/internal/identity"
)

func TestSplitNormalized(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty",
			input:    "",
			expected: nil,
		},
		{
			name:     "single segment",
			input:    "Symbol",
			expected: []string{"Symbol"},
		},
		{
			name:     "two segments",
			input:    "Foo.Bar",
			expected: []string{"Foo", "Bar"},
		},
		{
			name:     "three segments",
			input:    "a.b.c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "many segments",
			input:    "internal.auth.UserService.Authenticate",
			expected: []string{"internal", "auth", "UserService", "Authenticate"},
		},
		{
			name:     "consecutive dots",
			input:    "a..b",
			expected: []string{"a", "b"},
		},
		{
			name:     "leading dot",
			input:    ".a.b",
			expected: []string{"a", "b"},
		},
		{
			name:     "trailing dot",
			input:    "a.b.",
			expected: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitNormalized(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("splitNormalized(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i, s := range result {
				if s != tt.expected[i] {
					t.Errorf("splitNormalized(%q)[%d] = %q, want %q", tt.input, i, s, tt.expected[i])
				}
			}
		})
	}
}

func TestGetSymbolDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		sym      *identity.SymbolMapping
		expected string
	}{
		{
			name: "with container and name",
			sym: &identity.SymbolMapping{
				StableId: "ckb:repo:sym:abc123",
				Fingerprint: &identity.SymbolFingerprint{
					QualifiedContainer: "pkg.UserService",
					Name:               "Authenticate",
				},
			},
			expected: "pkg.UserService.Authenticate",
		},
		{
			name: "name only",
			sym: &identity.SymbolMapping{
				StableId: "ckb:repo:sym:abc123",
				Fingerprint: &identity.SymbolFingerprint{
					Name: "Authenticate",
				},
			},
			expected: "Authenticate",
		},
		{
			name: "fallback to StableId",
			sym: &identity.SymbolMapping{
				StableId:    "ckb:repo:sym:abc123",
				Fingerprint: nil,
			},
			expected: "ckb:repo:sym:abc123",
		},
		{
			name: "empty fingerprint fields",
			sym: &identity.SymbolMapping{
				StableId: "ckb:repo:sym:abc123",
				Fingerprint: &identity.SymbolFingerprint{
					QualifiedContainer: "",
					Name:               "",
				},
			},
			expected: "ckb:repo:sym:abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getSymbolDisplayName(tt.sym)
			if result != tt.expected {
				t.Errorf("getSymbolDisplayName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestRenameInfo(t *testing.T) {
	ri := renameInfo{
		NewSymbolID: "ckb:repo:sym:new123",
		NewName:     "NewUserService.Authenticate",
		Reason:      "renamed",
		Confidence:  0.95,
	}

	if ri.NewSymbolID != "ckb:repo:sym:new123" {
		t.Errorf("NewSymbolID = %q, want %q", ri.NewSymbolID, "ckb:repo:sym:new123")
	}
	if ri.NewName != "NewUserService.Authenticate" {
		t.Errorf("NewName = %q, want %q", ri.NewName, "NewUserService.Authenticate")
	}
	if ri.Reason != "renamed" {
		t.Errorf("Reason = %q, want %q", ri.Reason, "renamed")
	}
	if ri.Confidence != 0.95 {
		t.Errorf("Confidence = %f, want %f", ri.Confidence, 0.95)
	}
}

func TestStalenessReport(t *testing.T) {
	report := StalenessReport{
		DocPath:            "docs/auth.md",
		TotalReferences:    10,
		Valid:              7,
		CheckedAt:          time.Now(),
		SymbolIndexVersion: "v1.2.3",
		Stale: []StaleReference{
			{
				RawText:     "OldSymbol.Name",
				Line:        42,
				Reason:      StalenessMissing,
				Message:     "Symbol not found in index",
				Suggestions: []string{"NewSymbol.Name"},
			},
		},
	}

	if report.DocPath != "docs/auth.md" {
		t.Errorf("DocPath = %q, want %q", report.DocPath, "docs/auth.md")
	}
	if report.TotalReferences != 10 {
		t.Errorf("TotalReferences = %d, want %d", report.TotalReferences, 10)
	}
	if report.Valid != 7 {
		t.Errorf("Valid = %d, want %d", report.Valid, 7)
	}
	if len(report.Stale) != 1 {
		t.Errorf("len(Stale) = %d, want %d", len(report.Stale), 1)
	}
	if report.Stale[0].Reason != StalenessMissing {
		t.Errorf("Stale[0].Reason = %v, want %v", report.Stale[0].Reason, StalenessMissing)
	}
}

func TestStaleReference(t *testing.T) {
	newSymbolID := "ckb:repo:sym:new123"
	ref := StaleReference{
		RawText:     "`UserService.Auth`",
		Line:        15,
		Reason:      StalenessRenamed,
		Message:     "Symbol was renamed",
		Suggestions: []string{"UserService.Authenticate"},
		NewSymbolID: &newSymbolID,
	}

	if ref.RawText != "`UserService.Auth`" {
		t.Errorf("RawText = %q, want %q", ref.RawText, "`UserService.Auth`")
	}
	if ref.Line != 15 {
		t.Errorf("Line = %d, want %d", ref.Line, 15)
	}
	if ref.Reason != StalenessRenamed {
		t.Errorf("Reason = %v, want %v", ref.Reason, StalenessRenamed)
	}
	if ref.NewSymbolID == nil || *ref.NewSymbolID != newSymbolID {
		t.Error("NewSymbolID not set correctly")
	}
}
