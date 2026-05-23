package proxy

import (
	"testing"

	"github.com/xzxiong/ai-coding/internal/model"
)

func TestConvertOpenAIToAnthropic_Basic(t *testing.T) {
	resp := &model.OpenAIResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1700000000,
		Model:   "gpt-4o",
		Choices: []model.OpenAIChoice{
			{
				Index:        0,
				Message:      model.OpenAIMessage{Role: "assistant", Content: "Hello there!"},
				FinishReason: "stop",
			},
		},
		Usage: model.OpenAIUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	result := ConvertOpenAIToAnthropic(resp, "claude-sonnet-4-6")

	if result.ID != "msg_chatcmpl-123" {
		t.Errorf("expected id msg_chatcmpl-123, got %s", result.ID)
	}
	if result.Type != "message" {
		t.Errorf("expected type message, got %s", result.Type)
	}
	if result.Role != "assistant" {
		t.Errorf("expected role assistant, got %s", result.Role)
	}
	if result.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %s", result.Model)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("expected stop_reason end_turn, got %s", result.StopReason)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Errorf("expected content type text, got %s", result.Content[0].Type)
	}
	if result.Content[0].Text != "Hello there!" {
		t.Errorf("expected content text, got %s", result.Content[0].Text)
	}
	if result.Usage.InputTokens != 10 {
		t.Errorf("expected input_tokens 10, got %d", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 5 {
		t.Errorf("expected output_tokens 5, got %d", result.Usage.OutputTokens)
	}
}

func TestConvertOpenAIToAnthropic_LengthStop(t *testing.T) {
	resp := &model.OpenAIResponse{
		ID: "chatcmpl-456",
		Choices: []model.OpenAIChoice{
			{
				Index:        0,
				Message:      model.OpenAIMessage{Role: "assistant", Content: "partial"},
				FinishReason: "length",
			},
		},
		Usage: model.OpenAIUsage{},
	}

	result := ConvertOpenAIToAnthropic(resp, "claude-sonnet-4-6")

	if result.StopReason != "max_tokens" {
		t.Errorf("expected stop_reason max_tokens, got %s", result.StopReason)
	}
}

func TestConvertOpenAIToAnthropic_EmptyChoices(t *testing.T) {
	resp := &model.OpenAIResponse{
		ID:      "chatcmpl-789",
		Choices: []model.OpenAIChoice{},
		Usage:   model.OpenAIUsage{},
	}

	result := ConvertOpenAIToAnthropic(resp, "claude-sonnet-4-6")

	if len(result.Content) != 0 {
		t.Errorf("expected 0 content blocks, got %d", len(result.Content))
	}
	if result.StopReason != "" {
		t.Errorf("expected empty stop_reason, got %s", result.StopReason)
	}
}

func TestBuildStreamMessageStart(t *testing.T) {
	result := BuildStreamMessageStart("claude-sonnet-4-6")

	if result.Type != "message_start" {
		t.Errorf("expected type message_start, got %s", result.Type)
	}
	if result.Message.Role != "assistant" {
		t.Errorf("expected role assistant, got %s", result.Message.Role)
	}
	if result.Message.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %s", result.Message.Model)
	}
	if result.Message.ID == "" {
		t.Error("expected non-empty message ID")
	}
}

func TestConvertOpenAIToAnthropic_ToolCalls(t *testing.T) {
	resp := &model.OpenAIResponse{
		ID: "chatcmpl-tool",
		Choices: []model.OpenAIChoice{{
			Index: 0,
			Message: model.OpenAIMessage{
				Role:    "assistant",
				Content: "",
				ToolCalls: []model.OpenAIToolCall{{
					ID:   "call_abc",
					Type: "function",
					Function: model.OpenAIToolCallFunc{
						Name:      "get_weather",
						Arguments: `{"city":"NYC"}`,
					},
				}},
			},
			FinishReason: "tool_calls",
		}},
		Usage: model.OpenAIUsage{PromptTokens: 20, CompletionTokens: 10},
	}

	result := ConvertOpenAIToAnthropic(resp, "claude-sonnet-4-6")

	if result.StopReason != "tool_use" {
		t.Errorf("expected stop_reason tool_use, got %s", result.StopReason)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(result.Content))
	}
	block := result.Content[0]
	if block.Type != "tool_use" {
		t.Errorf("expected type tool_use, got %s", block.Type)
	}
	if block.ID != "call_abc" {
		t.Errorf("expected id call_abc, got %s", block.ID)
	}
	if block.Name != "get_weather" {
		t.Errorf("expected name get_weather, got %s", block.Name)
	}
	if string(block.Input) != `{"city":"NYC"}` {
		t.Errorf("expected input, got %s", string(block.Input))
	}
}

func TestConvertOpenAIToAnthropic_TextAndToolCalls(t *testing.T) {
	resp := &model.OpenAIResponse{
		ID: "chatcmpl-mixed",
		Choices: []model.OpenAIChoice{{
			Index: 0,
			Message: model.OpenAIMessage{
				Role:    "assistant",
				Content: "Let me check.",
				ToolCalls: []model.OpenAIToolCall{{
					ID:       "call_xyz",
					Type:     "function",
					Function: model.OpenAIToolCallFunc{Name: "lookup", Arguments: `{}`},
				}},
			},
			FinishReason: "tool_calls",
		}},
		Usage: model.OpenAIUsage{},
	}

	result := ConvertOpenAIToAnthropic(resp, "test")

	if len(result.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(result.Content))
	}
	if result.Content[0].Type != "text" {
		t.Errorf("expected first block text, got %s", result.Content[0].Type)
	}
	if result.Content[1].Type != "tool_use" {
		t.Errorf("expected second block tool_use, got %s", result.Content[1].Type)
	}
}

func TestMapFinishReason(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"stop", "end_turn"},
		{"length", "max_tokens"},
		{"tool_calls", "tool_use"},
		{"content_filter", "end_turn"},
		{"unknown", "end_turn"},
		{"", "end_turn"},
	}

	for _, tc := range tests {
		got := mapFinishReason(tc.input)
		if got != tc.expected {
			t.Errorf("mapFinishReason(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
