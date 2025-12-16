#!/bin/bash

# Simple test script - run server in separate terminal first with: ./ckb serve --port 8081

PORT=8081
BASE_URL="http://localhost:$PORT"

echo "Testing CKB HTTP API endpoints..."
echo "Make sure server is running: ./ckb serve --port $PORT"
echo ""

# Test endpoints with curl
echo "1. Testing GET / (root)..."
curl -s "$BASE_URL/" | jq .

echo ""
echo "2. Testing GET /health..."
curl -s "$BASE_URL/health" | jq .

echo ""
echo "3. Testing GET /ready..."
curl -s "$BASE_URL/ready" | jq .

echo ""
echo "4. Testing GET /status..."
curl -s "$BASE_URL/status" | jq .

echo ""
echo "5. Testing GET /doctor..."
curl -s "$BASE_URL/doctor" | jq .

echo ""
echo "6. Testing GET /symbol/test-id..."
curl -s "$BASE_URL/symbol/test-id" | jq .

echo ""
echo "7. Testing GET /search?q=test..."
curl -s "$BASE_URL/search?q=test" | jq .

echo ""
echo "8. Testing GET /refs/test-id..."
curl -s "$BASE_URL/refs/test-id" | jq .

echo ""
echo "9. Testing GET /architecture..."
curl -s "$BASE_URL/architecture" | jq .

echo ""
echo "10. Testing GET /impact/test-id..."
curl -s "$BASE_URL/impact/test-id" | jq .

echo ""
echo "11. Testing POST /doctor/fix..."
curl -s -X POST "$BASE_URL/doctor/fix" | jq .

echo ""
echo "12. Testing POST /cache/warm..."
curl -s -X POST "$BASE_URL/cache/warm" | jq .

echo ""
echo "13. Testing POST /cache/clear..."
curl -s -X POST "$BASE_URL/cache/clear" | jq .

echo ""
echo "14. Testing GET /openapi.json..."
curl -s "$BASE_URL/openapi.json" | jq '.openapi, .info.title'

echo ""
echo "All endpoints tested."
