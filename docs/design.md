# Design Document

## Overview

ai-coding is an HTTP proxy server that exposes the Anthropic Messages API interface and forwards requests to any OpenAI-compatible chat/completions backend. It tracks token usage per request and provides a web dashboard for monitoring.

## Architecture

```
┌──────────────┐         ┌─────────────────┐         ┌──────────────────┐
│   Client     │  POST   │   ai-coding     │  POST   │  OpenAI-compat   │
│ (Anthropic   │────────▶│   Proxy Server  │────────▶│  Backend         │
│  SDK/CLI)    │◀────────│                 │◀────────│  (OpenAI/Azure/  │
│              │ Anthropic│                 │  OpenAI │   vLLM/Ollama)   │
└──────────────┘  format └────────┬────────┘  format └──────────────────┘
                                  │
                                  │ record usage
                                  ▼
                          ┌───────────────┐
                          │   bbolt DB    │
                          │  (usage.db)   │
                          └───────┬───────┘
                                  │
                                  │ query
                                  ▼
                          ┌───────────────┐
                          │  Dashboard    │
                          │  /dashboard/  │
                          └───────────────┘
```

## Request Flow

### Non-Streaming

1. Client sends `POST /v1/messages` with Anthropic-format JSON body
2. Server parses and validates the request
3. Converter transforms Anthropic request → OpenAI request:
   - System prompt (string or content blocks) → system message
   - Message content (string or content blocks) → plain text messages
   - Model name passthrough (sent as-is to backend)
   - Parameter mapping (max_tokens, temperature, top_p, stop_sequences)
4. Proxy client sends request to OpenAI backend
5. On response, token usage is recorded to bbolt:
   - Timestamp, model, input/output tokens, duration
6. Response converter transforms OpenAI response → Anthropic response
7. Server returns Anthropic-format JSON response

### Streaming (SSE)

1. Client sends `POST /v1/messages` with `"stream": true`
2. Server opens SSE stream to OpenAI backend
3. Server emits Anthropic SSE events in order:
   - `message_start` → `content_block_start` → `content_block_delta`* → `content_block_stop` → `message_delta` → `message_stop`
4. After stream completes, usage is recorded (from final chunk if available)

## Token Usage Tracking

### Data Flow

```
Request completes
    → handler.recordUsage(model, input, output, stream, duration)
    → store.Record(UsageRecord{...})
    → bbolt Update tx:
        1. Generate key: [8-byte timestamp nanoseconds][8-byte sequence]
        2. json.Marshal the record
        3. Put(key, value) — single B+ tree insert
        4. Commit (fsync)
```

### Storage Design

- **Engine**: bbolt (etcd's fork of BoltDB)
- **File**: Single `usage.db` file, configurable via `DATA_FILE` env
- **Key format**: 16 bytes = `uint64(UnixNano)` + `uint64(sequence)`
  - Ordered by time, unique via sequence number
  - Enables efficient range scans with Cursor.Seek()
- **Value**: JSON-encoded `UsageRecord`
- **Bucket**: Single bucket `"usage"`

### Query Patterns

| Operation | Method | Complexity |
|-----------|--------|------------|
| Write one record | `Record()` | O(log n) — single tx with fsync |
| Read all | `Records()` | O(n) — full cursor scan |
| Read since time | `Since(t)` | O(k) — seek + scan k matching |
| Read time range | `Between(start, end)` | O(k) — seek + bounded scan |
| Count records | `Count()` | O(n) — cursor count |

### Performance (Intel Xeon Silver 4314 @ 2.40GHz)

| Operation | QPS | Latency | Notes |
|-----------|-----|---------|-------|
| Write (Record) | ~2,750 | 363μs | Bottleneck is fsync |
| Read 100 records | ~4,000 | 250μs | |
| Read 1,000 records | ~415 | 2.4ms | |
| Read 10,000 records | ~39 | 25ms | JSON deserialization dominates |
| Since (1h from 10K) | ~106 | 9.4ms | Seek skips old entries |

Write throughput is well above proxy usage (~1 req/s typical, bounded by LLM latency).

## Module Responsibilities

| Package | Responsibility |
|---------|---------------|
| `cmd/server` | Entry point, HTTP server setup, route registration |
| `internal/config` | Environment variable loading with defaults |
| `internal/dashboard` | HTML dashboard page + JSON API endpoint |
| `internal/handler` | HTTP handlers, request parsing, response writing, SSE streaming, usage recording |
| `internal/model` | Data structures for Anthropic and OpenAI API schemas |
| `internal/proxy` | OpenAI HTTP client, request/response conversion logic |
| `internal/storage` | bbolt-based persistent token usage storage |

## Model Handling

Model names are passed through as-is to the backend. No mapping is applied. If the request model is empty, `DEFAULT_MODEL` is used as a fallback.

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `LISTEN_ADDR` | `:9465` | Server listen address |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Target OpenAI-compatible endpoint |
| `OPENAI_API_KEY` | (empty) | API key for the backend |
| `DEFAULT_MODEL` | `gpt-4o` | Fallback model when request model is empty |
| `DATA_FILE` | `usage.db` | Path to bbolt database file |

## Dashboard

### Routes

| Path | Method | Description |
|------|--------|-------------|
| `/dashboard/` | GET | HTML dashboard page |
| `/dashboard/api/usage` | GET | JSON usage statistics |
| `/dashboard/api/usage?range=1h` | GET | Last 1 hour |
| `/dashboard/api/usage?range=24h` | GET | Last 24 hours |
| `/dashboard/api/usage?range=7d` | GET | Last 7 days |
| `/dashboard/api/usage?range=30d` | GET | Last 30 days |

### Dashboard Features

- Summary cards: total requests, input/output/total tokens, avg duration
- Model breakdown: request count per model
- Recent requests table: time, model, type (stream/sync), tokens, duration
- Auto-refresh every 10 seconds
- Time range filtering via toolbar buttons

## Limitations

- Image content blocks are not converted
- Streaming token usage depends on backend reporting usage in final chunk (mitigated by stream_options.include_usage)
- No retry or circuit breaker logic
- No request authentication on the proxy side
- Dashboard API caps at 1000 records per response

## Future Work

- Support multi-modal content (images via base64/URL)
- Request authentication (API key validation)
- Rate limiting and request queuing
- Dashboard: pagination, export
- Metrics export (Prometheus)
