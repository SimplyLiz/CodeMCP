package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"ckb/internal/config"
	"ckb/internal/logging"
	"ckb/internal/query"
	"ckb/internal/storage"
	"ckb/internal/version"
)

// TestResourcesList tests the resources/list method
func TestResourcesList(t *testing.T) {
	server := newTestMCPServer(t)

	response := sendRequest(t, server, "resources/list", 1, nil)

	if response == nil {
		t.Fatal("Response should not be nil")
	}

	if response.Error != nil {
		t.Errorf("Should not have error: %v", response.Error.Message)
	}

	if response.Result == nil {
		t.Fatal("Response should have result")
	}

	// Check that result has resources
	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Result should be a map, got %T", response.Result)
	}

	// Check resources
	resources, ok := result["resources"]
	if !ok {
		t.Fatal("Result should have resources")
	}

	resourcesList, ok := resources.([]Resource)
	if !ok {
		t.Fatalf("Resources should be []Resource, got %T", resources)
	}

	// Should have at least status and architecture resources
	if len(resourcesList) < 2 {
		t.Errorf("Expected at least 2 resources, got %d", len(resourcesList))
	}

	// Check resource templates
	templates, ok := result["resourceTemplates"]
	if !ok {
		t.Fatal("Result should have resourceTemplates")
	}

	templatesList, ok := templates.([]ResourceTemplate)
	if !ok {
		t.Fatalf("ResourceTemplates should be []ResourceTemplate, got %T", templates)
	}

	if len(templatesList) < 2 {
		t.Errorf("Expected at least 2 resource templates, got %d", len(templatesList))
	}
}

// TestResourcesRead tests the resources/read method
func TestResourcesRead(t *testing.T) {
	server := newTestMCPServer(t)

	testCases := []struct {
		name        string
		uri         string
		expectError bool
		errorSubstr string
	}{
		{
			name:        "read status resource",
			uri:         "ckb://status",
			expectError: false,
		},
		{
			name:        "read architecture resource",
			uri:         "ckb://architecture",
			expectError: false,
		},
		{
			name:        "invalid URI scheme",
			uri:         "http://invalid",
			expectError: true,
			errorSubstr: "invalid URI scheme",
		},
		{
			name:        "unknown resource type",
			uri:         "ckb://unknown",
			expectError: true,
			errorSubstr: "unknown resource type",
		},
		{
			name:        "module without ID",
			uri:         "ckb://module",
			expectError: true,
			errorSubstr: "module URI requires module ID",
		},
		{
			name:        "symbol without ID",
			uri:         "ckb://symbol",
			expectError: true,
			errorSubstr: "symbol URI requires symbol ID",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]interface{}{
				"uri": tc.uri,
			}

			response := sendRequest(t, server, "resources/read", 1, params)

			if response == nil {
				t.Fatal("Response should not be nil")
			}

			if tc.expectError {
				if response.Error == nil {
					t.Error("Expected error but got none")
				} else if tc.errorSubstr != "" && !contains(response.Error.Message, tc.errorSubstr) {
					t.Errorf("Expected error containing '%s', got '%s'", tc.errorSubstr, response.Error.Message)
				}
			} else {
				if response.Error != nil {
					t.Errorf("Unexpected error: %v", response.Error.Message)
				}
			}
		})
	}
}

// TestResourcesReadMissingURI tests resources/read with missing URI
func TestResourcesReadMissingURI(t *testing.T) {
	server := newTestMCPServer(t)

	params := map[string]interface{}{}

	response := sendRequest(t, server, "resources/read", 1, params)

	if response == nil {
		t.Fatal("Response should not be nil")
	}

	if response.Error == nil {
		t.Error("Expected error for missing URI")
	}
}

// TestResourcesReadModuleResource tests reading a module resource
func TestResourcesReadModuleResource(t *testing.T) {
	server := newTestMCPServer(t)

	params := map[string]interface{}{
		"uri": "ckb://module/test-module-id",
	}

	response := sendRequest(t, server, "resources/read", 1, params)

	if response == nil {
		t.Fatal("Response should not be nil")
	}

	// Module resource is a placeholder, should not error
	if response.Error != nil {
		t.Errorf("Unexpected error: %v", response.Error.Message)
	}
}

