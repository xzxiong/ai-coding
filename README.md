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
- Streaming (SSE) and non-streaming responses
- Parameters: `max_tokens`, `temperature`, `top_p`, `stop_sequences`
- Model name passthrough (any model name is forwarded as-is)
- Token usage tracking per request

### Not Yet Supported

- Image content blocks
- Tool use / function calling
- Proxy-side authentication

## License

MIT
