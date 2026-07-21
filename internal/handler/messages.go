package handler

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/xzxiong/ai-coding/internal/config"
	"github.com/xzxiong/ai-coding/internal/model"
	"github.com/xzxiong/ai-coding/internal/proxy"
	"github.com/xzxiong/ai-coding/internal/storage"
)

func genReqID() string {
	return fmt.Sprintf("%04x", rand.Intn(0xFFFF))
}

type MessagesHandler struct {
	client *proxy.Client
	cfg    *config.Config
	store  *storage.Store
}

func NewMessagesHandler(cfg *config.Config, opts ...MessagesOption) *MessagesHandler {
	h := &MessagesHandler{
		client: proxy.NewClient(cfg),
		cfg:    cfg,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

type MessagesOption func(*MessagesHandler)

func WithStore(s *storage.Store) MessagesOption {
	return func(h *MessagesHandler) {
		h.store = s
	}
}

func (h *MessagesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only POST is allowed")
		return
	}

	var req model.AnthropicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON: "+err.Error())
		return
	}

	openaiReq, err := proxy.ConvertAnthropicToOpenAI(&req, h.cfg.DefaultModel)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	// Some upstreams reject any non-empty stop with an opaque 400. Drop it here
	// (rather than in the converter) so the converter stays upstream-agnostic.
	if h.cfg.StripStopSequences {
		openaiReq.Stop = nil
	}

	inputPreview := extractInputPreview(req.Messages)

	if req.Stream {
		h.handleStream(w, r, openaiReq, req.Model, inputPreview)
	} else {
		h.handleNonStream(w, r, openaiReq, req.Model, inputPreview)
	}
}

func (h *MessagesHandler) handleNonStream(w http.ResponseWriter, r *http.Request, openaiReq *model.OpenAIRequest, reqModel string, inputPreview string) {
	rid := genReqID()
	start := time.Now()

	if h.cfg.Debug {
		reqBody, _ := json.Marshal(openaiReq)
		log.Printf("[%s] DEBUG REQ_BODY: %s", rid, string(reqBody))
	}

	resp, err := h.client.ChatCompletion(r.Context(), openaiReq)
	if err != nil {
		log.Printf("[%s] ERROR: proxy request failed: model=%s tools=%d msgs=%d est_tokens=%d in=\"%s\" %v",
			rid, reqModel, len(openaiReq.Tools), len(openaiReq.Messages), estimateTokens(openaiReq), inputPreview, err)
		writeUpstreamError(w, rid, err, openaiReq, h.cfg.ContextOverflowTokens)
		return
	}

	duration := time.Since(start).Milliseconds()

	if h.cfg.Debug {
		respBody, _ := json.Marshal(resp)
		log.Printf("[%s] DEBUG RESP_BODY: %s", rid, string(respBody))
	}

	log.Printf("[%s] REQ model=%s stream=false input_tokens=%d output_tokens=%d duration=%dms in=\"%s\"",
		rid, reqModel, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, duration, inputPreview)

	h.recordUsage(reqModel, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, false, duration, inputPreview)

	anthropicResp := proxy.ConvertOpenAIToAnthropic(resp, reqModel)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(anthropicResp)
}

