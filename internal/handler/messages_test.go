package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xzxiong/ai-coding/internal/config"
	"github.com/xzxiong/ai-coding/internal/model"
	"github.com/xzxiong/ai-coding/internal/storage"
)

func setupMockOpenAI(t *testing.T, response model.OpenAIResponse) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}

		var req model.OpenAIRequest
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("failed to parse request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
}

func setupMockOpenAIStream(t *testing.T, chunks []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, chunk := range chunks {
			w.Write([]byte("data: " + chunk + "\n\n"))
			flusher.Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
}

func TestMessagesHandler_NonStream(t *testing.T) {
	mockResp := model.OpenAIResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Created: 1700000000,
		Model:   "gpt-4o",
		Choices: []model.OpenAIChoice{
			{
				Index:        0,
				Message:      model.OpenAIMessage{Role: "assistant", Content: "Hi there!"},
				FinishReason: "stop",
			},
		},
		Usage: model.OpenAIUsage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
	}

	server := setupMockOpenAI(t, mockResp)
	defer server.Close()

	cfg := &config.Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
		DefaultModel:  "gpt-4o",
	}

	handler := NewMessagesHandler(cfg)

	body := `{"model":"claude-sonnet-4-6","max_tokens":1024,"messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp model.AnthropicResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Type != "message" {
		t.Errorf("expected type message, got %s", resp.Type)
	}
	if resp.Role != "assistant" {
		t.Errorf("expected role assistant, got %s", resp.Role)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "Hi there!" {
		t.Errorf("unexpected content: %+v", resp.Content)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("expected stop_reason end_turn, got %s", resp.StopReason)
	}
	if resp.Usage.InputTokens != 5 {
		t.Errorf("expected input_tokens 5, got %d", resp.Usage.InputTokens)
	}
}

func TestMessagesHandler_Stream(t *testing.T) {
	chunks := []string{
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
	}

	server := setupMockOpenAIStream(t, chunks)
	defer server.Close()

	cfg := &config.Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
		DefaultModel:  "gpt-4o",
	}

	handler := NewMessagesHandler(cfg)

	body := `{"model":"claude-sonnet-4-6","max_tokens":1024,"stream":true,"messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	respBody := rec.Body.String()

	if !strings.Contains(respBody, "event: message_start") {
		t.Error("missing message_start event")
	}
	if !strings.Contains(respBody, "event: content_block_start") {
		t.Error("missing content_block_start event")
	}
	if !strings.Contains(respBody, "event: content_block_delta") {
		t.Error("missing content_block_delta event")
	}
	if !strings.Contains(respBody, "Hello") {
		t.Error("missing 'Hello' in stream")
	}
	if !strings.Contains(respBody, " world") {
		t.Error("missing ' world' in stream")
	}
	if !strings.Contains(respBody, "event: message_stop") {
		t.Error("missing message_stop event")
	}
}

