// Package llm is a minimal client for the OpenAI-compatible Chat
// Completions API, including tool (function) calling. It works with
// OpenRouter and any other provider that speaks the same API shape —
// which provider and model to use is pure configuration (base URL, API
// key, model name), nothing provider-specific is hardcoded here. This is
// what the task means by "LLM access is configured via config".
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Config selects the provider: any OpenAI-compatible endpoint.
type Config struct {
	APIKey  string
	BaseURL string // e.g. https://openrouter.ai/api/v1
	Model   string // e.g. nvidia/nemotron-3.5-super-vl:free
}

// Client sends chat completion requests.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

func New(cfg Config) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// Message is one turn in a chat completion request or response. The same
// struct is used in both directions, matching the OpenAI API shape:
//   - "system"/"user": Content only.
//   - "assistant": Content and/or ToolCalls (the model asking to run tools).
//   - "tool": Content is the tool's result, ToolCallID says which call it
//     answers.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall is one function call the model asked for.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // always "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON object, as a string
	} `json:"function"`
}

// Tool is one function the model is allowed to call.
type Tool struct {
	Type     string         `json:"type"` // always "function"
	Function FunctionSchema `json:"function"`
}

// FunctionSchema describes one callable function: name, description, and
// its arguments as a JSON Schema object.
type FunctionSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ChatRequest is one chat completion request. Tools may be empty for a
// plain text exchange.
type ChatRequest struct {
	Messages []Message
	Tools    []Tool
}

type chatRequestBody struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
}

type chatResponseBody struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// CreateChatCompletion sends one request and returns the assistant's
// reply message (which may carry ToolCalls instead of, or in addition
// to, Content).
func (c *Client) CreateChatCompletion(ctx context.Context, req ChatRequest) (Message, error) {
	body, err := json.Marshal(chatRequestBody{Model: c.cfg.Model, Messages: req.Messages, Tools: req.Tools})
	if err != nil {
		return Message{}, fmt.Errorf("llm: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Message{}, fmt.Errorf("llm: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return Message{}, fmt.Errorf("llm: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Message{}, fmt.Errorf("llm: read response: %w", err)
	}

	var out chatResponseBody
	if err := json.Unmarshal(raw, &out); err != nil {
		return Message{}, fmt.Errorf("llm: decode response (status %d): %w: %s", resp.StatusCode, err, raw)
	}

	if out.Error != nil {
		return Message{}, fmt.Errorf("llm: api error (status %d): %s", resp.StatusCode, out.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return Message{}, fmt.Errorf("llm: unexpected status %d: %s", resp.StatusCode, raw)
	}
	if len(out.Choices) == 0 {
		return Message{}, fmt.Errorf("llm: empty response: %s", raw)
	}

	return out.Choices[0].Message, nil
}

// Complete is a convenience wrapper for a plain text exchange with no
// tools, used e.g. by the llmcheck command.
func (c *Client) Complete(ctx context.Context, messages []Message) (string, error) {
	reply, err := c.CreateChatCompletion(ctx, ChatRequest{Messages: messages})
	if err != nil {
		return "", err
	}
	return reply.Content, nil
}
