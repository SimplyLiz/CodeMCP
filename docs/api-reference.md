# CKB HTTP API

REST API for Code Knowledge Backend (CKB) providing codebase comprehension capabilities.

## Quick Start

```bash
# Start the server (default: localhost:8080)
./ckb serve

# Start on custom port
./ckb serve --port 8081

# Start on custom host
./ckb serve --host 0.0.0.0 --port 8080
```

## API Endpoints

### System & Health

#### `GET /`
Root endpoint with API information and available endpoints.

```bash
curl http://localhost:8080/
```

#### `GET /health`
Simple liveness check for load balancers.

```bash
curl http://localhost:8080/health
```

Response:
```json
{
  "status": "healthy",
  "timestamp": "2024-12-16T12:00:00Z",
  "version": "0.1.0"
}
```

#### `GET /ready`
Readiness check that verifies backend availability.

```bash
curl http://localhost:8080/ready
```

Response:
```json
{
  "status": "ready",
  "timestamp": "2024-12-16T12:00:00Z",
  "backends": {
    "scip": true,
    "lsp": true,
    "git": true
  }
}
```

#### `GET /status`
Comprehensive system status including repository, backends, and cache.

```bash
curl http://localhost:8080/status
```

### Diagnostics

#### `GET /doctor`
Run diagnostic checks on all system components.

```bash
curl http://localhost:8080/doctor
```

Response:
```json
{
  "status": "healthy",
  "timestamp": "2024-12-16T12:00:00Z",
  "checks": [
    {"name": "config", "status": "pass", "message": "Configuration is valid"},
    {"name": "scip", "status": "pass", "message": "SCIP backend available"}
  ],
  "issues": [],
  "summary": {
    "total": 4,
    "passed": 4,
    "warnings": 0,
    "failed": 0
  }
}
```

#### `POST /doctor/fix`
Get a fix script for detected issues.

```bash
curl -X POST http://localhost:8080/doctor/fix
```

### Symbol Operations

#### `GET /symbol/:id`
Retrieve detailed information about a symbol.

```bash
curl http://localhost:8080/symbol/my-symbol-id
```

Response:
```json
{
  "id": "my-symbol-id",
  "name": "ExampleSymbol",
  "kind": "function",
  "location": {
    "file": "example.go",
    "line": 42
  },
  "module": "example"
}
```

#### `GET /search`
Search for symbols matching a query.

**Query Parameters:**
- `q` (required) - Search query
- `scope` - Module ID to search within
- `kinds` - Comma-separated list of symbol kinds
- `limit` - Maximum results (default: 50)
- `merge` - Merge strategy: "prefer-first" or "union"
- `repoStateMode` - Repository state: "head" or "full"
- `depth` - Search depth (default: 1)
- `includeExternal` - Include external symbols (true/false)
- `refresh` - Force refresh cache (true/false)

```bash
curl "http://localhost:8080/search?q=myFunction&limit=10&kinds=function,method"
```

Response:
```json
{
  "query": "myFunction",
  "results": [],
  "total": 0,
  "hasMore": false,
  "timestamp": "2024-12-16T12:00:00Z"
}
```

#### `GET /refs/:id`
Find all references to a symbol.

```bash
curl http://localhost:8080/refs/my-symbol-id
```

Response:
```json
{
  "symbolId": "my-symbol-id",
  "references": [],
  "total": 0,
  "timestamp": "2024-12-16T12:00:00Z"
}
```

### Analysis

#### `GET /architecture`
Get an overview of the codebase architecture.

```bash
curl http://localhost:8080/architecture
```

Response:
```json
{
  "timestamp": "2024-12-16T12:00:00Z",
  "modules": [],
  "dependencies": [],
  "metrics": {}
}
```

#### `GET /impact/:id`
Analyze the impact of changing a symbol.

```bash
curl http://localhost:8080/impact/my-symbol-id
```

Response:
```json
{
  "symbolId": "my-symbol-id",
  "timestamp": "2024-12-16T12:00:00Z",
  "impact": {},
  "affected": [],
  "risk": "low"
}
```

### Cache Operations

#### `POST /cache/warm`
Initiate cache warming for commonly accessed data.

```bash
curl -X POST http://localhost:8080/cache/warm
```

Response:
```json
{
  "status": "success",
  "message": "Cache warming initiated",
  "timestamp": "2024-12-16T12:00:00Z"
}
```

#### `POST /cache/clear`
Clear all cached data.

```bash
curl -X POST http://localhost:8080/cache/clear
```

Response:
```json
{
  "status": "success",
  "message": "Cache cleared",
  "timestamp": "2024-12-16T12:00:00Z"
}
```

### Documentation

