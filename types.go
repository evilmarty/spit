package main

import "encoding/json"

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model           string          `json:"model"`
	Messages        []chatMessage   `json:"messages"`
	Stream          bool            `json:"stream,omitempty"`
	ResponseFormat  *responseFormat `json:"response_format,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
	MaxTokens       *int            `json:"max_tokens,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
	Reasoning       json.RawMessage `json:"reasoning,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

type apiErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

type config struct {
	Endpoint        string
	Port            int
	Model           string
	APIKey          string
	Format          string
	Temperature     *float64
	TopP            *float64
	MaxTokens       *int
	ReasoningEffort string
	Reasoning       json.RawMessage
	Messages        []chatMessage
}

type messageCollector struct {
	messages []chatMessage
}
