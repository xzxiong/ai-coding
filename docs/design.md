# Design Document

## Overview

ai-coding is an HTTP proxy server that exposes the Anthropic Messages API interface and forwards requests to any OpenAI-compatible chat/completions backend. This enables clients built for the Anthropic API to seamlessly use OpenAI, Azure OpenAI, or any OpenAI-compatible provider.

## Architecture

```
┌──────────────┐         ┌─────────────────┐         ┌──────────────────┐
│   Client     │  POST   │   ai-coding     │  POST   │  OpenAI-compat   │
│ (Anthropic   │────────▶│   Proxy Server  │────────▶│  Backend         │
│  SDK/CLI)    │◀────────│                 │◀────────│  (OpenAI/Azure/  │
│              │ Anthropic│                 │  OpenAI │   vLLM/Ollama)   │
└──────────────┘  format └─────────────────┘  format └──────────────────┘
```

## Request Flow

### Non-Streaming

1. Client sends `POST /v1/messages` with Anthropic-format JSON body
2. Server parses and validates the request
3. Converter transforms Anthropic request → OpenAI request:
   - System prompt (string or content blocks) → system message
   - Message content (string or content blocks) → plain text messages
   - Model name mapping (claude-* → gpt-*)
   - Parameter mapping (max_tokens, temperature, top_p, stop_sequences)
4. Proxy client sends request to OpenAI backend
5. Response converter transforms OpenAI response → Anthropic response:
   - Choices[0].message → content blocks
   - finish_reason → stop_reason
   - usage mapping
6. Server returns Anthropic-format JSON response

### Streaming (SSE)

1. Client sends `POST /v1/messages` with `"stream": true`
2. Server opens SSE stream to OpenAI backend
3. Server emits Anthropic SSE events in order:
   - `message_start` — message metadata
   - `content_block_start` — begin text block
   - `content_block_delta` — incremental text chunks (mapped from OpenAI deltas)
   - `content_block_stop` — end text block
   - `message_delta` — stop reason and final usage
   - `message_stop` — end of stream
4. Each OpenAI `data: {...}` chunk is translated to the corresponding Anthropic event

## Module Responsibilities

| Package | Responsibility |
|---------|---------------|
| `cmd/server` | Entry point, HTTP server setup, route registration |
| `internal/config` | Environment variable loading with defaults |
| `internal/handler` | HTTP handlers, request parsing, response writing, SSE streaming |
| `internal/model` | Data structures for Anthropic and OpenAI API schemas |
| `internal/proxy` | OpenAI HTTP client, request/response conversion logic |

## Model Mapping

| Anthropic Model | OpenAI Model |
|-----------------|--------------|
| claude-opus-4-7 | gpt-4o |
| claude-sonnet-4-6 | gpt-4o |
| claude-haiku-4-5 | gpt-4o-mini |
| claude-3-5-sonnet-latest | gpt-4o |
| claude-3-5-haiku-latest | gpt-4o-mini |
| (unknown) | DEFAULT_MODEL env |

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `LISTEN_ADDR` | `:8080` | Server listen address |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Target OpenAI-compatible endpoint |
| `OPENAI_API_KEY` | (empty) | API key for the backend |
| `DEFAULT_MODEL` | `gpt-4o` | Fallback model when no mapping exists |

## Limitations

- Only text content is supported; image/tool_use content blocks are not converted
- Tool calling (function calling) is not proxied
- Token usage in streaming mode does not report accurate counts
- No retry or circuit breaker logic
- No request authentication on the proxy side

## Future Work

- Support multi-modal content (images via base64/URL)
- Tool use / function calling translation
- Request authentication (API key validation)
- Rate limiting and request queuing
- Response caching for identical requests
- Metrics and observability (Prometheus)