// TestNotificationHandling tests notification handling
func TestNotificationHandling(t *testing.T) {
	server := newTestMCPServer(t)

	// Create notification message (no ID)
	notification := MCPMessage{
		Jsonrpc: "2.0",
		Method:  "notifications/initialized",
	}

	response := server.handleMessage(&notification)

	// Notifications should not produce a response
	if response != nil {
		t.Error("Notification should not produce a response")
	}
}

// TestUnknownNotification tests handling of unknown notifications
func TestUnknownNotification(t *testing.T) {
	server := newTestMCPServer(t)

	notification := MCPMessage{
		Jsonrpc: "2.0",
		Method:  "unknown/notification",
	}

	response := server.handleMessage(&notification)

	// Unknown notifications should still not produce a response
	if response != nil {
		t.Error("Unknown notification should not produce a response")
	}
}

// TestInvalidMessage tests handling of invalid messages
func TestInvalidMessage(t *testing.T) {
	server := newTestMCPServer(t)

	// Message with both method and result (invalid)
	msg := MCPMessage{
		Jsonrpc: "2.0",
		Result:  "ok",
	}

	response := server.handleMessage(&msg)

	if response == nil {
		t.Fatal("Should return error response for invalid message")
	}

	if response.Error == nil {
		t.Error("Should have error for invalid message")
	}

	if response.Error.Code != InvalidRequest {
		t.Errorf("Expected InvalidRequest error, got %d", response.Error.Code)
	}
}

// TestToolCallGetStatus tests the getStatus tool
func TestToolCallGetStatus(t *testing.T) {
	server := newTestMCPServer(t)

	params := map[string]interface{}{
		"name":      "getStatus",
		"arguments": map[string]interface{}{},
	}

	response := sendRequest(t, server, "tools/call", 1, params)

	if response == nil {
		t.Fatal("Response should not be nil")
	}

	// Status might succeed or fail depending on backend setup, both are valid
	if response.Result != nil {
		// Verify result structure
		result, ok := response.Result.(map[string]interface{})
		if !ok {
			t.Errorf("Expected result to be a map, got %T", response.Result)
		}

		if _, ok := result["content"]; !ok {
			t.Error("Result should have content field")
		}
	}
}

// TestToolCallDoctor tests the doctor tool
func TestToolCallDoctor(t *testing.T) {
	server := newTestMCPServer(t)

	params := map[string]interface{}{
		"name":      "doctor",
		"arguments": map[string]interface{}{},
	}

	response := sendRequest(t, server, "tools/call", 1, params)

	if response == nil {
		t.Fatal("Response should not be nil")
	}

	// Doctor might succeed or fail, both are valid for this test
}

// TestToolCallGetArchitecture tests the getArchitecture tool
func TestToolCallGetArchitecture(t *testing.T) {
	server := newTestMCPServer(t)

	params := map[string]interface{}{
		"name": "getArchitecture",
		"arguments": map[string]interface{}{
			"depth": 2,
		},
	}

	response := sendRequest(t, server, "tools/call", 1, params)

	if response == nil {
		t.Fatal("Response should not be nil")
	}
}

// TestToolCallWithInvalidParams tests tools/call with invalid params type
func TestToolCallWithInvalidParams(t *testing.T) {
	server := newTestMCPServer(t)

	// Send request with invalid params type (string instead of object)
	request := MCPMessage{
		Jsonrpc: "2.0",
		Id:      1,
		Method:  "tools/call",
		Params:  "invalid",
	}

	response := server.handleMessage(&request)

	if response == nil {
		t.Fatal("Response should not be nil")
	}

	if response.Error == nil {
		t.Error("Should have error for invalid params type")
	}
}

// TestGetResourceDefinitions tests the GetResourceDefinitions method
func TestGetResourceDefinitions(t *testing.T) {
	server := newTestMCPServer(t)

	resources, templates := server.GetResourceDefinitions()

	// Verify resources
	if len(resources) == 0 {
		t.Error("Expected at least one resource")
	}

	foundStatus := false
	foundArchitecture := false
	for _, r := range resources {
		if r.URI == "ckb://status" {
			foundStatus = true
		}
		if r.URI == "ckb://architecture" {
			foundArchitecture = true
		}
	}

	if !foundStatus {
		t.Error("Expected status resource")
	}
	if !foundArchitecture {
		t.Error("Expected architecture resource")
	}

	// Verify templates
	if len(templates) == 0 {
		t.Error("Expected at least one resource template")
	}

	foundModule := false
	foundSymbol := false
	for _, tmpl := range templates {
		if tmpl.URITemplate == "ckb://module/{moduleId}" {
			foundModule = true
		}
		if tmpl.URITemplate == "ckb://symbol/{symbolId}" {
			foundSymbol = true
		}
	}

	if !foundModule {
		t.Error("Expected module template")
	}
	if !foundSymbol {
		t.Error("Expected symbol template")
	}
}

