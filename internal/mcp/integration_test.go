package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"

	"ckb/internal/backends/scip"
	"ckb/internal/config"
	"ckb/internal/query"
	"ckb/internal/storage"
	"ckb/internal/version"
)

// TestMultipleMessagesInSession tests that the MCP server can handle multiple
// messages in a single session. This catches the scanner bug where a new scanner
// was created for each message, losing buffered data.
func TestMultipleMessagesInSession(t *testing.T) {
	server := newTestMCPServer(t)

	// Create multiple messages
	messages := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"getStatus","arguments":{}}}`,
	}

	// Join messages with newlines
	input := strings.Join(messages, "\n") + "\n"

	// Set up stdin/stdout buffers
	stdin := strings.NewReader(input)
	stdout := &bytes.Buffer{}

	server.SetStdin(stdin)
	server.SetStdout(stdout)

	// Process all messages
	responses := []map[string]interface{}{}
	for i := 0; i < len(messages); i++ {
		msg, err := server.readMessage()
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("Failed to read message %d: %v", i+1, err)
		}

		response := server.handleMessage(msg)
		if response != nil {
			if err := server.writeMessage(response); err != nil {
				t.Fatalf("Failed to write response %d: %v", i+1, err)
			}

			// Parse the response
			var parsed map[string]interface{}
			lines := strings.Split(stdout.String(), "\n")
			for _, line := range lines {
				if line != "" {
					if err := json.Unmarshal([]byte(line), &parsed); err == nil {
						if id, ok := parsed["id"]; ok && id == float64(i+1) {
							responses = append(responses, parsed)
						}
					}
				}
			}
		}
	}

	// Verify we got all responses
	if len(responses) < 3 {
		t.Errorf("Expected 3 responses, got %d", len(responses))
	}

	// Verify each response has no error
	for i, resp := range responses {
		if _, hasError := resp["error"]; hasError {
			t.Errorf("Response %d has error: %v", i+1, resp["error"])
		}
	}
}

// TestScannerReusedAcrossMessages verifies the scanner is reused properly
func TestScannerReusedAcrossMessages(t *testing.T) {
	server := newTestMCPServer(t)

	// Two messages
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}
`

	stdin := strings.NewReader(input)
	stdout := &bytes.Buffer{}

	server.SetStdin(stdin)
	server.SetStdout(stdout)

	// Read first message
	msg1, err := server.readMessage()
	if err != nil {
		t.Fatalf("Failed to read first message: %v", err)
	}
	if msg1.Method != "initialize" {
		t.Errorf("Expected initialize method, got %s", msg1.Method)
	}

	// Read second message - this would fail with the old scanner bug
	msg2, err := server.readMessage()
	if err != nil {
		t.Fatalf("Failed to read second message: %v (scanner bug?)", err)
	}
	if msg2.Method != "tools/list" {
		t.Errorf("Expected tools/list method, got %s", msg2.Method)
	}
}