func TestMessagesHandler_MethodNotAllowed(t *testing.T) {
	cfg := &config.Config{DefaultModel: "gpt-4o"}
	handler := NewMessagesHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestMessagesHandler_InvalidJSON(t *testing.T) {
	cfg := &config.Config{DefaultModel: "gpt-4o"}
	handler := NewMessagesHandler(cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader("not json"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}

	var errResp map[string]any
	json.Unmarshal(rec.Body.Bytes(), &errResp)
	if errResp["type"] != "error" {
		t.Errorf("expected error type, got %v", errResp["type"])
	}
}

func TestMessagesHandler_WithSystemPrompt(t *testing.T) {
	var capturedReq model.OpenAIRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &capturedReq)

		resp := model.OpenAIResponse{
			ID:      "chatcmpl-sys",
			Choices: []model.OpenAIChoice{{Index: 0, Message: model.OpenAIMessage{Role: "assistant", Content: "OK"}, FinishReason: "stop"}},
			Usage:   model.OpenAIUsage{},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &config.Config{OpenAIBaseURL: server.URL, DefaultModel: "gpt-4o"}
	handler := NewMessagesHandler(cfg)

	body := `{"model":"claude-sonnet-4-6","max_tokens":100,"system":"Be brief.","messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if len(capturedReq.Messages) != 2 {
		t.Fatalf("expected 2 messages forwarded, got %d", len(capturedReq.Messages))
	}
	if capturedReq.Messages[0].Role != "system" || capturedReq.Messages[0].Content != "Be brief." {
		t.Errorf("system message not forwarded correctly: %+v", capturedReq.Messages[0])
	}
}

func TestMessagesHandler_StreamToolCalls(t *testing.T) {
	chunks := []string{
		`{"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`{"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Let me check."},"finish_reason":null}]}`,
		`{"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`,
		`{"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]},"finish_reason":null}]}`,
		`{"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"NYC\"}"}}]},"finish_reason":null}]}`,
		`{"id":"chatcmpl-tc","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":20,"completion_tokens":15,"total_tokens":35}}`,
	}

	server := setupMockOpenAIStream(t, chunks)
	defer server.Close()

	cfg := &config.Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
		DefaultModel:  "gpt-4o",
	}
	handler := NewMessagesHandler(cfg)

	body := `{"model":"claude-sonnet-4-6","max_tokens":1024,"stream":true,"tools":[{"name":"get_weather","description":"Get weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}],"messages":[{"role":"user","content":"Weather in NYC?"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	respBody := rec.Body.String()

	if !strings.Contains(respBody, "event: message_start") {
		t.Error("missing message_start event")
	}
	if !strings.Contains(respBody, "Let me check.") {
		t.Error("missing text content in stream")
	}
	if !strings.Contains(respBody, "get_weather") {
		t.Error("missing tool name in stream")
	}
	if !strings.Contains(respBody, "tool_use") {
		t.Error("missing tool_use content block type")
	}
	if !strings.Contains(respBody, "event: message_stop") {
		t.Error("missing message_stop event")
	}
}

func TestMessagesHandler_NonStreamToolCalls(t *testing.T) {
	mockResp := model.OpenAIResponse{
		ID:      "chatcmpl-tool",
		Object:  "chat.completion",
		Created: 1700000000,
		Model:   "gpt-4o",
		Choices: []model.OpenAIChoice{
			{
				Index: 0,
				Message: model.OpenAIMessage{
					Role:    "assistant",
					Content: "Let me look that up.",
					ToolCalls: []model.OpenAIToolCall{
						{
							ID:   "call_xyz",
							Type: "function",
							Function: model.OpenAIToolCallFunc{
								Name:      "get_weather",
								Arguments: `{"city":"NYC"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
		Usage: model.OpenAIUsage{PromptTokens: 12, CompletionTokens: 8, TotalTokens: 20},
	}

	server := setupMockOpenAI(t, mockResp)
	defer server.Close()

	cfg := &config.Config{
		OpenAIBaseURL: server.URL,
		OpenAIAPIKey:  "test-key",
		DefaultModel:  "gpt-4o",
	}
	handler := NewMessagesHandler(cfg)

	body := `{"model":"claude-sonnet-4-6","max_tokens":1024,"messages":[{"role":"user","content":"Weather?"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp model.AnthropicResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.StopReason != "tool_use" {
		t.Errorf("expected stop_reason tool_use, got %s", resp.StopReason)
	}
	if len(resp.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(resp.Content))
	}
	if resp.Content[0].Type != "text" || resp.Content[0].Text != "Let me look that up." {
		t.Errorf("unexpected text block: %+v", resp.Content[0])
	}
	if resp.Content[1].Type != "tool_use" {
		t.Errorf("expected tool_use block, got %s", resp.Content[1].Type)
	}
	if resp.Content[1].ID != "call_xyz" {
		t.Errorf("expected tool id call_xyz, got %s", resp.Content[1].ID)
	}
	if resp.Content[1].Name != "get_weather" {
		t.Errorf("expected tool name get_weather, got %s", resp.Content[1].Name)
	}
}

func TestMessagesHandler_WithStore_NonStream(t *testing.T) {
	mockResp := model.OpenAIResponse{
		ID:      "chatcmpl-store",
		Choices: []model.OpenAIChoice{{Index: 0, Message: model.OpenAIMessage{Role: "assistant", Content: "Hi"}, FinishReason: "stop"}},
		Usage:   model.OpenAIUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
	server := setupMockOpenAI(t, mockResp)
	defer server.Close()

	store, err := storage.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := &config.Config{OpenAIBaseURL: server.URL, DefaultModel: "gpt-4o"}
	h := NewMessagesHandler(cfg, WithStore(store))

	body := `{"model":"test-model","max_tokens":100,"messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	records, err := store.Records()
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 usage record, got %d", len(records))
	}
	if records[0].Model != "test-model" {
		t.Errorf("expected model test-model, got %s", records[0].Model)
	}
	if records[0].InputTokens != 10 {
		t.Errorf("expected 10 input tokens, got %d", records[0].InputTokens)
	}
	if records[0].OutputTokens != 5 {
		t.Errorf("expected 5 output tokens, got %d", records[0].OutputTokens)
	}
	if records[0].Stream {
		t.Error("expected stream=false")
	}
}

func TestMessagesHandler_WithStore_Stream(t *testing.T) {
	chunks := []string{
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-1","object":"chat.completion.chunk","created":1700000000,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}}`,
	}
	server := setupMockOpenAIStream(t, chunks)
	defer server.Close()

	store, err := storage.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	cfg := &config.Config{OpenAIBaseURL: server.URL, DefaultModel: "gpt-4o"}
	h := NewMessagesHandler(cfg, WithStore(store))

	body := `{"model":"stream-model","max_tokens":100,"stream":true,"messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	records, err := store.Records()
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 usage record, got %d", len(records))
	}
	if records[0].Model != "stream-model" {
		t.Errorf("expected model stream-model, got %s", records[0].Model)
	}
	if !records[0].Stream {
		t.Error("expected stream=true")
	}
}

// The combined judgment: an upstream failure is mapped to a prompt-too-long
// invalid_request_error (so Claude Code auto-compacts) only when it is a genuine
// context overflow — a 413, a 400 whose body names the reason, or a 400 on a
// request we estimate to be over the token threshold. Everything else, including
// a small opaque 400, stays a transient 502 api_error.
func TestMessagesHandler_UpstreamErrorMapping(t *testing.T) {
	// A body large enough that estimateTokens (~len/4) clears the low test
	// threshold below. 40 KB / 4 = ~10k tokens.
	bigContent := strings.Repeat("x", 40000)

	for _, tc := range []struct {
		name         string
		status       int
		upstreamBody string
		content      string
		stream       bool
		wantStatus   int
		wantType     string
	}{
		{
			name:         "opaque 400 on large request -> prompt too long (token estimate)",
			status:       http.StatusBadRequest,
			upstreamBody: `{"error":{"type":"bad_response_status_code","code":"bad_response_status_code"}}`,
			content:      bigContent,
			wantStatus:   http.StatusBadRequest,
			wantType:     "invalid_request_error",
		},
		{
			name:         "opaque 400 on large request, streaming -> prompt too long",
			status:       http.StatusBadRequest,
			upstreamBody: `{"error":{"type":"bad_response_status_code","code":"bad_response_status_code"}}`,
			content:      bigContent,
			stream:       true,
			wantStatus:   http.StatusBadRequest,
			wantType:     "invalid_request_error",
		},
		{
			name:         "400 with context-length keyword on small request -> prompt too long",
			status:       http.StatusBadRequest,
			upstreamBody: `{"error":{"message":"This model's maximum context length is 200000 tokens"}}`,
			content:      "Hi",
			wantStatus:   http.StatusBadRequest,
			wantType:     "invalid_request_error",
		},
		{
			name:         "413 -> prompt too long unconditionally",
			status:       http.StatusRequestEntityTooLarge,
			upstreamBody: `too large`,
			content:      "Hi",
			wantStatus:   http.StatusBadRequest,
			wantType:     "invalid_request_error",
		},
		{
			name:         "opaque 400 on small request -> transient 502",
			status:       http.StatusBadRequest,
			upstreamBody: `{"error":{"type":"bad_response_status_code","code":"bad_response_status_code"}}`,
			content:      "Hi",
			wantStatus:   http.StatusBadGateway,
			wantType:     "api_error",
		},
		{
			name:         "500 -> transient 502",
			status:       http.StatusInternalServerError,
			upstreamBody: `upstream boom`,
			content:      "Hi",
			wantStatus:   http.StatusBadGateway,
			wantType:     "api_error",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.upstreamBody))
			}))
			defer server.Close()

			// Low threshold so bigContent clears it while "Hi" does not.
			cfg := &config.Config{OpenAIBaseURL: server.URL, DefaultModel: "gpt-4o", ContextOverflowTokens: 1000}
			handler := NewMessagesHandler(cfg)

			payload := map[string]any{
				"model":      "claude-sonnet-4-6",
				"max_tokens": 100,
				"stream":     tc.stream,
				"messages":   []map[string]any{{"role": "user", "content": tc.content}},
			}
			bodyBytes, _ := json.Marshal(payload)
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(string(bodyBytes)))
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tc.wantStatus, rec.Code, rec.Body.String())
			}
			var errResp struct {
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if errResp.Error.Type != tc.wantType {
				t.Errorf("expected error type %q, got %q", tc.wantType, errResp.Error.Type)
			}
			if tc.wantType == "invalid_request_error" && !strings.Contains(errResp.Error.Message, "prompt is too long") {
				t.Errorf("expected prompt-too-long message, got %q", errResp.Error.Message)
			}
		})
	}
}
