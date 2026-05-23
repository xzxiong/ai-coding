package model

import "encoding/json"

type OpenAIRequest struct {
	Model         string            `json:"model"`
	Messages      []OpenAIMessage   `json:"messages"`
	MaxTokens     *int              `json:"max_tokens,omitempty"`
	Stream        bool              `json:"stream,omitempty"`
	StreamOptions *OpenAIStreamOpts `json:"stream_options,omitempty"`
	Temperature   *float64          `json:"temperature,omitempty"`
	TopP          *float64          `json:"top_p,omitempty"`
	Stop          []string          `json:"stop,omitempty"`
	Tools         []OpenAITool      `json:"tools,omitempty"`
	ToolChoice    any               `json:"tool_choice,omitempty"`
}

type OpenAIStreamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

type OpenAITool struct {
	Type     string             `json:"type"`
	Function OpenAIToolFunction `json:"function"`
}

type OpenAIToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type OpenAIContentPart struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *OpenAIImageURL `json:"image_url,omitempty"`
}

type OpenAIImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

type OpenAIMessage struct {
	Role       string          `json:"role"`
	Content    any             `json:"content"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type OpenAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
}

type OpenAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function OpenAIToolCallFunc `json:"function"`
}

type OpenAIToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIChoice struct {
	Index        int           `json:"index"`
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIStreamResponse struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []OpenAIStreamChoice `json:"choices"`
	Usage   *OpenAIUsage         `json:"usage,omitempty"`
}

type OpenAIStreamChoice struct {
	Index        int                `json:"index"`
	Delta        OpenAIStreamDelta  `json:"delta"`
	FinishReason *string            `json:"finish_reason"`
}

type OpenAIStreamDelta struct {
	Role      string                  `json:"role,omitempty"`
	Content   string                  `json:"content,omitempty"`
	ToolCalls []OpenAIStreamToolCall  `json:"tool_calls,omitempty"`
}

type OpenAIStreamToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function OpenAIToolCallFunc `json:"function"`
}
