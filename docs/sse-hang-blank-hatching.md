# SSE Hang: Blank "Hatching" with Rising Tokens

**Date**: 2026-07-20  
**Status**: Fixed  
**Symptom**: Claude Code shows `Hatching...` for minutes, token counter rises (`↓ 10.9k`), almost no visible text/tool progress in the UI.

## Observed Symptoms

- UI spinner stays on with empty content area
- Token usage keeps increasing (downstream still generating)
- Local proxy process idle / low CPU, not crashed
- Active TCP still established:
  - Claude Code → `localhost:9465`
  - proxy → upstream OpenAI-compatible host (`cn.xapex.cc` / `1.13.71.192:443`)
- Dashboard usage only records **after** a stream finishes, so in-flight hangs do not appear until completion or cancel

## Root Cause

Primary bug in `internal/handler/messages.go` streaming path:

1. **Text deltas were flushed incrementally**
2. **Tool call deltas were fully buffered until the upstream SSE ended**, then emitted as one big Anthropic `tool_use` block

When the model spends most of a turn generating large tool arguments (e.g. `Write` / `Edit` of multi-KB docs), Claude Code only receives:

1. `message_start` (sometimes delayed by buffering)
2. long silence while arguments stream from upstream
3. a single late burst of `tool_use` after upstream finishes

That matches the blank hatching UI with rising tokens.

### Secondary issues

| Issue | Effect |
|---|---|
| No `Flush()` after `message_start` | Client may wait longer for first visible event |
| `reasoning_content` / `reasoning` ignored | Grok-style thinking tokens burn but never surface |
| `bufio.Scanner` default max token ~64KB | Oversized single SSE line can fail mid-stream |
| Usage recorded only at end | Hang looks like "nothing happening" in dashboard |

## Fix (2026-07-20)

Implemented in `internal/handler/messages.go`:

1. **Incremental tool_call → Anthropic SSE**
   - first sight of a tool index → `content_block_start` (`tool_use`)
   - each `arguments` fragment → `content_block_delta` (`input_json_delta`) immediately
   - stream end / finish → `content_block_stop`
2. **Flush after `message_start`** and after each tool delta
3. **Map `reasoning_content` / `reasoning` into text deltas** so thinking is not dropped
4. **Raise scanner buffer** (1 MiB start, 16 MiB max) for large tool-argument lines
5. Close open text/tool blocks cleanly before `message_delta` / `message_stop`

## How to Verify

```bash
make test
# optional live check
export DEBUG=1
source active && go run ./cmd/server
```

Live expectations:

- During large tool generation, Claude Code should show tool activity sooner (not only at the end)
- Proxy logs should keep progressing while upstream chunks arrive
- DEBUG mode should show continuous `DEBUG CHUNK` lines, and client-visible SSE should include intermediate `input_json_delta` events

## Related Files

- `internal/handler/messages.go` — stream translation
- `internal/handler/messages_test.go` — stream/tool tests
- `internal/model/openai.go` — stream delta fields
- `AGENTS.md` — debug / stream notes
