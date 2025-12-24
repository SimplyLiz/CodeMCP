package errors

import (
	"errors"
	"strings"
	"testing"
)

func TestNewCkbError(t *testing.T) {
	cause := errors.New("underlying error")
	fixes := []FixAction{{Type: RunCommand, Command: "ckb doctor"}}
	drilldowns := []Drilldown{{Label: "Check", Query: "status"}}

	err := NewCkbError(IndexMissing, "SCIP index not found", cause, fixes, drilldowns)

	if err.Code != IndexMissing {
		t.Errorf("Code = %v, want %v", err.Code, IndexMissing)
	}
	if err.Message != "SCIP index not found" {
		t.Errorf("Message = %q, want %q", err.Message, "SCIP index not found")
	}
	if len(err.SuggestedFixes) != 1 {
		t.Errorf("len(SuggestedFixes) = %d, want 1", len(err.SuggestedFixes))
	}
	if len(err.Drilldowns) != 1 {
		t.Errorf("len(Drilldowns) = %d, want 1", len(err.Drilldowns))
	}
}

func TestCkbError_Error(t *testing.T) {
	tests := []struct {
		name      string
		code      ErrorCode
		message   string
		cause     error
		wantParts []string
	}{
		{
			name:      "with cause",
			code:      BackendUnavailable,
			message:   "LSP not running",
			cause:     errors.New("connection refused"),
			wantParts: []string{"BACKEND_UNAVAILABLE", "LSP not running", "connection refused"},
		},
		{
			name:      "without cause",
			code:      SymbolNotFound,
			message:   "Symbol 'foo' not found",
			cause:     nil,
			wantParts: []string{"SYMBOL_NOT_FOUND", "Symbol 'foo' not found"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewCkbError(tt.code, tt.message, tt.cause, nil, nil)
			got := err.Error()

			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("Error() = %q, want to contain %q", got, part)
				}
			}
		})
	}
}

func TestCkbError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := NewCkbError(InternalError, "something went wrong", cause, nil, nil)

	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}

	// Test nil cause
	errNoCause := NewCkbError(Timeout, "request timed out", nil, nil, nil)
	if errNoCause.Unwrap() != nil {
		t.Errorf("Unwrap() on error without cause should return nil")
	}
}

func TestCkbError_WithDetails(t *testing.T) {
	err := NewCkbError(BudgetExceeded, "response too large", nil, nil, nil)
	details := map[string]int{"size": 10000, "limit": 4000}

	result := err.WithDetails(details)

	// Check that it returns the same error (for chaining)
	if result != err {
		t.Error("WithDetails should return the same error for chaining")
	}

	// Check details are set
	if err.Details == nil {
		t.Error("Details should be set")
	}
}

func TestGetSuggestedFixes(t *testing.T) {
	tests := []struct {
		code    ErrorCode
		wantNil bool
		wantLen int
	}{
		{IndexMissing, false, 1},
		{IndexStale, false, 1},
		{WorkspaceNotReady, false, 1},
		{RateLimited, false, 1},
		{BackendUnavailable, false, 1},
		{SymbolNotFound, true, 0}, // No predefined fixes
		{AliasCycle, true, 0},     // No predefined fixes
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			fixes := GetSuggestedFixes(tt.code)

			if tt.wantNil && fixes != nil {
				t.Errorf("GetSuggestedFixes(%v) = %v, want nil", tt.code, fixes)
			}
			if !tt.wantNil && len(fixes) != tt.wantLen {
				t.Errorf("GetSuggestedFixes(%v) len = %d, want %d", tt.code, len(fixes), tt.wantLen)
			}
		})
	}
}

func TestErrorCodes(t *testing.T) {
	// Ensure all error codes are unique
	codes := []ErrorCode{
		BackendUnavailable,
		IndexMissing,
		IndexStale,
		WorkspaceNotReady,
		Timeout,
		RateLimited,
		SymbolNotFound,
		SymbolDeleted,
		ScopeInvalid,
		AliasCycle,
		AliasChainTooDeep,
		BudgetExceeded,
		InternalError,
	}

	seen := make(map[ErrorCode]bool)
	for _, code := range codes {
		if seen[code] {
			t.Errorf("Duplicate error code: %v", code)
		}
		seen[code] = true

		// Ensure each code is a non-empty string
		if string(code) == "" {
			t.Error("Error code should not be empty")
		}
	}
}

func TestFixActionTypes(t *testing.T) {
	types := []FixActionType{RunCommand, OpenDocs, InstallTool}

	for _, ft := range types {
		if string(ft) == "" {
			t.Error("FixActionType should not be empty")
		}
	}
}

func TestInstallMethods(t *testing.T) {
	methods := []InstallMethod{Brew, NPM, Cargo, Manual}

	for _, m := range methods {
		if string(m) == "" {
			t.Error("InstallMethod should not be empty")
		}
	}
}

func TestFixActionStructure(t *testing.T) {
	action := FixAction{
		Type:        RunCommand,
		Command:     "ckb doctor",
		Safe:        true,
		Description: "Run diagnostics",
		URL:         "https://example.com",
		Tool:        "scip-go",
		Methods:     []InstallMethod{Brew, NPM},
	}

	if action.Type != RunCommand {
		t.Errorf("Type = %v, want %v", action.Type, RunCommand)
	}
	if !action.Safe {
		t.Error("Safe should be true")
	}
	if len(action.Methods) != 2 {
		t.Errorf("len(Methods) = %d, want 2", len(action.Methods))
	}
}

func TestDrilldownStructure(t *testing.T) {
	dd := Drilldown{
		Label: "View references",
		Query: "findReferences --symbol=Engine",
	}

	if dd.Label != "View references" {
		t.Errorf("Label = %q, want %q", dd.Label, "View references")
	}
	if dd.Query != "findReferences --symbol=Engine" {
		t.Errorf("Query = %q, want %q", dd.Query, "findReferences --symbol=Engine")
	}
}

func TestErrorActionsMap(t *testing.T) {
	// Verify ErrorActions map has expected entries
	expectedCodes := []ErrorCode{
		IndexMissing,
		IndexStale,
		WorkspaceNotReady,
		RateLimited,
		BackendUnavailable,
	}

	for _, code := range expectedCodes {
		if _, ok := ErrorActions[code]; !ok {
			t.Errorf("ErrorActions missing entry for %v", code)
		}
	}

	// Verify each entry has valid fix actions
	for code, fixes := range ErrorActions {
		if len(fixes) == 0 {
			t.Errorf("ErrorActions[%v] has no fix actions", code)
		}
		for i, fix := range fixes {
			if fix.Type == "" {
				t.Errorf("ErrorActions[%v][%d].Type is empty", code, i)
			}
		}
	}
}
