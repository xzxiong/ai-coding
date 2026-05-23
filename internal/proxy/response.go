package proxy

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/xzxiong/ai-coding/internal/model"
)

func ConvertOpenAIToAnthropic(resp *model.OpenAIResponse, reqModel string) *model.AnthropicResponse {
	var content []model.AnthropicContentBlock
	var stopReason string

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		if text, _ := choice.Message.Content.(string); text != "" {
			content = append(content, model.AnthropicContentBlock{
				Type: "text",
				Text: text,
			})
		}
		for _, tc := range choice.Message.ToolCalls {
			content = append(content, model.AnthropicContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: json.RawMessage(tc.Function.Arguments),
			})
		}
		stopReason = mapFinishReason(choice.FinishReason)
	}

	return &model.AnthropicResponse{
		ID:         fmt.Sprintf("msg_%s", resp.ID),
		Type:       "message",
		Role:       "assistant",
		Content:    content,
		Model:      reqModel,
		StopReason: stopReason,
		Usage: model.AnthropicUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}
}

func BuildStreamMessageStart(reqModel string) *model.AnthropicMessageStart {
	return &model.AnthropicMessageStart{
		Type: "message_start",
		Message: model.AnthropicResponse{
			ID:      fmt.Sprintf("msg_%d", time.Now().UnixNano()),
			Type:    "message",
			Role:    "assistant",
			Content: []model.AnthropicContentBlock{},
			Model:   reqModel,
			Usage:   model.AnthropicUsage{},
		},
	}
}

func mapFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "end_turn"
	default:
		return "end_turn"
	}
}
