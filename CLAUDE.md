# ai-coding

Anthropic-to-OpenAI proxy for Claude Code.

See [AGENTS.md](AGENTS.md) for detailed build, debug, and development reference.

## Quick Reference

- **Setup**: `cp active.example active` then edit with your API keys
- **Build & run**: `make docker-build && source active && make docker-down && make docker-up`
- **Debug mode**: `export DEBUG=1` before `make docker-up`
- **Watch logs**: `docker compose logs -f`
- **Tests**: `make test` / `make test-real` / `make e2e`
- **Local dev**: `source active && go run ./cmd/server`
