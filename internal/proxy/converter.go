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

	return openaiReq, nil
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
		text, err := extractContentText(msg.Content)
		if err != nil {
			return nil, err
		}
		result = append(result, model.OpenAIMessage{Role: msg.Role, Content: text})
	}

	return result, nil
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
