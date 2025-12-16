#!/bin/bash
# Test script for MCP server

echo "Testing MCP server..."
echo ""

# Test 1: Initialize
echo "Test 1: Initialize"
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}}}' | ./ckb mcp
echo ""

# Test 2: List tools
echo "Test 2: List tools"
echo '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | ./ckb mcp
echo ""

# Test 3: Call getStatus tool
echo "Test 3: Call getStatus tool"
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"getStatus","arguments":{}}}' | ./ckb mcp
echo ""

# Test 4: List resources
echo "Test 4: List resources"
echo '{"jsonrpc":"2.0","id":4,"method":"resources/list","params":{}}' | ./ckb mcp
echo ""

echo "Tests complete!"
