# CKB 8.2 Streaming Design

## Overview

This document outlines the streaming architecture for CKB 8.2, enabling efficient transfer of large results to AI agents without overwhelming context windows or causing timeouts.

## Problem Statement

Current limitations:
1. **Large result sets** - Some queries return hundreds of symbols, references, or modules
2. **Token budget pressure** - AI agents have limited context windows
3. **Timeouts** - Large aggregation operations may timeout on slow connections
4. **Memory pressure** - Buffering entire results before sending

## Proposed Solution: Server-Sent Events (SSE)

SSE provides a simple, HTTP-native streaming protocol that:
- Works through proxies and load balancers
- Supports automatic reconnection
- Has native browser/client support
- Is simpler than WebSockets for unidirectional data

### Protocol Design

#### Request Format

Streaming is opt-in via the `stream` parameter:

```json
{
  "tool": "findReferences",
  "params": {
    "symbolId": "ckb:myrepo:sym:abc123",
    "limit": 500,
    "stream": true
  }
}
```

#### Response Format

SSE events follow this structure:

```
event: meta
data: {"total": 523, "backends": ["scip", "git"]}

event: chunk
data: {"references": [{"location": {...}, "kind": "call"}, ...]}

event: chunk
data: {"references": [{"location": {...}, "kind": "import"}, ...]}

event: done
data: {"elapsed": "1.2s", "truncated": false}
```

### Event Types

| Event | Purpose | Payload |
|-------|---------|---------|
| `meta` | Initial metadata | Total count, backends, confidence |
| `chunk` | Result batch | Array of items (10-50 per chunk) |
| `progress` | Progress update | Percentage, current item |
| `warning` | Non-fatal issue | Warning message |
| `done` | Stream complete | Summary, timing |
| `error` | Fatal error | Error details, remediation |

### Chunking Strategy

Results are chunked by:
1. **Item count** - 20-50 items per chunk (configurable)
2. **Byte size** - Chunks stay under 16KB for efficient parsing
3. **Logical groups** - Group by file, module, or backend when possible

### Backpressure Handling

If the client can't keep up:
1. Server buffers up to 100 chunks
2. If buffer fills, pause backend queries
3. Resume when client acknowledges (via heartbeat)

### Heartbeat Mechanism

To detect stale connections:
```
event: heartbeat
data: {"seq": 42}
```

Sent every 15 seconds if no other events.

## Implementation Plan

### Phase 1: Infrastructure (8.2.0)

1. **SSE Transport Layer**
   - `internal/streaming/sse.go` - SSE writer
   - `internal/streaming/chunker.go` - Result chunking
   - Content-type negotiation in MCP server

2. **Streamable Tools**
   - `findReferences` - High cardinality, often exceeds limits
   - `searchSymbols` - Large search results
   - `getArchitecture` - Deep dependency graphs

### Phase 2: Compound Operations (8.2.1)

Compound tools from 8.1 naturally benefit from streaming:
- `explore` - Progressive revelation of area details
- `understand` - Symbol info, then references, then call graph
- `prepareChange` - Impact analysis in stages

### Phase 3: Client Integration (8.2.2)

1. **MCP SDK updates** - Streaming-aware tool responses
2. **Claude Code integration** - Progressive result display
3. **Cursor/VS Code** - Incremental UI updates

## API Changes

### New Envelope Fields

```go
type Meta struct {
    // ... existing fields ...
    Streaming *StreamingInfo `json:"streaming,omitempty"`
}

type StreamingInfo struct {
    Enabled     bool   `json:"enabled"`
    ChunkSize   int    `json:"chunkSize"`
    TotalChunks int    `json:"totalChunks,omitempty"` // if known
}
```

### HTTP Headers

```
Accept: text/event-stream
X-CKB-Stream-Chunk-Size: 50
X-CKB-Stream-Max-Chunks: 100
```

### MCP Protocol Extension

For MCP (JSON-RPC), streaming uses a notification pattern:

```json
// Initial response with stream ID
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "streamId": "abc123",
    "streaming": true,
    "meta": {"total": 500}
  }
}

// Subsequent notifications (no id = notification)
{
  "jsonrpc": "2.0",
  "method": "ckb/streamChunk",
  "params": {
    "streamId": "abc123",
    "sequence": 1,
    "chunk": {"references": [...]}
  }
}

// Completion notification
{
  "jsonrpc": "2.0",
  "method": "ckb/streamComplete",
  "params": {
    "streamId": "abc123",
    "summary": {"elapsed": "1.2s"}
  }
}
```

## Compatibility

### Non-streaming Fallback

If `stream: true` is not supported or SSE unavailable:
1. Fall back to buffered response
2. Apply truncation as normal
3. Include warning about truncated results

### Version Detection

Clients can query streaming support:
```json
{
  "tool": "getStatus",
  "params": {}
}
// Response includes:
{
  "capabilities": {
    "streaming": true,
    "streamingTools": ["findReferences", "searchSymbols", ...]
  }
}
```

## Performance Considerations

### Memory Usage

- Streaming reduces peak memory by 10-50x for large results
- Chunks are GC'd as they're sent
- Backend queries can be lazy-evaluated

### Latency

- Time-to-first-chunk: <100ms target
- Chunk processing: ~10ms per chunk
- Heartbeat overhead: Negligible

### Throughput

- Target: 1000 items/second sustained
- Burst: 5000 items/second
- Backpressure at 10000 queued items

## Testing Strategy

1. **Unit tests** - Chunker logic, SSE encoding
2. **Integration tests** - Full stream lifecycle
3. **Load tests** - 10K+ items, slow clients
4. **Chaos tests** - Disconnects, reconnects

## Future Considerations

### Bidirectional Streaming

For interactive exploration:
- Client sends refinement queries mid-stream
- Server adjusts remaining results

### Compression

For large streams over slow connections:
- gzip compression per-chunk
- Dictionary-based compression for repeated structures

## Timeline

| Milestone | Target | Scope |
|-----------|--------|-------|
| 8.2.0-alpha | TBD | SSE infrastructure + findReferences |
| 8.2.0-beta | TBD | searchSymbols + getArchitecture |
| 8.2.0 | TBD | Full streaming support |
| 8.2.1 | TBD | Compound operation streaming |
| 8.2.2 | TBD | Client SDK integration |

## References

- [Server-Sent Events (SSE) Spec](https://html.spec.whatwg.org/multipage/server-sent-events.html)
- [MCP Specification](https://modelcontextprotocol.io/docs/specification)
- [CKB 8.0 Plan](../CLAUDE.md)
