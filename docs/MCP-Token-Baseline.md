# MCP Token Baseline Metrics

Captured: 2024-12-23 (v7.4 presets implementation)

## Bottleneck #1: Tool Discovery Overhead (tools/list)

### Before Optimization (v7.3)

All 74 tools exposed to every session:

| Metric | Value |
|--------|-------|
| Tool count | 74 |
| JSON size | 35,386 bytes (34.6 KB) |
| Estimated tokens | ~8,846 |

### After Optimization (v7.4)

| Preset | Tools | Bytes | Tokens | vs Baseline |
|--------|------:|------:|-------:|------------:|
| **core** (default) | 14 | 6,127 | ~1,531 | **-83%** |
| review | 19 | 9,177 | ~2,294 | -74% |
| refactor | 19 | 8,864 | ~2,216 | -75% |
| federation | 28 | 12,488 | ~3,122 | -65% |
| docs | 20 | 8,375 | ~2,093 | -77% |
| ops | 24 | 9,207 | ~2,301 | -74% |
| full | 76 | 36,172 | ~9,043 | 0% (new baseline) |

**Result: Default (core) saves ~7,500 tokens per session (83% reduction)**

---

## Bottleneck #2: Wide-Result Tool Output

Tool responses that return large result sets:

| Tool | Bytes | Tokens | Notes |
|------|------:|-------:|-------|
| getStatus | 993 | ~248 | Baseline (small) |
| getArchitecture | 558 | ~139 | Baseline (small) |
| getHotspots | 8,860 | ~2,215 | Moderate |
| searchSymbols (50 results) | 24,862 | ~6,215 | Wide result |

Wide-result tools tracked via `getWideResultMetrics`:
- searchSymbols
- findReferences
- getCallGraph
- analyzeImpact

---

## Measurement Commands

```bash
# Tool discovery (tools/list) metrics
go test -v -run TestTokenMetrics ./internal/mcp/

# Wide-result tool output metrics
go test -v -run TestWideResultMetricsOutput ./internal/mcp/

# All metrics
go test -v -run "TestTokenMetrics|TestWideResultMetricsOutput" ./internal/mcp/
```

Token estimate: `bytes / 4` (conservative for structured JSON)