func (h *MessagesHandler) handleStream(w http.ResponseWriter, r *http.Request, openaiReq *model.OpenAIRequest, reqModel string, inputPreview string) {
	rid := genReqID()
	start := time.Now()

	log.Printf("[%s] START model=%s tools=%d msgs=%d in=\"%s\"",
		rid, reqModel, len(openaiReq.Tools), len(openaiReq.Messages), inputPreview)

	if h.cfg.Debug {
		reqBody, _ := json.Marshal(openaiReq)
		log.Printf("[%s] DEBUG REQ_BODY: %s", rid, string(reqBody))
	}

	resp, err := h.client.ChatCompletionStream(r.Context(), openaiReq)
	if err != nil {
		log.Printf("[%s] ERROR: proxy stream request failed: model=%s tools=%d msgs=%d est_tokens=%d in=\"%s\" %v",
			rid, reqModel, len(openaiReq.Tools), len(openaiReq.Messages), estimateTokens(openaiReq), inputPreview, err)
		writeUpstreamError(w, rid, err, openaiReq, h.cfg.ContextOverflowTokens)
		return
	}
	defer resp.Body.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "api_error", "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	msgStart := proxy.BuildStreamMessageStart(reqModel)
	writeSSE(w, "message_start", msgStart)
	flusher.Flush()

	var streamUsage *model.OpenAIUsage
	var nextBlockIndex int
	var textBlockIndex int = -1
	// Upstream reasoning (reasoning_content) maps to a separate Anthropic
	// "thinking" content block so it is never concatenated into the answer's
	// text block. -1 = none open.
	var thinkingBlockIndex int = -1
	// Anthropic SSE allows only one open content block at a time. openToolIdx is
	// the currently open tool_use block (-1 = none). Other tools accumulate in
	// memory and are emitted in index order after the open one is stopped.
	var openToolIdx = -1
	var finishReason string
	var textContent strings.Builder
	var thinkingContent strings.Builder
	// Text/reasoning that arrives while a tool block is open is buffered and
	// only used for logging — reopening text would violate sequential blocks.
	var deferredText strings.Builder

	type toolCallAccum struct {
		ID          string
		Name        string
		Arguments   string
		EmittedArgs int
		BlockIndex  int
		Started     bool
		Stopped     bool
	}
	var toolCalls []toolCallAccum

	hasToolData := func(tc *toolCallAccum) bool {
		return tc.ID != "" || tc.Name != "" || tc.Arguments != ""
	}

	closeTextBlock := func() {
		if textBlockIndex < 0 {
			return
		}
		writeSSE(w, "content_block_stop", model.AnthropicContentBlockStop{
			Type:  "content_block_stop",
			Index: textBlockIndex,
		})
		textBlockIndex = -1
		flusher.Flush()
	}

	closeThinkingBlock := func() {
		if thinkingBlockIndex < 0 {
			return
		}
		writeSSE(w, "content_block_stop", model.AnthropicContentBlockStop{
			Type:  "content_block_stop",
			Index: thinkingBlockIndex,
		})
		thinkingBlockIndex = -1
		flusher.Flush()
	}

	stopTool := func(idx int) {
		if idx < 0 || idx >= len(toolCalls) {
			return
		}
		tc := &toolCalls[idx]
		if !tc.Started || tc.Stopped {
			return
		}
		writeSSE(w, "content_block_stop", model.AnthropicContentBlockStop{
			Type:  "content_block_stop",
			Index: tc.BlockIndex,
		})
		tc.Stopped = true
		if openToolIdx == idx {
			openToolIdx = -1
		}
		flusher.Flush()
	}

	// Lowest-index unfinished tool that has any data.
	nextToolIdx := func() int {
		for i := range toolCalls {
			tc := &toolCalls[i]
			if tc.Stopped || !hasToolData(tc) {
				continue
			}
			return i
		}
		return -1
	}

	startToolBlock := func(idx int, force bool) bool {
		tc := &toolCalls[idx]
		if tc.Started {
			return true
		}
		// Avoid empty id/name on content_block_start when possible. At finalize
		// (force=true) synthesize an id so the client still gets a closed block.
		if tc.ID == "" && tc.Name == "" && !force {
			return false
		}
		if tc.ID == "" && force {
			tc.ID = fmt.Sprintf("toolu_%d", idx)
		}
		closeThinkingBlock()
		closeTextBlock()
		tc.BlockIndex = nextBlockIndex
		nextBlockIndex++
		tc.Started = true
		openToolIdx = idx
		writeSSE(w, "content_block_start", model.AnthropicContentBlockStart{
			Type:  "content_block_start",
			Index: tc.BlockIndex,
			ContentBlock: model.AnthropicContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Name,
				Input: json.RawMessage("{}"),
			},
		})
		flusher.Flush()
		return true
	}

	emitUnsentArgs := func(idx int) {
		tc := &toolCalls[idx]
		if !tc.Started || tc.Stopped {
			return
		}
		if tc.EmittedArgs >= len(tc.Arguments) {
			return
		}
		frag := tc.Arguments[tc.EmittedArgs:]
		tc.EmittedArgs = len(tc.Arguments)
		writeSSE(w, "content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": tc.BlockIndex,
			"delta": map[string]any{
				"type":         "input_json_delta",
				"partial_json": frag,
			},
		})
		flusher.Flush()
	}

	// Stream at most one tool_use block: the lowest unfinished index. Later
	// tools stay buffered until finalizeTools stops the open one and walks on.
	progressToolEmission := func() {
		i := nextToolIdx()
		if i < 0 {
			return
		}
		// A different tool is already open — keep accumulating only.
		if openToolIdx >= 0 && openToolIdx != i {
			if openToolIdx < len(toolCalls) {
				emitUnsentArgs(openToolIdx)
			}
			return
		}
		if !startToolBlock(i, false) {
			return
		}
		emitUnsentArgs(i)
	}

	// Close every tool in index order. force-start remaining tools so partial
	// metadata still becomes a complete Anthropic block sequence.
	finalizeTools := func() {
		closeThinkingBlock()
		closeTextBlock()
		for i := range toolCalls {
			tc := &toolCalls[i]
			if tc.Stopped || !hasToolData(tc) {
				continue
			}
			if openToolIdx >= 0 && openToolIdx != i {
				stopTool(openToolIdx)
			}
			if !tc.Started {
				startToolBlock(i, true)
			}
			emitUnsentArgs(i)
			stopTool(i)
		}
		if openToolIdx >= 0 {
			stopTool(openToolIdx)
		}
	}

	// Close any open block before emitting a terminal error event.
	closeOpenBlocks := func() {
		closeThinkingBlock()
		closeTextBlock()
		if openToolIdx >= 0 {
			stopTool(openToolIdx)
		}
	}

	// Reasoning tokens map to a separate "thinking" block that always precedes
	// the answer's text block. Once the answer text or any tool has started,
	// thinking is complete: further reasoning would violate sequential blocks,
	// so keep it for logs only (no reopen).
	emitThinkingDelta := func(text string) {
		if text == "" {
			return
		}
		thinkingContent.WriteString(text)
		if openToolIdx >= 0 || textBlockIndex >= 0 {
			return
		}
		for _, tc := range toolCalls {
			if tc.Started {
				return
			}
		}
		if thinkingBlockIndex < 0 {
			thinkingBlockIndex = nextBlockIndex
			nextBlockIndex++
			writeSSE(w, "content_block_start", model.AnthropicContentBlockStart{
				Type:  "content_block_start",
				Index: thinkingBlockIndex,
				ContentBlock: model.AnthropicContentBlock{
					Type:     "thinking",
					Thinking: "",
				},
			})
		}
		writeSSE(w, "content_block_delta", model.AnthropicContentBlockDelta{
			Type:  "content_block_delta",
			Index: thinkingBlockIndex,
			Delta: model.AnthropicContentBlock{Type: "thinking_delta", Thinking: text},
		})
		flusher.Flush()
	}

	emitTextDelta := func(text string) {
		if text == "" {
			return
		}
		textContent.WriteString(text)
		// Do not open/reopen text while a tool block is active — Anthropic
		// clients require sequential content blocks.
		if openToolIdx >= 0 {
			deferredText.WriteString(text)
			return
		}
		// Once any tool has started, further text would sit between/after tool
		// blocks mid-stream; keep it for logs only until finalize (no reopen).
		for _, tc := range toolCalls {
			if tc.Started {
				deferredText.WriteString(text)
				return
			}
		}
		// The answer has begun: close the thinking block so text is a distinct,
		// sequential block rather than a continuation of reasoning.
		closeThinkingBlock()
		if textBlockIndex < 0 {
			textBlockIndex = nextBlockIndex
			nextBlockIndex++
			writeSSE(w, "content_block_start", model.AnthropicContentBlockStart{
				Type:  "content_block_start",
				Index: textBlockIndex,
				ContentBlock: model.AnthropicContentBlock{
					Type: "text",
					Text: "",
				},
			})
		}
		writeSSE(w, "content_block_delta", model.AnthropicContentBlockDelta{
			Type:  "content_block_delta",
			Index: textBlockIndex,
			Delta: model.AnthropicContentBlock{Type: "text_delta", Text: text},
		})
		flusher.Flush()
	}

	scanner := bufio.NewScanner(resp.Body)
	// Large tool-call argument lines can exceed the default 64KB token limit.
	scanner.Buffer(make([]byte, 0, 1024*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		if h.cfg.Debug {
			log.Printf("[%s] DEBUG CHUNK: %s", rid, data)
		}

		var chunk model.OpenAIStreamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			log.Printf("[%s] WARN: malformed stream chunk: %v", rid, err)
			continue
		}

		if chunk.Usage != nil {
			streamUsage = chunk.Usage
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		// Grok / some OpenAI-compat backends stream thinking separately. Surface
		// it as a distinct Anthropic "thinking" block so the client is not blank
		// while tokens burn, without concatenating reasoning into the answer.
		if reasoning := delta.ReasoningContent; reasoning != "" {
			emitThinkingDelta(reasoning)
		} else if reasoning := delta.Reasoning; reasoning != "" {
			emitThinkingDelta(reasoning)
		}

		if delta.Content != "" {
			emitTextDelta(delta.Content)
		}

		for _, tc := range delta.ToolCalls {
			for tc.Index >= len(toolCalls) {
				toolCalls = append(toolCalls, toolCallAccum{BlockIndex: -1})
			}
			accum := &toolCalls[tc.Index]
			if tc.ID != "" {
				accum.ID = tc.ID
			}
			if tc.Function.Name != "" {
				accum.Name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				accum.Arguments += tc.Function.Arguments
			}
			// Incremental emit for the current frontier tool only.
			progressToolEmission()
		}

		if chunk.Choices[0].FinishReason != nil {
			finishReason = *chunk.Choices[0].FinishReason
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[%s] ERROR: stream read error: %v", rid, err)
		// Close any half-open content blocks before the error event so clients
		// do not see a dangling tool_use/text block.
		closeOpenBlocks()
		// Send Anthropic error event so Claude Code retries instead of seeing a silent empty response.
		writeSSE(w, "error", map[string]any{
			"type": "error",
			"error": map[string]string{
				"type":    "api_error",
				"message": fmt.Sprintf("upstream stream error: %v", err),
			},
		})
		flusher.Flush()
		return
	}

	finalizeTools()
	flusher.Flush()

	duration := time.Since(start).Milliseconds()

	var inTok, outTok int
	if streamUsage != nil {
		inTok, outTok = streamUsage.PromptTokens, streamUsage.CompletionTokens
	}

	if len(toolCalls) > 0 {
		for i, tc := range toolCalls {
			if tc.Name == "" && tc.Arguments == "" && tc.ID == "" {
				continue
			}
			log.Printf("[%s] REQ model=%s stream=true tool[%d]=%s args=%s",
				rid, reqModel, i, tc.Name, truncate(tc.Arguments, 80))
		}
	}
	if textContent.Len() > 0 {
		log.Printf("[%s] REQ model=%s stream=true out=\"%s\"",
			rid, reqModel, truncate(textContent.String(), 200))
	}

	log.Printf("[%s] REQ model=%s stream=true input_tokens=%d output_tokens=%d duration=%dms in=\"%s\"",
		rid, reqModel, inTok, outTok, duration, inputPreview)

	h.recordUsage(reqModel, inTok, outTok, true, duration, inputPreview)

	stopReason := "end_turn"
	hasTool := false
	for _, tc := range toolCalls {
		if tc.Started {
			hasTool = true
			break
		}
	}
	if finishReason == "tool_calls" || hasTool {
		stopReason = "tool_use"
	}

	msgDelta := model.AnthropicMessageDelta{
		Type:  "message_delta",
		Delta: model.AnthropicMessageDeltaBody{StopReason: stopReason},
		Usage: model.AnthropicUsage{OutputTokens: outTok},
	}
	writeSSE(w, "message_delta", msgDelta)

	writeSSERaw(w, "message_stop", "{}")
	flusher.Flush()
}

func (h *MessagesHandler) recordUsage(reqModel string, input, output int, stream bool, duration int64, inputPreview string) {
	if h.store == nil {
		return
	}
	if err := h.store.Record(storage.UsageRecord{
		Timestamp:    time.Now(),
		Model:        reqModel,
		InputTokens:  input,
		OutputTokens: output,
		TotalTokens:  input + output,
		Stream:       stream,
		Duration:     duration,
		InputPreview: inputPreview,
	}); err != nil {
		log.Printf("ERROR: record usage: %v", err)
	}
}

func writeSSE(w io.Writer, event string, data any) {
	raw, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, raw)
}

