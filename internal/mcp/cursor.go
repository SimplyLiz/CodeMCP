package mcp

import (
	"encoding/base64"
	"encoding/json"

	"ckb/internal/errors"
)

// DefaultPageSize is the default number of tools per page
const DefaultPageSize = 15

// ToolsCursorPayload contains pagination state for tools/list
//
// Design note: We use toolsetHash for cursor invalidation rather than session
// snapshots because tool definitions are currently static within a session.
// If we add plugins or feature flags that dynamically modify tool schemas
// mid-session, consider adding a SnapshotId field for session-scoped validation.
type ToolsCursorPayload struct {
	V           int    `json:"v"` // cursor version
	Preset      string `json:"p"` // active preset
	Offset      int    `json:"o"` // position in tool list
	ToolsetHash string `json:"h"` // hash of tool definitions
}

// EncodeToolsCursor encodes cursor data to a URL-safe base64 string
func EncodeToolsCursor(preset string, offset int, toolsetHash string) string {
	payload := ToolsCursorPayload{
		V:           1,
		Preset:      preset,
		Offset:      offset,
		ToolsetHash: toolsetHash,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}

	return base64.RawURLEncoding.EncodeToString(data)
}

// DecodeToolsCursor decodes and validates a cursor string.
// Returns the offset if valid, or an error if invalid/stale.
func DecodeToolsCursor(cursor string, currentPreset string, currentHash string) (int, error) {
	if cursor == "" {
		return 0, nil // Empty cursor = first page
	}

	// Decode base64
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, errors.NewInvalidParameterError("cursor", "invalid encoding")
	}

	// Parse JSON
	var payload ToolsCursorPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return 0, errors.NewInvalidParameterError("cursor", "invalid format")
	}

	// Validate version
	if payload.V != 1 {
		return 0, errors.NewInvalidParameterError("cursor", "version mismatch")
	}

	// Validate preset matches
	if payload.Preset != currentPreset {
		return 0, errors.NewInvalidParameterError("cursor", "preset changed since cursor was issued")
	}

	// Validate toolset hash matches
	if payload.ToolsetHash != currentHash {
		return 0, errors.NewInvalidParameterError("cursor", "toolset changed since cursor was issued")
	}

	// Validate offset is reasonable
	if payload.Offset < 0 {
		return 0, errors.NewInvalidParameterError("cursor", "invalid offset")
	}

	return payload.Offset, nil
}

// PaginateTools returns a page of tools and the next cursor (if more exist).
// Returns (tools, nextCursor, error)
func PaginateTools(allTools []Tool, offset int, pageSize int, preset string, toolsetHash string) ([]Tool, string, error) {
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}

	total := len(allTools)

	// Validate offset
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []Tool{}, "", nil // Past end
	}

	// Calculate end
	end := offset + pageSize
	if end > total {
		end = total
	}

	// Get page slice
	page := allTools[offset:end]

	// Generate next cursor if more results exist
	var nextCursor string
	if end < total {
		nextCursor = EncodeToolsCursor(preset, end, toolsetHash)
	}

	return page, nextCursor, nil
}
