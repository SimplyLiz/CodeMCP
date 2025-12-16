package identity

import "strings"

// BackendIdRole describes how a backend's IDs should be used
// Section 4.2: Backends as ID anchors
type BackendIdRole string

const (
	// RolePrimaryAnchor means this backend's IDs are stable and should be used as anchors
	RolePrimaryAnchor BackendIdRole = "primary-anchor"
	// RoleResolverOnly means this backend's IDs are unstable and should only be used for resolution
	RoleResolverOnly BackendIdRole = "resolver-only"
)

// Backend ID stability rules:
// - SCIP: High stability (primary anchor)
// - Glean: High stability (primary anchor)
// - LSP: Low stability (resolver only, never anchor)
//
// LSP IDs are not stable because:
// 1. They can change between LSP server restarts
// 2. They depend on internal LSP state and parsing
// 3. Different LSP implementations may generate different IDs
// 4. They're not designed to be persistent identifiers

// GetBackendIdRole determines the role of a backend ID based on its prefix
func GetBackendIdRole(backendId string) BackendIdRole {
	if backendId == "" {
		return RoleResolverOnly
	}

	// SCIP IDs typically start with "scip:" or contain "scip" in the scheme
	if strings.HasPrefix(backendId, "scip:") {
		return RolePrimaryAnchor
	}

	// Glean IDs typically contain "glean" in the identifier
	if strings.Contains(backendId, "glean:") {
		return RolePrimaryAnchor
	}

	// LSP IDs are typically just numeric or file-position based
	// They don't have a consistent prefix scheme and are inherently unstable
	// Examples: "file:///path/to/file.ts#L10:5" or internal numeric IDs
	// Default to resolver-only for safety
	return RoleResolverOnly
}

// CanBeIdAnchor returns true if a backend ID can be used as a stable anchor
func CanBeIdAnchor(backendId string) bool {
	return GetBackendIdRole(backendId) == RolePrimaryAnchor
}

// IsScipId checks if a backend ID is from SCIP
func IsScipId(backendId string) bool {
	return strings.HasPrefix(backendId, "scip:")
}

// IsGleanId checks if a backend ID is from Glean
func IsGleanId(backendId string) bool {
	return strings.Contains(backendId, "glean:")
}

// IsLspId checks if a backend ID appears to be from LSP
func IsLspId(backendId string) bool {
	// LSP IDs don't have a standard format, but they're typically not SCIP or Glean
	return !IsScipId(backendId) && !IsGleanId(backendId)
}

// GetBackendType returns the backend type from a backend ID
func GetBackendType(backendId string) string {
	if IsScipId(backendId) {
		return "scip"
	}
	if IsGleanId(backendId) {
		return "glean"
	}
	return "lsp"
}