func writeSSERaw(w io.Writer, event string, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}

func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

func extractInputPreview(messages []model.AnthropicMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			switch v := messages[i].Content.(type) {
			case string:
				return truncate(v, 10)
			}
		}
	}
	return ""
}

// contextOverflowKeywords are substrings a well-behaved upstream includes in
// its error body when the prompt exceeds the model's context window. The
// backend behind cn.xapex.cc collapses this into an opaque
// "bad_response_status_code" 400 that matches none of these — hence the token
// estimate below is the primary signal; keyword matching is a fallback for
// upstreams that report the real reason.
var contextOverflowKeywords = []string{
	"context_length_exceeded",
	"maximum context length",
	"prompt is too long",
	"too many tokens",
	"reduce the length",
}

func looksLikeContextOverflow(body string) bool {
	b := strings.ToLower(body)
	for _, kw := range contextOverflowKeywords {
		if strings.Contains(b, kw) {
			return true
		}
	}
	return false
}

// estimateTokens is a coarse magnitude estimate of the request size, ~4 chars
// per token. Used only to disambiguate an opaque upstream 400, never to reject
// a request before forwarding it.
func estimateTokens(req *model.OpenAIRequest) int {
	b, err := json.Marshal(req)
	if err != nil {
		return 0
	}
	return len(b) / 4
}

