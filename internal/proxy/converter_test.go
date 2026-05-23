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

	result, err := ConvertAnthropicToOpenAI(req, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %s", result.Model)
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

	result, err := ConvertAnthropicToOpenAI(req, "")
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

	result, err := ConvertAnthropicToOpenAI(req, "")
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

	result, err := ConvertAnthropicToOpenAI(req, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Model != "claude-haiku-4-5" {
		t.Errorf("expected claude-haiku-4-5 passthrough, got %s", result.Model)
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

	result, err := ConvertAnthropicToOpenAI(req, "")
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

	result, err := ConvertAnthropicToOpenAI(req, "")
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

func TestConvertAnthropicToOpenAI_WithTools(t *testing.T) {
	raw := `{
		"model": "claude-sonnet-4-6",
		"max_tokens": 1024,
		"tools": [{"name": "get_weather", "description": "Get weather", "input_schema": {"type":"object","properties":{"city":{"type":"string"}}}}],
		"tool_choice": {"type": "auto"},
		"messages": [{"role": "user", "content": "What is the weather in NYC?"}]
	}`

	var req model.AnthropicRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	result, err := ConvertAnthropicToOpenAI(&req, "")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	if len(result.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result.Tools))
	}
	if result.Tools[0].Type != "function" {
		t.Errorf("expected type function, got %s", result.Tools[0].Type)
	}
	if result.Tools[0].Function.Name != "get_weather" {
		t.Errorf("expected name get_weather, got %s", result.Tools[0].Function.Name)
	}
	if result.Tools[0].Function.Description != "Get weather" {
		t.Errorf("expected description, got %s", result.Tools[0].Function.Description)
	}
	if result.ToolChoice != "auto" {
		t.Errorf("expected tool_choice auto, got %v", result.ToolChoice)
	}
}

func TestConvertToolChoice(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected any
	}{
		{"auto", map[string]any{"type": "auto"}, "auto"},
		{"any", map[string]any{"type": "any"}, "required"},
		{"specific_tool", map[string]any{"type": "tool", "name": "get_weather"},
			map[string]any{"type": "function", "function": map[string]string{"name": "get_weather"}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := convertToolChoice(tc.input)
			gotJSON, _ := json.Marshal(got)
			expectedJSON, _ := json.Marshal(tc.expected)
			if string(gotJSON) != string(expectedJSON) {
				t.Errorf("got %s, want %s", gotJSON, expectedJSON)
			}
		})
	}
}

func TestConvertAssistantBlocks_ToolUse(t *testing.T) {
	raw := `{
		"model": "claude-sonnet-4-6", "max_tokens": 1024,
		"messages": [{
			"role": "assistant",
			"content": [
				{"type": "text", "text": "Let me check."},
				{"type": "tool_use", "id": "toolu_123", "name": "get_weather", "input": {"city": "NYC"}}
			]
		}]
	}`

	var req model.AnthropicRequest
	json.Unmarshal([]byte(raw), &req)

	result, err := ConvertAnthropicToOpenAI(&req, "")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	msg := result.Messages[0]
	if msg.Role != "assistant" {
		t.Errorf("expected assistant, got %s", msg.Role)
	}
	if msg.Content != "Let me check." {
		t.Errorf("expected text content, got %s", msg.Content)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].ID != "toolu_123" {
		t.Errorf("expected id toolu_123, got %s", msg.ToolCalls[0].ID)
	}
	if msg.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("expected name get_weather, got %s", msg.ToolCalls[0].Function.Name)
	}
	if msg.ToolCalls[0].Function.Arguments != `{"city":"NYC"}` {
		t.Errorf("expected arguments, got %s", msg.ToolCalls[0].Function.Arguments)
	}
}

func TestConvertUserBlocks_ToolResult(t *testing.T) {
	raw := `{
		"model": "claude-sonnet-4-6", "max_tokens": 1024,
		"messages": [{
			"role": "user",
			"content": [
				{"type": "tool_result", "tool_call_id": "toolu_123", "content": "72F and sunny"}
			]
		}]
	}`

	var req model.AnthropicRequest
	json.Unmarshal([]byte(raw), &req)

	result, err := ConvertAnthropicToOpenAI(&req, "")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	msg := result.Messages[0]
	if msg.Role != "tool" {
		t.Errorf("expected tool role, got %s", msg.Role)
	}
	if msg.ToolCallID != "toolu_123" {
		t.Errorf("expected tool_call_id toolu_123, got %s", msg.ToolCallID)
	}
	if msg.Content != "72F and sunny" {
		t.Errorf("expected content, got %s", msg.Content)
	}
}

func TestConvertUserBlocks_ImageBase64(t *testing.T) {
	raw := `{
		"model": "claude-sonnet-4-6", "max_tokens": 1024,
		"messages": [{
			"role": "user",
			"content": [
				{"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "iVBOR"}},
				{"type": "text", "text": "What is this?"}
			]
		}]
	}`

	var req model.AnthropicRequest
	json.Unmarshal([]byte(raw), &req)

	result, err := ConvertAnthropicToOpenAI(&req, "")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	if len(result.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result.Messages))
	}
	parts, ok := result.Messages[0].Content.([]model.OpenAIContentPart)
	if !ok {
		t.Fatalf("expected content parts array, got %T", result.Messages[0].Content)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0].Type != "image_url" {
		t.Errorf("expected image_url, got %s", parts[0].Type)
	}
	if parts[0].ImageURL.URL != "data:image/png;base64,iVBOR" {
		t.Errorf("unexpected url: %s", parts[0].ImageURL.URL)
	}
	if parts[1].Type != "text" || parts[1].Text != "What is this?" {
		t.Errorf("unexpected text part: %+v", parts[1])
	}
}

func TestConvertUserBlocks_ImageURL(t *testing.T) {
	raw := `{
		"model": "claude-sonnet-4-6", "max_tokens": 1024,
		"messages": [{
			"role": "user",
			"content": [
				{"type": "image", "source": {"type": "url", "url": "https://example.com/img.png"}},
				{"type": "text", "text": "Describe"}
			]
		}]
	}`

	var req model.AnthropicRequest
	json.Unmarshal([]byte(raw), &req)

	result, err := ConvertAnthropicToOpenAI(&req, "")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	parts, ok := result.Messages[0].Content.([]model.OpenAIContentPart)
	if !ok {
		t.Fatalf("expected content parts array, got %T", result.Messages[0].Content)
	}
	if parts[0].ImageURL.URL != "https://example.com/img.png" {
		t.Errorf("unexpected url: %s", parts[0].ImageURL.URL)
	}
}

func TestMapModel(t *testing.T) {
	t.Run("passthrough", func(t *testing.T) {
		tests := []struct {
			input    string
			expected string
		}{
			{"claude-sonnet-4-6", "claude-sonnet-4-6"},
			{"deepseek-v4-pro", "deepseek-v4-pro"},
			{"gpt-4o", "gpt-4o"},
		}
		for _, tc := range tests {
			got := mapModel(tc.input, "whatever")
			if got != tc.expected {
				t.Errorf("mapModel(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		}
	})

	t.Run("empty_model_uses_default", func(t *testing.T) {
		got := mapModel("", "deepseek-v4-pro")
		if got != "deepseek-v4-pro" {
			t.Errorf("expected deepseek-v4-pro, got %s", got)
		}
	})

	t.Run("empty_model_no_default_uses_gpt4o", func(t *testing.T) {
		got := mapModel("", "")
		if got != "gpt-4o" {
			t.Errorf("expected gpt-4o, got %s", got)
		}
	})
}
