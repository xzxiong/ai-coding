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

	if req.Stream {
		h.handleStream(w, r, openaiReq, req.Model)
	} else {
		h.handleNonStream(w, r, openaiReq, req.Model)
	}
}

func (h *MessagesHandler) handleNonStream(w http.ResponseWriter, r *http.Request, openaiReq *model.OpenAIRequest, reqModel string) {
	start := time.Now()

	resp, err := h.client.ChatCompletion(r.Context(), openaiReq)
	if err != nil {
		log.Printf("ERROR: proxy request failed: %v", err)
		writeError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}

	duration := time.Since(start).Milliseconds()
	h.recordUsage(reqModel, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, false, duration)

	anthropicResp := proxy.ConvertOpenAIToAnthropic(resp, reqModel)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(anthropicResp)
}

func (h *MessagesHandler) handleStream(w http.ResponseWriter, r *http.Request, openaiReq *model.OpenAIRequest, reqModel string) {
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

	contentBlockStart := model.AnthropicContentBlockStart{
		Type:         "content_block_start",
		Index:        0,
		ContentBlock: model.AnthropicContentBlock{Type: "text", Text: ""},
	}
	writeSSE(w, "content_block_start", contentBlockStart)
	flusher.Flush()

	var streamUsage *model.OpenAIUsage
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

		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta
			if delta.Content != "" {
				event := model.AnthropicContentBlockDelta{
					Type:  "content_block_delta",
					Index: 0,
					Delta: model.AnthropicContentBlock{Type: "text_delta", Text: delta.Content},
				}
				writeSSE(w, "content_block_delta", event)
				flusher.Flush()
			}

			if chunk.Choices[0].FinishReason != nil {
				break
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("ERROR: stream read error: %v", err)
	}

	duration := time.Since(start).Milliseconds()
	if streamUsage != nil {
		h.recordUsage(reqModel, streamUsage.PromptTokens, streamUsage.CompletionTokens, true, duration)
	} else {
		h.recordUsage(reqModel, 0, 0, true, duration)
	}

	contentBlockStop := model.AnthropicContentBlockStop{Type: "content_block_stop", Index: 0}
	writeSSE(w, "content_block_stop", contentBlockStop)

	msgDelta := model.AnthropicMessageDelta{
		Type:  "message_delta",
		Delta: model.AnthropicMessageDeltaBody{StopReason: "end_turn"},
		Usage: model.AnthropicUsage{OutputTokens: 0},
	}
	writeSSE(w, "message_delta", msgDelta)

	writeSSERaw(w, "message_stop", "{}")
	flusher.Flush()
}

func (h *MessagesHandler) recordUsage(reqModel string, input, output int, stream bool, duration int64) {
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
