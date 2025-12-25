package identity

import (
	"testing"
)

func TestGetBackendIdRole(t *testing.T) {
	tests := []struct {
		backendId string
		want      BackendIdRole
	}{
		// SCIP IDs (primary anchor)
		{"scip:go github.com/example/pkg", RolePrimaryAnchor},
		{"scip:typescript npm @types/node", RolePrimaryAnchor},
		{"scip:", RolePrimaryAnchor},

		// Glean IDs (primary anchor)
		{"glean:symbol/123", RolePrimaryAnchor},
		{"prefix:glean:suffix", RolePrimaryAnchor},

		// LSP and other IDs (resolver only)
		{"file:///path/to/file.ts#L10:5", RoleResolverOnly},
		{"123456", RoleResolverOnly},
		{"lsp:internal:id", RoleResolverOnly},
		{"", RoleResolverOnly},
	}

	for _, tt := range tests {
		t.Run(tt.backendId, func(t *testing.T) {
			if got := GetBackendIdRole(tt.backendId); got != tt.want {
				t.Errorf("GetBackendIdRole(%q) = %v, want %v", tt.backendId, got, tt.want)
			}
		})
	}
}

func TestCanBeIdAnchor(t *testing.T) {
	tests := []struct {
		backendId string
		want      bool
	}{
		{"scip:go github.com/example", true},
		{"glean:symbol/123", true},
		{"file:///path", false},
		{"123", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.backendId, func(t *testing.T) {
			if got := CanBeIdAnchor(tt.backendId); got != tt.want {
				t.Errorf("CanBeIdAnchor(%q) = %v, want %v", tt.backendId, got, tt.want)
			}
		})
	}
}

func TestIsScipId(t *testing.T) {
	tests := []struct {
		backendId string
		want      bool
	}{
		{"scip:go github.com/example", true},
		{"scip:", true},
		{"SCIP:uppercase", false}, // case sensitive
		{"xscip:not-prefix", false},
		{"glean:not-scip", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.backendId, func(t *testing.T) {
			if got := IsScipId(tt.backendId); got != tt.want {
				t.Errorf("IsScipId(%q) = %v, want %v", tt.backendId, got, tt.want)
			}
		})
	}
}

func TestIsGleanId(t *testing.T) {
	tests := []struct {
		backendId string
		want      bool
	}{
		{"glean:symbol/123", true},
		{"prefix:glean:suffix", true},
		{"glean:", true},
		{"GLEAN:uppercase", false}, // case sensitive
		{"scip:not-glean", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.backendId, func(t *testing.T) {
			if got := IsGleanId(tt.backendId); got != tt.want {
				t.Errorf("IsGleanId(%q) = %v, want %v", tt.backendId, got, tt.want)
			}
		})
	}
}

func TestIsLspId(t *testing.T) {
	tests := []struct {
		backendId string
		want      bool
	}{
		{"file:///path/to/file.ts", true},
		{"123456", true},
		{"internal-id", true},
		{"", true}, // empty is considered LSP (not SCIP or Glean)
		{"scip:something", false},
		{"glean:something", false},
	}

	for _, tt := range tests {
		t.Run(tt.backendId, func(t *testing.T) {
			if got := IsLspId(tt.backendId); got != tt.want {
				t.Errorf("IsLspId(%q) = %v, want %v", tt.backendId, got, tt.want)
			}
		})
	}
}

func TestGetBackendType(t *testing.T) {
	tests := []struct {
		backendId string
		want      string
	}{
		{"scip:go github.com/example", "scip"},
		{"scip:", "scip"},
		{"glean:symbol/123", "glean"},
		{"prefix:glean:suffix", "glean"},
		{"file:///path", "lsp"},
		{"123", "lsp"},
		{"", "lsp"},
	}

	for _, tt := range tests {
		t.Run(tt.backendId, func(t *testing.T) {
			if got := GetBackendType(tt.backendId); got != tt.want {
				t.Errorf("GetBackendType(%q) = %q, want %q", tt.backendId, got, tt.want)
			}
		})
	}
}

func TestBackendIdRoleConstants(t *testing.T) {
	// Verify role constants have expected string values
	if string(RolePrimaryAnchor) != "primary-anchor" {
		t.Errorf("RolePrimaryAnchor = %q, want primary-anchor", RolePrimaryAnchor)
	}
	if string(RoleResolverOnly) != "resolver-only" {
		t.Errorf("RoleResolverOnly = %q, want resolver-only", RoleResolverOnly)
	}
}
