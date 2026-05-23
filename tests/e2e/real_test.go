package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/xzxiong/ai-coding/internal/config"
	"github.com/xzxiong/ai-coding/internal/handler"
	"github.com/xzxiong/ai-coding/internal/model"
)

func getTestConfig(t *testing.T) *config.Config {
	t.Helper()
	baseURL := os.Getenv("TEST_BASE_URL")
	token := os.Getenv("TEST_TOKEN")

	if baseURL == "" || token == "" {
		t.Skip("TEST_BASE_URL and TEST_TOKEN not set, skipping real API test")
	}

	return &config.Config{
		OpenAIBaseURL: baseURL,
		OpenAIAPIKey:  token,
		DefaultModel:  "",
	}
}

func getTestModel() string {
	if m := os.Getenv("TEST_MODEL"); m != "" {
		return m
	}
	return "gpt-4o"
}

func validateResponse(t *testing.T, rec *httptest.ResponseRecorder) model.AnthropicResponse {
	t.Helper()

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var resp model.AnthropicResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response JSON: %v\nbody: %s", err, rec.Body.String())
	}

	if resp.ID == "" {
		t.Error("response missing id")
	}
	if resp.Type != "message" {
		t.Errorf("expected type=message, got %q", resp.Type)
	}
	if resp.Role != "assistant" {
		t.Errorf("expected role=assistant, got %q", resp.Role)
	}
	if resp.Model == "" {
		t.Error("response missing model")
	}
	if len(resp.Content) == 0 {
		t.Fatal("response has empty content")
	}
	for i, block := range resp.Content {
		if block.Type != "text" {
			t.Errorf("content[%d]: expected type=text, got %q", i, block.Type)
		}
		if block.Text == "" {
			t.Errorf("content[%d]: text is empty", i)
		}
	}
	if resp.StopReason == "" {
		t.Error("response missing stop_reason")
	}
	if resp.StopReason != "end_turn" && resp.StopReason != "max_tokens" {
		t.Errorf("unexpected stop_reason: %q", resp.StopReason)
	}
	if resp.Usage.InputTokens <= 0 {
		t.Errorf("expected input_tokens > 0, got %d", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens <= 0 {
		t.Errorf("expected output_tokens > 0, got %d", resp.Usage.OutputTokens)
	}

	return resp
}

func validateStreamResponse(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		t.Errorf("expected Content-Type text/event-stream, got %s", contentType)
	}

	body := rec.Body.String()

	requiredEvents := []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"}
	for _, ev := range requiredEvents {
		if !strings.Contains(body, "event: "+ev) {
			t.Errorf("missing required event: %s", ev)
		}
	}

	// Validate message_start structure
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var raw map[string]any
		if json.Unmarshal([]byte(data), &raw) != nil {
			continue
		}
		if raw["type"] == "message_start" {
			msg, ok := raw["message"].(map[string]any)
			if !ok {
				t.Error("message_start missing message field")
			} else {
				if msg["id"] == nil || msg["id"] == "" {
					t.Error("message_start message missing id")
				}
				if msg["role"] != "assistant" {
					t.Errorf("message_start: expected role=assistant, got %v", msg["role"])
				}
			}
		}
	}

	// Collect text from deltas
	var collected string
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if json.Unmarshal([]byte(data), &event) == nil && event.Type == "content_block_delta" {
			if event.Delta.Type != "text_delta" {
				t.Errorf("content_block_delta: expected delta.type=text_delta, got %q", event.Delta.Type)
			}
			collected += event.Delta.Text
		}
	}

	if collected == "" {
		t.Error("no text collected from stream deltas")
	}

	return collected
}

func TestReal_BasicMessage(t *testing.T) {
	cfg := getTestConfig(t)
	h := handler.NewMessagesHandler(cfg)

	body := fmt.Sprintf(`{"model":"%s","max_tokens":64,"messages":[{"role":"user","content":"What is 2+3? Answer with just the number."}]}`, getTestModel())
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	resp := validateResponse(t, rec)
	if !strings.Contains(resp.Content[0].Text, "5") {
		t.Errorf("expected '5' in response, got: %s", resp.Content[0].Text)
	}
	t.Logf("Response: %s", resp.Content[0].Text)
}

func TestReal_SystemPrompt(t *testing.T) {
	cfg := getTestConfig(t)
	h := handler.NewMessagesHandler(cfg)

	body := fmt.Sprintf(`{"model":"%s","max_tokens":64,"system":"Always respond with exactly: OK","messages":[{"role":"user","content":"Hello"}]}`, getTestModel())
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	resp := validateResponse(t, rec)
	t.Logf("Response: %s", resp.Content[0].Text)
}

func TestReal_MultiTurn(t *testing.T) {
	cfg := getTestConfig(t)
	h := handler.NewMessagesHandler(cfg)

	body := fmt.Sprintf(`{"model":"%s","max_tokens":64,"messages":[{"role":"user","content":"My name is Bob."},{"role":"assistant","content":"Hello Bob!"},{"role":"user","content":"What is my name?"}]}`, getTestModel())
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	resp := validateResponse(t, rec)
	if !strings.Contains(resp.Content[0].Text, "Bob") {
		t.Errorf("expected 'Bob' in response, got: %s", resp.Content[0].Text)
	}
	t.Logf("Response: %s", resp.Content[0].Text)
}

func TestReal_Streaming(t *testing.T) {
	cfg := getTestConfig(t)
	h := handler.NewMessagesHandler(cfg)

	body := fmt.Sprintf(`{"model":"%s","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"Say hello in one word."}]}`, getTestModel())
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	collected := validateStreamResponse(t, rec)
	t.Logf("Streamed text: %s", collected)
}

func TestReal_Usage(t *testing.T) {
	cfg := getTestConfig(t)
	h := handler.NewMessagesHandler(cfg)

	body := fmt.Sprintf(`{"model":"%s","max_tokens":32,"messages":[{"role":"user","content":"Hi"}]}`, getTestModel())
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	resp := validateResponse(t, rec)
	t.Logf("Usage: input=%d output=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens)
}

func TestReal_DirectHTTP(t *testing.T) {
	cfg := getTestConfig(t)
	h := handler.NewMessagesHandler(cfg)

	srv := httptest.NewServer(h)
	defer srv.Close()

	payload := map[string]any{
		"model":      getTestModel(),
		"max_tokens": 32,
		"messages":   []map[string]string{{"role": "user", "content": "Say yes."}},
	}
	raw, _ := json.Marshal(payload)

	resp, err := http.Post(srv.URL+"/v1/messages", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, string(body))
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}

	var result model.AnthropicResponse
	json.NewDecoder(resp.Body).Decode(&result)

	if result.ID == "" {
		t.Error("response missing id")
	}
	if result.Type != "message" {
		t.Errorf("expected type=message, got %s", result.Type)
	}
	if result.Role != "assistant" {
		t.Errorf("expected role=assistant, got %s", result.Role)
	}
	if len(result.Content) == 0 {
		t.Fatal("empty content")
	}
	if result.Content[0].Type != "text" || result.Content[0].Text == "" {
		t.Errorf("invalid content block: %+v", result.Content[0])
	}
	if result.StopReason == "" {
		t.Error("response missing stop_reason")
	}
	if result.Usage.InputTokens <= 0 || result.Usage.OutputTokens <= 0 {
		t.Errorf("invalid usage: %+v", result.Usage)
	}
	t.Logf("Response: %s", result.Content[0].Text)
}