// TestGetToolDefinitions tests the GetToolDefinitions method
func TestGetToolDefinitions(t *testing.T) {
	server := newTestMCPServer(t)

	tools := server.GetToolDefinitions()

	if len(tools) == 0 {
		t.Fatal("Expected at least one tool")
	}

	// Verify some expected tools exist
	expectedTools := []string{
		"getStatus",
		"doctor",
		"getSymbol",
		"searchSymbols",
		"findReferences",
		"getArchitecture",
		"analyzeImpact",
		"explainSymbol",
		"justifySymbol",
		"getCallGraph",
		"getModuleOverview",
	}

	toolMap := make(map[string]bool)
	for _, tool := range tools {
		toolMap[tool.Name] = true

		// Verify tool has required fields
		if tool.Name == "" {
			t.Error("Tool should have name")
		}
		if tool.Description == "" {
			t.Errorf("Tool %s should have description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("Tool %s should have inputSchema", tool.Name)
		}
	}

	for _, expected := range expectedTools {
		if !toolMap[expected] {
			t.Errorf("Expected tool '%s' not found", expected)
		}
	}
}

// TestMCPServerWithCustomIO tests server with custom stdin/stdout
func TestMCPServerWithCustomIO(t *testing.T) {
	server := newTestMCPServer(t)

	// Create custom buffers
	stdin := &bytes.Buffer{}
	stdout := &bytes.Buffer{}

	server.SetStdin(stdin)
	server.SetStdout(stdout)

	// Write a request to stdin
	request := MCPMessage{
		Jsonrpc: "2.0",
		Id:      1,
		Method:  "tools/list",
	}
	requestBytes, _ := json.Marshal(request)
	stdin.Write(requestBytes)
	stdin.WriteString("\n")

	// Read and handle message
	msg, err := server.readMessage()
	if err != nil && err != io.EOF {
		t.Fatalf("Failed to read message: %v", err)
	}

	response := server.handleMessage(msg)
	if response == nil {
		t.Fatal("Response should not be nil")
	}

	if response.Error != nil {
		t.Errorf("Unexpected error: %v", response.Error.Message)
	}
}

// TestHandleInitialize tests the handleInitialize method
func TestHandleInitialize(t *testing.T) {
	server := newTestMCPServer(t)

	params := map[string]interface{}{
		"protocolVersion": "0.1.0",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "test-client",
			"version": "1.0.0",
		},
	}

	result, err := server.handleInitialize(params)
	if err != nil {
		t.Fatalf("handleInitialize failed: %v", err)
	}

	initResult := result

	if initResult.ProtocolVersion == "" {
		t.Error("Should have protocol version")
	}

	if initResult.ServerInfo.Name != "ckb" {
		t.Errorf("Expected server name 'ckb', got '%s'", initResult.ServerInfo.Name)
	}

	if initResult.Capabilities.Tools == nil {
		t.Error("Should have tools capability")
	}

	if initResult.Capabilities.Resources == nil {
		t.Error("Should have resources capability")
	}
}

// TestMultipleToolCalls tests multiple sequential tool calls
func TestMultipleToolCalls(t *testing.T) {
	server := newTestMCPServer(t)

	// Call getStatus first
	params1 := map[string]interface{}{
		"name":      "getStatus",
		"arguments": map[string]interface{}{},
	}
	response1 := sendRequest(t, server, "tools/call", 1, params1)
	if response1 == nil {
		t.Fatal("First response should not be nil")
	}

	// Call doctor second
	params2 := map[string]interface{}{
		"name":      "doctor",
		"arguments": map[string]interface{}{},
	}
	response2 := sendRequest(t, server, "tools/call", 2, params2)
	if response2 == nil {
		t.Fatal("Second response should not be nil")
	}

	// Verify responses are returned (IDs are interface{} so compare as such)
	if response1.Id == nil {
		t.Error("First response should have an ID")
	}
	if response2.Id == nil {
		t.Error("Second response should have an ID")
	}
}

