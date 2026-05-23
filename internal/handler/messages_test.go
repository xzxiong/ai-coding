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
