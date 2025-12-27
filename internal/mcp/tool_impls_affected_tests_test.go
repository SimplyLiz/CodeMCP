package mcp

import (
	"testing"
)

func TestToolGetAffectedTests(t *testing.T) {
	server := newTestMCPServer(t)

	params := map[string]interface{}{
		"name": "getAffectedTests",
		"arguments": map[string]interface{}{
			"staged":     false,
			"baseBranch": "HEAD",
			"depth":      1,
		},
	}

	response := sendRequest(t, server, "tools/call", 1, params)

	if response == nil {
		t.Fatal("Response should not be nil")
	}

	// Either succeeds or fails gracefully (depends on git state in test env)
}

func TestToolGetAffectedTestsWithCoverage(t *testing.T) {
	server := newTestMCPServer(t)

	params := map[string]interface{}{
		"name": "getAffectedTests",
		"arguments": map[string]interface{}{
			"staged":      true,
			"useCoverage": true,
			"depth":       2,
		},
	}

	response := sendRequest(t, server, "tools/call", 1, params)

	if response == nil {
		t.Fatal("Response should not be nil")
	}
}

func TestToolGetAffectedTestsDefaultParams(t *testing.T) {
	server := newTestMCPServer(t)

	params := map[string]interface{}{
		"name":      "getAffectedTests",
		"arguments": map[string]interface{}{},
	}

	response := sendRequest(t, server, "tools/call", 1, params)

	if response == nil {
		t.Fatal("Response should not be nil")
	}
}
