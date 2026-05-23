package model

type OpenAIRequest struct {
	Model         string             `json:"model"`
	Messages      []OpenAIMessage    `json:"messages"`
	MaxTokens     *int               `json:"max_tokens,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
	StreamOptions *OpenAIStreamOpts  `json:"stream_options,omitempty"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	Stop          []string           `json:"stop,omitempty"`
}

type OpenAIStreamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OpenAIResponse struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
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
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}
