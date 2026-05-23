# ai-coding

An HTTP proxy server that accepts Anthropic Messages API requests and forwards them to OpenAI-compatible chat/completions endpoints. Includes built-in token usage tracking and a web dashboard.

## Quick Start

```bash
export OPENAI_BASE_URL="https://api.openai.com/v1"
export OPENAI_API_KEY="sk-xxx"
make run
```

The server starts on `:9465` by default. Dashboard available at `http://localhost:9465/dashboard/`.

## Usage

Send requests using the Anthropic Messages API format:

```bash
curl -X POST http://localhost:9465/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-v4-pro",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Hello, who are you?"}
    ]
  }'
```

### Streaming

```bash
curl -X POST http://localhost:9465/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-v4-pro",
    "max_tokens": 1024,
    "stream": true,
    "messages": [
      {"role": "user", "content": "Write a haiku about coding."}
    ]
  }'
```

### With System Prompt

```bash
curl -X POST http://localhost:9465/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4.6",
    "max_tokens": 1024,
    "system": "You are a helpful assistant that responds in Chinese.",
    "messages": [
      {"role": "user", "content": "What is Go?"}
    ]
  }'
```

## Token Usage Dashboard

Access the dashboard at `http://localhost:9465/dashboard/` to view:

- Total requests, input/output/total tokens
- Average request duration
- Model breakdown (requests per model)
- Recent request history with per-request details
- Time range filters: 1h, 24h, 7d, 30d, all

### Dashboard API

```bash
# All usage data
curl http://localhost:9465/dashboard/api/usage

# Filter by time range
curl http://localhost:9465/dashboard/api/usage?range=24h
curl http://localhost:9465/dashboard/api/usage?range=7d
```

Response:
```json
{
  "total_requests": 42,
  "total_input_tokens": 1250,
  "total_output_tokens": 8300,
  "total_tokens": 9550,
  "avg_duration_ms": 2100,
  "model_breakdown": {"deepseek-v4-pro": 30, "claude-sonnet-4.6": 12},
  "records": [...]
}
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `LISTEN_ADDR` | `:9465` | Server listen address |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Target OpenAI-compatible endpoint |
| `OPENAI_API_KEY` | (empty) | API key for the backend |
| `DEFAULT_MODEL` | `gpt-4o` | Fallback model when request model is empty |
| `DATA_FILE` | `usage.db` | Path to bbolt database file for token tracking |
| `DEBUG` | (empty) | Set to `1` or `true` to enable full request/response logging |

## Claude Code Integration

Use this proxy as the backend for [Claude Code](https://docs.anthropic.com/en/docs/claude-code), routing all requests to your preferred OpenAI-compatible models.

### Setup

Add the following to your shell profile (`~/.bashrc`, `~/.zshrc`, etc.) or export before launching Claude Code:

```bash
# Point Claude Code to the proxy
export ANTHROPIC_BASE_URL="http://localhost:9465"
export ANTHROPIC_AUTH_TOKEN="dummy"

# Model mapping
export ANTHROPIC_MODEL="deepseek-v4-flash"
export ANTHROPIC_DEFAULT_OPUS_MODEL="<your-large-model>"
export ANTHROPIC_DEFAULT_SONNET_MODEL="<your-mid-model>"
export ANTHROPIC_DEFAULT_HAIKU_MODEL="<your-fast-model>"