// newTestMCPServerWithSCIP creates an MCP server with SCIP backend for integration testing
func newTestMCPServerWithSCIP(t *testing.T, scipIndexPath string) *MCPServer {
	t.Helper()

	cfg := &config.Config{
		Version:  5,
		RepoRoot: ".",
		Backends: config.BackendsConfig{
			Git: config.GitConfig{
				Enabled: false,
			},
			Scip: config.ScipConfig{
				Enabled:   true,
				IndexPath: scipIndexPath,
			},
			Lsp: config.LspConfig{
				Enabled: false,
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := storage.Open(":memory:", logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	engine, err := query.NewEngine(".", db, logger, cfg)
	if err != nil {
		t.Fatalf("Failed to create query engine: %v", err)
	}

	server := NewMCPServer(version.Version, engine, logger)
	return server
}

// TestSearchThenGetSymbol_EndToEnd tests the full workflow of searching for a symbol
// and then getting its details. This catches the identity resolution bug where symbols
// found by search couldn't be looked up by getSymbol.
func TestSearchThenGetSymbol_EndToEnd(t *testing.T) {
	// Skip if SCIP index doesn't exist
	if !fileExists("../../.scip/index.scip") {
		t.Skip("Skipping: SCIP index not found at ../../.scip/index.scip")
	}

	// Load SCIP index to get a valid symbol ID
	index, err := scip.LoadSCIPIndex("../../.scip/index.scip")
	if err != nil {
		t.Skipf("Skipping: Failed to load SCIP index: %v", err)
	}

	// Find a symbol to test with
	var testSymbolId string
	for symbolId := range index.Symbols {
		if strings.Contains(symbolId, "Engine") {
			testSymbolId = symbolId
			break
		}
	}

	if testSymbolId == "" {
		t.Skip("Skipping: No suitable test symbol found")
	}

	server := newTestMCPServerWithSCIP(t, "../../.scip/index.scip")

	// Build getSymbol request
	getSymbolReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "getSymbol",
			"arguments": map[string]interface{}{
				"symbolId": testSymbolId,
			},
		},
	}

	reqBytes, _ := json.Marshal(getSymbolReq)
	reqBytes = append(reqBytes, '\n')

	stdin := bytes.NewReader(reqBytes)
	stdout := &bytes.Buffer{}

	server.SetStdin(stdin)
	server.SetStdout(stdout)

	msg, err := server.readMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	response := server.handleMessage(msg)
	if response == nil {
		t.Fatal("Response should not be nil")
	}

	// The response should not have an error
	if response.Error != nil {
		t.Errorf("getSymbol should not fail for valid SCIP ID: %v", response.Error.Message)
		t.Errorf("Symbol ID was: %s", testSymbolId)
	}

	// The response should have content with symbol info
	if response.Result != nil {
		result, ok := response.Result.(map[string]interface{})
		if ok {
			content, hasContent := result["content"]
			if !hasContent {
				t.Error("Response should have content")
			} else {
				// Content can be []map[string]interface{} or []interface{}
				switch c := content.(type) {
				case []map[string]interface{}:
					if len(c) == 0 {
						t.Error("Content should be non-empty array")
					}
				case []interface{}:
					if len(c) == 0 {
						t.Error("Content should be non-empty array")
					}
				default:
					t.Errorf("Unexpected content type: %T", content)
				}
			}
		}
	}
}

// TestExplainSymbol_WithSCIPFallback tests that explainSymbol works with raw SCIP IDs
func TestExplainSymbol_WithSCIPFallback(t *testing.T) {
	if !fileExists("../../.scip/index.scip") {
		t.Skip("Skipping: SCIP index not found")
	}

	index, err := scip.LoadSCIPIndex("../../.scip/index.scip")
	if err != nil {
		t.Skipf("Skipping: Failed to load SCIP index: %v", err)
	}

	var testSymbolId string
	for symbolId := range index.Symbols {
		if strings.Contains(symbolId, "Engine#") && !strings.Contains(symbolId, "()") {
			testSymbolId = symbolId
			break
		}
	}

	if testSymbolId == "" {
		t.Skip("Skipping: No suitable test symbol found")
	}

	server := newTestMCPServerWithSCIP(t, "../../.scip/index.scip")

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "explainSymbol",
			"arguments": map[string]interface{}{
				"symbolId": testSymbolId,
			},
		},
	}

	reqBytes, _ := json.Marshal(req)
	reqBytes = append(reqBytes, '\n')

	stdin := bytes.NewReader(reqBytes)
	stdout := &bytes.Buffer{}

	server.SetStdin(stdin)
	server.SetStdout(stdout)

	msg, err := server.readMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	response := server.handleMessage(msg)
	if response == nil {
		t.Fatal("Response should not be nil")
	}

	if response.Error != nil {
		t.Errorf("explainSymbol should work with raw SCIP IDs: %v", response.Error.Message)
	}
}

// TestGetCallGraph_WithSCIPFallback tests that getCallGraph works with raw SCIP IDs
func TestGetCallGraph_WithSCIPFallback(t *testing.T) {
	if !fileExists("../../.scip/index.scip") {
		t.Skip("Skipping: SCIP index not found")
	}

	index, err := scip.LoadSCIPIndex("../../.scip/index.scip")
	if err != nil {
		t.Skipf("Skipping: Failed to load SCIP index: %v", err)
	}

	// Find a function symbol (more likely to have call graph data)
	var testSymbolId string
	for symbolId := range index.Symbols {
		if strings.Contains(symbolId, "SearchSymbols()") {
			testSymbolId = symbolId
			break
		}
	}

	if testSymbolId == "" {
		t.Skip("Skipping: No suitable test symbol found")
	}

	server := newTestMCPServerWithSCIP(t, "../../.scip/index.scip")

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "getCallGraph",
			"arguments": map[string]interface{}{
				"symbolId":  testSymbolId,
				"direction": "both",
				"depth":     1,
			},
		},
	}

	reqBytes, _ := json.Marshal(req)
	reqBytes = append(reqBytes, '\n')

	stdin := bytes.NewReader(reqBytes)
	stdout := &bytes.Buffer{}

	server.SetStdin(stdin)
	server.SetStdout(stdout)

	msg, err := server.readMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	response := server.handleMessage(msg)
	if response == nil {
		t.Fatal("Response should not be nil")
	}

	if response.Error != nil {
		t.Errorf("getCallGraph should work with raw SCIP IDs: %v", response.Error.Message)
	}
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