// TestErrorCodes tests various error codes
func TestErrorCodes(t *testing.T) {
	// Test error code values
	if ParseError != -32700 {
		t.Errorf("ParseError should be -32700, got %d", ParseError)
	}
	if InvalidRequest != -32600 {
		t.Errorf("InvalidRequest should be -32600, got %d", InvalidRequest)
	}
	if MethodNotFound != -32601 {
		t.Errorf("MethodNotFound should be -32601, got %d", MethodNotFound)
	}
	if InvalidParams != -32602 {
		t.Errorf("InvalidParams should be -32602, got %d", InvalidParams)
	}
	if InternalError != -32603 {
		t.Errorf("InternalError should be -32603, got %d", InternalError)
	}
}

// TestMCPServerVersion tests that the server version is set correctly
func TestMCPServerVersion(t *testing.T) {
	cfg := &config.Config{
		Version:  5,
		RepoRoot: ".",
		Backends: config.BackendsConfig{
			Git:  config.GitConfig{Enabled: false},
			Scip: config.ScipConfig{Enabled: false},
			Lsp:  config.LspConfig{Enabled: false},
		},
	}

	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.JSONFormat,
		Output: io.Discard,
	})

	db, err := storage.Open(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	engine, err := query.NewEngine(".", db, logger, cfg)
	if err != nil {
		t.Fatalf("Failed to create query engine: %v", err)
	}

	testVersion := "1.2.3-test"
	server := NewMCPServer(testVersion, engine, logger)

	if server.version != testVersion {
		t.Errorf("Expected version '%s', got '%s'", testVersion, server.version)
	}

	// Verify version is returned in initialize response
	params := map[string]interface{}{
		"protocolVersion": "0.1.0",
		"capabilities":    map[string]interface{}{},
	}

	result, err := server.handleInitialize(params)
	if err != nil {
		t.Fatalf("handleInitialize failed: %v", err)
	}

	if result.ServerInfo.Version != testVersion {
		t.Errorf("Expected version '%s' in response, got '%s'", testVersion, result.ServerInfo.Version)
	}
}

// Helper function to create test server with custom version
func newTestMCPServerWithVersion(t *testing.T, ver string) *MCPServer {
	t.Helper()

	cfg := &config.Config{
		Version:  5,
		RepoRoot: ".",
		Backends: config.BackendsConfig{
			Git:  config.GitConfig{Enabled: false},
			Scip: config.ScipConfig{Enabled: false},
			Lsp:  config.LspConfig{Enabled: false},
		},
	}

	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.JSONFormat,
		Output: io.Discard,
	})

	db, err := storage.Open(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	engine, err := query.NewEngine(".", db, logger, cfg)
	if err != nil {
		t.Fatalf("Failed to create query engine: %v", err)
	}

	return NewMCPServer(ver, engine, logger)
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// BenchmarkToolsList benchmarks the tools/list method
func BenchmarkToolsList(b *testing.B) {
	cfg := &config.Config{
		Version:  5,
		RepoRoot: ".",
		Backends: config.BackendsConfig{
			Git:  config.GitConfig{Enabled: false},
			Scip: config.ScipConfig{Enabled: false},
			Lsp:  config.LspConfig{Enabled: false},
		},
	}

	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.JSONFormat,
		Output: io.Discard,
	})

	db, _ := storage.Open(":memory:", logger)
	engine, _ := query.NewEngine(".", db, logger, cfg)
	server := NewMCPServer(version.Version, engine, logger)

	msg := &MCPMessage{
		Jsonrpc: "2.0",
		Id:      1,
		Method:  "tools/list",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.handleMessage(msg)
	}
}

// BenchmarkResourcesList benchmarks the resources/list method
func BenchmarkResourcesList(b *testing.B) {
	cfg := &config.Config{
		Version:  5,
		RepoRoot: ".",
		Backends: config.BackendsConfig{
			Git:  config.GitConfig{Enabled: false},
			Scip: config.ScipConfig{Enabled: false},
			Lsp:  config.LspConfig{Enabled: false},
		},
	}

	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.JSONFormat,
		Output: io.Discard,
	})

	db, _ := storage.Open(":memory:", logger)
	engine, _ := query.NewEngine(".", db, logger, cfg)
	server := NewMCPServer(version.Version, engine, logger)

	msg := &MCPMessage{
		Jsonrpc: "2.0",
		Id:      1,
		Method:  "resources/list",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.handleMessage(msg)
	}
}
