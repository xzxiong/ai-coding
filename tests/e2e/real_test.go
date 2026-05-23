package e2e

import (
	"bytes"
	"encoding/json"
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
	testModel := os.Getenv("TEST_MODEL")

	if baseURL == "" || token == "" {
		t.Skip("TEST_BASE_URL and TEST_TOKEN not set, skipping real API test")
	}
	if testModel == "" {
		testModel = "gpt-4o"
	}

	return &config.Config{
		OpenAIBaseURL: baseURL,
		OpenAIAPIKey:  token,
		DefaultModel:  testModel,
	}
}

func TestReal_BasicMessage(t *testing.T) {
	cfg := getTestConfig(t)
	h := handler.NewMessagesHandler(cfg)

	body := `{"model":"claude-sonnet-4-6","max_tokens":64,"messages":[{"role":"user","content":"What is 2+3? Answer with just the number."}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	var resp model.AnthropicResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if resp.Type != "message" {
		t.Errorf("expected type=message, got %s", resp.Type)
	}
	if len(resp.Content) == 0 {
		t.Fatal("empty content")
	}
	if !strings.Contains(resp.Content[0].Text, "5") {
		t.Errorf("expected '5' in response, got: %s", resp.Content[0].Text)
	}
	t.Logf("Response: %s", resp.Content[0].Text)
}

func TestReal_SystemPrompt(t *testing.T) {
	cfg := getTestConfig(t)
	h := handler.NewMessagesHandler(cfg)

	body := `{"model":"claude-sonnet-4-6","max_tokens":64,"system":"Always respond with exactly: OK","messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	var resp model.AnthropicResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if len(resp.Content) == 0 {
		t.Fatal("empty content")
	}
	t.Logf("Response: %s", resp.Content[0].Text)
}

func TestReal_MultiTurn(t *testing.T) {
	cfg := getTestConfig(t)
	h := handler.NewMessagesHandler(cfg)

	body := `{"model":"claude-sonnet-4-6","max_tokens":64,"messages":[{"role":"user","content":"My name is Bob."},{"role":"assistant","content":"Hello Bob!"},{"role":"user","content":"What is my name?"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	var resp model.AnthropicResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if len(resp.Content) == 0 {
		t.Fatal("empty content")
	}
	if !strings.Contains(resp.Content[0].Text, "Bob") {
		t.Errorf("expected 'Bob' in response, got: %s", resp.Content[0].Text)
	}
	t.Logf("Response: %s", resp.Content[0].Text)
}

func TestReal_Streaming(t *testing.T) {
	cfg := getTestConfig(t)
	h := handler.NewMessagesHandler(cfg)

	body := `{"model":"claude-sonnet-4-6","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"Say hello in one word."}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	respBody := rec.Body.String()

	if !strings.Contains(respBody, "event: message_start") {
		t.Error("missing message_start")
	}
	if !strings.Contains(respBody, "event: content_block_delta") {
		t.Error("missing content_block_delta")
	}
	if !strings.Contains(respBody, "event: message_stop") {
		t.Error("missing message_stop")
	}

	var collected string
	for _, line := range strings.Split(respBody, "\n") {
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
			collected += event.Delta.Text
		}
	}

	if collected == "" {
		t.Error("no text collected from stream")
	}
	t.Logf("Streamed text: %s", collected)
}

func TestReal_Usage(t *testing.T) {
	cfg := getTestConfig(t)
	h := handler.NewMessagesHandler(cfg)

	body := `{"model":"claude-sonnet-4-6","max_tokens":32,"messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	var resp model.AnthropicResponse
	json.Unmarshal(rec.Body.Bytes(), &resp)

	if resp.Usage.InputTokens == 0 {
		t.Error("expected input_tokens > 0")
	}
	if resp.Usage.OutputTokens == 0 {
		t.Error("expected output_tokens > 0")
	}
	t.Logf("Usage: input=%d output=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens)
}

func TestReal_DirectHTTP(t *testing.T) {
	cfg := getTestConfig(t)
	h := handler.NewMessagesHandler(cfg)

	srv := httptest.NewServer(h)
	defer srv.Close()

	payload := map[string]any{
		"model":      "claude-sonnet-4-6",
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

	var result model.AnthropicResponse
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Type != "message" {
		t.Errorf("expected type=message, got %s", result.Type)
	}
	if len(result.Content) == 0 {
		t.Fatal("empty content")
	}
	t.Logf("Response: %s", result.Content[0].Text)
}
