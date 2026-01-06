package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"ckb/internal/config"
	"ckb/internal/query"
	"ckb/internal/storage"
	"ckb/internal/version"
)

// newTestMCPServer creates an MCP server for testing
func newTestMCPServer(t *testing.T) *MCPServer {
	t.Helper()

	// Use a unique temp directory for each test to avoid database locking
	// issues when running parallel tests with -race
	tempDir := t.TempDir()

	// Create minimal config
	cfg := &config.Config{
		Version:  5,
		RepoRoot: tempDir,
		Backends: config.BackendsConfig{
			Git: config.GitConfig{
				Enabled: false,
			},
			Scip: config.ScipConfig{
				Enabled: false,
			},
			Lsp: config.LspConfig{
				Enabled: false,
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create database in temp directory (each test gets its own isolated DB)
	db, err := storage.Open(tempDir, logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Create query engine with minimal setup
	engine, err := query.NewEngine(tempDir, db, logger, cfg)
	if err != nil {
		t.Fatalf("Failed to create query engine: %v", err)
	}

	// Create MCP server
	server := NewMCPServer(version.Version, engine, logger)

	return server
}

// sendRequest sends a request and returns the response
func sendRequest(t *testing.T, server *MCPServer, method string, id int, params interface{}) *MCPMessage {
	t.Helper()

	// Create request message
	request := MCPMessage{
		Jsonrpc: "2.0",
		Id:      id,
		Method:  method,
		Params:  params,
	}

	// Encode request
	requestBytes, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}
	requestBytes = append(requestBytes, '\n')

	// Set up stdin/stdout buffers
	stdin := bytes.NewReader(requestBytes)
	stdout := &bytes.Buffer{}

	server.SetStdin(stdin)
	server.SetStdout(stdout)

	// Read and handle one message
	msg, err := server.readMessage()
	if err != nil && err != io.EOF {
		t.Fatalf("Failed to read message: %v", err)
	}

	response := server.handleMessage(msg)
	return response
}

func TestMCPServerCreation(t *testing.T) {
	server := newTestMCPServer(t)

	if server == nil {
		t.Fatal("Server should not be nil")
	}

	// Should have registered tools
	if len(server.tools) == 0 {
		t.Error("Server should have registered tools")
	}
}

func TestInitializeMethod(t *testing.T) {
	server := newTestMCPServer(t)

	params := map[string]interface{}{
		"protocolVersion": "0.1.0",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "test-client",
			"version": "1.0.0",
		},
	}

	response := sendRequest(t, server, "initialize", 1, params)

	if response == nil {
		t.Fatal("Response should not be nil")
	}

	if response.Error != nil {
		t.Errorf("Should not have error: %v", response.Error.Message)
	}

	if response.Result == nil {
		t.Fatal("Response should have result")
	}

	// Result is an InitializeResult struct
	result, ok := response.Result.(*InitializeResult)
	if !ok {
		t.Fatalf("Result should be an InitializeResult, got %T", response.Result)
	}

	if result.ProtocolVersion == "" {
		t.Error("Result should have protocolVersion")
	}

	if result.ServerInfo.Name == "" {
		t.Error("Result should have serverInfo.name")
	}
}

func TestToolsListMethod(t *testing.T) {
	server := newTestMCPServer(t)

	response := sendRequest(t, server, "tools/list", 1, nil)

	if response == nil {
		t.Fatal("Response should not be nil")
	}

	if response.Error != nil {
		t.Errorf("Should not have error: %v", response.Error.Message)
	}

	if response.Result == nil {
		t.Fatal("Response should have result")
	}

	// Check that result has tools - result is a map[string]interface{} containing tool definitions
	result, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Result should be a map, got %T", response.Result)
	}

	tools, ok := result["tools"]
	if !ok {
		t.Fatal("Result should have tools")
	}

	// Tools is a slice of Tool structs
	toolsList, ok := tools.([]Tool)
	if !ok {
		t.Fatalf("Tools should be []Tool, got %T", tools)
	}

	// Should have at least one tool
	if len(toolsList) == 0 {
		t.Error("Should have at least one tool")
	}

	// Check first tool has required fields
	if len(toolsList) > 0 {
		tool := toolsList[0]
		if tool.Name == "" {
			t.Error("Tool should have name")
		}
		if tool.Description == "" {
			t.Error("Tool should have description")
		}
		if tool.InputSchema == nil {
			t.Error("Tool should have inputSchema")
		}
	}
}

func TestUnknownMethod(t *testing.T) {
	server := newTestMCPServer(t)

	response := sendRequest(t, server, "unknown/method", 1, nil)

	if response == nil {
		t.Fatal("Response should not be nil")
	}

	if response.Error == nil {
		t.Error("Should have error for unknown method")
	}

	if response.Error != nil && response.Error.Code != MethodNotFound {
		t.Errorf("Expected MethodNotFound error code, got %d", response.Error.Code)
	}
}

func TestToolCallSearchSymbols(t *testing.T) {
	server := newTestMCPServer(t)

	params := map[string]interface{}{
		"name": "searchSymbols",
		"arguments": map[string]interface{}{
			"query": "test",
		},
	}

	response := sendRequest(t, server, "tools/call", 1, params)

	if response == nil {
		t.Fatal("Response should not be nil")
	}

	// May succeed with empty results or fail if no backends
	// Either is acceptable for this test
	if response.Error == nil {
		if response.Result == nil {
			t.Error("Response should have result if no error")
		}
	}
}

func TestToolCallWithMissingName(t *testing.T) {
	server := newTestMCPServer(t)

	params := map[string]interface{}{
		"arguments": map[string]interface{}{
			"query": "test",
		},
	}

	response := sendRequest(t, server, "tools/call", 1, params)

	if response == nil {
		t.Fatal("Response should not be nil")
	}

	if response.Error == nil {
		t.Error("Should have error for missing tool name")
	}
}

func TestToolCallUnknownTool(t *testing.T) {
	server := newTestMCPServer(t)

	params := map[string]interface{}{
		"name": "unknownTool",
		"arguments": map[string]interface{}{
			"query": "test",
		},
	}

	response := sendRequest(t, server, "tools/call", 1, params)

	if response == nil {
		t.Fatal("Response should not be nil")
	}

	if response.Error == nil {
		t.Error("Should have error for unknown tool")
	}
}

func TestMCPMessageTypes(t *testing.T) {
	// Test request detection
	request := &MCPMessage{
		Jsonrpc: "2.0",
		Id:      1,
		Method:  "test",
	}
	if !request.IsRequest() {
		t.Error("Should be detected as request")
	}
	if request.IsNotification() {
		t.Error("Should not be detected as notification")
	}
	if request.IsResponse() {
		t.Error("Should not be detected as response")
	}

	// Test notification detection
	notification := &MCPMessage{
		Jsonrpc: "2.0",
		Method:  "test",
	}
	if notification.IsRequest() {
		t.Error("Should not be detected as request")
	}
	if !notification.IsNotification() {
		t.Error("Should be detected as notification")
	}
	if notification.IsResponse() {
		t.Error("Should not be detected as response")
	}

	// Test response detection
	response := &MCPMessage{
		Jsonrpc: "2.0",
		Id:      1,
		Result:  "ok",
	}
	if response.IsRequest() {
		t.Error("Should not be detected as request")
	}
	if response.IsNotification() {
		t.Error("Should not be detected as notification")
	}
	if !response.IsResponse() {
		t.Error("Should be detected as response")
	}
}

func TestNewErrorMessage(t *testing.T) {
	msg := NewErrorMessage(1, InvalidParams, "Invalid parameters", nil)

	if msg.Jsonrpc != "2.0" {
		t.Error("Should have jsonrpc 2.0")
	}

	if msg.Id != 1 {
		t.Error("Should have id 1")
	}

	if msg.Error == nil {
		t.Fatal("Should have error")
	}

	if msg.Error.Code != InvalidParams {
		t.Error("Should have InvalidParams code")
	}

	if msg.Error.Message != "Invalid parameters" {
		t.Error("Should have correct message")
	}
}

func TestNewResultMessage(t *testing.T) {
	result := map[string]string{"status": "ok"}
	msg := NewResultMessage(1, result)

	if msg.Jsonrpc != "2.0" {
		t.Error("Should have jsonrpc 2.0")
	}

	if msg.Id != 1 {
		t.Error("Should have id 1")
	}

	if msg.Result == nil {
		t.Fatal("Should have result")
	}

	if msg.Error != nil {
		t.Error("Should not have error")
	}
}

func TestNewNotificationMessage(t *testing.T) {
	msg := NewNotificationMessage("test/event", map[string]string{"key": "value"})

	if msg.Jsonrpc != "2.0" {
		t.Error("Should have jsonrpc 2.0")
	}

	if msg.Id != nil {
		t.Error("Should not have id")
	}

	if msg.Method != "test/event" {
		t.Error("Should have correct method")
	}

	if msg.Params == nil {
		t.Error("Should have params")
	}
}
