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

	// v8.0 error codes

	// AmbiguousQuery indicates multiple matches for a query
	AmbiguousQuery ErrorCode = "AMBIGUOUS_QUERY"
	// PartialResult indicates some backends failed but partial data returned
	PartialResult ErrorCode = "PARTIAL_RESULT"
	// InvalidParameter indicates a missing or invalid parameter
	InvalidParameter ErrorCode = "INVALID_PARAMETER"
	// ResourceNotFound indicates a requested resource doesn't exist
	ResourceNotFound ErrorCode = "RESOURCE_NOT_FOUND"
	// PreconditionFailed indicates a required precondition is not met
	PreconditionFailed ErrorCode = "PRECONDITION_FAILED"
	// OperationFailed indicates an operation failed
	OperationFailed ErrorCode = "OPERATION_FAILED"
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
	AmbiguousQuery: {
		{
			Type:        OpenDocs,
			Description: "Refine your query to be more specific, or use symbol IDs for exact matches",
		},
	},
	PartialResult: {
		{
			Type:        RunCommand,
			Command:     "ckb status",
			Safe:        true,
			Description: "Check which backends are available",
		},
	},
	InvalidParameter: {
		{
			Type:        OpenDocs,
			Description: "Check parameter requirements in tool documentation",
		},
	},
	ResourceNotFound: {
		{
			Type:        RunCommand,
			Command:     "ckb status",
			Safe:        true,
			Description: "Check system status and available resources",
		},
	},
	PreconditionFailed: {
		{
			Type:        RunCommand,
			Command:     "ckb doctor",
			Safe:        true,
			Description: "Run diagnostics to check system configuration",
		},
	},
	OperationFailed: {
		{
			Type:        RunCommand,
			Command:     "ckb doctor",
			Safe:        true,
			Description: "Run diagnostics to identify the issue",
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

// Convenience constructors for common error types (v8.0)

// NewInvalidParameterError creates an error for missing or invalid parameters
func NewInvalidParameterError(paramName string, reason string) *CkbError {
	msg := fmt.Sprintf("missing or invalid '%s' parameter", paramName)
	if reason != "" {
		msg = fmt.Sprintf("invalid '%s' parameter: %s", paramName, reason)
	}
	return &CkbError{
		Code:           InvalidParameter,
		Message:        msg,
		SuggestedFixes: GetSuggestedFixes(InvalidParameter),
	}
}

// NewResourceNotFoundError creates an error for missing resources
func NewResourceNotFoundError(resourceType, resourceName string) *CkbError {
	return &CkbError{
		Code:           ResourceNotFound,
		Message:        fmt.Sprintf("%s not found: %s", resourceType, resourceName),
		SuggestedFixes: GetSuggestedFixes(ResourceNotFound),
	}
}

// NewPreconditionError creates an error for unmet preconditions
func NewPreconditionError(condition string, remediation string) *CkbError {
	fixes := GetSuggestedFixes(PreconditionFailed)
	if remediation != "" {
		fixes = append([]FixAction{{
			Type:        RunCommand,
			Description: remediation,
		}}, fixes...)
	}
	return &CkbError{
		Code:           PreconditionFailed,
		Message:        condition,
		SuggestedFixes: fixes,
	}
}

// NewOperationError creates an error for failed operations
func NewOperationError(operation string, cause error) *CkbError {
	return &CkbError{
		Code:           OperationFailed,
		Message:        fmt.Sprintf("%s failed", operation),
		cause:          cause,
		SuggestedFixes: GetSuggestedFixes(OperationFailed),
	}
}

// NewAmbiguousQueryError creates an error for queries with multiple matches
func NewAmbiguousQueryError(query string, matchCount int, topMatches []string) *CkbError {
	details := map[string]interface{}{
		"query":      query,
		"matchCount": matchCount,
	}
	if len(topMatches) > 0 {
		details["topMatches"] = topMatches
	}
	return &CkbError{
		Code:           AmbiguousQuery,
		Message:        fmt.Sprintf("query '%s' matched %d symbols; refine your query or use a symbol ID", query, matchCount),
		Details:        details,
		SuggestedFixes: GetSuggestedFixes(AmbiguousQuery),
	}
}

// NewPartialResultError creates an error when partial results are returned
func NewPartialResultError(operation string, failedBackends []string) *CkbError {
	return &CkbError{
		Code:    PartialResult,
		Message: fmt.Sprintf("%s returned partial results; some backends unavailable", operation),
		Details: map[string]interface{}{
			"failedBackends": failedBackends,
		},
		SuggestedFixes: GetSuggestedFixes(PartialResult),
	}
}

// WrapError wraps an error with a CkbError, preserving the cause chain
func WrapError(code ErrorCode, message string, cause error) *CkbError {
	return &CkbError{
		Code:           code,
		Message:        message,
		cause:          cause,
		SuggestedFixes: GetSuggestedFixes(code),
	}
}