# Subagent and effort settings
export CLAUDE_CODE_SUBAGENT_MODEL="<your-fast-model>"
export CLAUDE_CODE_EFFORT_LEVEL="max"
```

> **Note:** `ANTHROPIC_AUTH_TOKEN` can be any non-empty value — the proxy does not validate client-side tokens. The real backend API key is configured server-side via `OPENAI_API_KEY`.

### Model mapping explained

| Env Variable | Purpose | Example |
|---|---|---|
| `ANTHROPIC_MODEL` | Default model for main conversation | `deepseek-v4-flash` |
| `ANTHROPIC_DEFAULT_OPUS_MODEL` | Used when Claude Code requests Opus-tier | `kimi-k2.5` |
| `ANTHROPIC_DEFAULT_SONNET_MODEL` | Used when Claude Code requests Sonnet-tier | `deepseek-v4-pro` |
| `ANTHROPIC_DEFAULT_HAIKU_MODEL` | Used when Claude Code requests Haiku-tier | `deepseek-v4-flash` |
| `CLAUDE_CODE_SUBAGENT_MODEL` | Model for spawned sub-agents | `deepseek-v4-flash` |
| `CLAUDE_CODE_EFFORT_LEVEL` | Thinking effort: `low`, `medium`, `high`, `max` | `max` |

### Known limitations

- Smaller/faster models (e.g. `deepseek-v4-flash`) may fail to make tool calls when context is large (50K+ tokens) with many tools (48+). Use a larger model (e.g. `deepseek-v4-pro`) for complex tool-heavy sessions. See [#1](https://github.com/xzxiong/ai-coding/issues/1).

## Client Configuration

Point any Anthropic-compatible client to the proxy:

### Claude Code CLI (minimal)

```bash
export ANTHROPIC_BASE_URL="http://localhost:9465"
export ANTHROPIC_API_KEY="any-value"
```

### Python (Anthropic SDK)

```python
import anthropic

client = anthropic.Anthropic(
    base_url="http://localhost:9465",
    api_key="any-value",
)
resp = client.messages.create(
    model="deepseek-v4-pro",
    max_tokens=1024,
    messages=[{"role": "user", "content": "Hello"}],
)
```

### Node.js (Anthropic SDK)

```javascript
import Anthropic from "@anthropic-ai/sdk";

const client = new Anthropic({
  baseURL: "http://localhost:9465",
  apiKey: "any-value",
});
```

> **Note:** The proxy does not validate `api_key` on the client side. The real backend token is configured server-side via `OPENAI_API_KEY`.

## Supported Backends

Any OpenAI-compatible API works as a backend:

- OpenAI API
- Azure OpenAI
- vLLM
- Ollama (with OpenAI compatibility mode)
- LiteLLM
- Any other OpenAI-compatible server

## Build & Test

```bash
make build       # Output: bin/server
make run         # Run server
make test        # Unit tests
make test-real   # Real API e2e tests (requires TEST_BASE_URL, TEST_TOKEN)
make docker-build
make docker-up
make docker-down
make e2e         # Python SDK e2e tests
make clean       # Remove build artifacts
```

### Real API Tests

```bash
TEST_BASE_URL="https://api.openai.com/v1" \
TEST_TOKEN="sk-xxx" \
TEST_MODEL="gpt-4o" \
make test-real
```

### Benchmarks

```bash
go test -bench=. -benchmem ./internal/storage/
```

## Project Structure

```
├── cmd/server/main.go              # Entry point
├── internal/
│   ├── config/config.go            # Environment configuration
│   ├── dashboard/dashboard.go      # HTML dashboard + JSON API
│   ├── handler/messages.go         # HTTP handler (Anthropic API)
│   ├── model/
│   │   ├── anthropic.go            # Anthropic request/response types
│   │   └── openai.go               # OpenAI request/response types
│   ├── proxy/
│   │   ├── client.go               # OpenAI HTTP client
│   │   ├── converter.go            # Anthropic → OpenAI conversion
│   │   └── response.go             # OpenAI → Anthropic conversion
│   └── storage/
│       ├── storage.go              # bbolt-based persistent storage
│       ├── storage_test.go         # Storage unit tests
│       └── storage_bench_test.go   # Storage benchmarks
├── tests/e2e/
│   ├── real_test.go                # Real API e2e tests (Go)
│   └── test_anthropic_sdk.py       # Anthropic SDK e2e tests (Python)
├── docs/design.md                  # Design document
├── Dockerfile
├── docker-compose.yaml
├── Makefile
└── go.mod
```

## API Compatibility

### Supported Features

- Text messages (single string and content block array)
- System prompt (string and content block array)
- Tool use / function calling (Anthropic tools ↔ OpenAI functions)
- Streaming (SSE) and non-streaming responses
- Parameters: `max_tokens`, `temperature`, `top_p`, `stop_sequences`
- Model name passthrough (any model name is forwarded as-is)
- Token usage tracking per request

### Not Yet Supported

- Image content blocks
- Proxy-side authentication

## License

MIT
