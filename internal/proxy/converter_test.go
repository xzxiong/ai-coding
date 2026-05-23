package proxy

import (
	"encoding/json"
	"testing"

	"github.com/xzxiong/ai-coding/internal/model"
)

func TestConvertAnthropicToOpenAI_BasicMessage(t *testing.T) {
	req := &model.AnthropicRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 1024,
		Messages: []model.AnthropicMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	result, err := ConvertAnthropicToOpenAI(req, "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Model != "gpt-4o" {
		t.Errorf("expected model gpt-4o, got %s", result.Model)
	}
	if *result.MaxTokens != 1024 {
		t.Errorf("expected max_tokens 1024, got %d", *result.MaxTokens)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	if result.Messages[0].Role != "user" {
		t.Errorf("expected role user, got %s", result.Messages[0].Role)
	}
	if result.Messages[0].Content != "Hello" {
		t.Errorf("expected content Hello, got %s", result.Messages[0].Content)
	}
}

func TestConvertAnthropicToOpenAI_WithSystemString(t *testing.T) {
	req := &model.AnthropicRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 512,
		System:    "You are helpful.",
		Messages: []model.AnthropicMessage{
			{Role: "user", Content: "Hi"},
		},
	}

	result, err := ConvertAnthropicToOpenAI(req, "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result.Messages))
	}
	if result.Messages[0].Role != "system" {
		t.Errorf("expected system role, got %s", result.Messages[0].Role)
	}
	if result.Messages[0].Content != "You are helpful." {
		t.Errorf("expected system content, got %s", result.Messages[0].Content)
	}
}

func TestConvertAnthropicToOpenAI_WithSystemBlocks(t *testing.T) {
	system := []any{
		map[string]any{"type": "text", "text": "First part."},
		map[string]any{"type": "text", "text": "Second part."},
	}

	req := &model.AnthropicRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 256,
		System:    system,
		Messages: []model.AnthropicMessage{
			{Role: "user", Content: "Hi"},
		},
	}

	result, err := ConvertAnthropicToOpenAI(req, "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result.Messages))
	}
	if result.Messages[0].Content != "First part.\nSecond part." {
		t.Errorf("unexpected system content: %s", result.Messages[0].Content)
	}
}

func TestConvertAnthropicToOpenAI_ContentBlocks(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "Hello "},
		map[string]any{"type": "text", "text": "World"},
	}

	req := &model.AnthropicRequest{
		Model:     "claude-haiku-4-5",
		MaxTokens: 100,
		Messages: []model.AnthropicMessage{
			{Role: "user", Content: content},
		},
	}

	result, err := ConvertAnthropicToOpenAI(req, "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Model != "gpt-4o-mini" {
		t.Errorf("expected gpt-4o-mini for haiku, got %s", result.Model)
	}
	if result.Messages[0].Content != "Hello \nWorld" {
		t.Errorf("unexpected content: %q", result.Messages[0].Content)
	}
}

func TestConvertAnthropicToOpenAI_Parameters(t *testing.T) {
	temp := 0.7
	topP := 0.9
	req := &model.AnthropicRequest{
		Model:         "claude-sonnet-4-6",
		MaxTokens:     200,
		Temperature:   &temp,
		TopP:          &topP,
		StopSequences: []string{"\n\n", "END"},
		Messages: []model.AnthropicMessage{
			{Role: "user", Content: "test"},
		},
	}

	result, err := ConvertAnthropicToOpenAI(req, "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Temperature == nil || *result.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7")
	}
	if result.TopP == nil || *result.TopP != 0.9 {
		t.Errorf("expected top_p 0.9")
	}
	if len(result.Stop) != 2 || result.Stop[0] != "\n\n" || result.Stop[1] != "END" {
		t.Errorf("unexpected stop: %v", result.Stop)
	}
}

func TestConvertAnthropicToOpenAI_MultiTurn(t *testing.T) {
	req := &model.AnthropicRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 1024,
		Messages: []model.AnthropicMessage{
			{Role: "user", Content: "What is Go?"},
			{Role: "assistant", Content: "Go is a programming language."},
			{Role: "user", Content: "Tell me more."},
		},
	}

	result, err := ConvertAnthropicToOpenAI(req, "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result.Messages))
	}
	if result.Messages[1].Role != "assistant" {
		t.Errorf("expected assistant role, got %s", result.Messages[1].Role)
	}
}

func TestConvertAnthropicToOpenAI_JSONRoundTrip(t *testing.T) {
	raw := `{
		"model": "claude-sonnet-4-6",
		"max_tokens": 1024,
		"system": "Be concise.",
		"messages": [
			{"role": "user", "content": "Hello"}
		]
	}`

	var req model.AnthropicRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	result, err := ConvertAnthropicToOpenAI(&req, "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result.Messages))
	}
	if result.Messages[0].Role != "system" {
		t.Errorf("expected system message first")
	}
}

func TestMapModel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"claude-opus-4-7", "gpt-4o"},
		{"claude-sonnet-4-6", "gpt-4o"},
		{"claude-haiku-4-5", "gpt-4o-mini"},
		{"claude-3-5-sonnet-latest", "gpt-4o"},
		{"claude-3-5-haiku-latest", "gpt-4o-mini"},
		{"unknown-model", "gpt-4o"},
	}

	for _, tc := range tests {
		got := mapModel(tc.input, "gpt-4o")
		if got != tc.expected {
			t.Errorf("mapModel(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
