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

	var streamUsage *model.OpenAIUsage
	var blockIndex int
	var textBlockStarted bool
	var finishReason string
	var textContent strings.Builder

	type toolCallAccum struct {
		ID        string
		Name      string
		Arguments string
	}
	var toolCalls []toolCallAccum

	scanner := bufio.NewScanner(resp.Body)
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
			log.Printf("WARN: malformed stream chunk: %v", err)
			continue
		}

		if chunk.Usage != nil {
			streamUsage = chunk.Usage
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		if delta.Content != "" {
			textContent.WriteString(delta.Content)
			if !textBlockStarted {
				writeSSE(w, "content_block_start", model.AnthropicContentBlockStart{
					Type: "content_block_start", Index: blockIndex,
					ContentBlock: model.AnthropicContentBlock{Type: "text", Text: ""},
				})
				textBlockStarted = true
			}
			writeSSE(w, "content_block_delta", model.AnthropicContentBlockDelta{
				Type: "content_block_delta", Index: blockIndex,
				Delta: model.AnthropicContentBlock{Type: "text_delta", Text: delta.Content},
			})
			flusher.Flush()
		}

		for _, tc := range delta.ToolCalls {
			for tc.Index >= len(toolCalls) {
				toolCalls = append(toolCalls, toolCallAccum{})
			}
			if tc.ID != "" {
				toolCalls[tc.Index].ID = tc.ID
			}
			if tc.Function.Name != "" {
				toolCalls[tc.Index].Name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				toolCalls[tc.Index].Arguments += tc.Function.Arguments
			}
		}

		if chunk.Choices[0].FinishReason != nil {
			finishReason = *chunk.Choices[0].FinishReason
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[%s] ERROR: stream read error: %v", rid, err)
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

	if textBlockStarted {
		writeSSE(w, "content_block_stop", model.AnthropicContentBlockStop{Type: "content_block_stop", Index: blockIndex})
		blockIndex++
	}

	for _, tc := range toolCalls {
		writeSSE(w, "content_block_start", model.AnthropicContentBlockStart{
			Type: "content_block_start", Index: blockIndex,
			ContentBlock: model.AnthropicContentBlock{
				Type: "tool_use", ID: tc.ID, Name: tc.Name,
				Input: json.RawMessage("{}"),
			},
		})
		if tc.Arguments != "" {
			writeSSE(w, "content_block_delta", map[string]any{
				"type": "content_block_delta", "index": blockIndex,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": tc.Arguments},
			})
		}
		writeSSE(w, "content_block_stop", model.AnthropicContentBlockStop{Type: "content_block_stop", Index: blockIndex})
		blockIndex++
	}
	flusher.Flush()

	duration := time.Since(start).Milliseconds()

	var inTok, outTok int
	if streamUsage != nil {
		inTok, outTok = streamUsage.PromptTokens, streamUsage.CompletionTokens
	}

	if len(toolCalls) > 0 {
		for i, tc := range toolCalls {
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
	if finishReason == "tool_calls" || len(toolCalls) > 0 {
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
