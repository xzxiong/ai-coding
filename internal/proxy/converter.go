package proxy

import (
	"encoding/json"
	"fmt"

	"github.com/xzxiong/ai-coding/internal/model"
)

func ConvertAnthropicToOpenAI(req *model.AnthropicRequest, defaultModel string) (*model.OpenAIRequest, error) {
	openaiReq := &model.OpenAIRequest{
		Model:       mapModel(req.Model, defaultModel),
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.StopSequences,
	}

	if req.MaxTokens > 0 {
		openaiReq.MaxTokens = &req.MaxTokens
	}

	messages, err := convertMessages(req.System, req.Messages)
	if err != nil {
		return nil, err
	}
	openaiReq.Messages = messages

	if len(req.Tools) > 0 {
		openaiReq.Tools = convertTools(req.Tools)
	}
	if req.ToolChoice != nil {
		openaiReq.ToolChoice = convertToolChoice(req.ToolChoice)
	}

	return openaiReq, nil
}

func convertTools(tools []model.AnthropicTool) []model.OpenAITool {
	result := make([]model.OpenAITool, len(tools))
	for i, t := range tools {
		result[i] = model.OpenAITool{
			Type: "function",
			Function: model.OpenAIToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		}
	}
	return result
}

func convertToolChoice(choice any) any {
	switch v := choice.(type) {
	case map[string]any:
		if t, _ := v["type"].(string); t == "tool" {
			if name, _ := v["name"].(string); name != "" {
				return map[string]any{
					"type":     "function",
					"function": map[string]string{"name": name},
				}
			}
		}
		if t, _ := v["type"].(string); t == "auto" {
			return "auto"
		}
		if t, _ := v["type"].(string); t == "any" {
			return "required"
		}
	}
	return choice
}

func convertMessages(system any, messages []model.AnthropicMessage) ([]model.OpenAIMessage, error) {
	var result []model.OpenAIMessage

	if system != nil {
		systemText, err := extractSystemText(system)
		if err != nil {
			return nil, err
		}
		if systemText != "" {
			result = append(result, model.OpenAIMessage{Role: "system", Content: systemText})
		}
	}

	for _, msg := range messages {
		converted, err := convertMessage(msg)
		if err != nil {
			return nil, err
		}
		result = append(result, converted...)
	}

	return result, nil
}

func convertMessage(msg model.AnthropicMessage) ([]model.OpenAIMessage, error) {
	blocks, isArray := parseContentBlocks(msg.Content)
	if !isArray {
		text, err := extractContentText(msg.Content)
		if err != nil {
			return nil, err
		}
		return []model.OpenAIMessage{{Role: msg.Role, Content: text}}, nil
	}

	if msg.Role == "assistant" {
		return convertAssistantBlocks(blocks)
	}
	if msg.Role == "user" {
		return convertUserBlocks(blocks)
	}
	text, err := extractContentText(msg.Content)
	if err != nil {
		return nil, err
	}
	return []model.OpenAIMessage{{Role: msg.Role, Content: text}}, nil
}

func convertAssistantBlocks(blocks []map[string]any) ([]model.OpenAIMessage, error) {
	var textParts []string
	var toolCalls []model.OpenAIToolCall

	for _, block := range blocks {
		switch block["type"] {
		case "text":
			if t, ok := block["text"].(string); ok {
				textParts = append(textParts, t)
			}
		case "tool_use":
			id, _ := block["id"].(string)
			name, _ := block["name"].(string)
			input, _ := block["input"]
			inputBytes, _ := json.Marshal(input)
			toolCalls = append(toolCalls, model.OpenAIToolCall{
				ID:   id,
				Type: "function",
				Function: model.OpenAIToolCallFunc{
					Name:      name,
					Arguments: string(inputBytes),
				},
			})
		}
	}

	content := ""
	if len(textParts) > 0 {
		content = textParts[0]
		for _, p := range textParts[1:] {
			content += "\n" + p
		}
	}

	msg := model.OpenAIMessage{
		Role:      "assistant",
		Content:   content,
		ToolCalls: toolCalls,
	}
	return []model.OpenAIMessage{msg}, nil
}

func convertUserBlocks(blocks []map[string]any) ([]model.OpenAIMessage, error) {
	var messages []model.OpenAIMessage
	var textParts []string

	for _, block := range blocks {
		switch block["type"] {
		case "text":
			if t, ok := block["text"].(string); ok {
				textParts = append(textParts, t)
			}
		case "tool_result":
			toolCallID, _ := block["tool_call_id"].(string)
			content := extractToolResultContent(block["content"])
			messages = append(messages, model.OpenAIMessage{
				Role:       "tool",
				Content:    content,
				ToolCallID: toolCallID,
			})
		}
	}

	if len(textParts) > 0 {
		content := textParts[0]
		for _, p := range textParts[1:] {
			content += "\n" + p
		}
		messages = append([]model.OpenAIMessage{{Role: "user", Content: content}}, messages...)
	}

	return messages, nil
}

func extractToolResultContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := m["type"].(string); t == "text" {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		if len(parts) > 0 {
			result := parts[0]
			for _, p := range parts[1:] {
				result += "\n" + p
			}
			return result
		}
	}
	raw, _ := json.Marshal(content)
	return string(raw)
}

func parseContentBlocks(content any) ([]map[string]any, bool) {
	arr, ok := content.([]any)
	if !ok {
		return nil, false
	}
	var blocks []map[string]any
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		blocks = append(blocks, m)
	}
	return blocks, true
}

func extractSystemText(system any) (string, error) {
	switch v := system.(type) {
	case string:
		return v, nil
	case []any:
		var parts []string
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := m["type"].(string); t == "text" {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		if len(parts) > 0 {
			result := parts[0]
			for _, p := range parts[1:] {
				result += "\n" + p
			}
			return result, nil
		}
		return "", nil
	default:
		raw, err := json.Marshal(system)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}
}

func extractContentText(content any) (string, error) {
	switch v := content.(type) {
	case string:
		return v, nil
	case []any:
		var parts []string
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if t, _ := m["type"].(string); t == "text" {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		if len(parts) > 0 {
			result := parts[0]
			for _, p := range parts[1:] {
				result += "\n" + p
			}
			return result, nil
		}
		return "", nil
	default:
		return "", fmt.Errorf("unsupported content type: %T", content)
	}
}

func mapModel(anthropicModel, defaultModel string) string {
	if anthropicModel != "" {
		return anthropicModel
	}
	if defaultModel != "" {
		return defaultModel
	}
	return "gpt-4o"
}
