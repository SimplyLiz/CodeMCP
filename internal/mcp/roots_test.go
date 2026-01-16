package mcp

import (
	"testing"
)

func TestRootPath(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected string
	}{
		{
			name:     "file URI with standard path",
			uri:      "file:///Users/test/project",
			expected: "/Users/test/project",
		},
		{
			name:     "file URI with spaces",
			uri:      "file:///Users/test/my%20project",
			expected: "/Users/test/my project",
		},
		{
			name:     "non-file URI returns as-is",
			uri:      "/direct/path",
			expected: "/direct/path",
		},
		{
			name:     "empty URI",
			uri:      "",
			expected: "",
		},
		{
			name:     "file URI with nested path",
			uri:      "file:///home/user/projects/my-app/src",
			expected: "/home/user/projects/my-app/src",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := Root{URI: tt.uri}
			got := root.Path()
			if got != tt.expected {
				t.Errorf("Root.Path() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestRootsManager(t *testing.T) {
	t.Run("new roots manager has no roots", func(t *testing.T) {
		rm := newRootsManager()
		roots := rm.GetRoots()
		if roots != nil {
			t.Errorf("expected nil roots, got %v", roots)
		}
		if rm.IsClientSupported() {
			t.Error("expected client not supported by default")
		}
	})

	t.Run("set and get client supported", func(t *testing.T) {
		rm := newRootsManager()
		rm.SetClientSupported(true)
		if !rm.IsClientSupported() {
			t.Error("expected client supported to be true")
		}
		rm.SetClientSupported(false)
		if rm.IsClientSupported() {
			t.Error("expected client supported to be false")
		}
	})

	t.Run("set and get roots", func(t *testing.T) {
		rm := newRootsManager()
		roots := []Root{
			{URI: "file:///project1", Name: "Project 1"},
			{URI: "file:///project2", Name: "Project 2"},
		}
		rm.SetRoots(roots)

		got := rm.GetRoots()
		if len(got) != 2 {
			t.Errorf("expected 2 roots, got %d", len(got))
		}
		if got[0].URI != "file:///project1" {
			t.Errorf("expected first root URI %q, got %q", "file:///project1", got[0].URI)
		}
		if got[1].Name != "Project 2" {
			t.Errorf("expected second root name %q, got %q", "Project 2", got[1].Name)
		}
	})

	t.Run("get paths", func(t *testing.T) {
		rm := newRootsManager()
		roots := []Root{
			{URI: "file:///project1", Name: "Project 1"},
			{URI: "file:///project2", Name: "Project 2"},
		}
		rm.SetRoots(roots)

		paths := rm.GetPaths()
		if len(paths) != 2 {
			t.Errorf("expected 2 paths, got %d", len(paths))
		}
		if paths[0] != "/project1" {
			t.Errorf("expected first path %q, got %q", "/project1", paths[0])
		}
		if paths[1] != "/project2" {
			t.Errorf("expected second path %q, got %q", "/project2", paths[1])
		}
	})

	t.Run("get paths with empty root URI", func(t *testing.T) {
		rm := newRootsManager()
		roots := []Root{
			{URI: "file:///project1", Name: "Project 1"},
			{URI: "", Name: "Empty"},
			{URI: "file:///project3", Name: "Project 3"},
		}
		rm.SetRoots(roots)

		paths := rm.GetPaths()
		// Empty URI should be skipped
		if len(paths) != 2 {
			t.Errorf("expected 2 paths (empty skipped), got %d", len(paths))
		}
	})

	t.Run("roots returns copy", func(t *testing.T) {
		rm := newRootsManager()
		roots := []Root{
			{URI: "file:///project1", Name: "Project 1"},
		}
		rm.SetRoots(roots)

		got := rm.GetRoots()
		got[0].Name = "Modified"

		// Original should be unchanged
		got2 := rm.GetRoots()
		if got2[0].Name == "Modified" {
			t.Error("GetRoots should return a copy, not the original")
		}
	})
}

func TestRootsManagerPendingRequests(t *testing.T) {
	t.Run("request ID generation", func(t *testing.T) {
		rm := newRootsManager()
		id1 := rm.NextRequestID()
		id2 := rm.NextRequestID()
		id3 := rm.NextRequestID()

		if id1 != 1 || id2 != 2 || id3 != 3 {
			t.Errorf("expected IDs 1,2,3, got %d,%d,%d", id1, id2, id3)
		}
	})

	t.Run("register and resolve pending request", func(t *testing.T) {
		rm := newRootsManager()
		id := rm.NextRequestID()
		ch := rm.RegisterPendingRequest(id)

		// Simulate response
		response := &MCPMessage{
			Jsonrpc: "2.0",
			Id:      id,
			Result:  map[string]interface{}{"roots": []interface{}{}},
		}

		// Resolve in goroutine
		go func() {
			rm.ResolvePendingRequest(id, response)
		}()

		// Wait for response
		got := <-ch
		if got.Result == nil {
			t.Error("expected result in response")
		}
	})

	t.Run("resolve unknown request returns false", func(t *testing.T) {
		rm := newRootsManager()
		response := &MCPMessage{Jsonrpc: "2.0", Id: int64(999)}
		if rm.ResolvePendingRequest(999, response) {
			t.Error("expected ResolvePendingRequest to return false for unknown ID")
		}
	})
}

func TestParseClientCapabilities(t *testing.T) {
	t.Run("no capabilities", func(t *testing.T) {
		params := map[string]interface{}{}
		caps := parseClientCapabilities(params)
		if caps.Roots != nil {
			t.Error("expected nil Roots capability")
		}
	})

	t.Run("empty capabilities", func(t *testing.T) {
		params := map[string]interface{}{
			"capabilities": map[string]interface{}{},
		}
		caps := parseClientCapabilities(params)
		if caps.Roots != nil {
			t.Error("expected nil Roots capability")
		}
	})

	t.Run("roots capability with listChanged", func(t *testing.T) {
		params := map[string]interface{}{
			"capabilities": map[string]interface{}{
				"roots": map[string]interface{}{
					"listChanged": true,
				},
			},
		}
		caps := parseClientCapabilities(params)
		if caps.Roots == nil {
			t.Fatal("expected Roots capability to be set")
		}
		if !caps.Roots.ListChanged {
			t.Error("expected listChanged to be true")
		}
	})

	t.Run("roots capability without listChanged", func(t *testing.T) {
		params := map[string]interface{}{
			"capabilities": map[string]interface{}{
				"roots": map[string]interface{}{},
			},
		}
		caps := parseClientCapabilities(params)
		if caps.Roots == nil {
			t.Fatal("expected Roots capability to be set")
		}
		if caps.Roots.ListChanged {
			t.Error("expected listChanged to be false")
		}
	})
}

func TestParseRootsResponse(t *testing.T) {
	t.Run("valid roots response", func(t *testing.T) {
		result := map[string]interface{}{
			"roots": []interface{}{
				map[string]interface{}{
					"uri":  "file:///project1",
					"name": "Project 1",
				},
				map[string]interface{}{
					"uri":  "file:///project2",
					"name": "Project 2",
				},
			},
		}
		roots := parseRootsResponse(result)
		if len(roots) != 2 {
			t.Fatalf("expected 2 roots, got %d", len(roots))
		}
		if roots[0].URI != "file:///project1" {
			t.Errorf("expected first root URI %q, got %q", "file:///project1", roots[0].URI)
		}
		if roots[1].Name != "Project 2" {
			t.Errorf("expected second root name %q, got %q", "Project 2", roots[1].Name)
		}
	})

	t.Run("roots with missing name", func(t *testing.T) {
		result := map[string]interface{}{
			"roots": []interface{}{
				map[string]interface{}{
					"uri": "file:///project1",
				},
			},
		}
		roots := parseRootsResponse(result)
		if len(roots) != 1 {
			t.Fatalf("expected 1 root, got %d", len(roots))
		}
		if roots[0].Name != "" {
			t.Errorf("expected empty name, got %q", roots[0].Name)
		}
	})

	t.Run("nil result", func(t *testing.T) {
		roots := parseRootsResponse(nil)
		if roots != nil {
			t.Errorf("expected nil roots, got %v", roots)
		}
	})

	t.Run("empty roots array", func(t *testing.T) {
		result := map[string]interface{}{
			"roots": []interface{}{},
		}
		roots := parseRootsResponse(result)
		if len(roots) != 0 {
			t.Errorf("expected 0 roots, got %d", len(roots))
		}
	})

	t.Run("root with empty URI is skipped", func(t *testing.T) {
		result := map[string]interface{}{
			"roots": []interface{}{
				map[string]interface{}{
					"uri":  "",
					"name": "Empty",
				},
				map[string]interface{}{
					"uri":  "file:///valid",
					"name": "Valid",
				},
			},
		}
		roots := parseRootsResponse(result)
		if len(roots) != 1 {
			t.Fatalf("expected 1 root (empty URI skipped), got %d", len(roots))
		}
		if roots[0].Name != "Valid" {
			t.Errorf("expected root name %q, got %q", "Valid", roots[0].Name)
		}
	})

	t.Run("invalid type for result", func(t *testing.T) {
		roots := parseRootsResponse("not a map")
		if roots != nil {
			t.Errorf("expected nil roots for invalid type, got %v", roots)
		}
	})

	t.Run("invalid type for roots field", func(t *testing.T) {
		result := map[string]interface{}{
			"roots": "not an array",
		}
		roots := parseRootsResponse(result)
		if roots != nil {
			t.Errorf("expected nil roots for invalid type, got %v", roots)
		}
	})
}