#### `GET /openapi.json`
Get the OpenAPI 3.0 specification for the API.

```bash
curl http://localhost:8080/openapi.json
```

## Error Responses

All errors return a consistent JSON structure:

```json
{
  "error": "Symbol not found",
  "code": "SYMBOL_NOT_FOUND",
  "details": null,
  "suggestedFixes": [
    {
      "type": "run-command",
      "command": "ckb doctor",
      "safe": true,
      "description": "Check system configuration"
    }
  ],
  "drilldowns": []
}
```

### HTTP Status Codes

| Status | Meaning |
|--------|---------|
| 200 | Success |
| 400 | Bad Request - Invalid parameters |
| 404 | Not Found - Resource doesn't exist |
| 410 | Gone - Resource was deleted |
| 412 | Precondition Failed - Index stale |
| 413 | Payload Too Large - Budget exceeded |
| 422 | Unprocessable Entity - Validation error |
| 429 | Too Many Requests - Rate limited |
| 500 | Internal Server Error |
| 503 | Service Unavailable - Backend unavailable |
| 504 | Gateway Timeout |

## Request Headers

### Supported Headers
- `X-Request-ID` - Custom request ID (auto-generated if not provided)
- `Content-Type: application/json` - For POST requests

### Response Headers
- `Content-Type: application/json` - All responses are JSON
- `X-Request-ID` - Request ID for tracing
- `Access-Control-Allow-Origin: *` - CORS enabled for local dev

## Middleware

The API includes the following middleware (in order):
1. **CORS** - Enables cross-origin requests
2. **Request ID** - Generates unique request IDs
3. **Logging** - Logs all requests and responses
4. **Recovery** - Recovers from panics

## Testing

### Manual Testing

```bash
# Start server
./ckb serve --port 8081

# Test all endpoints
curl http://localhost:8081/health | jq .
curl http://localhost:8081/status | jq .
curl "http://localhost:8081/search?q=test" | jq .
curl -X POST http://localhost:8081/cache/clear | jq .
```

### Using jq for Pretty Output

```bash
# Install jq if not already installed
brew install jq  # macOS
apt-get install jq  # Linux

# Pretty print responses
curl -s http://localhost:8080/status | jq .
```

## Configuration

Server configuration via command-line flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | 8080 | Port to listen on |
| `--host` | localhost | Host to bind to |

## Graceful Shutdown

The server supports graceful shutdown via interrupt signals:

```bash
# Press Ctrl+C to stop the server
# Or send SIGTERM
kill -TERM <pid>
```

The server will:
1. Stop accepting new connections
2. Wait for active requests to complete (up to 10 seconds)
3. Shut down cleanly

## Logging

The API logs all requests and responses with:
- HTTP method and path
- Query parameters
- Status code
- Response time
- Request ID

Example log output:
```
2024-12-16T12:00:00Z [info] HTTP request | method=GET, path=/status, requestID=abc-123
2024-12-16T12:00:01Z [info] HTTP response | method=GET, path=/status, status=200, duration=100ms, requestID=abc-123
```

## Development

### Building

```bash
go build -o ckb ./cmd/ckb
```

### Running in Development

```bash
# Start with default settings
./ckb serve

# Start with custom port for development
./ckb serve --port 8081
```

### Testing with curl

```bash
# Save to file
curl http://localhost:8080/status > status.json

# Show headers
curl -i http://localhost:8080/health

# Show request/response with verbose
curl -v http://localhost:8080/ready
```

## Integration

### With Frontend Applications

```javascript
// Fetch status
fetch('http://localhost:8080/status')
  .then(res => res.json())
  .then(data => console.log(data));

// Search symbols
fetch('http://localhost:8080/search?q=myFunction&limit=10')
  .then(res => res.json())
  .then(data => console.log(data));
```

### With Other Tools

```bash
# HTTPie
http GET localhost:8080/status

# wget
wget -qO- http://localhost:8080/health

# Python
python -c "import requests; print(requests.get('http://localhost:8080/status').json())"
```

## Troubleshooting

### Server won't start

```bash
# Check if port is already in use
lsof -i :8080

# Use different port
./ckb serve --port 8081
```

### Connection refused

```bash
# Verify server is running
ps aux | grep ckb

# Check server logs for errors
./ckb serve 2>&1 | tee server.log
```

### Invalid JSON responses

```bash
# Validate JSON
curl http://localhost:8080/status | jq .

# Check response headers
curl -i http://localhost:8080/status
```

## Support

For issues or questions:
1. Check the OpenAPI spec: `GET /openapi.json`
2. Review server logs
3. Check the implementation documentation: `PHASE-4.2-IMPLEMENTATION.md`
