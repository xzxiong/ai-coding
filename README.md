# ai-coding

An HTTP proxy server that accepts Anthropic Messages API requests and forwards them to OpenAI-compatible chat/completions endpoints.

## Quick Start

```bash
export OPENAI_BASE_URL="https://api.openai.com/v1"
export OPENAI_API_KEY="sk-xxx"
make run
```

The server starts on `:8080` by default.

## Usage

Send requests using the Anthropic Messages API format:

```bash
curl -X POST http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-6",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Hello, who are you?"}
    ]
  }'
```

### Streaming

```bash
curl -X POST http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-6",
    "max_tokens": 1024,
    "stream": true,
    "messages": [
      {"role": "user", "content": "Write a haiku about coding."}
    ]
  }'
```

### With System Prompt

```bash
curl -X POST http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-6",
    "max_tokens": 1024,
    "system": "You are a helpful assistant that responds in Chinese.",
    "messages": [
      {"role": "user", "content": "What is Go?"}
    ]
  }'
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `LISTEN_ADDR` | `:8080` | Server listen address |
| `OPENAI_BASE_URL` | `https://api.openai.com/v1` | Target OpenAI-compatible endpoint |
| `OPENAI_API_KEY` | (empty) | API key for the backend |
| `DEFAULT_MODEL` | `gpt-4o` | Fallback model for unmapped model names |

## Supported Backends

Any OpenAI-compatible API works as a backend:

- OpenAI API
- Azure OpenAI
- vLLM
- Ollama (with OpenAI compatibility mode)
- LiteLLM
- Any other OpenAI-compatible server

## Build

```bash
make build    # Output: bin/server
make test     # Run tests
make clean    # Remove build artifacts
```

## Project Structure

```
├── cmd/server/main.go           # Entry point
├── internal/
│   ├── config/config.go         # Environment configuration
│   ├── handler/messages.go      # HTTP handler (Anthropic API)
│   ├── model/
│   │   ├── anthropic.go         # Anthropic request/response types
│   │   └── openai.go            # OpenAI request/response types
│   └── proxy/
│       ├── client.go            # OpenAI HTTP client
│       ├── converter.go         # Anthropic → OpenAI conversion
│       └── response.go          # OpenAI → Anthropic conversion
├── docs/design.md               # Design document
├── Makefile
└── go.mod
```

## API Compatibility

### Supported Features

- Text messages (single string and content block array)
- System prompt (string and content block array)
- Streaming (SSE) and non-streaming responses
- Parameters: `max_tokens`, `temperature`, `top_p`, `stop_sequences`
- Model name auto-mapping

### Not Yet Supported

- Image content blocks
- Tool use / function calling
- Proxy-side authentication

## License

MIT
