#!/bin/bash

# Test script for CKB HTTP API
# Tests all endpoints to ensure they respond with proper JSON

set -e

PORT=8081
BASE_URL="http://localhost:$PORT"

echo "Starting CKB server on port $PORT..."
./ckb serve --port $PORT &
SERVER_PID=$!

# Give server time to start
sleep 2

echo "Testing endpoints..."

# Function to test endpoint
test_endpoint() {
    local method=$1
    local path=$2
    local desc=$3

    echo -n "Testing $method $path ($desc)... "
    response=$(curl -s -X $method "$BASE_URL$path" -w "\n%{http_code}")
    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)

    if [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
        echo "OK (HTTP $http_code)"
        # Validate JSON
        if echo "$body" | jq . > /dev/null 2>&1; then
            echo "  ✓ Valid JSON response"
        else
            echo "  ✗ Invalid JSON response"
            exit 1
        fi
    else
        echo "FAILED (HTTP $http_code)"
        echo "Response: $body"
        exit 1
    fi
}

# Test all GET endpoints
test_endpoint "GET" "/" "Root endpoint"
test_endpoint "GET" "/health" "Health check"
test_endpoint "GET" "/ready" "Readiness check"
test_endpoint "GET" "/status" "System status"
test_endpoint "GET" "/doctor" "Diagnostic checks"
test_endpoint "GET" "/symbol/test-symbol-id" "Get symbol"
test_endpoint "GET" "/search?q=test" "Search symbols"
test_endpoint "GET" "/refs/test-symbol-id" "Find references"
test_endpoint "GET" "/architecture" "Architecture overview"
test_endpoint "GET" "/impact/test-symbol-id" "Impact analysis"
test_endpoint "GET" "/openapi.json" "OpenAPI spec"

# Test POST endpoints
test_endpoint "POST" "/doctor/fix" "Get fix script"
test_endpoint "POST" "/cache/warm" "Warm cache"
test_endpoint "POST" "/cache/clear" "Clear cache"

echo ""
echo "All tests passed! ✓"

# Cleanup
echo "Stopping server..."
kill $SERVER_PID
wait $SERVER_PID 2>/dev/null || true

echo "Done."
