package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// BenchmarkWideResultSize measures output size and latency for wide-result tools.
// Use with benchstat for before/after comparison when implementing frontierMode:
//
//	go test -bench=BenchmarkWideResult ./internal/mcp/... -count=5 > before.txt
//	# implement frontierMode
//	go test -bench=BenchmarkWideResult ./internal/mcp/... -count=5 > after.txt
//	benchstat before.txt after.txt
//
// Latency is reported via b.ReportMetric but NOT used as a CI gate (too flaky).
// Track latency trends via benchstat, not pass/fail tests.
func BenchmarkWideResultSize(b *testing.B) {
	server := newTestMCPServerWithIndex(b)
	if server == nil {
		b.Skip("Skipping: no SCIP index available (run 'ckb index' first)")
	}

	// Find a symbol ID to use for tests
	symbolID := findTestSymbolID(b, server)
	if symbolID == "" {
		b.Skip("Could not find a test symbol")
	}

	scenarios := []struct {
		name   string
		tool   string
		args   map[string]interface{}
	}{
		{
			name: "getCallGraph_depth1",
			tool: "getCallGraph",
			args: map[string]interface{}{
				"symbolId":  symbolID,
				"direction": "both",
				"depth":     1,
			},
		},
		{
			name: "getCallGraph_depth2",
			tool: "getCallGraph",
			args: map[string]interface{}{
				"symbolId":  symbolID,
				"direction": "both",
				"depth":     2,
			},
		},
		{
			name: "findReferences_limit50",
			tool: "findReferences",
			args: map[string]interface{}{
				"symbolId": symbolID,
				"limit":    50,
			},
		},
		{
			name: "findReferences_limit100",
			tool: "findReferences",
			args: map[string]interface{}{
				"symbolId": symbolID,
				"limit":    100,
			},
		},
		{
			name: "analyzeImpact_depth2",
			tool: "analyzeImpact",
			args: map[string]interface{}{
				"symbolId": symbolID,
				"depth":    2,
			},
		},
		{
			name: "getHotspots_limit20",
			tool: "getHotspots",
			args: map[string]interface{}{
				"limit": 20,
			},
		},
		{
			name: "getHotspots_limit50",
			tool: "getHotspots",
			args: map[string]interface{}{
				"limit": 50,
			},
		},
		{
			name: "searchSymbols_limit20",
			tool: "searchSymbols",
			args: map[string]interface{}{
				"query": "Engine",
				"limit": 20,
			},
		},
		{
			name: "searchSymbols_limit50",
			tool: "searchSymbols",
			args: map[string]interface{}{
				"query": "Engine",
				"limit": 50,
			},
		},
		{
			name: "getArchitecture_depth2",
			tool: "getArchitecture",
			args: map[string]interface{}{
				"depth": 2,
			},
		},
	}

	for _, sc := range scenarios {
		b.Run(sc.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			var totalBytes int64
			var totalLatencyNs int64
			var successCount int

			for i := 0; i < b.N; i++ {
				start := time.Now()
				resp := sendRequestBench(b, server, "tools/call", i, map[string]interface{}{
					"name":      sc.tool,
					"arguments": sc.args,
				})
				elapsed := time.Since(start)

				if resp.Error == nil {
					data, _ := json.Marshal(resp.Result)
					totalBytes += int64(len(data))
					totalLatencyNs += elapsed.Nanoseconds()
					successCount++
				}
			}

			if successCount > 0 {
				avgBytes := float64(totalBytes) / float64(successCount)
				avgLatencyMs := float64(totalLatencyNs) / float64(successCount) / 1e6
				b.ReportMetric(avgBytes, "bytes/op")
				b.ReportMetric(avgBytes/4, "est_tokens/op")
				b.ReportMetric(avgLatencyMs, "latency_ms/op")
			}
		})
	}
}

