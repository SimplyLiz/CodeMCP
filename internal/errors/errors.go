package errors

import (
	"fmt"
)

// ErrorCode represents stable error codes for all failure modes
type ErrorCode string

const (
	// BackendUnavailable indicates a backend is not running or reachable
	BackendUnavailable ErrorCode = "BACKEND_UNAVAILABLE"
	// IndexMissing indicates SCIP index not found
	IndexMissing ErrorCode = "INDEX_MISSING"
	// IndexStale indicates SCIP index is too old
	IndexStale ErrorCode = "INDEX_STALE"
	// WorkspaceNotReady indicates LSP is still initializing
	WorkspaceNotReady ErrorCode = "WORKSPACE_NOT_READY"
	// Timeout indicates query timed out
	Timeout ErrorCode = "TIMEOUT"
	// RateLimited indicates too many concurrent requests
	RateLimited ErrorCode = "RATE_LIMITED"
	// SymbolNotFound indicates symbol doesn't exist
	SymbolNotFound ErrorCode = "SYMBOL_NOT_FOUND"
	// SymbolDeleted indicates symbol was deleted
	SymbolDeleted ErrorCode = "SYMBOL_DELETED"
	// ScopeInvalid indicates invalid scope parameter
	ScopeInvalid ErrorCode = "SCOPE_INVALID"
	// AliasCycle indicates circular alias chain
	AliasCycle ErrorCode = "ALIAS_CYCLE"
	// AliasChainTooDeep indicates alias chain exceeds max depth
	AliasChainTooDeep ErrorCode = "ALIAS_CHAIN_TOO_DEEP"
	// BudgetExceeded indicates hit backend/response limits
	BudgetExceeded ErrorCode = "BUDGET_EXCEEDED"
	// InternalError indicates unexpected error
	InternalError ErrorCode = "INTERNAL_ERROR"
)

// FixActionType represents the type of fix action
type FixActionType string

const (
	// RunCommand suggests running a command
	RunCommand FixActionType = "run-command"
	// OpenDocs suggests opening documentation
	OpenDocs FixActionType = "open-docs"
	// InstallTool suggests installing a tool
	InstallTool FixActionType = "install-tool"
)

// InstallMethod represents methods for installing tools
type InstallMethod string

const (
	// Brew installation via Homebrew
	Brew InstallMethod = "brew"
	// NPM installation via npm
	NPM InstallMethod = "npm"
	// Cargo installation via cargo
	Cargo InstallMethod = "cargo"
	// Manual installation
	Manual InstallMethod = "manual"
)

// FixAction represents a suggested fix for an error
type FixAction struct {
	Type        FixActionType   `json:"type"`
	Command     string          `json:"command,omitempty"`
	Safe        bool            `json:"safe,omitempty"`
	Description string          `json:"description,omitempty"`
	URL         string          `json:"url,omitempty"`
	Tool        string          `json:"tool,omitempty"`
	Methods     []InstallMethod `json:"methods,omitempty"`
}

// Drilldown represents a suggested follow-up query
type Drilldown struct {
	Label string `json:"label"`
	Query string `json:"query"`
}

// CkbError represents a CKB error with code, message, and suggestions
type CkbError struct {
	Code           ErrorCode   `json:"code"`
	Message        string      `json:"message"`
	Details        interface{} `json:"details,omitempty"`
	SuggestedFixes []FixAction `json:"suggestedFixes,omitempty"`
	Drilldowns     []Drilldown `json:"drilldowns,omitempty"`
	cause          error       // Underlying error (not exported to JSON)
}

// NewCkbError creates a new CkbError
func NewCkbError(code ErrorCode, message string, cause error, suggestedFixes []FixAction, drilldowns []Drilldown) *CkbError {
	return &CkbError{
		Code:           code,
		Message:        message,
		cause:          cause,
		SuggestedFixes: suggestedFixes,
		Drilldowns:     drilldowns,
	}
}

// Error implements the error interface
func (e *CkbError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *CkbError) Unwrap() error {
	return e.cause
}

// WithDetails adds details to the error
func (e *CkbError) WithDetails(details interface{}) *CkbError {
	e.Details = details
	return e
}

// ErrorActions maps error codes to suggested fix actions
var ErrorActions = map[ErrorCode][]FixAction{
	IndexMissing: {
		{
			Type:        RunCommand,
			Command:     "ckb doctor --check=scip",
			Safe:        true,
			Description: "Check SCIP configuration and generate index",
		},
	},
	IndexStale: {
		{
			Type:        RunCommand,
			Command:     "${detected_scip_command}",
			Safe:        true,
			Description: "Regenerate SCIP index",
		},
	},
	WorkspaceNotReady: {
		{
			Type:        RunCommand,
			Command:     "ckb status --wait-for-ready",
			Safe:        true,
			Description: "Wait for LSP workspace to be ready",
		},
	},
	RateLimited: {
		{
			Type:        RunCommand,
			Command:     "sleep 2 && ckb ${retry_command}",
			Safe:        true,
			Description: "Retry after brief delay",
		},
	},
	BackendUnavailable: {
		{
			Type:        RunCommand,
			Command:     "ckb doctor",
			Safe:        true,
			Description: "Check backend configuration",
		},
	},
}

// GetSuggestedFixes returns suggested fixes for an error code
func GetSuggestedFixes(code ErrorCode) []FixAction {
	if fixes, ok := ErrorActions[code]; ok {
		return fixes
	}
	return nil
}
