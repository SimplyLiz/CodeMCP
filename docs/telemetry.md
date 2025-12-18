# CKB Telemetry Integration Guide

## Overview

CKB v6.4 introduces **runtime telemetry integration**, enabling confident answers to "is this code actually used?" — the question static analysis alone cannot reliably answer at scale.

This guide covers:
- [OTEL Collector Configuration](#otel-collector-configuration)
- [Service Map Configuration](#service-map-configuration)
- [Coverage Requirements](#coverage-requirements)
- [Dead Code Detection](#dead-code-detection)
- [Migration Guide](#migration-guide)

---

## OTEL Collector Configuration

CKB accepts telemetry data via two endpoints:

| Endpoint | Format | Use Case |
|----------|--------|----------|
| `POST /v1/metrics` | OTLP JSON | Standard OpenTelemetry pipeline |
| `POST /api/v1/ingest/json` | Simplified JSON | Testing/development |

### Standard OTLP Pipeline

Configure your OpenTelemetry Collector to export metrics to CKB:

```yaml
# otel-collector-config.yaml
exporters:
  otlphttp/ckb:
    endpoint: "http://localhost:9120"
    tls:
      insecure: true

service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [otlphttp/ckb]
```

### Required Metric: `calls`

CKB looks for a counter metric named `calls` with these attributes:

| Attribute | Required | Description |
|-----------|----------|-------------|
| `code.function` | Yes | Function name |
| `code.filepath` | Recommended | Source file path |
| `code.namespace` | Recommended | Package/namespace |
| `code.lineno` | Optional | Line number (enables exact matching) |

### Resource Attributes

| Attribute | Required | Description |
|-----------|----------|-------------|
| `service.name` | Yes | Service identifier for repo mapping |
| `service.version` | Optional | Service version for trend analysis |

### Instrumentation Example (Go)

```go
import (
    "go.opentelemetry.io/otel/metric"
)

var callsCounter metric.Int64Counter

func init() {
    callsCounter, _ = meter.Int64Counter("calls",
        metric.WithDescription("Function call count"),
    )
}

func HandleRequest(ctx context.Context) {
    callsCounter.Add(ctx, 1,
        metric.WithAttributes(
            attribute.String("code.function", "HandleRequest"),
            attribute.String("code.filepath", "internal/api/handler.go"),
            attribute.String("code.namespace", "api"),
            attribute.Int("code.lineno", 42),
        ),
    )
    // ... function implementation
}
```

### Simplified JSON Format (Development)

For testing without full OTEL setup:

```bash
curl -X POST http://localhost:9120/api/v1/ingest/json \
  -H "Content-Type: application/json" \
  -d '{
    "calls": [
      {
        "service_name": "api-gateway",
        "function_name": "HandleRequest",
        "file_path": "internal/api/handler.go",
        "namespace": "api",
        "call_count": 1500,
        "error_count": 3
      }
    ]
  }'
```

---

## Service Map Configuration

CKB must map `service.name` from telemetry to repository IDs. Configure this in `.ckb/config.json`:

```json
{
  "telemetry": {
    "enabled": true,
    "service_map": {
      "api-gateway": "repo-api",
      "user-service": "repo-users",
      "payment-service": "repo-payments"
    },
    "service_patterns": [
      {
        "pattern": "^order-.*$",
        "repo": "repo-orders"
      },
      {
        "pattern": "^inventory-.*$",
        "repo": "repo-inventory"
      }
    ]
  }
}
```

### Resolution Order

1. **Exact match** in `service_map`
2. **Pattern match** in `service_patterns` (first match wins)
3. **Payload override** via `ckb_repo_id` attribute in telemetry
4. **Unmapped** — logged for review

### Testing Service Mapping

```bash
# Test how a service name resolves
ckb telemetry test-map "payment-service"
# Output: payment-service → repo-payments (exact match)

# View unmapped services
ckb telemetry unmapped
```

### Full Telemetry Configuration

```json
{
  "telemetry": {
    "enabled": true,
    "service_map": {},
    "service_patterns": [],
    "aggregation": {
      "bucket_size": "weekly",
      "retention_days": 365,
      "min_calls_to_store": 1
    },
    "dead_code": {
      "enabled": true,
      "min_observation_days": 30,
      "exclude_patterns": ["**/test/**", "**/migrations/**"],
      "exclude_functions": ["*Migration*", "Test*", "*Backup*"]
    },
    "privacy": {
      "redact_caller_names": false,
      "log_unmatched_events": true
    }
  }
}
```

---

## Coverage Requirements

CKB computes telemetry coverage to gate features and communicate confidence:

### Coverage Components

| Component | Weight | What It Measures |
|-----------|--------|------------------|
| Attribute Coverage | 30% | % of events with file_path, namespace, line_number |
| Match Coverage | 50% | % of events matched to symbols (exact + strong) |
| Service Coverage | 20% | % of repos with telemetry data |

### Coverage Levels

| Level | Score | Feature Access |
|-------|-------|----------------|
| High | ≥ 0.80 | Full: dead code, impact enrichment, usage display |
| Medium | ≥ 0.60 | Partial: usage display, impact enrichment (with warnings) |
| Low | ≥ 0.40 | Limited: usage display only (with caveats) |
| Insufficient | < 0.40 | None: telemetry features disabled |

### Match Quality Levels

| Quality | Confidence | Criteria |
|---------|------------|----------|
| Exact | 0.95 | file_path + function_name + line_number |
| Strong | 0.85 | file_path + function_name |
| Weak | 0.60 | namespace + function_name (no file) |
| Unmatched | — | No symbol match found |

### Feature Gating by Match Quality

| Feature | Exact | Strong | Weak |
|---------|-------|--------|------|
| Dead code candidates | Yes | Yes | No |
| Usage display | Yes | Yes | Cautioned |
| Impact enrichment | Yes | Yes | No |

### Checking Coverage

```bash
# View current telemetry status and coverage
ckb telemetry status

# Example output:
# Telemetry: enabled
# Last sync: 2024-12-18T10:30:00Z
#
# Coverage:
#   Attribute: 85% (file_path: 92%, namespace: 78%, line_number: 45%)
#   Match: 72% (exact: 45%, strong: 27%, weak: 8%, unmatched: 20%)
#   Service: 80% (8/10 repos reporting)
#   Overall: 76% (medium)
#
# Unmapped services: 2
#   - legacy-batch-processor
#   - internal-cron-runner
```

---

## Dead Code Detection

CKB identifies **dead code candidates** — symbols that have static references but zero observed runtime calls.

### Algorithm

1. Query all symbols in repository
2. Apply exclusion filters (test files, migrations, etc.)
3. For each symbol with exact/strong telemetry match:
   - Check if call_count = 0 over observation window
   - Compute confidence based on coverage, refs, observation time
4. Return candidates above minimum confidence threshold

### Confidence Scoring

Base confidence by match quality:
- Exact match: 0.90
- Strong match: 0.80

Adjustments:
- Coverage level (high: +0, medium: -0.05, low: N/A)
- Static reference count (many refs: -0.05)
- Observation window (< 90 days: -0.10)
- Sampling detected: -0.15

**Cap: 0.90** — CKB never claims certainty about dead code.

### Using Dead Code Detection

```bash
# Find dead code candidates
ckb dead-code --min-confidence=0.7

# Example output:
# Dead Code Candidates (coverage: medium, 72%)
#
# Symbol                          File                    Refs  Calls  Confidence
# LegacyExporter.Export          internal/export/v1.go   3     0      0.82
# formatOldResponse              internal/api/compat.go  2     0      0.78
# validateLegacyConfig           internal/config/v1.go   1     0      0.75
#
# Summary: 3 candidates / 1,247 symbols analyzed
# Coverage context: 72% effective match rate over 120 days
#
# Limitations:
# - Sampling may cause false positives for low-traffic functions
# - Async/scheduled jobs may not be captured
# - Reflection-based calls are not tracked
```

### Exclusion Configuration

```json
{
  "telemetry": {
    "dead_code": {
      "exclude_patterns": [
        "**/test/**",
        "**/testdata/**",
        "**/migrations/**",
        "**/mocks/**"
      ],
      "exclude_functions": [
        "*Migration*",
        "Test*",
        "*Scheduled*",
        "*Backup*",
        "*Cron*"
      ]
    }
  }
}
```

### Limitations

| Limitation | Impact | Mitigation |
|------------|--------|------------|
| Sampling | May miss low-traffic functions | Check for sampling patterns in coverage |
| Async jobs | Scheduled tasks may not instrument | Exclude `*Scheduled*`, `*Cron*` patterns |
| Reflection | Dynamic calls invisible | Manual review of reflection-heavy code |
| Coverage gaps | Services without telemetry | Check service coverage before trusting results |
| New code | Recently deployed code needs observation time | Require min_observation_days |

**Important:** Dead code detection is advisory. Always verify before deleting code.

---

## Migration Guide

### Enabling Telemetry (New Setup)

1. **Update CKB** to v6.4+

2. **Configure telemetry** in `.ckb/config.json`:
   ```json
   {
     "telemetry": {
       "enabled": true,
       "service_map": {
         "your-service": "this-repo"
       }
     }
   }
   ```

3. **Instrument your application** with OpenTelemetry (see [OTEL Configuration](#otel-collector-configuration))

4. **Deploy instrumented code** and let telemetry accumulate (minimum 30 days recommended)

5. **Check coverage**:
   ```bash
   ckb telemetry status
   ```

6. **Use telemetry features** once coverage reaches medium+:
   ```bash
   ckb telemetry usage --symbol="path/to/file.go:FunctionName"
   ckb dead-code --min-confidence=0.7
   ```

### For Existing CKB Users

If upgrading from v6.3 or earlier:

1. **Schema migration is automatic** — v3 schema adds telemetry tables

2. **No breaking changes** — all existing features work without telemetry

3. **Telemetry is opt-in** — set `telemetry.enabled: true` to activate

4. **Gradual rollout recommended**:
   - Start with one service
   - Verify mapping works: `ckb telemetry test-map "service-name"`
   - Monitor coverage growth
   - Expand to more services

### Recommended Observation Period

| Feature | Minimum | Recommended |
|---------|---------|-------------|
| Usage display | 7 days | 30 days |
| Dead code detection | 30 days | 90 days |
| Impact enrichment | 7 days | 30 days |

### Troubleshooting

**No telemetry data appearing:**
- Check `ckb telemetry status` for last sync time
- Verify OTEL Collector is sending to correct endpoint
- Check for unmapped services: `ckb telemetry unmapped`

**Low match rate:**
- Ensure `code.filepath` attribute is set in instrumentation
- Check file paths match repository structure
- Add `code.lineno` for better matching

**Dead code false positives:**
- Review exclusion patterns
- Check if function is called via reflection
- Verify observation period is sufficient
- Check for sampling in telemetry pipeline

---

## CLI Reference

```bash
# Telemetry status and coverage
ckb telemetry status

# View usage for a symbol
ckb telemetry usage --repo=<repo-id> --symbol=<path:function>

# List unmapped services
ckb telemetry unmapped

# Test service mapping
ckb telemetry test-map <service-name>

# Find dead code candidates
ckb dead-code [--repo=<repo-id>] [--min-confidence=0.7]
```

## MCP Tools

| Tool | Description | Budget |
|------|-------------|--------|
| `getTelemetryStatus` | Coverage metrics and sync status | Cheap |
| `getObservedUsage` | Usage data for a symbol | Cheap |
| `findDeadCodeCandidates` | Dead code analysis | Heavy |

---

*Document version: 1.0*
*CKB version: 6.4.0*
