package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/xzxiong/ai-coding/internal/config"
	"github.com/xzxiong/ai-coding/internal/model"
	"github.com/xzxiong/ai-coding/internal/proxy"
	"github.com/xzxiong/ai-coding/internal/storage"
)

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
	start := time.Now()

	resp, err := h.client.ChatCompletion(r.Context(), openaiReq)
	if err != nil {
		log.Printf("ERROR: proxy request failed: %v", err)
		writeError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}

	duration := time.Since(start).Milliseconds()

	log.Printf("REQ model=%s stream=false input_tokens=%d output_tokens=%d duration=%dms in=\"%s\"",
		reqModel, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, duration, inputPreview)

	h.recordUsage(reqModel, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, false, duration, inputPreview)

	anthropicResp := proxy.ConvertOpenAIToAnthropic(resp, reqModel)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(anthropicResp)
}

func (h *MessagesHandler) handleStream(w http.ResponseWriter, r *http.Request, openaiReq *model.OpenAIRequest, reqModel string, inputPreview string) {
	start := time.Now()

	resp, err := h.client.ChatCompletionStream(r.Context(), openaiReq)
	if err != nil {
		log.Printf("ERROR: proxy stream request failed: %v", err)
		writeError(w, http.StatusBadGateway, "api_error", err.Error())
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
		log.Printf("ERROR: stream read error: %v", err)
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

	log.Printf("REQ model=%s stream=true input_tokens=%d output_tokens=%d duration=%dms in=\"%s\"",
		reqModel, inTok, outTok, duration, inputPreview)

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