// BenchmarkWideResultWithFrontier compares normal vs frontier mode.
// This benchmark is for use after frontierMode is implemented.
func BenchmarkWideResultWithFrontier(b *testing.B) {
	server := newTestMCPServerWithIndex(b)
	if server == nil {
		b.Skip("Skipping: no SCIP index available")
	}

	symbolID := findTestSymbolID(b, server)
	if symbolID == "" {
		b.Skip("Could not find a test symbol")
	}

	// Test both normal and frontier mode (when implemented)
	modes := []struct {
		name         string
		frontierMode bool
		k            int
	}{
		{"normal", false, 0},
		// Uncomment when frontierMode is implemented:
		// {"frontier_k8", true, 8},
		// {"frontier_k12", true, 12},
	}

	for _, mode := range modes {
		b.Run("getCallGraph_"+mode.name, func(b *testing.B) {
			args := map[string]interface{}{
				"symbolId":  symbolID,
				"direction": "both",
				"depth":     2,
			}
			if mode.frontierMode {
				args["frontierMode"] = true
				args["k"] = mode.k
			}

			b.ResetTimer()
			var totalBytes int64
			var totalLatencyNs int64
			var successCount int

			for i := 0; i < b.N; i++ {
				start := time.Now()
				resp := sendRequestBench(b, server, "tools/call", i, map[string]interface{}{
					"name":      "getCallGraph",
					"arguments": args,
				})
				elapsed := time.Since(start)

				if resp.Error == nil {
					data, _ := json.Marshal(resp.Result)
					totalBytes += int64(len(data))
					totalLatencyNs += elapsed.Nanoseconds()
					successCount++
				}
			}

			if successCount > 0 {
				avgBytes := float64(totalBytes) / float64(successCount)
				avgLatencyMs := float64(totalLatencyNs) / float64(successCount) / 1e6
				b.ReportMetric(avgBytes, "bytes/op")
				b.ReportMetric(avgBytes/4, "est_tokens/op")
				b.ReportMetric(avgLatencyMs, "latency_ms/op")
			}
		})
	}
}

// findTestSymbolID finds a symbol ID to use for benchmarks.
// Uses the MCP protocol to ensure consistent response format.
func findTestSymbolID(tb testing.TB, server *MCPServer) string {
	tb.Helper()

	// Create the request
	request := MCPMessage{
		Jsonrpc: "2.0",
		Id:      999,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name": "searchSymbols",
			"arguments": map[string]interface{}{
				"query": "Engine",
				"limit": 1,
			},
		},
	}

	requestBytes, _ := json.Marshal(request)
	requestBytes = append(requestBytes, '\n')

	stdin := bytes.NewReader(requestBytes)
	stdout := &bytes.Buffer{}

	server.SetStdin(stdin)
	server.SetStdout(stdout)

	msg, err := server.readMessage()
	if err != nil && err != io.EOF {
		return ""
	}
	if msg == nil {
		return ""
	}

	response := server.handleMessage(msg)
	if response == nil || response.Error != nil {
		return ""
	}

	// Write to get proper format, then parse
	server.writeMessage(response)

	// Parse the response from stdout
	stdoutStr := stdout.String()
	if stdoutStr == "" {
		return ""
	}

	lines := strings.Split(stdoutStr, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			continue
		}

		result, ok := parsed["result"].(map[string]interface{})
		if !ok {
			continue
		}

		content, ok := result["content"].([]interface{})
		if !ok || len(content) == 0 {
			continue
		}

		firstContent, ok := content[0].(map[string]interface{})
		if !ok {
			continue
		}

		text, ok := firstContent["text"].(string)
		if !ok {
			continue
		}

		var envelope map[string]interface{}
		if err := json.Unmarshal([]byte(text), &envelope); err != nil {
			continue
		}

		// The envelope has {"schemaVersion": "1.0", "data": {"symbols": [...]}}
		data, ok := envelope["data"].(map[string]interface{})
		if !ok {
			continue
		}

		symbols, ok := data["symbols"].([]interface{})
		if !ok || len(symbols) == 0 {
			continue
		}

		firstSymbol, ok := symbols[0].(map[string]interface{})
		if !ok {
			continue
		}

		// Symbol ID is in "stableId" field
		symbolID, _ := firstSymbol["stableId"].(string)
		return symbolID
	}

	return ""
}

// sendRequestBench is like sendRequest but for benchmarks.
func sendRequestBench(tb testing.TB, server *MCPServer, method string, id int, params interface{}) *MCPMessage {
	tb.Helper()

	// Use the handler directly instead of stdin/stdout for benchmarks
	handler := server.tools[params.(map[string]interface{})["name"].(string)]
	if handler == nil {
		return &MCPMessage{Error: &MCPError{Code: -1, Message: "tool not found"}}
	}

	args, ok := params.(map[string]interface{})["arguments"].(map[string]interface{})
	if !ok {
		return &MCPMessage{Error: &MCPError{Code: -1, Message: "invalid arguments"}}
	}
	result, err := handler(args)
	if err != nil {
		return &MCPMessage{Error: &MCPError{Code: -1, Message: err.Error()}}
	}

	return &MCPMessage{Result: result}
}
