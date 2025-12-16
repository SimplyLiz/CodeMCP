#!/usr/bin/env python3
"""
Test script for CKB MCP server
Sends JSON-RPC requests and validates responses
"""

import subprocess
import json
import sys

def send_request(proc, request_id, method, params=None):
    """Send a JSON-RPC request to the MCP server"""
    request = {
        "jsonrpc": "2.0",
        "id": request_id,
        "method": method,
    }
    if params is not None:
        request["params"] = params

    request_json = json.dumps(request) + "\n"
    print(f"Sending: {method}", file=sys.stderr)
    print(f"  Request: {request_json.strip()}", file=sys.stderr)

    proc.stdin.write(request_json.encode())
    proc.stdin.flush()

    # Read response
    response_line = proc.stdout.readline()
    print(f"  Response: {response_line.strip()}", file=sys.stderr)

    try:
        response = json.loads(response_line)
        return response
    except json.JSONDecodeError as e:
        print(f"  ERROR: Failed to parse response: {e}", file=sys.stderr)
        return None

def test_initialize(proc):
    """Test the initialize method"""
    print("\n=== Test 1: Initialize ===", file=sys.stderr)
    response = send_request(proc, 1, "initialize", {
        "protocolVersion": "2024-11-05",
        "capabilities": {},
        "clientInfo": {
            "name": "test-client",
            "version": "1.0.0"
        }
    })

    if response and "result" in response:
        result = response["result"]
        print(f"  ✓ Protocol Version: {result.get('protocolVersion')}", file=sys.stderr)
        print(f"  ✓ Server: {result.get('serverInfo', {}).get('name')} {result.get('serverInfo', {}).get('version')}", file=sys.stderr)
        return True
    else:
        print(f"  ✗ Failed: {response}", file=sys.stderr)
        return False

def test_list_tools(proc):
    """Test the tools/list method"""
    print("\n=== Test 2: List Tools ===", file=sys.stderr)
    response = send_request(proc, 2, "tools/list", {})

    if response and "result" in response:
        tools = response["result"].get("tools", [])
        print(f"  ✓ Found {len(tools)} tools:", file=sys.stderr)
        for tool in tools:
            print(f"    - {tool['name']}: {tool['description']}", file=sys.stderr)
        return True
    else:
        print(f"  ✗ Failed: {response}", file=sys.stderr)
        return False

def test_call_tool(proc):
    """Test the tools/call method"""
    print("\n=== Test 3: Call Tool (getStatus) ===", file=sys.stderr)
    response = send_request(proc, 3, "tools/call", {
        "name": "getStatus",
        "arguments": {}
    })

    if response and "result" in response:
        result = response["result"]
        print(f"  ✓ Tool executed successfully", file=sys.stderr)
        if "content" in result:
            print(f"  ✓ Response has content", file=sys.stderr)
        return True
    else:
        print(f"  ✗ Failed: {response}", file=sys.stderr)
        return False

def test_list_resources(proc):
    """Test the resources/list method"""
    print("\n=== Test 4: List Resources ===", file=sys.stderr)
    response = send_request(proc, 4, "resources/list", {})

    if response and "result" in response:
        result = response["result"]
        resources = result.get("resources", [])
        templates = result.get("resourceTemplates", [])
        print(f"  ✓ Found {len(resources)} static resources", file=sys.stderr)
        print(f"  ✓ Found {len(templates)} resource templates", file=sys.stderr)
        return True
    else:
        print(f"  ✗ Failed: {response}", file=sys.stderr)
        return False

def main():
    print("Testing CKB MCP Server", file=sys.stderr)
    print("======================", file=sys.stderr)

    # Start the MCP server
    proc = subprocess.Popen(
        ["./ckb", "mcp", "--stdio"],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE
    )

    try:
        results = []

        # Run tests
        results.append(("Initialize", test_initialize(proc)))
        results.append(("List Tools", test_list_tools(proc)))
        results.append(("Call Tool", test_call_tool(proc)))
        results.append(("List Resources", test_list_resources(proc)))

        # Summary
        print("\n=== Test Summary ===", file=sys.stderr)
        passed = sum(1 for _, result in results if result)
        total = len(results)

        for name, result in results:
            status = "✓ PASS" if result else "✗ FAIL"
            print(f"  {status}: {name}", file=sys.stderr)

        print(f"\nTotal: {passed}/{total} tests passed", file=sys.stderr)

        if passed == total:
            print("\n🎉 All tests passed!", file=sys.stderr)
            return 0
        else:
            print(f"\n❌ {total - passed} test(s) failed", file=sys.stderr)
            return 1

    finally:
        proc.terminate()
        proc.wait(timeout=5)

if __name__ == "__main__":
    sys.exit(main())
