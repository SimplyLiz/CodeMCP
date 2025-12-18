package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// QueryParams represents common query parameters for API requests
type QueryParams struct {
	Scope           string   // moduleId
	Kinds           []string // symbol kinds
	Limit           int
	Merge           string // "prefer-first" or "union"
	RepoStateMode   string // "head" or "full"
	Depth           int
	IncludeExternal bool
	Refresh         bool
}

// ParseQueryParams extracts and validates query parameters from the request
func ParseQueryParams(r *http.Request) (*QueryParams, error) {
	query := r.URL.Query()

	params := &QueryParams{
		Scope:         query.Get("scope"),
		Merge:         query.Get("merge"),
		RepoStateMode: query.Get("repoStateMode"),
	}

	// Parse kinds as comma-separated list
	if kindsStr := query.Get("kinds"); kindsStr != "" {
		params.Kinds = strings.Split(kindsStr, ",")
		// Trim whitespace from each kind
		for i, kind := range params.Kinds {
			params.Kinds[i] = strings.TrimSpace(kind)
		}
	}

	// Parse limit
	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, fmt.Errorf("invalid limit parameter: %w", err)
		}
		if limit < 0 {
			return nil, fmt.Errorf("limit must be non-negative")
		}
		params.Limit = limit
	} else {
		params.Limit = 50 // Default limit
	}

	// Parse depth
	if depthStr := query.Get("depth"); depthStr != "" {
		depth, err := strconv.Atoi(depthStr)
		if err != nil {
			return nil, fmt.Errorf("invalid depth parameter: %w", err)
		}
		if depth < 0 {
			return nil, fmt.Errorf("depth must be non-negative")
		}
		params.Depth = depth
	} else {
		params.Depth = 1 // Default depth
	}

	// Parse boolean flags
	params.IncludeExternal = query.Get("includeExternal") == "true"
	params.Refresh = query.Get("refresh") == "true"

	// Validate merge mode
	if params.Merge != "" && params.Merge != "prefer-first" && params.Merge != "union" {
		return nil, fmt.Errorf("invalid merge mode: must be 'prefer-first' or 'union'")
	}

	// Validate repo state mode
	if params.RepoStateMode != "" && params.RepoStateMode != "head" && params.RepoStateMode != "full" {
		return nil, fmt.Errorf("invalid repoStateMode: must be 'head' or 'full'")
	}

	return params, nil
}

// GetPathParam extracts a path parameter from the URL
// For example, with pattern "/symbol/{id}", GetPathParam(r, "/symbol/", "id") returns the ID
func GetPathParam(r *http.Request, prefix string) string {
	path := r.URL.Path
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	return strings.TrimPrefix(path, prefix)
}

// QueryParamInt extracts an integer query parameter with a default value
func QueryParamInt(r *http.Request, name string, defaultVal int) int {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}

// QueryParamFloat extracts a float query parameter with a default value
func QueryParamFloat(r *http.Request, name string, defaultVal float64) float64 {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return defaultVal
	}
	return f
}

// QueryParamBool extracts a boolean query parameter with a default value
func QueryParamBool(r *http.Request, name string, defaultVal bool) bool {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	return val == "true" || val == "1" || val == "yes"
}