// isContextOverflow decides whether an upstream failure is a context-length
// overflow. 413 is unconditional (semantically "request too large"); 400 is a
// grab-bag, so it counts only when the body names the reason OR the request we
// sent is itself large enough to plausibly overflow.
func isContextOverflow(apiErr *proxy.APIError, req *model.OpenAIRequest, threshold int) bool {
	switch apiErr.StatusCode {
	case http.StatusRequestEntityTooLarge:
		return true
	case http.StatusBadRequest:
		return looksLikeContextOverflow(apiErr.Body) || estimateTokens(req) > threshold
	}
	return false
}

// writeUpstreamError maps an upstream proxy failure onto an Anthropic-native
// error. A context-length overflow is translated into the "prompt is too long"
// invalid_request_error that Claude Code recognizes to trigger its
// auto-compaction and retry. Everything else is surfaced as a 502 api_error so
// the client treats it as a transient failure.
func writeUpstreamError(w http.ResponseWriter, rid string, err error, req *model.OpenAIRequest, threshold int) {
	var apiErr *proxy.APIError
	if errors.As(err, &apiErr) {
		if isContextOverflow(apiErr, req, threshold) {
			log.Printf("[%s] context overflow: mapping upstream %d to prompt-too-long (est_tokens=%d threshold=%d)",
				rid, apiErr.StatusCode, estimateTokens(req), threshold)
			writeError(w, http.StatusBadRequest, "invalid_request_error",
				"prompt is too long: exceeds the model's maximum context length")
			return
		}
		// An upstream 400 that did NOT qualify as overflow is the interesting
		// case: log why it stays a transient 502 so it is not mistaken for a
		// context-length problem later.
		if apiErr.StatusCode == http.StatusBadRequest {
			log.Printf("[%s] upstream 400 not treated as overflow (est_tokens=%d threshold=%d) -> 502; body=%s",
				rid, estimateTokens(req), threshold, apiErr.Body)
		}
	}
	writeError(w, http.StatusBadGateway, "api_error", err.Error())
}

func writeError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]string{
			"type":    errType,
			"message": message,
		},
	})
}
