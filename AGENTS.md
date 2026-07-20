# AGENTS.md — Development Reference

## Overview

Anthropic-to-OpenAI proxy that translates Anthropic Messages API requests to OpenAI chat/completions format. Includes streaming support, tool calling translation, token usage tracking, and a web dashboard.

## Build & Run

```bash
# Local dev
source active          # loads OPENAI_BASE_URL, OPENAI_API_KEY, DEFAULT_MODEL
go run ./cmd/server

# Docker
make docker-build      # builds ai-coding:latest image
source active          # export env vars into shell
make docker-down       # stop existing container
make docker-up         # start container (picks up env from shell)
```

## Debug Flow

```bash
# Enable full request/response logging
export DEBUG=1
make docker-build && source active && export DEBUG=1 && make docker-down && make docker-up

# Watch logs
docker compose logs -f
```

### Log Format

Each request is tagged with a 4-char hex ID (`[a1b2]`) to group related lines:

```
[rid] START model=xxx tools=N msgs=N in="..."         ← request received
[rid] DEBUG REQ_BODY: {...}                            ← full upstream request (DEBUG=1 only)
[rid] DEBUG CHUNK: {...}                               ← each SSE chunk received (DEBUG=1 only)
[rid] REQ model=xxx stream=true tool[0]=name args=... ← tool calls detected
[rid] REQ model=xxx stream=true out="..."             ← text output (first 200 chars)
[rid] REQ model=xxx stream=true input_tokens=N output_tokens=N duration=Nms in="..."
```

### Common Debug Scenarios

| Symptom | What to check |
|---|---|
| `input_tokens=0 output_tokens=0` | Usage chunk not received; check if backend supports `stream_options.include_usage` |
| No `tool[N]=` line in logs | Model returned text instead of tool call; check `out=` line and `tools=` count in START |
| `ERROR: proxy stream request failed` | Upstream returned non-200; check API key and URL |

## Key Files

| File | Purpose |
|---|---|
| `cmd/server/main.go` | Entry point, HTTP mux setup |
| `internal/handler/messages.go` | HTTP handler, streaming loop, logging |
| `internal/proxy/converter.go` | Anthropic → OpenAI request conversion |
| `internal/proxy/response.go` | OpenAI → Anthropic response conversion |
| `internal/proxy/client.go` | Upstream HTTP client |
| `internal/config/config.go` | Env var config loading |
| `internal/model/anthropic.go` | Anthropic request/response types |
| `internal/model/openai.go` | OpenAI request/response types |
| `internal/storage/storage.go` | bbolt token usage persistence |
| `internal/dashboard/dashboard.go` | Web dashboard + JSON API |

## Configuration

| Env Variable | Default | Description |
|---|---|---|
| `LISTEN_ADDR` | `:9465` | Server listen address |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Target OpenAI-compatible endpoint |
| `OPENAI_API_KEY` | (empty) | API key for the backend |
| `DEFAULT_MODEL` | `gpt-4o` | Fallback model when request model is empty |
| `DATA_FILE` | `usage.db` | Path to bbolt database file |
| `DEBUG` | (empty) | Set `1` or `true` for full request/response logging |

## Testing

```bash
make test              # unit tests
make test-real         # real API e2e (needs TEST_BASE_URL, TEST_TOKEN, TEST_MODEL)
make e2e               # Python SDK e2e
```

## The `active` File

Local env config for running the proxy. Gitignored (contains API keys).

```bash
cp active.example active   # create from template
vi active                  # fill in your real API key and endpoint
source active              # load into shell before make docker-up or go run
```

The file is sourced as a shell script — it exports `OPENAI_BASE_URL`, `OPENAI_API_KEY`, and `DEFAULT_MODEL`. See `active.example` for the template.

## Conventions

- `active` file holds local env vars (gitignored, contains API keys)
- Model names pass through as-is to upstream — no mapping in proxy
- Streaming: usage chunk arrives after finish_reason chunk (don't break loop early)
- Tool calls stream incrementally: first tool delta opens `tool_use`, each `arguments` fragment is `input_json_delta`
- Grok-style `reasoning_content` / `reasoning` is forwarded as text deltas so the UI is not blank while thinking tokens burn

## Known Issues

- `deepseek-v4-flash` fails tool calling with large context (50K+) and many tools (48+). Use `deepseek-v4-pro` for tool-heavy sessions. See [#1](https://github.com/xzxiong/ai-coding/issues/1).
- Historical: blank Claude Code `Hatching...` with rising tokens was caused by buffering tool-call SSE until upstream finished. Fixed 2026-07-20 — tool args now stream as incremental `input_json_delta`. See [docs/sse-hang-blank-hatching.md](docs/sse-hang-blank-hatching.md).
